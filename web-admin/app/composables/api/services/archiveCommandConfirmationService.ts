import { useApiClient } from "../index";
import {
  decodeCommandReview,
  decodeCommandReceipt,
  type CommandConfirmationInput,
} from "~/utils/archive-command-confirmation";
export function useArchiveCommandConfirmationService() {
  const api = useApiClient(),
    base = "/admin/ops/backup/archives",
    config = (signal?: AbortSignal) => ({
      signal,
      retry: 0,
      timeout: 60000,
      useGlobalLoading: false,
      useGlobalError: false,
    });
  return {
    async review(id: string, signal?: AbortSignal) {
      return decodeCommandReview(
        api.unwrap(
          await api.get(
            `${base}/commands/${encodeURIComponent(id)}/review`,
            config(signal),
          ),
        ),
      );
    },
    async confirm(
      id: string,
      input: CommandConfirmationInput,
      signal?: AbortSignal,
    ) {
      return decodeCommandReceipt(
        api.unwrap(
          await api.post(
            `${base}/commands/${encodeURIComponent(id)}/confirmations`,
            input,
            config(signal),
          ),
        ),
      );
    },
    async request(id: string, signal?: AbortSignal) {
      return decodeCommandReceipt(
        api.unwrap(
          await api.get(
            `${base}/command-confirmation-requests/${encodeURIComponent(id)}`,
            config(signal),
          ),
        ),
      );
    },
  };
}
