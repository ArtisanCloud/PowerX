// app/composables/agent/useThinkParser.ts
import { computed, type Ref } from "vue";

export interface ThinkBlock {
  content: string;
  index: number;
}

export interface ParsedMessage {
  thinkBlocks: ThinkBlock[];
  mainContent: string;
  hasThink: boolean;
}

/**
 * 非流式：从完整文本中解析 <think>…</think>
 */
export function useThinkParser(content: Ref<string>) {
  const parsedMessage = computed<ParsedMessage>(() => {
    const raw = content.value || "";

    // ✅ 先去掉自闭合占位标签
    const withoutSelfClosing = raw.replace(/<think\s*\/>/gi, "");

    // 再匹配成对的 <think>…</think>
    const thinkRegex = /<think>([\s\S]*?)<\/think>/gi;
    const thinkMatches = Array.from(withoutSelfClosing.matchAll(thinkRegex));

    const thinkBlocks: ThinkBlock[] = thinkMatches.map((m, i) => ({
      content: (m[1] ?? "").trim(),
      index: i,
    }));

    // 主体文本也基于去除了自闭合标签的内容
    const mainContent = withoutSelfClosing.replace(thinkRegex, "").trim();

    return {
      thinkBlocks,
      mainContent,
      hasThink: thinkBlocks.length > 0,
    };
  });

  return { parsedMessage };
}

/**
 * 流式解析器：
 * - TOKEN/CHUNK：按增量 append
 * - DATA/FINAL：按快照 snapshot（重算，不追加）
 * 这样可以避免后端反复下发“全量文本”导致的重复块。
 */
export function useStreamingThinkParser() {
  let buffer = "";
  let completedThinks: { content: string; index: number }[] = [];
  let lastSnapshot = "";

  const completeThinkRegex = /<think>([\s\S]*?)<\/think>/gi;
  const incompleteTailRegex = /<think>(?![\s\S]*<\/think>)([\s\S]*)$/i;

  function recomputeFromSnapshot(snapshotText: string) {
    buffer = snapshotText;

    if (snapshotText === lastSnapshot) {
      const incomplete = buffer.match(incompleteTailRegex);
      const currentThinkContent = incomplete ? incomplete[1] : "";
      const mainContent = buffer
        .replace(completeThinkRegex, "")
        .replace(incompleteTailRegex, "")
        .trim();
      return {
        completedThinks: [...completedThinks],
        currentThinkContent,
        mainContent,
        hasActiveThink: !!incomplete,
        hasThink: completedThinks.length > 0 || !!incomplete,
      };
    }
    lastSnapshot = snapshotText;

    const matches = Array.from(buffer.matchAll(completeThinkRegex));
    completedThinks = matches.map((m, i) => ({
      content: (m[1] ?? "").trim(),
      index: i,
    }));

    const incomplete = buffer.match(incompleteTailRegex);
    const currentThinkContent = incomplete ? incomplete[1] : "";
    const mainContent = buffer
      .replace(completeThinkRegex, "")
      .replace(incompleteTailRegex, "")
      .trim();

    return {
      completedThinks: [...completedThinks],
      currentThinkContent,
      mainContent,
      hasActiveThink: !!incomplete,
      hasThink: completedThinks.length > 0 || !!incomplete,
    };
  }

  function appendDelta(delta: string) {
    buffer += delta;

    const matches = Array.from(buffer.matchAll(completeThinkRegex));
    const newOnes = matches.slice(completedThinks.length);
    if (newOnes.length > 0) {
      completedThinks.push(
        ...newOnes.map((m, i) => ({
          content: (m[1] ?? "").trim(),
          index: completedThinks.length + i,
        }))
      );
    }

    const incomplete = buffer.match(incompleteTailRegex);
    const currentThinkContent = incomplete ? incomplete[1] : "";
    const mainContent = buffer
      .replace(completeThinkRegex, "")
      .replace(incompleteTailRegex, "")
      .trim();

    return {
      completedThinks: [...completedThinks],
      currentThinkContent,
      mainContent,
      hasActiveThink: !!incomplete,
      hasThink: completedThinks.length > 0 || !!incomplete,
    };
  }

  /**
   * @param chunk 文本分片
   * @param mode  'delta'（TOKEN/CHUNK） | 'snapshot'（DATA/FINAL）
   */
  function parseStreamingContent(
    chunk: string,
    mode: "delta" | "snapshot" = "delta"
  ) {
    if (!chunk) {
      const incomplete = buffer.match(incompleteTailRegex);
      const mainContent = buffer
        .replace(completeThinkRegex, "")
        .replace(incompleteTailRegex, "")
        .trim();
      return {
        completedThinks: [...completedThinks],
        currentThinkContent: incomplete ? incomplete[1] : "",
        mainContent,
        hasActiveThink: !!incomplete,
        hasThink: completedThinks.length > 0 || !!incomplete,
      };
    }
    return mode === "snapshot"
      ? recomputeFromSnapshot(chunk)
      : appendDelta(chunk);
  }

  function reset() {
    buffer = "";
    completedThinks = [];
    lastSnapshot = "";
  }

  return { parseStreamingContent, reset };
}
