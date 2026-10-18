import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises } from "@vue/test-utils";
const auth = vi.hoisted(() => ({ current: { sessionId: "owner", status: "active" } as any }));
vi.mock("~/utils/auth-session", () => ({ AUTH_SESSION_EVENT: "auth-test", readAuthSession: () => auth.current }));
import { connectBackupEvents } from "~/utils/backup-sse";

let stop: (() => void) | undefined;
beforeEach(() => { vi.useFakeTimers(); auth.current = { sessionId: "owner", status: "active" }; });
afterEach(() => { stop?.(); stop = undefined; vi.useRealTimers(); });
const source = () => {
  let controller!: ReadableStreamDefaultController<Uint8Array>;
  const cancel = vi.fn();
  const stream = new ReadableStream<Uint8Array>({ start(c) { controller = c; }, cancel });
  return { stream, cancel, send: (text: string) => controller.enqueue(new TextEncoder().encode(text)), end: () => controller.close() };
};
describe("backup SSE transport", () => {
  it("renews the authorized connection without reporting normal renewal as failure", async () => {
    const first = source(), next = source(), state = vi.fn();
    const open = vi.fn().mockResolvedValueOnce(first.stream).mockResolvedValue(next.stream);
    stop = connectBackupEvents(open, vi.fn(), state); await flushPromises();
    first.send('event: reconnect\ndata: {"reason":"reauthorize"}\n\n'); first.end();
    await flushPromises(); await vi.advanceTimersByTimeAsync(0);
    expect(open).toHaveBeenCalledTimes(2); expect(state).not.toHaveBeenCalledWith("disconnected");
  });
  it("consumes split frames and heartbeats without requesting status on a timer", async () => {
    const input = source(), receive = vi.fn(), open = vi.fn().mockResolvedValue(input.stream);
    stop = connectBackupEvents(open, receive); await flushPromises();
    input.send('event: invalidate\r\n'); input.send('data: {"resources":["snapshots"]}\r'); input.send('\n\r\n: keepalive\n\n');
    await flushPromises(); expect(receive).toHaveBeenCalledWith({ name: "invalidate", data: { resources: ["snapshots"] } });
    await vi.advanceTimersByTimeAsync(30000); expect(open).toHaveBeenCalledTimes(1); expect(receive).toHaveBeenCalledTimes(1);
    stop(); expect(input.cancel).toHaveBeenCalledTimes(1);
  });
  it("reconnects only after disconnection and cancels pending reconnection on unsubscribe", async () => {
    const first = source(), next = source(); const open = vi.fn().mockResolvedValueOnce(first.stream).mockResolvedValue(next.stream);
    const state = vi.fn(); stop = connectBackupEvents(open, vi.fn(), state); await flushPromises();
    first.end(); await flushPromises(); expect(state).toHaveBeenLastCalledWith("disconnected");
    await vi.advanceTimersByTimeAsync(500); expect(open).toHaveBeenCalledTimes(2);
    stop(); expect(next.cancel).toHaveBeenCalledTimes(1); await vi.advanceTimersByTimeAsync(10000); expect(open).toHaveBeenCalledTimes(2);
  });
  it("backs off repeated short lived connections instead of reopening every half second", async () => {
    const first=source(), next=source(), third=source();
    const open=vi.fn().mockResolvedValueOnce(first.stream).mockResolvedValueOnce(next.stream).mockResolvedValue(third.stream);
    stop=connectBackupEvents(open,vi.fn()); await flushPromises();
    first.end(); await flushPromises(); await vi.advanceTimersByTimeAsync(500); expect(open).toHaveBeenCalledTimes(2);
    next.end(); await flushPromises(); await vi.advanceTimersByTimeAsync(500); expect(open).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(500); expect(open).toHaveBeenCalledTimes(3);
  });
  it("closes old task streams immediately when the login identity changes", async () => {
    const input = source(), receive = vi.fn(), open = vi.fn().mockResolvedValue(input.stream);
    stop = connectBackupEvents(open, receive); await flushPromises();
    auth.current = { sessionId: "different-user", status: "active" };
    window.dispatchEvent(new Event("auth-test")); await flushPromises();
    expect(input.cancel).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(60000); expect(open).toHaveBeenCalledTimes(1); expect(receive).not.toHaveBeenCalled();
  });
});
