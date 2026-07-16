import { shallowMount } from "@vue/test-utils";
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";

const listDictionaries = vi.fn();
const listDictionaryItems = vi.fn();

vi.mock("~/composables/api/services/metadataGovernanceService", () => ({
  useMetadataGovernanceService: () => ({
    listDictionaries,
    listDictionaryItems,
    listTaxonomies: vi.fn().mockResolvedValue({ items: [] }),
    listTaxonomyNodes: vi.fn().mockResolvedValue({ items: [] }),
    listTags: vi.fn().mockResolvedValue({ items: [] }),
    listResourceTypes: vi.fn().mockResolvedValue({ items: [] }),
  }),
}));

describe("metadata governance page", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    vi.stubGlobal("useI18n", () => ({
      locale: { value: "zh-CN" },
      t: (key: string) => key,
    }));
    vi.stubGlobal("useMe", () => ({
      hasPermission: vi.fn().mockResolvedValue(true),
    }));
    listDictionaries.mockResolvedValue({
      items: [],
      pagination: { total: 0, page: 1, page_size: 20 },
    });
    listDictionaryItems.mockResolvedValue({ items: [] });
  });

  it("renders page shell with i18n keys and filter controls", async () => {
    const page = await import("./index.vue");
    const wrapper = shallowMount(page.default, {
      global: {
        stubs: {
          UButton: true,
          UCard: true,
          UInput: true,
          USelect: true,
          UTabs: true,
          UPagination: true,
          UBadge: true,
          UAlert: true,
          UIcon: true,
          MetadataStatePanel: true,
        },
      },
    });
    expect(wrapper.text()).toContain("settings.metadataGovernance.title");
    expect(wrapper.text()).toContain("settings.metadataGovernance.description");
  });
});
