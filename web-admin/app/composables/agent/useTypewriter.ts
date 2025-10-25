import { ref, computed, watch, nextTick } from "vue";

export interface TypewriterOptions {
  speed?: number; // 打字速度（毫秒/字符）
  minSpeed?: number; // 最小速度
  maxSpeed?: number; // 最大速度
  chunkSize?: number; // 每次显示的字符数
  autoStart?: boolean; // 是否自动开始
  onComplete?: () => void; // 完成回调
  onProgress?: (progress: number) => void; // 进度回调
}

export function useTypewriter(options: TypewriterOptions = {}) {
  const {
    speed = 30,
    minSpeed = 10,
    maxSpeed = 100,
    chunkSize = 1,
    autoStart = true,
    onComplete,
    onProgress,
  } = options;

  const fullText = ref("");
  const displayedText = ref("");
  const isTyping = ref(false);
  const isPaused = ref(false);
  const currentIndex = ref(0);

  let animationId: number | null = null;
  let lastTime = 0;

  // 计算进度
  const progress = computed(() => {
    if (!fullText.value) return 0;
    return Math.min(currentIndex.value / fullText.value.length, 1);
  });

  // 是否完成
  const isComplete = computed(() => {
    return currentIndex.value >= fullText.value.length;
  });

  // 动态调整速度（根据内容类型）
  const getAdaptiveSpeed = (char: string, nextChar?: string) => {
    // 标点符号后稍微停顿
    if (/[。！？；：，]/.test(char)) {
      return speed * 2;
    }
    // 英文单词间正常速度
    if (char === " " && /[a-zA-Z]/.test(nextChar || "")) {
      return speed * 0.8;
    }
    // 数字和字母稍快
    if (/[a-zA-Z0-9]/.test(char)) {
      return speed * 0.7;
    }
    return speed;
  };

  // 开始打字动画
  const startTyping = () => {
    if (isComplete.value || isTyping.value) return;

    isTyping.value = true;
    isPaused.value = false;
    lastTime = performance.now();

    const animate = (currentTime: number) => {
      if (isPaused.value) {
        animationId = requestAnimationFrame(animate);
        return;
      }

      const elapsed = currentTime - lastTime;
      const currentChar = fullText.value[currentIndex.value];
      const nextChar = fullText.value[currentIndex.value + 1];
      const currentSpeed = Math.max(
        minSpeed,
        Math.min(maxSpeed, getAdaptiveSpeed(currentChar, nextChar))
      );

      if (elapsed >= currentSpeed) {
        // 更新显示的文本
        const endIndex = Math.min(
          currentIndex.value + chunkSize,
          fullText.value.length
        );
        displayedText.value = fullText.value.slice(0, endIndex);
        currentIndex.value = endIndex;

        // 触发进度回调
        onProgress?.(progress.value);

        lastTime = currentTime;
      }

      if (currentIndex.value < fullText.value.length) {
        animationId = requestAnimationFrame(animate);
      } else {
        // 完成
        isTyping.value = false;
        onComplete?.();
      }
    };

    animationId = requestAnimationFrame(animate);
  };

  // 暂停打字
  const pause = () => {
    isPaused.value = true;
  };

  // 恢复打字
  const resume = () => {
    if (isTyping.value) {
      isPaused.value = false;
    }
  };

  // 停止打字
  const stop = () => {
    if (animationId) {
      cancelAnimationFrame(animationId);
      animationId = null;
    }
    isTyping.value = false;
    isPaused.value = false;
  };

  // 重置
  const reset = () => {
    stop();
    displayedText.value = "";
    currentIndex.value = 0;
  };

  // 立即完成
  const complete = () => {
    stop();
    displayedText.value = fullText.value;
    currentIndex.value = fullText.value.length;
    onComplete?.();
  };

  // 设置新文本
  const setText = (text: string, startImmediately = autoStart) => {
    // 如果新文本是当前文本的扩展，则继续打字
    if (
      text.startsWith(fullText.value) &&
      text.length > fullText.value.length
    ) {
      fullText.value = text;
      if (!isTyping.value && startImmediately) {
        startTyping();
      }
      return;
    }

    // 否则重新开始
    stop();
    fullText.value = text;
    displayedText.value = "";
    currentIndex.value = 0;

    if (startImmediately && text) {
      nextTick(() => {
        startTyping();
      });
    }
  };

  // 追加文本（用于流式更新）
  const appendText = (text: string) => {
    const newFullText = fullText.value + text;
    setText(newFullText, !isComplete.value);
  };

  // 监听完整文本变化
  watch(fullText, (newText) => {
    if (newText && autoStart && !isTyping.value && currentIndex.value === 0) {
      nextTick(() => {
        startTyping();
      });
    }
  });

  // 清理
  const cleanup = () => {
    stop();
  };

  return {
    // 状态
    fullText: readonly(fullText),
    displayedText: readonly(displayedText),
    isTyping: readonly(isTyping),
    isPaused: readonly(isPaused),
    progress: readonly(progress),
    isComplete: readonly(isComplete),

    // 方法
    setText,
    appendText,
    startTyping,
    pause,
    resume,
    stop,
    reset,
    complete,
    cleanup,
  };
}

// 专门用于消息的打字机效果
export function useMessageTypewriter(options: TypewriterOptions = {}) {
  const typewriter = useTypewriter({
    speed: 25, // 稍快的默认速度
    chunkSize: 1,
    ...options,
  });

  // 处理消息内容更新
  const updateMessage = (content: string, isStreaming = false) => {
    if (!typewriter) return;

    if (isStreaming) {
      // 流式更新：如果内容是扩展的，继续打字
      if (content.startsWith(typewriter.fullText.value)) {
        typewriter.setText(content, true);
      } else {
        // 新消息，重新开始
        typewriter.setText(content, true);
      }
    } else {
      // 非流式：直接设置完整内容
      typewriter.setText(content, true);
    }
  };

  // 确保返回的对象始终有完整的结构
  return {
    // 状态 - 提供默认值防止 undefined
    fullText: typewriter?.fullText ?? ref(""),
    displayedText: typewriter?.displayedText ?? ref(""),
    isTyping: typewriter?.isTyping ?? ref(false),
    isPaused: typewriter?.isPaused ?? ref(false),
    progress: typewriter?.progress ?? ref(0),
    isComplete: typewriter?.isComplete ?? ref(true),

    // 方法 - 提供安全的默认实现
    setText: typewriter?.setText ?? (() => {}),
    appendText: typewriter?.appendText ?? (() => {}),
    startTyping: typewriter?.startTyping ?? (() => {}),
    pause: typewriter?.pause ?? (() => {}),
    resume: typewriter?.resume ?? (() => {}),
    stop: typewriter?.stop ?? (() => {}),
    reset: typewriter?.reset ?? (() => {}),
    complete: typewriter?.complete ?? (() => {}),
    cleanup: typewriter?.cleanup ?? (() => {}),
    updateMessage,
  };
}
