/**
 * Plugin Release Service Layer
 * 封装与后端API的交互，提供类型安全的方法
 */

export interface OfflinePackage {
  id: number
  releaseCandidateId: string
  packageUri: string
  status: string
  checksum: string
  createdAt: string
  updatedAt: string
}

export interface CreateOfflinePackageRequest {
  releaseCandidateId: string
  packageUri?: string
  checksum: string
  signatureFingerprint?: string
  dependencies?: string[]
  licenseReport?: Record<string, any>
}

export interface MarketplaceListing {
  id: number
  offlinePackageId: number
  channel: string
  reviewStatus: string
  reviewCount: number
  slaDeadline?: string
  publishedAt?: string
  pricing?: Record<string, any>
  supportPolicy?: Record<string, any>
  createdAt: string
  updatedAt: string
}

export interface ListMarketplaceListingsParams {
  page?: number
  size?: number
  status?: string
  channel?: string
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  size: number
}

export interface ReviewRequest {
  decision: 'approved' | 'rejected' | 'need_fix'
  comment?: string
}

export interface ReviewResponse {
  id: number
  reviewStatus: string
  reviewCount: number
  escalatedAt?: string
}

/**
 * 通用API错误处理
 */
class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code?: string,
    public auditId?: string
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

/**
 * 基础API客户端
 */
class ApiClient {
  private baseURL: string
  private token: string | null = null

  constructor() {
    this.baseURL = '/api/admin/plugin-release'
    // 从cookie获取token
    if (process.client) {
      const tokenCookie = useCookie('token')
      this.token = tokenCookie.value || null
    }
  }

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const url = `${this.baseURL}${endpoint}`
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(this.token && { Authorization: `Bearer ${this.token}` }),
      ...options.headers as Record<string, string>
    }

    try {
      const response = await fetch(url, {
        ...options,
        headers
      })

      // 解析响应
      const data = await response.json().catch(() => ({}))

      // 处理错误
      if (!response.ok) {
        const error = new ApiError(
          data.message || data.error || '请求失败',
          response.status,
          data.code,
          data.auditId
        )
        throw error
      }

      return data
    } catch (error) {
      if (error instanceof ApiError) {
        throw error
      }
      throw new ApiError('网络错误，请检查网络连接', 0)
    }
  }

  async get<T>(endpoint: string, params?: Record<string, any>): Promise<T> {
    const searchParams = new URLSearchParams()
    if (params) {
      Object.entries(params).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          searchParams.append(key, String(value))
        }
      })
    }
    const query = searchParams.toString()
    return this.request<T>(`${endpoint}${query ? `?${query}` : ''}`)
  }

  async post<T>(endpoint: string, body?: any): Promise<T> {
    return this.request<T>(endpoint, {
      method: 'POST',
      body: JSON.stringify(body)
    })
  }

  async put<T>(endpoint: string, body?: any): Promise<T> {
    return this.request<T>(endpoint, {
      method: 'PUT',
      body: JSON.stringify(body)
    })
  }

  async delete<T>(endpoint: string): Promise<T> {
    return this.request<T>(endpoint, {
      method: 'DELETE'
    })
  }
}

/**
 * Plugin Release Service
 */
class PluginReleaseService {
  private api: ApiClient

  constructor() {
    this.api = new ApiClient()
  }

  /**
   * 离线包管理
   */
  async createOfflinePackage(
    data: CreateOfflinePackageRequest
  ): Promise<OfflinePackage> {
    try {
      const result = await this.api.post<OfflinePackage>('/offline-packages', data)
      return result
    } catch (error) {
      console.error('创建离线包失败:', error)
      throw error
    }
  }

  async getOfflinePackages(params?: {
    page?: number
    size?: number
    status?: string
  }): Promise<PaginatedResponse<OfflinePackage>> {
    try {
      return await this.api.get<PaginatedResponse<OfflinePackage>>(
        '/offline-packages',
        params
      )
    } catch (error) {
      console.error('获取离线包列表失败:', error)
      throw error
    }
  }

  /**
   * Marketplace Listing管理
   */
  async getMarketplaceListings(
    params?: ListMarketplaceListingsParams
  ): Promise<PaginatedResponse<MarketplaceListing>> {
    try {
      return await this.api.get<PaginatedResponse<MarketplaceListing>>(
        '/marketplace/listings',
        params
      )
    } catch (error) {
      console.error('获取Marketplace列表失败:', error)
      throw error
    }
  }

  async getMarketplaceListing(id: number): Promise<MarketplaceListing> {
    try {
      return await this.api.get<MarketplaceListing>(`/marketplace/listings/${id}`)
    } catch (error) {
      console.error(`获取Marketplace详情失败 (ID: ${id}):`, error)
      throw error
    }
  }

  async reviewMarketplaceListing(
    id: number,
    data: ReviewRequest
  ): Promise<ReviewResponse> {
    try {
      return await this.api.post<ReviewResponse>(
        `/marketplace/listings/${id}/reviews`,
        data
      )
    } catch (error) {
      console.error(`审核Marketplace失败 (ID: ${id}):`, error)
      throw error
    }
  }

  async getReviewHistory(id: number): Promise<any[]> {
    try {
      return await this.api.get<any[]>(`/marketplace/listings/${id}/reviews`)
    } catch (error) {
      console.error(`获取审核历史失败 (ID: ${id}):`, error)
      throw error
    }
  }

  /**
   * 发布候选管理
   */
  async getReleaseCandidate(id: string): Promise<any> {
    try {
      return await this.api.get(`/candidates/${id}`)
    } catch (error) {
      console.error(`获取发布候选失败 (ID: ${id}):`, error)
      throw error
    }
  }

  async createReleasePlan(data: any): Promise<any> {
    try {
      return await this.api.post('/plans', data)
    } catch (error) {
      console.error('创建发布计划失败:', error)
      throw error
    }
  }

  /**
   * 灰度部署
   */
  async triggerCanary(planId: number, data: any): Promise<any> {
    try {
      return await this.api.post(`/plans/${planId}/deploy/canary`, data)
    } catch (error) {
      console.error(`触发灰度失败 (Plan ID: ${planId}):`, error)
      throw error
    }
  }

  async finalizeDeployment(planId: number, data: any): Promise<any> {
    try {
      return await this.api.post(`/plans/${planId}/deploy/finalize`, data)
    } catch (error) {
      console.error(`完成部署失败 (Plan ID: ${planId}):`, error)
      throw error
    }
  }

  async rollbackDeployment(planId: number, data?: any): Promise<any> {
    try {
      return await this.api.post(`/plans/${planId}/deploy/rollback`, data)
    } catch (error) {
      console.error(`回滚部署失败 (Plan ID: ${planId}):`, error)
      throw error
    }
  }
}

/**
 * 单例导出
 */
export const pluginReleaseService = new PluginReleaseService()

/**
 * 导出错误类型
 */
export { ApiError }

/**
 * 导出默认实例
 */
export default pluginReleaseService
