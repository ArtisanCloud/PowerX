import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { computed, defineAsyncComponent, effectScope, nextTick, ref, watch, type EffectScope } from "vue";

let scope: EffectScope;
let auto: ReturnType<typeof ref<boolean>>, manual: ReturnType<typeof ref<boolean>>, lock: ReturnType<typeof ref<number>>;
let modal: { open: ReturnType<typeof vi.fn>; close: ReturnType<typeof vi.fn>; patch: ReturnType<typeof vi.fn> };
let resolveClose: () => void;

beforeEach(async () => {
  vi.useFakeTimers();
  auto = ref(false); manual = ref(false); lock = ref(0);
  modal = {
    open: vi.fn(() => ({ result: new Promise<void>((resolve) => { resolveClose = resolve; }) })),
    close: vi.fn(() => resolveClose()), patch: vi.fn(),
  };
  vi.stubGlobal("defineNuxtPlugin", (fn: any) => fn);
  vi.stubGlobal("defineAsyncComponent", defineAsyncComponent);
  vi.stubGlobal("ref", ref); vi.stubGlobal("computed", computed); vi.stubGlobal("watch", watch);
  vi.stubGlobal("useOverlay", () => ({ create: () => modal }));
  vi.stubGlobal("useGL_AutoVisible", () => auto);
  vi.stubGlobal("useGL_ManualVisible", () => manual);
  vi.stubGlobal("useGL_LockCount", () => lock);
  vi.stubGlobal("useGL_Message", () => ref("加载中"));
  vi.stubGlobal("useGL_Progress", () => ref());
  const plugin = await import("~/plugins/gl-overlay.client");
  scope = effectScope();
  scope.run(() => (plugin.default as any)());
});
afterEach(() => { scope.stop(); vi.useRealTimers(); vi.unstubAllGlobals(); });

describe("fullscreen loading visibility", () => {
  it("does not flash a fullscreen modal for a fast navigation or request", async () => {
    auto.value = true; await nextTick();
    await vi.advanceTimersByTimeAsync(190);
    auto.value = false; await nextTick();
    await vi.advanceTimersByTimeAsync(1000);
    expect(modal.open).not.toHaveBeenCalled();
  });

  it("opens once for a longer load and closes when it finishes", async () => {
    auto.value = true; await nextTick();
    await vi.advanceTimersByTimeAsync(200);
    expect(modal.open).toHaveBeenCalledTimes(1);
    auto.value = false; await nextTick();
    expect(modal.close).toHaveBeenCalledTimes(1);
  });

  it.each(["manual", "lock"])("shows explicit %s loading immediately", async (kind) => {
    if (kind === "manual") manual.value = true;
    else lock.value = 1;
    await nextTick();
    expect(modal.open).toHaveBeenCalledTimes(1);
    auto.value = true; await nextTick();
    auto.value = false; await nextTick();
    expect(modal.close).not.toHaveBeenCalled();
  });

  it("opens a still-pending load after the previous modal finishes closing", async () => {
    modal.close.mockImplementation(() => {});
    auto.value = true; await nextTick(); await vi.advanceTimersByTimeAsync(200);
    auto.value = false; await nextTick();
    auto.value = true; await nextTick(); await vi.advanceTimersByTimeAsync(200);
    expect(modal.open).toHaveBeenCalledTimes(1);
    resolveClose(); await nextTick();
    expect(modal.open).toHaveBeenCalledTimes(2);
  });
});
