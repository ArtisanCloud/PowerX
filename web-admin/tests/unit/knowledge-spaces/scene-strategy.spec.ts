import { beforeEach, describe, expect, it } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { SCENE_STRATEGY_CATALOG } from "~/constants/sceneStrategyCatalog";
import { useKnowledgeSpaceStore } from "~/stores/knowledgeSpaces";

describe("knowledge-spaces scene/bundle mapping", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("contract_quote defaults to p2_high_accuracy and forbids p0_basic", () => {
    const scene = SCENE_STRATEGY_CATALOG.scenes.contract_quote;
    expect(scene.defaultBundle).toBe("p2_high_accuracy");
    expect(scene.allowedBundles).not.toContain("p0_basic");
  });

  it("sql_kg defaults to p3_kg_strong", () => {
    const scene = SCENE_STRATEGY_CATALOG.scenes.sql_kg;
    expect(scene.defaultBundle).toBe("p3_kg_strong");
    expect(scene.allowedBundles).toContain("p3_kg_strong");
  });

  it("store.setSceneAndBundle clamps bundle to allowed list", () => {
    const store = useKnowledgeSpaceStore();

    store.setSceneAndBundle("contract_quote", "p0_basic" as any);
    expect(store.sceneKey).toBe("contract_quote");
    expect(store.bundleKey).toBe("p2_high_accuracy");

    store.setSceneAndBundle("sql_kg");
    expect(store.bundleKey).toBe("p3_kg_strong");

    const channels = store.computeEnabledIndexChannels();
    expect(channels).toContain("kg");
  });
});
