import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { computed, nextTick, onMounted, ref, watch } from "vue";
import PluginsMarket from "~/pages/plugins/market.vue";

const marketplaceMock = vi.fn();
const installMock = vi.fn();

vi.mock("~/stores/user", () => ({
  useUserStore: () => ({
    isRoot: false,
    isCurrentTenantAdmin: true,
  }),
}));

vi.mock("~/composables/api/services/adminPluginsService", () => ({
  useAdminPluginsService: () => ({
    getMarketplace: marketplaceMock,
    setTenantEnabled: vi.fn(),
  }),
}));

vi.mock("#imports", () => ({
  definePageMeta: vi.fn(),
  useI18n: () => ({ t: (key: string) => key }),
}));

function mountMarket() {
  return mount(PluginsMarket, {
    global: {
      mocks: {
        $t: (key: string) => key,
      },
      stubs: {
        UIcon: { template: "<i />" },
        UInput: {
          props: ["modelValue"],
          emits: ["update:modelValue"],
          template:
            "<input :value='modelValue' @input=\"$emit('update:modelValue', $event.target.value)\" />",
        },
        USelect: {
          props: ["modelValue", "items"],
          emits: ["update:modelValue"],
          template:
            "<select :value='modelValue' @change=\"$emit('update:modelValue', $event.target.value)\"><option v-for='item in items' :key='item' :value='item'>{{ item }}</option></select>",
        },
        UPagination: { template: "<nav />" },
        UBadge: { template: "<span><slot /></span>" },
        UButton: {
          props: ["to", "variant", "size", "icon", "color"],
          emits: ["click"],
          template: "<button @click=\"$emit('click', $event)\"><slot /></button>",
        },
        InstallDialog: {
          props: ["modelValue", "plugin"],
          emits: ["update:modelValue", "installed"],
          template: "<div />",
        },
        PluginCard: {
          props: [
            "plugin",
            "isSystemInstalled",
            "isSystemEnabled",
            "systemStatus",
            "isTenantEnabled",
            "tenantStatus",
            "canInstall",
            "canTenantEnable",
          ],
          emits: ["install", "tenant-enable", "tenant-disable"],
          template:
            "<article class='plugin-card' :data-id='plugin.id' :data-system-enabled='String(isSystemEnabled)' :data-tenant-enabled='String(isTenantEnabled)' :data-tenant-status='tenantStatus || \"\"'><span>{{ plugin.name }}</span></article>",
        },
      },
    },
  });
}

describe("插件市场租户实例状态", () => {
beforeEach(() => {
  marketplaceMock.mockReset();
  installMock.mockReset();
  vi.stubGlobal("definePageMeta", vi.fn());
  vi.stubGlobal("ref", ref);
  vi.stubGlobal("computed", computed);
  vi.stubGlobal("watch", watch);
  vi.stubGlobal("onMounted", onMounted);
});

  it("必须区分全局插件包已启用和当前租户实例已启用", async () => {
    marketplaceMock.mockResolvedValue([
      {
        id: "com.powerx.plugins.scrm",
        name: "SCRM",
        version: "0.1.0",
        description: "SCRM plugin",
        author: "PowerX",
        category: "SCRM",
        tags: [],
        isSystemInstalled: true,
        isSystemEnabled: true,
        systemStatus: "enabled",
        tenantInstance: {
          plugin_id: "com.powerx.plugins.scrm",
          version: "0.1.0",
          enabled: false,
          status: "disabled",
        },
      },
    ]);

    const wrapper = mountMarket();
    await flushPromises();
    await flushPromises();
    await nextTick();

    const card = wrapper.find("[data-id='com.powerx.plugins.scrm']");
    expect(card.exists()).toBe(true);
    expect(card.attributes("data-system-enabled")).toBe("true");
    expect(card.attributes("data-tenant-enabled")).toBe("false");
    expect(card.attributes("data-tenant-status")).toBe("disabled");
  });
});
