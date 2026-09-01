import type { ChatMessage } from "~/types/message";

export const normalizePersistedMessageId = (
  value: string | number | null | undefined
): string | number | null => {
  if (typeof value === "number") {
    return Number.isSafeInteger(value) && value > 0 ? value : null;
  }
  const normalized = String(value ?? "").trim();
  return /^[1-9]\d*$/.test(normalized) ? normalized : null;
};

export const findPreviousPersistedUserMessageId = (
  messages: ReadonlyArray<ChatMessage>,
  messageId: string | number
): string | number | null => {
  const index = messages.findIndex(
    (item) => String(item?.id) === String(messageId)
  );
  if (index <= 0) return null;

  for (let i = index - 1; i >= 0; i--) {
    const candidate = messages[i];
    if (candidate?.role !== "user") continue;
    return normalizePersistedMessageId(candidate.id);
  }
  return null;
};
