import { proxyRequest, type H3Event } from "h3";

// 当前 h3 的响应流转发不会自动把下游关闭传给 fetch；显式取消上游，
// 使离开备份页或切换身份时，Core／协调器及时释放文件监听和连接。
export async function proxyBackupStream(event: H3Event, target: string, headers?: Record<string, string>) {
  const controller = new AbortController();
  const cancel = () => controller.abort();
  event.node.res.once("close", cancel);
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
