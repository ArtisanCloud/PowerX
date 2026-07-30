import { ref } from "vue";
import type { Node, Edge } from "@vue-flow/core";
import type { KindSpec, PaletteItem, Workflow, WfNode, Edge as WorkflowEdge } from "~/types/workflow";
import type { WorkflowDefinition, WorkflowStepDefinition } from "~/composables/api/services/workflowService";
import { useWorkflowTranslator } from "./useWorkflowTranslator";
import { useWorkflowService } from "~/composables/api/services/workflowService";

const emptyStepGraph = (): WorkflowStepDefinition[] => [
  {
    id: "input",
    type: "system",
    node_kind: "input.capture",
    config: {
      input_schema_ref: "workflow.input.manual.v1",
      source_policy: { text: true, form: true },
      artifact_output_path: "$.artifacts.source",
    },
    next_step_ids: ["end"],
  },
  {
    id: "end",
    type: "system",
    node_kind: "workflow.end",
    config: {},
  },
];

const normalizeSchema = (schema?: Record<string, any>) => schema || { type: "object", properties: {} };

const defaultPropsFromSchema = (schema?: Record<string, any>) => {
  const props: Record<string, any> = {};
  const properties = schema?.properties || {};
  for (const [key, property] of Object.entries(properties)) {
    const spec = property as Record<string, any>;
    if (Object.prototype.hasOwnProperty.call(spec, "default")) {
      props[key] = spec.default;
      continue;
    }
    if (spec.type === "boolean") props[key] = false;
    else if (spec.type === "number" || spec.type === "integer") props[key] = 0;
    else if (spec.type === "array") props[key] = [];
    else if (spec.type === "object") props[key] = {};
    else props[key] = "";
  }
  return props;
};

const workflowFromDefinition = (definition: WorkflowDefinition): Workflow => ({
  id: definition.uuid,
  uuid: definition.uuid,
  name: definition.name,
  description: definition.description,
  nodes: stepGraphToWorkflowNodes(definition.step_graph || []),
  edges: stepGraphToWorkflowEdges(definition.step_graph || []),
  version: String(definition.version),
  status: definition.status,
  createdAt: definition.created_at || "",
  updatedAt: definition.updated_at || "",
  raw: definition,
});

const sharedKinds = ref<Record<string, KindSpec>>({});
const sharedPalette = ref<PaletteItem[]>([]);
const sharedCurrentWorkflow = ref<Workflow | null>(null);
const sharedIsLoading = ref(false);
const sharedError = ref<string | null>(null);

export function useWorkflowManager() {
  const { makeNodeFromPalette, makeWorkflowFromVueFlow } = useWorkflowTranslator();
  const workflowService = useWorkflowService();

  const kinds = sharedKinds;
  const palette = sharedPalette;
  const currentWorkflow = sharedCurrentWorkflow;
  const isLoading = sharedIsLoading;
  const error = sharedError;

  async function loadNodeCatalog() {
    try {
      isLoading.value = true;
      const catalog = await workflowService.listNodeCatalog();
      const nextKinds: Record<string, KindSpec> = {};
      const nextPalette: PaletteItem[] = [];

      for (const item of catalog) {
        const label = workflowNodeLabelForKind(item.node_kind, item.display_name_i18n_key);
        const schema = normalizeSchema(item.config_schema as Record<string, any>);
        nextKinds[item.node_kind] = {
          kind: item.node_kind,
          version: "1",
          label,
          ports: workflowNodePorts(item.node_kind),
          defaultProps: defaultPropsFromSchema(schema),
          schema,
          ui: {
            shape: shapeForNodeKind(item.node_kind),
            colorToken: colorTokenForNodeKind(item.node_kind, item.category),
            icon: iconForNodeKind(item.node_kind),
            size: sizeForNodeKind(item.node_kind),
            badges: [item.category],
          },
        };
        nextPalette.push({
          id: item.node_kind,
          kind: item.node_kind,
          label,
          icon: iconForNodeKind(item.node_kind),
          defaultProps: defaultPropsFromSchema(schema),
          catalogItem: item,
        });
      }

      kinds.value = nextKinds;
      palette.value = nextPalette;
    } catch (err) {
      error.value = "workflow.errors.loadNodeCatalog";
      throw err;
    } finally {
      isLoading.value = false;
    }
  }

  async function loadWorkflow(definitionUUID: string) {
    try {
      isLoading.value = true;
      const definition = await workflowService.getDefinition(definitionUUID);
      currentWorkflow.value = workflowFromDefinition(definition);
      return currentWorkflow.value;
    } catch (err) {
      error.value = "workflow.errors.loadDefinition";
      throw err;
    } finally {
      isLoading.value = false;
    }
  }

  function addNodeFromPalette(paletteId: string, position: { x: number; y: number }) {
    const paletteItem = palette.value.find((item) => item.id === paletteId);
    if (!paletteItem) {
      error.value = "workflow.errors.nodeCatalogMissing";
      return null;
    }
    return makeNodeFromPalette(paletteItem, kinds.value, position);
  }

  async function saveWorkflow(nodes: Node[], edges: Edge[]) {
    if (!currentWorkflow.value) {
      error.value = "workflow.errors.noCurrentDefinition";
      return null;
    }
    const converted = makeWorkflowFromVueFlow(nodes, edges);
    currentWorkflow.value.nodes = converted.nodes;
    currentWorkflow.value.edges = converted.edges;
    return currentWorkflow.value;
  }

  async function createNewWorkflow(name: string, description = "") {
    try {
      isLoading.value = true;
      const result = await workflowService.createDefinition({
        name,
        description,
        steps: emptyStepGraph(),
      });
      currentWorkflow.value = workflowFromDefinition(result);
      return currentWorkflow.value;
    } catch (err) {
      error.value = "workflow.errors.createDefinition";
      throw err;
    } finally {
      isLoading.value = false;
    }
  }

  async function getWorkflowList(params?: { page?: number; pageSize?: number; keyword?: string; status?: any }) {
    try {
      isLoading.value = true;
      return await workflowService.listDefinitions(params);
    } catch (err) {
      error.value = "workflow.errors.listDefinitions";
      throw err;
    } finally {
      isLoading.value = false;
    }
  }

  return {
    kinds,
    palette,
    currentWorkflow,
    isLoading,
    error,
    loadNodeCatalog,
    loadWorkflow,
    addNodeFromPalette,
    saveWorkflow,
    createNewWorkflow,
    getWorkflowList,
  };
}

function iconForNodeKind(nodeKind: string) {
  if (nodeKind === "input.capture") return "i-heroicons-play";
  if (nodeKind === "workflow.end") return "i-heroicons-stop";
  if (nodeKind.startsWith("skill.")) return "i-heroicons-command-line";
  if (nodeKind.startsWith("capability.")) return "i-heroicons-bolt";
  if (nodeKind.startsWith("knowledge.")) return "i-heroicons-book-open";
  if (nodeKind.startsWith("metadata.")) return "i-heroicons-tag";
  if (nodeKind.startsWith("human.")) return "i-heroicons-user-circle";
  if (nodeKind.startsWith("decision.")) return "i-heroicons-adjustments-horizontal";
  if (nodeKind.startsWith("parallel.")) return "i-heroicons-arrows-right-left";
  if (nodeKind.startsWith("event.")) return "i-heroicons-megaphone";
  if (nodeKind.startsWith("compensation.")) return "i-heroicons-arrow-uturn-left";
  return "i-heroicons-square-3-stack-3d";
}

function workflowNodeLabelForKind(nodeKind: string, fallback: string) {
  if (nodeKind === "input.capture") return "workflow.node.start";
  if (nodeKind === "workflow.end") return "workflow.node.end";
  return fallback;
}

function workflowNodePorts(nodeKind: string) {
  if (nodeKind === "input.capture") {
    return { inputs: [], outputs: [{ name: "out" }] };
  }
  if (nodeKind === "workflow.end") {
    return { inputs: [{ name: "in" }], outputs: [] };
  }
  return {
    inputs: [{ name: "in" }],
    outputs: [{ name: "out" }, { name: "error" }],
  };
}

function shapeForNodeKind(nodeKind: string): "card" | "diamond" | "pill" | "oval" | "minimal" {
  if (nodeKind === "input.capture" || nodeKind === "workflow.end") return "oval";
  return "card";
}

function sizeForNodeKind(nodeKind: string) {
  if (nodeKind === "input.capture" || nodeKind === "workflow.end") return { w: 172, h: 76 };
  return { w: 240, h: 96 };
}

function colorTokenForNodeKind(nodeKind: string, category?: string) {
  if (nodeKind === "input.capture") return "start";
  if (nodeKind === "workflow.end") return "end";
  if (nodeKind.startsWith("skill.")) return "skill";
  if (nodeKind.startsWith("capability.")) return "capability";
  if (nodeKind.startsWith("knowledge.")) return "knowledge";
  if (nodeKind.startsWith("metadata.")) return "metadata";
  if (nodeKind.startsWith("human.")) return "human";
  if (nodeKind.startsWith("decision.")) return "decision";
  if (nodeKind.startsWith("parallel.")) return "parallel";
  if (nodeKind.startsWith("event.")) return "event";
  if (nodeKind.startsWith("compensation.")) return "compensation";
  if (category === "human") return "human";
  return "default";
}

function stepGraphToWorkflowNodes(steps: WorkflowStepDefinition[]): WfNode[] {
  return steps.map((step, index) => {
    const position = defaultNodePosition(index);
    const kind = step.node_kind;
    return {
      id: step.id,
      kind,
      paletteId: kind,
      label: `workflow.node.${kind}`,
      props: { ...(step.config || {}) },
      ui: {
        shape: shapeForNodeKind(kind),
        colorToken: colorTokenForNodeKind(kind),
        icon: iconForNodeKind(kind),
        size: sizeForNodeKind(kind),
        badges: [nodeKindCategory(kind)],
      },
      position,
    };
  });
}

function stepGraphToWorkflowEdges(steps: WorkflowStepDefinition[]): WorkflowEdge[] {
  const edges = new Map<string, WorkflowEdge>();
  const stepIDs = new Set(steps.map((step) => step.id));
  const add = (source: string, target: string, label?: string) => {
    if (!source || !target || !stepIDs.has(source) || !stepIDs.has(target)) return;
    const id = `${source}-${label || "out"}-${target}`;
    if (edges.has(id)) return;
    edges.set(id, {
      id,
      source,
      sourceHandle: "out",
      target,
      targetHandle: "in",
      label,
      type: "smoothstep",
    });
  };

  for (const step of steps) {
    const nextStepIDs = Array.isArray(step.next_step_ids) ? step.next_step_ids : [];
    for (const target of nextStepIDs) {
      add(step.id, target);
    }

    const approvedRoute = routeTarget(step.config?.approved_route);
    if (approvedRoute) add(step.id, approvedRoute, "approved");
    const rejectedRoute = routeTarget(step.config?.rejected_route);
    if (rejectedRoute) add(step.id, rejectedRoute, "rejected");

    const routes = step.config?.routes;
    if (routes && typeof routes === "object" && !Array.isArray(routes)) {
      for (const [label, target] of Object.entries(routes)) {
        const route = routeTarget(target);
        if (route) add(step.id, route, label);
      }
    }
  }

  for (const step of steps) {
    for (const source of step.depends_on || []) {
      add(source, step.id);
    }
  }

  return Array.from(edges.values());
}

function defaultNodePosition(index: number) {
  return {
    x: 120 + (index % 3) * 300,
    y: 100 + Math.floor(index / 3) * 170,
  };
}

function routeTarget(value: unknown) {
  if (typeof value === "string") return value.trim();
  if (Array.isArray(value) && typeof value[0] === "string") return value[0].trim();
  return "";
}

function nodeKindCategory(nodeKind: string) {
  const index = nodeKind.indexOf(".");
  return index > 0 ? nodeKind.slice(0, index) : "workflow";
}
