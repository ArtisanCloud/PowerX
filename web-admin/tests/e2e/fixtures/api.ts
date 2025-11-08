/**
 * REST API Helper
 * 用于在E2E测试中直接调用API
 */

import { request } from '@playwright/test'

export class APIHelper {
  private baseURL: string
  private token: string | null = null

  constructor(baseURL: string) {
    this.baseURL = baseURL
  }

  setToken(token: string) {
    this.token = token
  }

  private getHeaders() {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    }
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`
    }
    return headers
  }

  async post(endpoint: string, body?: any) {
    const response = await request.newContext({
      baseURL: this.baseURL,
      extraHTTPHeaders: this.getHeaders(),
    })

    return await response.post(endpoint, { data: body })
  }

  async get(endpoint: string, params?: Record<string, any>) {
    const response = await request.newContext({
      baseURL: this.baseURL,
      extraHTTPHeaders: this.getHeaders(),
    })

    const url = new URL(endpoint, this.baseURL)
    if (params) {
      Object.entries(params).forEach(([key, value]) => {
        url.searchParams.append(key, String(value))
      })
    }

    return await response.get(url.pathname + url.search)
  }

  async put(endpoint: string, body?: any) {
    const response = await request.newContext({
      baseURL: this.baseURL,
      extraHTTPHeaders: this.getHeaders(),
    })

    return await response.put(endpoint, { data: body })
  }

  async delete(endpoint: string) {
    const response = await request.newContext({
      baseURL: this.baseURL,
      extraHTTPHeaders: this.getHeaders(),
    })

    return await response.delete(endpoint)
  }
}

/**
 * 创建API实例的工厂函数
 */
export function createAPI(baseURL: string) {
  return new APIHelper(baseURL)
}
