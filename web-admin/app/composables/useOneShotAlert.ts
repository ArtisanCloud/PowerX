// composables/useOneShotAlert.ts
export const useOneShotAlert = () => {
  const alertedOnce = useState("chat-alerted-once", () => false);
  const visible = useState("chat-alert-visible", () => false);
  const title = useState("chat-alert-title", () => "");
  const description = useState("chat-alert-desc", () => "");
  const color = useState("chat-alert-color", () => "error");
  const variant = useState("chat-alert-variant", () => "solid");
  const icon = useState("chat-alert-icon", () => "i-heroicons-wifi-20-solid");

  const notifyOnce = (
    t: string,
    d?: string,
    c:
      | "error"
      | "primary"
      | "secondary"
      | "success"
      | "info"
      | "warning"
      | "neutral" = "error",
    v: "solid" | "soft" | "outline" = "solid"
  ) => {
    if (alertedOnce.value) return false;
    title.value = t;
    description.value = d || "";
    color.value = c;
    variant.value = v;
    visible.value = true;
    alertedOnce.value = true;
    return true;
  };

  // 仅临时关闭（本会话不再重复弹出）
  const hide = () => {
    visible.value = false;
  };

  // 若想"关闭后允许再次弹一次"，用这个替换上面的 hide：
  // const hide = () => { visible.value = false; alertedOnce.value = false }

  const reset = () => {
    visible.value = false;
    alertedOnce.value = false;
  };

  return {
    alertedOnce,
    visible,
    title,
    description,
    color,
    variant,
    icon,
    notifyOnce,
    hide,
    reset,
  };
};
