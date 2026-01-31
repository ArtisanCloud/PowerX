/**
 * @jest-environment jsdom
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import pluginReleaseService from '../pluginRelease'

declare global {
  // eslint-disable-next-line no-var
  var $fetch: typeof fetch;
}

// Mock fetch
const mockFetch = vi.fn()
const mockOfetch = vi.fn(async (url: string, options: any) => {
  const response = await mockFetch(url, options)
  if (response && typeof response.ok === 'boolean') {
    if (response.ok) {
      return await response.json()
    }
    const errorBody = typeof response.json === 'function' ? await response.json() : null
    const error = new Error(
      errorBody?.message || response.statusText || 'Request failed'
    )
    ;(error as any).data = errorBody
    ;(error as any).response = response
    throw error
  }
  return response
})

global.fetch = mockFetch
global.$fetch = mockOfetch as any

// Mock useCookie
vi.mock('#app', () => ({
  useCookie: vi.fn(() => ({
    value: 'test-token'
  }))
}))

describe('PluginReleaseService', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('createOfflinePackage', () => {
    it('应该成功创建离线包', async () => {
      const mockResponse = {
        id: 1,
        releaseCandidateId: 'candidate-123',
        status: 'pending',
        checksum: 'abc123'
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse)
      })

      const result = await pluginReleaseService.createOfflinePackage({
        releaseCandidateId: 'candidate-123',
        checksum: 'abc123'
      })

      expect(result).toEqual(mockResponse)
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/plugin-release/offline-packages',
        expect.objectContaining({
          method: 'POST',
          headers: expect.objectContaining({
            'Content-Type': 'application/json',
            'X-PowerX-Tenant': 'test-token'
          }),
          body: JSON.stringify({
            releaseCandidateId: 'candidate-123',
            checksum: 'abc123'
          })
        })
      )
    })

    it('应该在API错误时抛出错误', async () => {
      const mockError = {
        message: '校验和不能为空',
        status: 400,
        code: 'INVALID_INPUT'
      }

      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: () => Promise.resolve(mockError)
      })

      await expect(
        pluginReleaseService.createOfflinePackage({
          releaseCandidateId: '',
          checksum: ''
        })
      ).rejects.toThrow('校验和不能为空')
    })
  })

  describe('getMarketplaceListings', () => {
    it('应该成功获取列表', async () => {
      const mockResponse = {
        data: [
          {
            id: 1,
            offlinePackageId: 123,
            channel: 'online',
            reviewStatus: 'pending',
            reviewCount: 0
          }
        ],
        total: 1,
        page: 1,
        size: 20
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse)
      })

      const result = await pluginReleaseService.getMarketplaceListings({
        page: 1,
        size: 20
      })

      expect(result).toEqual(mockResponse)
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/plugin-release/marketplace/listings?page=1&size=20',
        expect.any(Object)
      )
    })

    it('应该处理空列表', async () => {
      const mockResponse = {
        data: [],
        total: 0,
        page: 1,
        size: 20
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse)
      })

      const result = await pluginReleaseService.getMarketplaceListings()

      expect(result.data).toHaveLength(0)
      expect(result.total).toBe(0)
    })
  })

  describe('getMarketplaceListing', () => {
    it('应该成功获取单个Listing', async () => {
      const mockResponse = {
        id: 1,
        offlinePackageId: 123,
        channel: 'online',
        reviewStatus: 'pending',
        reviewCount: 0
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse)
      })

      const result = await pluginReleaseService.getMarketplaceListing(1)

      expect(result).toEqual(mockResponse)
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/plugin-release/marketplace/listings/1',
        expect.any(Object)
      )
    })
  })

  describe('reviewMarketplaceListing', () => {
    it('应该成功审核Listing', async () => {
      const mockResponse = {
        id: 1,
        reviewStatus: 'approved',
        reviewCount: 1,
        escalatedAt: null
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse)
      })

      const result = await pluginReleaseService.reviewMarketplaceListing(1, {
        decision: 'approved',
        comment: '审核通过'
      })

      expect(result).toEqual(mockResponse)
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/plugin-release/marketplace/listings/1/reviews',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            decision: 'approved',
            comment: '审核通过'
          })
        })
      )
    })

    it('应该处理拒绝审核', async () => {
      const mockResponse = {
        id: 1,
        reviewStatus: 'rejected',
        reviewCount: 1
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse)
      })

      const result = await pluginReleaseService.reviewMarketplaceListing(1, {
        decision: 'rejected',
        comment: '缺少必要文档'
      })

      expect(result.reviewStatus).toBe('rejected')
    })
  })

  describe('triggerCanary', () => {
    it('应该成功触发灰度部署', async () => {
      const mockResponse = {
        status: 'started',
        batchId: 'batch-1'
      }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse)
      })

      const result = await pluginReleaseService.triggerCanary(1, {
        batchName: 'batch-1'
      })

      expect(result).toEqual(mockResponse)
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/plugin-release/plans/1/deploy/canary',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            batchName: 'batch-1'
          })
        })
      )
    })
  })

  describe('网络错误处理', () => {
    it('应该处理网络连接错误', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network Error'))

      await expect(
        pluginReleaseService.getMarketplaceListings()
      ).rejects.toThrow('网络错误，请检查网络连接')
    })

    it('应该处理超时错误', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Timeout'))

      await expect(
        pluginReleaseService.getMarketplaceListings()
      ).rejects.toThrow('网络错误，请检查网络连接')
    })
  })

  describe('参数验证', () => {
    it('应该正确传递可选参数', async () => {
      const mockResponse = { data: [], total: 0, page: 1, size: 20 }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse)
      })

      await pluginReleaseService.getMarketplaceListings({
        page: 2,
        size: 10,
        status: 'pending',
        channel: 'online'
      })

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining('page=2&size=10&status=pending&channel=online'),
        expect.any(Object)
      )
    })

    it('应该过滤空值参数', async () => {
      const mockResponse = { data: [], total: 0, page: 1, size: 20 }

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse)
      })

      await pluginReleaseService.getMarketplaceListings({
        page: 1,
        size: 20,
        status: undefined,
        channel: null
      } as any)

      // 验证URL中不包含空值参数
      const calledUrl = mockFetch.mock.calls[0][0]
      expect(calledUrl).not.toContain('status=')
      expect(calledUrl).not.toContain('channel=')
    })
  })
})
