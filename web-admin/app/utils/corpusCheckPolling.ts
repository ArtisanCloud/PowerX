import type { CorpusCheckJobRecord } from "~/composables/useKnowledgeSpaces";

type PollOptions = {
  maxAttempts?: number;
  initialDelayMs?: number;
  maxDelayMs?: number;
};

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

const isTerminal = (job?: CorpusCheckJobRecord | null) =>
  job?.status === "completed" || job?.status === "failed";

export const pollCorpusCheckJob = async (
  fetchLatest: () => Promise<CorpusCheckJobRecord>,
  onUpdate?: (job: CorpusCheckJobRecord) => void,
  opts?: PollOptions,
): Promise<CorpusCheckJobRecord> => {
  const maxAttempts = opts?.maxAttempts ?? 6;
  let delayMs = opts?.initialDelayMs ?? 1000;
  const maxDelayMs = opts?.maxDelayMs ?? 5000;

  let last: CorpusCheckJobRecord | null = null;
  for (let i = 0; i < maxAttempts; i++) {
    last = await fetchLatest();
    onUpdate?.(last);
    if (isTerminal(last)) return last;
    if (i < maxAttempts - 1) {
      await sleep(delayMs);
      delayMs = Math.min(maxDelayMs, Math.round(delayMs * 2));
    }
  }

  return last ?? (await fetchLatest());
};

