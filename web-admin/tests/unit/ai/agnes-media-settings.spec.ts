import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import ModalityParamsForm from "~/components/settings/ai/ModalityParamsForm.vue";
import { agnesImageSizes, agnesVideoResolutions, syncAgnesMediaSettings } from "~/utils/agnesMediaSettings";

describe("Agnes 媒体配置", () => {
  it("切换供应商后选择合法尺寸，保留已填写的合法参数", () => {
    const state = { provider: "agnes", size: "1024x1024" };
    syncAgnesMediaSettings("image", state);
    expect(state.size).toBe("1K");
    state.size = "3K";
    syncAgnesMediaSettings("image", state);
    expect(state.size).toBe("3K");
  });
  it("从普通视频型号切到 Flash，分辨率切换到 720p", () => {
    const state = { provider: "agnes", model: "agnes-video-2.5-flash", resolution: "1080p", maxDurationSec: 60 };
    syncAgnesMediaSettings("video", state);
    expect(state).toMatchObject({ resolution: "720p", maxDurationSec: 5 });
    expect(agnesVideoResolutions(state.model)).toEqual(["720p"]);
  });
  it("刷新后恢复该型号保存的尺寸和时长", () => {
    const image = { provider: "agnes", size: "1K" };
    syncAgnesMediaSettings("image", image, { size: "4K" });
    expect(image.size).toBe("4K");
    const video = { provider: "agnes", model: "agnes-video-2.5", resolution: "720p", maxDurationSec: 5 };
    syncAgnesMediaSettings("video", video, { resolution: "2K", max_duration_sec: 8 });
    expect(video).toMatchObject({ resolution: "2K", maxDurationSec: 8 });
  });
  it("保持其他供应商的已有配置", () => {
    const state = { provider: "openai", size: "1024x1024" };
    syncAgnesMediaSettings("image", state);
    expect(state.size).toBe("1024x1024");
  });
  const render = (modality: "image" | "video", provider: string) => mount(ModalityParamsForm, {
    props: {
      activeModality: modality,
      llm: {}, image: { provider, size: "1K" }, embedding: {}, audioTts: {}, audioAsr: {},
      video: { provider, model: "agnes-video-2.5-flash", resolution: "720p", maxDurationSec: 5 }, model3d: {}, rerank: {},
      imageSizeOptions: agnesImageSizes, imageQualityOptions: ["standard"], imageFormatOptions: ["png"],
      videoResolutionOptions: ["720p"], truncateOptions: [], model3dFormatOptions: [], audioFormatOptions: [],
      audioQualityOptions: [], languageOptions: [], responseFormatOptions: [], topKOptions: [],
    },
    global: {
      mocks: { $t: (key: string) => key },
      stubs: {
        USlider: true,
        UCheckbox: true,
        UFormField: { template: "<div><slot /></div>" },
        USelect: { props: ["items", "modelValue"], template: "<select><option v-for='item in items' :key='item'>{{item}}</option></select>" },
        UInput: { props: ["min", "max"], template: "<input :min='min' :max='max' />" },
      },
    },
  });
  it("生图仅展示已支持的尺寸，不展示质量和编码格式", () => {
    const wrapper = render("image", "agnes");
    expect(wrapper.findAll("option").map((option) => option.text())).toEqual(agnesImageSizes);
    expect(wrapper.text()).not.toContain("图像质量");
    expect(wrapper.text()).not.toContain("输出格式");
    wrapper.unmount();
  });
  it("视频隐藏 FPS，限制 4 到 12 秒", () => {
    const wrapper = render("video", "agnes");
    expect(wrapper.text()).not.toContain("帧率");
    expect(wrapper.find("input").attributes()).toMatchObject({ min: "4", max: "12" });
    expect(wrapper.findAll("option").map((option) => option.text())).toEqual(["720p"]);
    wrapper.unmount();
  });
  it("其他供应商仍保留原来的参数区", () => {
    const wrapper = render("image", "openai");
    expect(wrapper.text()).toContain("图像质量");
    expect(wrapper.text()).toContain("输出格式");
    wrapper.unmount();
  });
});
