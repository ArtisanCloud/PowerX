import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

/**
 * 设置相关类型定义
 */
export interface SystemSettings {
  siteName: string;
  siteDescription?: string;
  siteUrl: string;
  adminEmail: string;
  timezone: string;
  language: string;
  dateFormat: string;
  timeFormat: string;
  currency: string;
  currencySymbol: string;
  currencyPosition: "before" | "after";
  decimalPlaces: number;
  thousandSeparator: string;
  decimalSeparator: string;
  maintenanceMode: boolean;
  maintenanceMessage?: string;
  registrationEnabled: boolean;
  emailVerificationRequired: boolean;
  termsOfServiceUrl?: string;
  privacyPolicyUrl?: string;
}

export interface EmailSettings {
  driver: "smtp" | "sendmail" | "mailgun" | "ses" | "postmark";
  host?: string;
  port?: number;
  username?: string;
  password?: string;
  encryption?: "tls" | "ssl" | null;
  fromAddress: string;
  fromName: string;
  replyToAddress?: string;
  replyToName?: string;
  // Mailgun 配置
  mailgunDomain?: string;
  mailgunSecret?: string;
  mailgunEndpoint?: string;
  // AWS SES 配置
  sesKey?: string;
  sesSecret?: string;
  sesRegion?: string;
  // Postmark 配置
  postmarkToken?: string;
}

export interface BackupSettings {
  enabled: boolean;
  frequency: "daily" | "weekly" | "monthly";
  time: string; // HH:mm 格式
  retentionDays: number;
  includeDatabases: boolean;
  includeFiles: boolean;
  includeUploads: boolean;
  excludePaths: string[];
  storageType: "local" | "s3" | "ftp" | "sftp";
  // S3 配置
  s3Bucket?: string;
  s3Region?: string;
  s3AccessKey?: string;
  s3SecretKey?: string;
  s3Path?: string;
  // FTP/SFTP 配置
  ftpHost?: string;
  ftpPort?: number;
  ftpUsername?: string;
  ftpPassword?: string;
  ftpPath?: string;
  ftpPassive?: boolean;
  ftpSsl?: boolean;
}

export interface SecuritySettings {
  sessionTimeout: number; // 分钟
  maxLoginAttempts: number;
  lockoutDuration: number; // 分钟
  passwordMinLength: number;
  passwordRequireUppercase: boolean;
  passwordRequireLowercase: boolean;
  passwordRequireNumbers: boolean;
  passwordRequireSymbols: boolean;
  twoFactorEnabled: boolean;
  twoFactorRequired: boolean;
  ipWhitelist: string[];
  ipBlacklist: string[];
  corsEnabled: boolean;
  corsOrigins: string[];
  csrfProtection: boolean;
  rateLimitEnabled: boolean;
  rateLimitRequests: number;
  rateLimitWindow: number; // 分钟
}

export interface AISettings {
  providers: AIProvider[];
  defaultProvider: string;
  maxTokens: number;
  temperature: number;
  topP: number;
  frequencyPenalty: number;
  presencePenalty: number;
  timeout: number; // 秒
  retryAttempts: number;
  enableLogging: boolean;
  enableCaching: boolean;
  cacheExpiration: number; // 小时
}

export interface AIProvider {
  id: string;
  name: string;
  type: "openai" | "anthropic" | "google" | "azure" | "custom";
  enabled: boolean;
  apiKey: string;
  apiUrl?: string;
  models: AIModel[];
  defaultModel: string;
  maxTokens?: number;
  timeout?: number;
  rateLimitRpm?: number; // requests per minute
  rateLimitTpm?: number; // tokens per minute
}

export interface AIModel {
  id: string;
  name: string;
  description?: string;
  contextLength: number;
  inputCost?: number; // per 1K tokens
  outputCost?: number; // per 1K tokens
  supportsFunctions: boolean;
  supportsVision: boolean;
  supportsStreaming: boolean;
}

export interface LogSettings {
  level: "debug" | "info" | "warning" | "error";
  channels: string[];
  maxFiles: number;
  maxSize: number; // MB
  enableDatabaseLogging: boolean;
  enableFileLogging: boolean;
  enableEmailNotifications: boolean;
  emailNotificationLevel: "warning" | "error";
  emailRecipients: string[];
}

export interface CacheSettings {
  driver: "file" | "redis" | "memcached" | "database";
  ttl: number; // 秒
  prefix: string;
  // Redis 配置
  redisHost?: string;
  redisPort?: number;
  redisPassword?: string;
  redisDatabase?: number;
  // Memcached 配置
  memcachedHost?: string;
  memcachedPort?: number;
}

export interface SetupConfig {
  domain: {
    domain: string;
    api_subdomain?: string;
    enable_cdn?: boolean;
    cdn_domain?: string;
  };
  https: {
    mode: "auto" | "manual" | "disable";
    cert_email?: string;
    cert_content?: string;
    key_content?: string;
  };
  storage: {
    type: "local" | "s3" | "minio" | "oss" | "cos" | string;
    local_path?: string;
    access_key?: string;
    secret_key?: string;
    bucket?: string;
    region?: string;
    endpoint?: string;
    public_url?: string;
  };
  cache: {
    type: "redis" | "memcached" | "file" | string;
    redis_host?: string;
    redis_port?: number;
    redis_db?: number;
  };
  email: {
    enabled: boolean;
    smtp_host?: string;
    smtp_port?: number;
    from_name?: string;
    from_address?: string;
  };
  ports?: {
    backend_port?: number;
    web_admin_port?: number;
  };
}

/**
 * 设置服务 API
 */
export const useSettingsService = () => {
  const apiClient = useApiClient();
  const baseUrl = "/settings";

  return {
    /**
     * 获取系统设置
     */
    getSystemSettings: () => {
      return apiClient.get<ApiResponse<SystemSettings>>(`${baseUrl}/system`);
    },

    /**
     * 更新系统设置
     */
    updateSystemSettings: (data: Partial<SystemSettings>) => {
      return apiClient.put<ApiResponse<SystemSettings>>(
        `${baseUrl}/system`,
        data
      );
    },

    /**
     * 获取邮件设置
     */
    getEmailSettings: () => {
      return apiClient.get<ApiResponse<EmailSettings>>(`${baseUrl}/email`);
    },

    /**
     * 更新邮件设置
     */
    updateEmailSettings: (data: Partial<EmailSettings>) => {
      return apiClient.put<ApiResponse<EmailSettings>>(
        `${baseUrl}/email`,
        data
      );
    },

    /**
     * 测试邮件配置
     */
    testEmailSettings: (testEmail: string) => {
      return apiClient.post<ApiResponse<{ success: boolean; message: string }>>(
        `${baseUrl}/email/test`,
        { email: testEmail }
      );
    },

    /**
     * 获取备份设置
     */
    getBackupSettings: () => {
      return apiClient.get<ApiResponse<BackupSettings>>(`${baseUrl}/backup`);
    },

    /**
     * 更新备份设置
     */
    updateBackupSettings: (data: Partial<BackupSettings>) => {
      return apiClient.put<ApiResponse<BackupSettings>>(
        `${baseUrl}/backup`,
        data
      );
    },

    /**
     * 测试备份配置
     */
    testBackupSettings: () => {
      return apiClient.post<ApiResponse<{ success: boolean; message: string }>>(
        `${baseUrl}/backup/test`
      );
    },

    /**
     * 获取安全设置
     */
    getSecuritySettings: () => {
      return apiClient.get<ApiResponse<SecuritySettings>>(
        `${baseUrl}/security`
      );
    },

    /**
     * 更新安全设置
     */
    updateSecuritySettings: (data: Partial<SecuritySettings>) => {
      return apiClient.put<ApiResponse<SecuritySettings>>(
        `${baseUrl}/security`,
        data
      );
    },

    /**
     * 获取 AI 设置
     */
    getAISettings: () => {
      return apiClient.get<ApiResponse<AISettings>>(`${baseUrl}/ai`);
    },

    /**
     * 更新 AI 设置
     */
    updateAISettings: (data: Partial<AISettings>) => {
      return apiClient.put<ApiResponse<AISettings>>(`${baseUrl}/ai`, data);
    },

    /**
     * 测试 AI 提供商
     */
    testAIProvider: (providerId: string) => {
      return apiClient.post<ApiResponse<{ success: boolean; message: string }>>(
        `${baseUrl}/ai/test/${providerId}`
      );
    },

    /**
     * 获取日志设置
     */
    getLogSettings: () => {
      return apiClient.get<ApiResponse<LogSettings>>(`${baseUrl}/logs`);
    },

    /**
     * 更新日志设置
     */
    updateLogSettings: (data: Partial<LogSettings>) => {
      return apiClient.put<ApiResponse<LogSettings>>(`${baseUrl}/logs`, data);
    },

    /**
     * 获取缓存设置
     */
    getCacheSettings: () => {
      return apiClient.get<ApiResponse<CacheSettings>>(`${baseUrl}/cache`);
    },

    /**
     * 更新缓存设置
     */
    updateCacheSettings: (data: Partial<CacheSettings>) => {
      return apiClient.put<ApiResponse<CacheSettings>>(
        `${baseUrl}/cache`,
        data
      );
    },

    /**
     * 清除缓存
     */
    clearCache: (type?: "all" | "config" | "routes" | "views") => {
      return apiClient.post<ApiResponse<{ success: boolean; message: string }>>(
        `${baseUrl}/cache/clear`,
        { type }
      );
    },

    /**
     * 获取所有设置
     */
    getAllSettings: () => {
      return apiClient.get<
        ApiResponse<{
          system: SystemSettings;
          email: EmailSettings;
          backup: BackupSettings;
          security: SecuritySettings;
          ai: AISettings;
          logs: LogSettings;
          cache: CacheSettings;
        }>
      >(baseUrl);
    },

    /**
     * 导出设置
     */
    exportSettings: () => {
      return apiClient.get<Blob>(`${baseUrl}/export`);
    },

    /**
     * 导入设置
     */
    importSettings: (file: File) => {
      const formData = new FormData();
      formData.append("file", file);

      return apiClient.upload<
        ApiResponse<{ success: boolean; message: string }>
      >(`${baseUrl}/import`, formData);
    },

    /**
     * 重置设置到默认值
     */
    resetSettings: (section: string) => {
      return apiClient.post<ApiResponse<{ success: boolean; message: string }>>(
        `${baseUrl}/reset`,
        { section }
      );
    },

    getSetupConfig: () => {
      return apiClient.get<ApiResponse<{ config: SetupConfig }>>(
        "/admin/setup/config",
        { skipAuth: true }
      );
    },

    saveSetupConfig: (config: SetupConfig) => {
      return apiClient.put<ApiResponse<{ ok: boolean; config: SetupConfig }>>(
        "/admin/setup/config",
        config,
        { skipAuth: true }
      );
    },
  };
};
