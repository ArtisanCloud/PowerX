export const EVENT_TOPICS = {
  SYSTEM_NOTIFICATION: "_topic.system.notification",
  KNOWLEDGE_FEEDBACK_REPROCESS: "_topic.knowledge.space.feedback.reprocess",
} as const;

export const EVENT_NOTIFICATION_KIND = {
  EVENT_FABRIC_REPLAY_TASK: "_kind.event_fabric.replay.task",
} as const;

export const EVENT_SUBSCRIBERS = {
  KNOWLEDGE_REPROCESS: "_subscriber.knowledge_space.reprocess",
  KNOWLEDGE_CORPUS_CHECK: "_subscriber.knowledge_space.corpus_check",
  AUTH_CHALLENGE_TIMEOUT: "_subscriber.authorization.challenge_timeout",
  EVENT_FABRIC_REPLAY: "_subscriber.event_fabric.replay",
  SYSTEM_NOTIFICATION_DISPATCH: "_subscriber.system.notification_dispatch",
} as const;
