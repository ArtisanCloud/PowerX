export const WS_BUS_CMD = {
  SUBSCRIBE: "subscribe",
  UNSUBSCRIBE: "unsubscribe",
  PING: "ping",
} as const;

export const WS_BUS_TYPE = {
  WELCOME: "welcome",
  ACK: "ack",
  ERROR: "error",
  EVENT: "event",
} as const;

export type WSBusEnvelope<T = unknown> = {
  topic?: string;
  type: string;
  payload?: T;
  ts: number;
  trace_id?: string;
};

export type WSBusCommand = {
  type: string;
  topic?: string;
  topics?: string[];
  req_id?: string;
};

export type WSBusWelcomePayload = {
  protocol: string;
  server: string;
  heartbeat_sec?: number;
};

export type WSBusAckPayload = {
  req_id?: string;
  ok: boolean;
  message?: string;
  topics?: string[];
};

export type WSBusErrorPayload = {
  req_id?: string;
  code: string;
  message: string;
  detail?: string;
};

export type IngestionProgress = {
  tenant_uuid: string;
  space_uuid: string;
  job_uuid: string;
  status: string;
  stage: string;
  progress: number;
  chunk_total?: number;
  embedding_pct?: number;
  masking_pct?: number;
  updated_at: string;
};
