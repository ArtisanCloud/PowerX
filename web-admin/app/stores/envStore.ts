import { defineStore } from 'pinia'

// 环境常量定义
export const ENV_CONSTANTS = {
  // 主要环境
  Dev: 'dev',
  Staging: 'staging',
  Prod: 'prod',
  
  // 扩展环境
  Canary: 'canary',
  Blue: 'blue',
  Green: 'green',
  
  // 别名（输入兼容）
  Default: 'default',    // → dev
  Production: 'production', // → prod
  StageAlias: 'stage',      // → staging
  StgAlias: 'stg',         // → staging
  
  PreviewPrefix: 'preview-'
} as const

// 环境映射关系
export const ENV_MAPPING = {
  [ENV_CONSTANTS.Default]: ENV_CONSTANTS.Dev,
  [ENV_CONSTANTS.Production]: ENV_CONSTANTS.Prod,
  [ENV_CONSTANTS.StageAlias]: ENV_CONSTANTS.Staging,
  [ENV_CONSTANTS.StgAlias]: ENV_CONSTANTS.Staging,
} as const

// 环境选项配置
export interface EnvOption {
  value: string
  label: string
  description?: string
  color?: string
}

export const ENV_OPTIONS: EnvOption[] = [
  {
    value: ENV_CONSTANTS.Dev,
    label: '开发环境',
    description: '用于开发和测试',
    color: 'blue'
  },
  {
    value: ENV_CONSTANTS.Staging,
    label: '预发布环境',
    description: '用于预发布测试',
    color: 'orange'
  },
  {
    value: ENV_CONSTANTS.Prod,
    label: '生产环境',
    description: '正式生产环境',
    color: 'red'
  },
  {
    value: ENV_CONSTANTS.Canary,
    label: '金丝雀环境',
    description: '用于灰度发布',
    color: 'yellow'
  },
  {
    value: ENV_CONSTANTS.Blue,
    label: '蓝色环境',
    description: '蓝绿部署 - 蓝色',
    color: 'blue'
  },
  {
    value: ENV_CONSTANTS.Green,
    label: '绿色环境',
    description: '蓝绿部署 - 绿色',
    color: 'green'
  }
]

export interface EnvState {
  currentEnv: string
  availableEnvs: string[]
  envConfig: Record<string, any>
}

export const useEnvStore = defineStore('env', {
  state: (): EnvState => ({
    currentEnv: ENV_CONSTANTS.Dev,
    availableEnvs: Object.values(ENV_CONSTANTS).filter(env => 
      !env.includes('-') && env !== ENV_CONSTANTS.Default && 
      env !== ENV_CONSTANTS.Production && env !== ENV_CONSTANTS.StageAlias && 
      env !== ENV_CONSTANTS.StgAlias
    ),
    envConfig: {}
  }),

  getters: {
    /**
     * 获取当前环境的显示名称
     */
    currentEnvLabel(): string {
      const option = ENV_OPTIONS.find(opt => opt.value === this.currentEnv)
      return option?.label || this.currentEnv
    },

    /**
     * 获取当前环境的描述
     */
    currentEnvDescription(): string {
      const option = ENV_OPTIONS.find(opt => opt.value === this.currentEnv)
      return option?.description || ''
    },

    /**
     * 获取当前环境的颜色
     */
    currentEnvColor(): string {
      const option = ENV_OPTIONS.find(opt => opt.value === this.currentEnv)
      return option?.color || 'gray'
    },

    /**
     * 获取可用环境选项
     */
    envOptions(): EnvOption[] {
      return ENV_OPTIONS.filter(option => 
        this.availableEnvs.includes(option.value)
      )
    },

    /**
     * 判断是否为生产环境
     */
    isProduction(): boolean {
      return this.currentEnv === ENV_CONSTANTS.Prod
    },

    /**
     * 判断是否为开发环境
     */
    isDevelopment(): boolean {
      return this.currentEnv === ENV_CONSTANTS.Dev
    },

    /**
     * 判断是否为预发布环境
     */
    isStaging(): boolean {
      return this.currentEnv === ENV_CONSTANTS.Staging
    }
  },

  actions: {
    /**
     * 设置当前环境
     */
    setCurrentEnv(env: string) {
      // 处理别名映射
      const mappedEnv = ENV_MAPPING[env as keyof typeof ENV_MAPPING] || env
      
      if (this.availableEnvs.includes(mappedEnv)) {
        this.currentEnv = mappedEnv
        this.saveToStorage()
      } else {
        console.warn(`环境 ${env} 不在可用环境列表中`)
      }
    },

    /**
     * 设置可用环境列表
     */
    setAvailableEnvs(envs: string[]) {
      this.availableEnvs = envs
      // 如果当前环境不在可用列表中，切换到第一个可用环境
      if (!envs.includes(this.currentEnv) && envs.length > 0) {
        this.currentEnv = envs[0]
      }
      this.saveToStorage()
    },

    /**
     * 更新环境配置
     */
    updateEnvConfig(config: Record<string, any>) {
      this.envConfig = { ...this.envConfig, ...config }
      this.saveToStorage()
    },

    /**
     * 获取环境配置
     */
    getEnvConfig(key?: string) {
      if (key) {
        return this.envConfig[key]
      }
      return this.envConfig
    },

    /**
     * 重置环境设置
     */
    resetEnv() {
      this.currentEnv = ENV_CONSTANTS.Dev
      this.envConfig = {}
      this.saveToStorage()
    },

    /**
     * 从存储加载环境设置
     */
    loadFromStorage() {
      if (process.client) {
        try {
          const stored = localStorage.getItem('env-store')
          if (stored) {
            const data = JSON.parse(stored)
            this.currentEnv = data.currentEnv || ENV_CONSTANTS.Dev
            this.availableEnvs = data.availableEnvs || this.availableEnvs
            this.envConfig = data.envConfig || {}
          }
        } catch (error) {
          console.error('加载环境设置失败:', error)
        }
      }
    },

    /**
     * 保存环境设置到存储
     */
    saveToStorage() {
      if (process.client) {
        try {
          const data = {
            currentEnv: this.currentEnv,
            availableEnvs: this.availableEnvs,
            envConfig: this.envConfig
          }
          localStorage.setItem('env-store', JSON.stringify(data))
        } catch (error) {
          console.error('保存环境设置失败:', error)
        }
      }
    },

    /**
     * 初始化环境store
     */
    initialize() {
      this.loadFromStorage()
    }
  }
})