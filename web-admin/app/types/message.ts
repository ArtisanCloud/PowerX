// 消息类型常量定义
export const MESSAGE_TYPES = {
  TEXT: "text",
  MARKDOWN: "markdown",
  CODE: "code",
  IMAGE: "image",
  VIDEO: "video",
  CARD: "card",
  FILE: "file",
  SYSTEM: "system",
} as const;

export type MessageType = (typeof MESSAGE_TYPES)[keyof typeof MESSAGE_TYPES];

// SSE 事件类型常量定义
export const SSE_EVENT_TYPES = {
  START: "start",
  INTENT: "intent",
  PLAN: "plan",
  NODE_START: "node_start",
  NODE_END: "node_end",
  TOKEN: "token",
  DATA: "data",
  ACTION: "action",
  FINAL: "final",
  END: "end",
  ERROR: "error",
  HEARTBEAT: "heartbeat",
  ACK: "ack",
  META: "meta",
  CHUNK: "chunk",
  AGENT_RUN_STARTED: "agent_run.started",
  AGENT_RUN_RESPONSE_PLAN: "agent_run.response_plan",
  AGENT_RUN_INTENT_DETECTED: "agent_run.intent_detected",
  AGENT_RUN_PLAN_CREATED: "agent_run.plan_created",
  AGENT_RUN_TASK_STATUS: "agent_run.task_status",
  AGENT_RUN_TASK_STARTED: "agent_run.task_started",
  AGENT_RUN_AWAITING_PARAMS: "agent_run.awaiting_params",
  AGENT_RUN_TASK_COMPLETED: "agent_run.task_completed",
  AGENT_RUN_TASK_FAILED: "agent_run.task_failed",
  AGENT_RUN_FINAL: "agent_run.final",
  AGENT_RUN_ENDED: "agent_run.ended",
} as const;

export type SSEEventType =
  (typeof SSE_EVENT_TYPES)[keyof typeof SSE_EVENT_TYPES];

// 统一的消息角色
export type ChatRole = "user" | "assistant" | "system";

// 消息内容接口
export interface MessageContent {
  type: MessageType;
  data: any;
}

// 不同类型消息的数据结构
export interface TextContent {
  text: string;
}

export interface MarkdownContent {
  markdown: string;
}

export interface CodeContent {
  code: string;
  language: string;
  filename?: string;
}

export interface ImageContent {
  url: string;
  alt?: string;
  caption?: string;
  width?: number;
  height?: number;
}

export interface VideoContent {
  url: string;
  poster?: string;
  caption?: string;
  duration?: number;
}

export interface CardContent {
  title: string;
  description?: string;
  image?: string;
  url?: string;
  actions?: Array<{
    label: string;
    action: string;
    variant?: "primary" | "secondary" | "outline";
  }>;
  metadata?: Record<string, any>;
}

export interface FileContent {
  name: string;
  size: number;
  type: string;
  url: string;
  downloadUrl?: string;
}

export interface SystemContent {
  message: string;
  level: "info" | "warning" | "error" | "success";
}

// ✅ 统一的基础聊天消息（兼容历史：content 可为 string 或结构化内容数组）
export interface ChatMessage {
  id: string;
  role: ChatRole;
  content: string | MessageContent | MessageContent[];
  timestamp: Date;
  isThinking?: boolean; // 标记消息是否处于思考状态
  metadata?: Record<string, any>;
  status?: "sending" | "sent" | "error";
  done?: boolean; // 标记消息是否完成
  isError?: boolean; // 标记消息是否为错误消息
  isStreaming?: boolean; // 标记消息是否正在流式传输
}
