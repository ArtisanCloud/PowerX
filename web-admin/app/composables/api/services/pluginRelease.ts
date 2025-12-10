import { useApiClient } from "../index"

/**
 * Plugin Release Service Layer
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
  decision: "approved" | "rejected" | "need_fix"
  comment?: string
}

export interface ReviewResponse {
  id: number
  reviewStatus: string
  reviewCount: number
  escalatedAt?: string
}

export interface ReleaseCandidateSummary {
  candidateId: string
  tenantId: string
  pluginId: string
  version: string
  buildArtifactUri: string
  commitHash: string
  releaseNotes: string
  gateStatus: string
  approvalStatus: string
  planStatus?: string
  offlinePackageStatus?: string
  offlinePackageCount: number
  createdAt?: string
  updatedAt?: string
  labels?: Record<string, string>
}

export interface ReleaseCandidateListResponse {
  items: ReleaseCandidateSummary[]
  pagination?: {
    total: number
    page: number
    page_size?: number
    pageSize?: number
    pages?: number
  }
}

class PluginReleaseService {
  private api = useApiClient()
  private adminBase = "/plugin-release"
  private internalBase = "/internal/plugins"

  // 离线包管理
  createOfflinePackage(data: CreateOfflinePackageRequest) {
    return this.api.post<OfflinePackage>(`${this.adminBase}/offline-packages`, data)
  }

  getOfflinePackages(params?: { page?: number; size?: number; status?: string }) {
    return this.api.get<PaginatedResponse<OfflinePackage>>(
      `${this.adminBase}/offline-packages`,
      { params }
    )
  }

  // Marketplace Listing 管理
  getMarketplaceListings(params?: ListMarketplaceListingsParams) {
    return this.api.get<PaginatedResponse<MarketplaceListing>>(
      `${this.adminBase}/marketplace/listings`,
      { params }
    )
  }

  getMarketplaceListing(id: number) {
    return this.api.get<MarketplaceListing>(`${this.adminBase}/marketplace/listings/${id}`)
  }

  reviewMarketplaceListing(id: number, data: ReviewRequest) {
    return this.api.post<ReviewResponse>(
      `${this.adminBase}/marketplace/listings/${id}/reviews`,
      data
    )
  }

  getReviewHistory(id: number) {
    return this.api.get<any[]>(`${this.adminBase}/marketplace/listings/${id}/reviews`)
  }

  // 发布候选
  async listReleaseCandidates(params?: {
    page?: number
    size?: number
    tenantId?: string
    pluginId?: string
    version?: string
    approvalStatus?: string
    gateStatus?: string
    createdBy?: string
  }) {
    const resp = await this.api.get(`${this.internalBase}/releases`, { params })
    const data: any = resp && typeof resp === "object" && "data" in resp ? (resp as any).data : resp
    const items = Array.isArray(data?.items) ? (data.items as ReleaseCandidateSummary[]) : []
    const pagination = data?.pagination || {}
    return {
      items,
      pagination: {
        ...pagination,
        pageSize: pagination?.pageSize || pagination?.page_size,
      },
    }
  }

  async getReleaseCandidate(id: string) {
    const resp = await this.api.get(`${this.adminBase}/candidates/${id}`)
    return this.api.unwrap(resp)
  }

  async deleteReleaseCandidate(id: string) {
    const resp = await this.api.delete(`${this.internalBase}/releases/${id}`)
    return this.api.unwrap(resp)
  }

  async updateReleaseCandidate(id: string, payload: { buildArtifact?: string; releaseNotes?: string; labels?: Record<string, string> }) {
    const resp = await this.api.patch(`${this.internalBase}/releases/${id}`, payload)
    return this.api.unwrap(resp)
  }

  async createReleasePlan(data: any) {
    const resp = await this.api.post(`${this.adminBase}/plans`, data)
    return this.api.unwrap(resp)
  }

  // 灰度部署
  async triggerCanary(planId: number, data: any) {
    const resp = await this.api.post(`${this.adminBase}/plans/${planId}/deploy/canary`, data)
    return this.api.unwrap(resp)
  }

  async finalizeDeployment(planId: number, data: any) {
    const resp = await this.api.post(`${this.adminBase}/plans/${planId}/deploy/finalize`, data)
    return this.api.unwrap(resp)
  }

  async rollbackDeployment(planId: number, data?: any) {
    const resp = await this.api.post(`${this.adminBase}/plans/${planId}/deploy/rollback`, data)
    return this.api.unwrap(resp)
  }
}

export const pluginReleaseService = new PluginReleaseService()
export default pluginReleaseService
