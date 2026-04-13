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
  SkillTraceRecord,
  SkillCatalogItem,
  UpsertSkillCatalogPayload,
} from "./skillsService";

export { useDeployOpsService } from "./deployOpsService";
export type {
  DeployReleaseRecord,
  DeployHealthSummary,
  DeployReleaseListResult,
  TriggerReleasePayload,
  TriggerRollbackPayload,
} from "./deployOpsService";

export { usePluginOpsService } from "./pluginOpsService";
export type {
  PluginLifecycleAuditRecord,
  PluginLifecycleListResult,
  TriggerPluginLifecycleActionPayload,
} from "./pluginOpsService";

export { useBackupOpsService } from "./backupOpsService";
export type {
  BackupPolicy,
  BackupJob,
  RestoreDrillRecord,
} from "./backupOpsService";

export { useMigrationOpsService } from "./migrationOpsService";
export type {
  MigrationRunbookRecord,
  TriggerMigrationPayload,
  MigrationAcceptancePayload,
  TriggerTrafficSwitchPayload,
} from "./migrationOpsService";

export { useMonitorService } from "./monitorService";
export type {
  MonitorLogConfig,
  MonitorLogCapabilities,
  MonitorLogEntry,
  MonitorLogQueryMeta,
  MonitorLogQueryFilters,
} from "./monitorService";
