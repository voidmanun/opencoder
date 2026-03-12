package dingtalk

import (
	"fmt"
	"strings"
)

const streamingCardTemplate = `{
  "config": {
    "autoLayout": true,
    "enableForward": true
  },
  "header": {
    "title": {
      "type": "text",
      "text": "{{title}}"
    },
    "subtitle": {
      "type": "text",
      "text": "{{subtitle}}"
    }
  },
  "contents": [
    {
      "type": "markdown",
      "text": "{{content}}",
      "id": "markdown_content"
    },
    {
      "type": "divider",
      "id": "divider_1"
    },
    {
      "type": "box",
      "id": "status_box",
      "direction": "horizontal",
      "children": [
        {
          "type": "text",
          "text": "{{status}}",
          "id": "text_status"
        },
        {
          "type": "text",
          "text": "{{tokens}}",
          "id": "text_tokens"
        }
      ]
    },
    {
      "type": "divider",
      "id": "divider_2"
    },
    {
      "type": "box",
      "id": "thinking_box",
      "direction": "vertical",
      "children": [
        {
          "type": "text",
          "text": "{{thinkingLabel}}",
          "id": "thinking_label"
        },
        {
          "type": "markdown",
          "text": "{{thinkingContent}}",
          "id": "thinking_content"
        }
      ]
    },
    {
      "type": "box",
      "id": "tools_box",
      "direction": "vertical",
      "children": [
        {
          "type": "text",
          "text": "{{toolsLabel}}",
          "id": "tools_label"
        },
        {
          "type": "markdown",
          "text": "{{toolsContent}}",
          "id": "tools_content"
        }
      ]
    },
    {
      "type": "action",
      "id": "action_buttons",
      "actions": [
        {
          "type": "button",
          "id": "cancel_button",
          "label": {
            "type": "text",
            "text": "⏹️ 停止"
          },
          "actionType": "request",
          "data": {
            "action": "cancel",
            "sessionKey": "{{sessionKey}}"
          }
        }
      ]
    }
  ]
}`

type StreamingCard struct {
	Title           string
	Subtitle        string
	Content         string
	Status          string
	Tokens          string
	ThinkingLabel   string
	ThinkingContent string
	ToolsLabel      string
	ToolsContent    string
	SessionKey      string
}

func RenderStreamingCard(card *StreamingCard) string {
	result := streamingCardTemplate
	result = strings.ReplaceAll(result, "{{title}}", escapeJSON(card.Title))
	result = strings.ReplaceAll(result, "{{subtitle}}", escapeJSON(card.Subtitle))
	result = strings.ReplaceAll(result, "{{content}}", escapeJSON(card.Content))
	result = strings.ReplaceAll(result, "{{status}}", escapeJSON(card.Status))
	result = strings.ReplaceAll(result, "{{tokens}}", escapeJSON(card.Tokens))
	result = strings.ReplaceAll(result, "{{thinkingLabel}}", escapeJSON(card.ThinkingLabel))
	result = strings.ReplaceAll(result, "{{thinkingContent}}", escapeJSON(card.ThinkingContent))
	result = strings.ReplaceAll(result, "{{toolsLabel}}", escapeJSON(card.ToolsLabel))
	result = strings.ReplaceAll(result, "{{toolsContent}}", escapeJSON(card.ToolsContent))
	result = strings.ReplaceAll(result, "{{sessionKey}}", escapeJSON(card.SessionKey))
	return result
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

func FormatStatus(status string) string {
	switch status {
	case "thinking":
		return "🤔 思考中"
	case "streaming":
		return "✍️ 生成中"
	case "tool":
		return "🔧 执行工具"
	case "completed":
		return "✅ 完成"
	case "cancelled":
		return "⏹️ 已停止"
	case "error":
		return "❌ 出错"
	default:
		return status
	}
}

func FormatTokens(count int) string {
	if count <= 0 {
		return ""
	}
	return fmt.Sprintf("📊 %d tokens", count)
}

func FormatThinking(content string, isThinking bool) (label, formatted string) {
	if !isThinking || content == "" {
		return "", ""
	}

	if len(content) > 200 {
		content = content[:200] + "..."
	}

	return "💭 思考过程", content
}

func FormatTool(toolName string, isTool bool) (label, content string) {
	if !isTool || toolName == "" {
		return "", ""
	}

	return "🔧 工具调用", fmt.Sprintf("正在执行: `%s`", toolName)
}

func FormatCodeBlock(code, language string) string {
	if code == "" {
		return ""
	}

	lines := strings.Split(code, "\n")
	if len(lines) > 20 {
		code = strings.Join(lines[:20], "\n") + "\n... (已截断)"
	}

	return fmt.Sprintf("```%s\n%s\n```", language, code)
}

func TruncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "\n\n... (内容过长已截断)"
}
