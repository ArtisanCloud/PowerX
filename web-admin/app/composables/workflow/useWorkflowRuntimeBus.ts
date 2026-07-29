import { computed } from "vue";
import { useWSBus } from "~/composables/useWSBus";
import type { WorkflowInstance } from "~/composables/api/services/workflowService";

export const WORKFLOW_RUNTIME_TOPIC = "_topic.workflow.instance";

export interface WorkflowRuntimeEvent {
  event_type?: string;
  step_id?: string;
  occurred_at?: string;
  instance: WorkflowInstance;
  details?: Record<string, any>;
}

export interface WorkflowRuntimeSubscriptionFilter {
  definitionUUID?: () => string;
  instanceUUID?: () => string;
}

type WorkflowRuntimeHandler = (event: WorkflowRuntimeEvent) => void;

const normalizeWorkflowRuntimeEvent = (payload: any): WorkflowRuntimeEvent | null => {
  const instance = payload?.instance || payload;
  if (!instance?.uuid) return null;
  return {
    event_type: payload?.event_type,
    step_id: payload?.step_id,
    occurred_at: payload?.occurred_at,
    details: payload?.details,
    instance: instance as WorkflowInstance,
  };
};

export const useWorkflowRuntimeBus = () => {
  const wsBus = useWSBus();

  const subscribe = (
    handler: WorkflowRuntimeHandler,
    filter: WorkflowRuntimeSubscriptionFilter = {},
    reqId = "workflow-runtime",
  ) => {
    return wsBus.subscribe(WORKFLOW_RUNTIME_TOPIC, (payload) => {
      const event = normalizeWorkflowRuntimeEvent(payload);
      if (!event) return;

      const currentDefinitionUUID = filter.definitionUUID?.() || "";
      if (currentDefinitionUUID && event.instance.definition_uuid !== currentDefinitionUUID) return;

      const activeInstanceUUID = filter.instanceUUID?.() || "";
      if (activeInstanceUUID && event.instance.uuid !== activeInstanceUUID) return;

      handler(event);
    }, reqId);
  };

  return {
    connected: computed(() => wsBus.connected.value),
    connecting: computed(() => wsBus.connecting.value),
    lastError: computed(() => wsBus.lastError.value),
    activeTenant: wsBus.activeTenant,
    subscribe,
  };
};
