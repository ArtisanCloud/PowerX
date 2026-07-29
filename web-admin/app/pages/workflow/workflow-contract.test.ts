import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const root = resolve(__dirname, "../../..");

const readAppFile = (path: string) => readFileSync(resolve(root, path), "utf8");

describe("workflow frontend contracts", () => {
  it("uses admin workflow APIs without legacy mock kind or palette methods", () => {
    const service = readAppFile("app/composables/api/services/workflowService.ts");

    expect(service).toContain('const baseUrl = "/admin/workflows"');
    expect(service).toContain("listNodeCatalog");
    expect(service).not.toContain("getKinds");
    expect(service).not.toContain("getPalette");
    expect(service).not.toContain("mockKinds");
    expect(service).not.toContain("mockPalette");
  });

  it("loads workflow definitions and node catalog from real API methods", () => {
    const indexPage = readAppFile("app/pages/workflow/index.vue");
    const manager = readAppFile("app/composables/workflow/useWorkflowManager.ts");
    const editor = readAppFile("app/components/workflow/WorkflowEditor.vue");

    expect(indexPage).toContain("workflowService.listDefinitions");
    expect(indexPage).not.toContain("workflowList = ref");
    expect(manager).toContain("workflowService.listNodeCatalog");
    expect(editor).toContain("loadNodeCatalog");
  });
});
