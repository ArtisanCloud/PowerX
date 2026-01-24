import { beforeEach, describe, expect, it } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { STRATEGY_PACKAGE_CATALOG } from "~/constants/strategyPackageCatalog";
import { useKnowledgeSpaceStore } from "~/stores/knowledgeSpaces";

describe("knowledge-spaces strategy package mapping", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it("O_crag recommends p2_high_accuracy", () => {
    const pkg = STRATEGY_PACKAGE_CATALOG.O_crag;
    expect(pkg.recommendedProfileKey).toBe("p2_high_accuracy");
  });

  it("K_kg requires KG index channel", () => {
    const pkg = STRATEGY_PACKAGE_CATALOG.K_kg;
    expect(pkg.dependencies.index).toContain("index.kg");
  });

  it("store.setStrategyPackage updates flags and profiles", () => {
    const store = useKnowledgeSpaceStore();

    store.setStrategyPackage("O_crag");
    expect(store.strategyPackageKey).toBe("O_crag");
    expect(store.form.ragProfileKey).toBe("p2_high_accuracy");
    expect(store.form.featureFlags).toContain("rag.strategy_package:o_crag");

    store.setStrategyPackage("K_kg");
    const channels = store.computeEnabledIndexChannels();
    expect(channels).toContain("kg");
  });
});
