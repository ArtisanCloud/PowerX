import { useApiClient } from "../index";
import {
  consumeInterruptionReview,
  decodeRequestReview,
  type CloseInterruptionInput,
  decodeDispositionReceipt,
  type DispositionInput,
} from "~/utils/archive-disposition";
export function useArchiveDispositionService() {
  const api = useApiClient(),
    base = "/admin/ops/backup/archives",
    config = (signal?: AbortSignal) => ({
      signal,
      retry: 0,
      useGlobalLoading: false,
      useGlobalError: false,
      timeout: 330000,
    });
  return {
    async review(
      job: string,
      signal: AbortSignal,
      progress: (v: { objects: number; bytes: number }) => void,
    ) {
      const stream = await api.get<ReadableStream<Uint8Array>>(
        `${base}/jobs/${encodeURIComponent(job)}/interruption-review`,
        {
          ...config(signal),
          timeout: 0,
          responseType: "stream",
          headers: { Accept: "text/event-stream" },
        } as any,
      );
      return consumeInterruptionReview(stream, job, signal, progress);
    },
    async retain(job: string, input: DispositionInput, signal?: AbortSignal) {
      return decodeDispositionReceipt(
        api.unwrap(
          await api.post(
            `${base}/jobs/${encodeURIComponent(job)}/interruption-retentions`,
            input,
            config(signal),
          ),
        ),
      );
    },
    async requestReview(id: string, signal?: AbortSignal) {
      return decodeRequestReview(
        api.unwrap(
          await api.get(
            `${base}/interruption-requests/${encodeURIComponent(id)}/review`,
            config(signal),
          ),
        ),
      );
    },
    async closeRequest(
      id: string,
      input: CloseInterruptionInput,
      signal?: AbortSignal,
    ) {
      return decodeDispositionReceipt(
        api.unwrap(
          await api.post(
            `${base}/interruption-requests/${encodeURIComponent(id)}/closures`,
            input,
            config(signal),
          ),
        ),
      );
    },
    async request(id: string, signal?: AbortSignal) {
      return decodeDispositionReceipt(
        api.unwrap(
          await api.get(
            `${base}/interruption-requests/${encodeURIComponent(id)}`,
            config(signal),
          ),
        ),
      );
    },
  };
}
