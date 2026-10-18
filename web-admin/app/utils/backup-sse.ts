import { AUTH_SESSION_EVENT, readAuthSession } from "~/utils/auth-session";

export interface BackupEvent { name: string; data: any }

// 只在连接中断时重连；不定时请求业务状态。心跳超时只重建事件连接。
export function connectBackupEvents(
  open: (signal: AbortSignal) => Promise<ReadableStream<Uint8Array>>,
  receive: (event: BackupEvent) => void,
  state: (value: "connecting" | "connected" | "disconnected") => void = () => {},
): () => void {
  let stopped = false, generation = 0, attempts = 0;
  let controller: AbortController | undefined;
  let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let idle: ReturnType<typeof setTimeout> | undefined;
  const owner = readAuthSession()?.sessionId;
  const authorized = () => !!owner && readAuthSession()?.status === "active" && readAuthSession()?.sessionId === owner;
  const clear = () => { clearTimeout(retry); clearTimeout(idle); };
  const abort = () => { controller?.abort(); void reader?.cancel().catch(() => {}); reader = undefined; };
  const stop = () => {
    if (stopped) return;
    stopped = true; generation++; clear(); abort();
    window.removeEventListener("online", restart);
    window.removeEventListener("offline", offline);
    window.removeEventListener(AUTH_SESSION_EVENT, sessionChanged);
  };
  async function run() {
    if (stopped || !authorized() || !navigator.onLine) return;
    const version = ++generation;
    const current = new AbortController(); controller = current;
    const openedAt = Date.now();
    state("connecting");
    let currentReader: ReadableStreamDefaultReader<Uint8Array> | undefined;
    let renewal = false;
    try {
      const stream = await open(current.signal);
      if (stopped || version !== generation || !authorized()) { await stream.cancel(); return; }
      currentReader = stream.getReader(); reader = currentReader;
      state("connected");
      const decoder = new TextDecoder(); let buffer = "";
      for (;;) {
        clearTimeout(idle); idle = setTimeout(() => current.abort(), 45000);
        const { value, done } = await currentReader.read();
        if (stopped || version !== generation || !authorized()) return;
        if (done) break;
        buffer = (buffer + decoder.decode(value, { stream: true })).replace(/\r\n/g, "\n");
        if (buffer.length > 1048576) throw new Error("Backup event exceeds frame limit");
        let end: number;
        while ((end = buffer.indexOf("\n\n")) >= 0) {
          const frame = buffer.slice(0, end); buffer = buffer.slice(end + 2);
          let name = "message"; const data: string[] = [];
          for (const line of frame.split("\n")) {
            if (line.startsWith("event:")) name = line.slice(6).trim();
            if (line.startsWith("data:")) data.push(line.slice(5).replace(/^ /, ""));
          }
          if (stopped || version !== generation || !authorized()) return;
          if (name === "reconnect") { renewal = true; continue; }
          if (data.length) receive({ name, data: JSON.parse(data.join("\n")) });
        }
      }
    } catch { /* 重连并重新同步，保留当前任务与表单。 */ }
    finally {
      void currentReader?.cancel().catch(() => {});
      if (version === generation && !stopped) {
        clearTimeout(idle); reader = undefined;
        if (renewal || Date.now() - openedAt > 10000) attempts = 0;
        state(renewal ? "connecting" : "disconnected");
        if (authorized() && navigator.onLine) retry = setTimeout(run, renewal ? 0 : Math.min(30000, 500 * 2 ** attempts++));
      }
    }
  }
  function restart() { if (stopped) return; generation++; attempts = 0; clear(); abort(); void run(); }
  function offline() { generation++; clear(); abort(); state("disconnected"); }
  function sessionChanged() { if (!authorized()) stop(); else restart(); }
  window.addEventListener("online", restart);
  window.addEventListener("offline", offline);
  window.addEventListener(AUTH_SESSION_EVENT, sessionChanged);
  void run();
  return stop;
}
