# opencoder Review

日期: 2026-03-12

本文件基于当前 `opencoder` 工程代码的静态审查结果整理，重点覆盖两类问题:

1. 是否满足“钉钉机器人通过 Stream 连接本地 OpenCode，并支持流式响应”的目标
2. 当前实现中的稳定性、安全性、可维护性风险，以及对应修复方案

## 结论

当前工程可以视为“单人单会话 happy path 可演示”的原型，但还不满足可稳定上线的要求。

主要原因:

- 流式事件订阅使用了 OpenCode 全局事件总线，未按 session 过滤，存在串流风险
- 会话未复用，且未真正持久化
- 用户白名单和工具白名单没有形成完整闭环
- `mvp` 模式不会返回最终答案
- 卡片取消按钮没有实际处理链路
- 长运行场景下的 access token 续期未处理
- 自动化测试基本缺失

## P0: 必须先修

### 1. 全局事件流串会话

问题:

- 当前 `internal/opencode/server_client.go` 使用 `/event`
- OpenCode 文档中 `/event` 是全局总线事件
- `internal/bridge/router.go` 读取事件时未校验 `event.SessionID == sess.OpenCodeSessionID`

风险:

- 两个钉钉会话同时提问时，卡片内容会互相污染
- 一个用户可能看到另一个用户的回答片段
- 这是功能正确性和数据隔离问题

修复方案:

1. 保留对 `/event` 的订阅，但在 `streamResponse()` 中只处理当前 `sess.OpenCodeSessionID` 的事件
2. 在 `SSEReader.parseEvent()` 中尽量完整解析 `sessionID`
3. 忽略无 `sessionID` 且与当前 turn 无关的事件
4. 为每次异步发送生成 `messageID`，如果 OpenCode 事件里能关联 message，也一起过滤

建议改法:

- 在 `streamResponse()` 循环开头增加:

```go
if event.SessionID != "" && event.SessionID != sess.OpenCodeSessionID {
	continue
}
```

- 如果 `server.connected`、心跳类事件没有 `sessionID`，直接跳过

验收标准:

- 同时打开两个钉钉会话，分别发送不同问题
- 两张卡片只出现各自问题对应内容
- 不允许出现交叉 token 或交叉完成状态

### 2. 会话未复用，且未真正持久化

问题:

- 每条消息都会重新 `CreateSession()`
- `SessionStorePath` 只是配置项，没有被实际使用
- `session.Store` 仅是进程内 map

风险:

- 无法连续多轮对话
- 重启 bridge 后上下文全部丢失
- README 声称“Persistent sessions”，但实现不成立

修复方案:

1. 在收到消息时，优先读取已有 `sess.OpenCodeSessionID`
2. 如果存在，则直接复用原 session 发消息
3. 只有不存在时才 `CreateSession()`
4. 给 `session.Store` 增加 `Load(path)` 和 `Save(path)` 方法
5. 在以下时机持久化:
   - 新建 session
   - 更新 `OpenCodeSessionID`
   - 更新 `CardBizID`
   - 清理过期会话
   - 优雅退出前

建议落地:

- `internal/session/store.go`
  - 新增 JSON 文件读写
  - 增加 `LoadFromFile`, `SaveToFile`
- `cmd/dingtalk-bridge/main.go`
  - 启动时加载
  - 退出前保存
- `internal/bridge/router.go`
  - 仅当 `sess.OpenCodeSessionID == ""` 时创建新会话

验收标准:

- 同一用户在同一会话连续发两条消息，第二条能延续上下文
- 重启 bridge 后，同一会话还能继续接着问

### 3. `mvp` 模式无最终回复

问题:

- `handleMVPMode()` 只发了 “Processing your request...”
- `SendMessage()` 返回值没有被转成钉钉消息

风险:

- `mvp` 模式不可用
- 配置切换后会被误判为系统异常

修复方案:

1. 解析 `SendMessage()` 返回的 `parts`
2. 抽取 assistant 文本
3. 用 `ReplyMarkdown()` 或 `ReplyText()` 返回最终答案
4. 如果没有文本，至少回退成“任务执行完成，但无文本输出”

验收标准:

- `BRIDGE_MODE=mvp` 时，用户可以看到完整最终回复

### 4. 白名单未生效

问题:

- 用户白名单配置没有在消息入口使用
- 工具白名单插件没有证明被自动安装或加载
- bridge 也没有把每个会话的用户上下文注入到 OpenCode 服务调用链中

风险:

- 任意用户都可能触发机器人
- 插件逻辑和实际运行不一致
- 安全策略只存在于 README

修复方案:

1. 在 `router.HandleMessage()` 最前面增加用户白名单校验
2. 新增 `internal/config` 下的白名单加载函数
3. 启动时打印白名单是否启用、加载了多少用户
4. 明确插件部署方式:
   - 方案 A: README 中要求把 `plugins/dingtalk-guard.ts` 复制到 `.opencode/plugins/`
   - 方案 B: bridge 启动时检查该插件文件是否已安装
5. 不要依赖 `process.env.DINGTALK_USER_ID` 的全局进程变量做多用户隔离
6. 如果 OpenCode server API 不支持 per-request env，则把安全控制下沉到 bridge 层先拦截

建议策略:

- 第一阶段: 在 bridge 层做强制白名单和命令/模式拦截
- 第二阶段: 再让 plugin 做补充约束，而不是唯一约束

验收标准:

- 非白名单用户发消息，机器人不执行任务
- 危险工具在未授权情况下无法被触发

## P1: 尽快修

### 5. 卡片取消按钮没有闭环

问题:

- 卡片模板里有 `Stop` 按钮
- 代码里没有对应的卡片 action 回调处理
- `CancelStream()` 是孤立函数，没有被调用

风险:

- 用户以为可以终止任务，实际上不能
- UI 和实际行为不一致

修复方案:

1. 明确钉钉互动卡片 action 的接入方式
2. 为 action 建立回调处理:
   - 解析 `sessionKey`
   - 找到 active stream
   - 调用 `CancelStream()`
   - 调用 `AbortSession(sess.OpenCodeSessionID)`
3. 更新卡片状态为 `cancelled`
4. 如果当前版本暂时不接 action 回调，就先移除按钮，避免假功能

验收标准:

- 点击 `Stop` 后，OpenCode 当前任务中止
- 卡片状态更新为停止

### 6. HTTP 请求未绑定 context，取消不彻底

问题:

- `doRequest()` 和 `EventStream()` 都没有使用 `http.NewRequestWithContext`

风险:

- 上游取消后，HTTP 长连接和 SSE 仍可能继续阻塞
- 优雅关闭会变慢

修复方案:

1. `doRequest()` 改成接收 `ctx`
2. 所有请求改用 `http.NewRequestWithContext`
3. `EventStream(ctx)` 同样使用绑定 context 的 request

验收标准:

- 取消任务后 SSE 连接应在短时间内结束
- 进程退出时不残留阻塞 goroutine

### 7. DingTalk access token 未刷新

问题:

- `CardClient` 只缓存一次 token，没有过期时间管理

风险:

- 长时间运行后卡片发送/更新失败

修复方案:

1. 缓存 token 时记录过期时间
2. 提前 5 分钟刷新
3. 若请求返回认证失败，清空缓存并重试一次
4. 增加并发保护，避免多个 goroutine 同时刷新 token

验收标准:

- 服务运行超过 token 生命周期后，卡片更新仍正常

### 8. 模型配置硬编码

问题:

- `SendMessageAsync()` 中硬编码了 provider 和 model

风险:

- 换环境后直接失效
- 无法支持不同 OpenCode 配置

修复方案:

1. 在配置中增加:
   - `OPENCODE_PROVIDER_ID`
   - `OPENCODE_MODEL_ID`
   - 可选 `OPENCODE_AGENT`
2. 如果未配置，则不传 model，让 OpenCode 使用默认值
3. README 中说明如何查看当前 provider/model

验收标准:

- 不依赖特定 provider，也能运行

## P2: 提升质量

### 9. 输入去重、旧消息过滤、并发保护不足

问题:

- 当前没做 `msgID` 去重
- 没做旧消息过滤
- 同一 session 连续快速发消息时，旧流可能仍在运行

修复方案:

1. 增加最近消息 ID 去重缓存
2. 启动时忽略明显过旧的消息
3. 同一 `sessionKey` 新消息到来时:
   - 先取消旧 stream
   - 再启动新 turn

验收标准:

- 钉钉重投消息不会重复执行
- 快速连发消息不会产生两个并行 turn 更新同一张卡片

### 10. 日志可能泄露用户内容和敏感上下文

问题:

- 当前日志直接打印用户消息全文

修复方案:

1. 默认只打摘要和长度
2. `debug` 才允许打印完整内容
3. 对 webhook、token、密码做脱敏

验收标准:

- `info` 日志不应包含明文敏感字段

### 11. `SessionKey()` 存在潜在 panic

问题:

- `store.go` 中 `s.DingConversationID[:1]` 未做空串判断

风险:

- 极端脏数据或加载损坏的持久化文件时 panic

修复方案:

1. 先判断 `len(s.DingConversationID) == 0`
2. 或直接统一使用与 `ReceivedMessage.SessionKey()` 相同的编码规则，不做首字符推断

## 测试补齐建议

当前 `go test ./...` 可以通过，但所有包都没有测试文件。

建议最少补以下测试:

1. `internal/opencode/sse_reader_test.go`
   - 解析 `server.connected`
   - 解析 `message.part.delta`
   - 解析 `reasoning`
   - 解析 `session.error`
   - 解析带 `sessionID` 的事件
2. `internal/bridge/router_test.go`
   - 只消费当前 session 事件
   - `mvp` 模式返回最终答案
   - 非白名单用户被拒绝
   - group chat 未 @ 机器人时忽略
3. `internal/session/store_test.go`
   - 持久化读写
   - 清理过期会话
   - 空字符串字段安全处理
4. `internal/dingtalk/card_client_test.go`
   - token 过期刷新逻辑

## 推荐修复顺序

1. 修全局事件串流问题
2. 修 session 复用和持久化
3. 修 `mvp` 最终回复
4. 修白名单闭环
5. 修取消链路和 request context
6. 修 token 刷新
7. 去掉硬编码模型
8. 补测试

## 是否满足原始需求

以你最初的目标来看:

- 钉钉通过 Stream 连接本地服务: 基本满足
- 能触发本地 OpenCode 任务: 基本满足
- 有流式展示: 基本满足，但仅在单会话原型层面成立
- 可稳定支持真实多人/多会话使用: 当前不满足
- 安全控制和运维能力: 当前不满足

## 修复完成后的验收清单

- 单聊和群聊都能稳定触发机器人
- 群聊必须 @ 机器人才执行
- 同时两个会话并发时不串流
- 同一会话多轮上下文连续
- bridge 重启后上下文仍可继续
- 非白名单用户无法触发
- `advanced` 和 `mvp` 模式都可正常返回答案
- `Stop` 按钮真实可用，或被明确移除
- 服务长时间运行后卡片更新仍正常
- 至少有覆盖核心链路的自动化测试
