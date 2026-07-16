import { beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { useMetadataGovernanceStore } from "./metadata-governance";

const listDictionaries = vi.fn();
const listDictionaryItems = vi.fn();
const listResourceTypes = vi.fn();

vi.mock("~/composables/api/services/metadataGovernanceService", () => ({
  useMetadataGovernanceService: () => ({
    listDictionaries,
    listDictionaryItems,
    listResourceTypes,
  }),
}));

describe("metadata governance store", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it("loads dictionaries and selects the first namespace", async () => {
    listDictionaries.mockResolvedValueOnce({
      items: [{ uuid: "ns-1", display_name: "Status", namespace: "corex.metadata.status", module: "corex.metadata" }],
      pagination: { total: 1, page: 1, page_size: 20 },
    });
    listDictionaryItems.mockResolvedValueOnce({
      items: [{ uuid: "item-1", display_name: "Enabled", code: "enabled", status: "enabled", reference_count: 0 }],
    });
    const store = useMetadataGovernanceStore();
    await store.fetchDictionaries();
    expect(store.dictionaries).toHaveLength(1);
    expect(store.selectedDictionaryUuid).toBe("ns-1");
    expect(store.dictionaryItems).toHaveLength(1);
    expect(store.tabState("dictionaries")).toBe("ready");
  });

  it("keeps no-permission state separate from empty data", async () => {
    const store = useMetadataGovernanceStore();
    store.setPermissions(false, false);
    await store.fetchResourceTypes();
    expect(listResourceTypes).not.toHaveBeenCalled();
    expect(store.tabState("resourceTypes")).toBe("no_permission");
  });

  it("records backend errors as error state", async () => {
    listResourceTypes.mockRejectedValueOnce(new Error("backend failed"));
    const store = useMetadataGovernanceStore();
    await store.fetchResourceTypes();
    expect(store.error).toBe("backend failed");
    expect(store.tabState("resourceTypes")).toBe("error");
  });
});
