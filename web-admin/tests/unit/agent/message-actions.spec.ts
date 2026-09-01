import { describe, expect, it } from "vitest";
import type { ChatMessage } from "~/types/message";
import {
  findPreviousPersistedUserMessageId,
  normalizePersistedMessageId,
} from "~/utils/agent/messageActions";

describe("agent message actions", () => {
  it("finds the persisted user message that owns an assistant reply", () => {
    const messages = [
      { id: 41, role: "user", content: "first" },
      { id: 42, role: "assistant", content: "answer" },
    ] as ChatMessage[];

    expect(findPreviousPersistedUserMessageId(messages, 42)).toBe(41);
  });

  it("rejects temporary, zero, negative, and unsafe message ids", () => {
    expect(normalizePersistedMessageId("client_1")).toBeNull();
    expect(normalizePersistedMessageId("0")).toBeNull();
    expect(normalizePersistedMessageId(-1)).toBeNull();
    expect(normalizePersistedMessageId(Number.MAX_SAFE_INTEGER + 1)).toBeNull();
  });

  it("does not fall back to an older turn when the owning user message is temporary", () => {
    const messages = [
      { id: 41, role: "user", content: "older" },
      { id: 42, role: "assistant", content: "older answer" },
      { id: "client_pending", role: "user", content: "pending" },
      { id: "thinking_1", role: "assistant", content: "" },
    ] as ChatMessage[];

    expect(
      findPreviousPersistedUserMessageId(messages, "thinking_1")
    ).toBeNull();
  });
});
