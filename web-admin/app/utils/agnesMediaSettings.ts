export const agnesImageSizes = ["1K", "2K", "3K", "4K"];

export const isAgnesProvider = (provider: unknown) =>
  String(provider || "").trim().toLowerCase() === "agnes";

export function agnesVideoResolutions(model: unknown): string[] {
  return model === "agnes-video-2.5-flash"
    ? ["720p"]
    : ["720p", "1080p", "2K"];
}

// 仅在切换供应商／型号或加载保存值时应用；不改写用户正在填写的参数。
export function syncAgnesMediaSettings(
  modality: string,
  state: Record<string, any>,
  saved: Record<string, any> = {},
) {
  if (!isAgnesProvider(state.provider)) return;
  if (modality === "image") {
    const size = saved.size ?? state.size;
    state.size = agnesImageSizes.includes(size) ? size : "1K";
  } else if (modality === "video") {
    const resolutions = agnesVideoResolutions(state.model);
    const resolution = saved.resolution ?? state.resolution;
    state.resolution = resolutions.includes(resolution) ? resolution : "720p";
    const seconds = Number(saved.max_duration_sec ?? saved.maxDurationSec ?? state.maxDurationSec);
    state.maxDurationSec = Number.isInteger(seconds) && seconds >= 4 && seconds <= 12 ? seconds : 5;
  }
}
