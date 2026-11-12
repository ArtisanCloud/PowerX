import { proxyRequest, type H3Event } from "h3";

// Only detect the listed media type; the upstream still owns negotiation and
// its response status/content type. This never turns a JSON response into SSE.
export function requestsEventStream(accept?: string) {
  return accept?.split(",").some(range => range.split(";")[0]?.trim().toLowerCase() === "text/event-stream") ?? false;
}

// h3 does not propagate downstream closure to fetch. Share this lifecycle for
// plugin, Core, backup, installation and maintenance event subscriptions.
export async function proxyEventStream(event: H3Event, target: string, headers?: Record<string, string>) {
  const controller = new AbortController();
  const cancel = () => controller.abort();
  event.node.res.once("close", cancel);
  if (event.node.res.destroyed || event.node.res.writableEnded) cancel();
  try {
    return await proxyRequest(event, target, { headers, fetchOptions: { signal: controller.signal } });
  } catch (error) {
    // 上游在 Core exec 时可能在已经发出 SSE 响应头后断开。h3 将这种
    // 响应视为 handled，默认错误处理不会结束它，浏览器会永久等在 read()。
    // 明确关闭下游，让浏览器按连接故障重连；不能向 SSE 中写入 JSON 错误。
    if (event.node.res.headersSent) {
      if (!event.node.res.writableEnded) event.node.res.destroy();
      return;
    }
    if (!controller.signal.aborted) throw error;
  } finally {
    event.node.res.off("close", cancel);
  }
}
