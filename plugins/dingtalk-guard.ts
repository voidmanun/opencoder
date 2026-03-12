import type { Plugin } from "@opencode-ai/plugin"

interface WhitelistConfig {
  default_allowed: string[]
  default_blocked: string[]
  user_overrides: Record<string, { allowed: string[]; blocked: string[] }>
  project_overrides: Record<string, { allowed: string[]; blocked: string[] }>
}

const DEFAULT_ALLOWED = [
  "read",
  "glob",
  "grep",
  "lsp_goto_definition",
  "lsp_find_references",
  "lsp_symbols",
  "lsp_diagnostics",
  "websearch_web_search_exa",
  "webfetch",
  "context7_resolve-library-id",
  "context7_query-docs",
]

const DEFAULT_BLOCKED = [
  "bash",
  "interactive_bash",
  "write",
  "edit",
  "ast_grep_replace",
]

export const DingTalkGuardPlugin: Plugin = async (ctx) => {
  return {
    "tool.execute.before": async (input, output) => {
      const toolName = input.tool
      const userId = process.env.DINGTALK_USER_ID || ""
      const projectPath = ctx.directory

      const allowed = getAllowedTools(userId, projectPath)
      const blocked = getBlockedTools(userId, projectPath)

      if (blocked.includes(toolName)) {
        throw new Error(`Tool "${toolName}" is blocked for this session`)
      }

      if (allowed.length > 0 && !allowed.includes(toolName)) {
        throw new Error(`Tool "${toolName}" is not in the allowed list`)
      }
    },

    "shell.env": async (input, output) => {
      output.env.DINGTALK_USER_ID = process.env.DINGTALK_USER_ID || ""
      output.env.DINGTALK_CONVERSATION_ID = process.env.DINGTALK_CONVERSATION_ID || ""
      output.env.DINGTALK_SESSION_KEY = process.env.DINGTALK_SESSION_KEY || ""
    },
  }
}

function getAllowedTools(userId: string, projectPath: string): string[] {
  const config = loadWhitelistConfig()

  if (userId && config.user_overrides?.[userId]?.allowed) {
    return config.user_overrides[userId].allowed
  }

  if (projectPath && config.project_overrides?.[projectPath]?.allowed) {
    return config.project_overrides[projectPath].allowed
  }

  return config.default_allowed || DEFAULT_ALLOWED
}

function getBlockedTools(userId: string, projectPath: string): string[] {
  const config = loadWhitelistConfig()

  if (userId && config.user_overrides?.[userId]?.blocked) {
    return config.user_overrides[userId].blocked
  }

  if (projectPath && config.project_overrides?.[projectPath]?.blocked) {
    return config.project_overrides[projectPath].blocked
  }

  return config.default_blocked || DEFAULT_BLOCKED
}

function loadWhitelistConfig(): WhitelistConfig {
  const configPath = process.env.TOOL_WHITELIST_PATH || "./config/tool_whitelist.json"

  try {
    const fs = require("fs")
    if (fs.existsSync(configPath)) {
      const content = fs.readFileSync(configPath, "utf-8")
      return JSON.parse(content)
    }
  } catch (err) {
    console.error("Failed to load whitelist config:", err)
  }

  return {
    default_allowed: DEFAULT_ALLOWED,
    default_blocked: DEFAULT_BLOCKED,
    user_overrides: {},
    project_overrides: {},
  }
}

export default DingTalkGuardPlugin