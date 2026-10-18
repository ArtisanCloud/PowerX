import { EventEmitter } from "node:events";
import { expect, it, vi } from "vitest";
const upstream = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("h3", () => ({ proxyRequest: upstream.request }));
import { proxyBackupStream } from "../../../server/utils/backup-stream-proxy";

it("propagates downstream close to the upstream subscription and releases the listener", async () => {
  const response = new EventEmitter(); let signal!: AbortSignal;
  upstream.request.mockImplementation((_event, _target, options) => {
    signal = options.fetchOptions.signal;
    return new Promise((_resolve, reject) => signal.addEventListener("abort", () => reject(new Error("aborted"))));
  });
  const result = proxyBackupStream({ node: { res: response } } as any, "http://127.0.0.1/events");
  expect(signal.aborted).toBe(false);
  response.emit("close"); await expect(result).resolves.toBeUndefined();
  expect(signal.aborted).toBe(true); expect(response.listenerCount("close")).toBe(0);
});

it("closes an already started SSE response when Core exits so the browser can reconnect", async () => {
  const response = Object.assign(new EventEmitter(), { headersSent: true, writableEnded: false, destroy: vi.fn() });
  upstream.request.mockRejectedValue(new Error("upstream connection terminated during exec"));
  await expect(proxyBackupStream({ node: { res: response } } as any, "http://127.0.0.1/events")).resolves.toBeUndefined();
  expect(response.destroy).toHaveBeenCalledOnce();
  expect(response.listenerCount("close")).toBe(0);
});

it("preserves gateway errors before SSE headers have been sent", async () => {
  const response = Object.assign(new EventEmitter(), { headersSent: false, destroy: vi.fn() });
  const error = new Error("connection refused");
  upstream.request.mockRejectedValue(error);
  await expect(proxyBackupStream({ node: { res: response } } as any, "http://127.0.0.1/events")).rejects.toBe(error);
  expect(response.destroy).not.toHaveBeenCalled();
  expect(response.listenerCount("close")).toBe(0);
});
