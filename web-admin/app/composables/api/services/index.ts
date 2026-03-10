// API配置
export { API_CONFIG, ApiEndpoints } from "../config";

// AI设置服务 - 管理员功能
export { AISettingService } from "./aiSettingService";
export type {
  AgentProfile,
  AgentCredential,
  SaveSettingsPayload,
  TestConnectionPayload,
  TestQuickCallPayload,
} from "./aiSettingService";

// Agent服务 - 用户交互功能
export { AgentService } from "./agentService";
export type {
  AgentStatus,
  ProcessIntentRequest,
  ProcessIntentResponse,
  PlanPreviewRequest,
  PlanPreviewResponse,
  ChatMessage,
  ChatRequest,
  ChatResponse,
} from "./agentService";

export {
  CapabilityRegistryService,
} from "./capabilityRegistryService";
export type {
  CapabilityRecord,
  CapabilityListParams,
  CapabilityListResult,
  CapabilitySyncJob,
  CapabilitySyncJobParams,
  CapabilitySyncJobResult,
} from "./capabilityRegistryService";

export { useIntegrationGatewayApiKeyService } from "./integrationGatewayApiKeyService";
export type {
  IntegrationGatewayApiKeyPermission,
  IntegrationGatewayApiKeyRecord,
  IntegrationGatewayApiKeyListResult,
  IntegrationGatewayServiceAccount,
  CreateIntegrationGatewayApiKeyPayload,
  RotateIntegrationGatewayApiKeyPayload,
  RevokeIntegrationGatewayApiKeyPayload,
  CreateIntegrationGatewayApiKeyResult,
  RotateIntegrationGatewayApiKeyResult,
} from "./integrationGatewayApiKeyService";

export { useSkillsService } from "./skillsService";
export type {
  SkillRecord,
  SkillListResult,
  SkillImportPayload,
  SkillInvokePayload,
  SkillAuditRecord,
} from "./skillsService";
