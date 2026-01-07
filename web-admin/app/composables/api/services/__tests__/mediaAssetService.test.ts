/**
 * @jest-environment jsdom
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { useMediaAssetService } from "../mediaAssetService";

declare global {
  // eslint-disable-next-line no-var
  var $fetch: typeof fetch;
}

const mockFetch = vi.fn();
const mockOfetch = vi.fn(async (url: string, options: any) => {
  const response = await mockFetch(url, options);
  if (response && typeof response.ok === "boolean") {
    if (response.ok) {
      return await response.json();
    }
    const errorBody =
      typeof response.json === "function" ? await response.json() : null;
    const error = new Error(
      errorBody?.message || response.statusText || "Request failed"
    );
    (error as any).data = errorBody;
    (error as any).response = response;
    throw error;
  }
  return response;
});

global.fetch = mockFetch;
global.$fetch = mockOfetch as any;

vi.mock("#app", () => ({
  useCookie: vi.fn(() => ({
    value: "test-tenant",
  })),
}));

describe("MediaAssetService", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("listAssets 应该拼装查询参数并解包列表", async () => {
    const svc = useMediaAssetService();
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () =>
        Promise.resolve({
          code: 200,
          message: "success",
          data: {
            items: [{ uuid: "u1", name: "n1", tenant_uuid: "t", driver: "local", objectKey: "u1", businessStatus: "draft", createdAt: "", updatedAt: "" }],
            pagination: { total: 1, page: 1, page_size: 20, pages: 1 },
          },
          timestamp: Date.now(),
        }),
    });

    const result = await svc.listAssets({
      page: 1,
      pageSize: 20,
      keyword: "logo",
      tags: ["a", "b"],
    });

    expect(result.items).toHaveLength(1);
    expect(result.pagination.total).toBe(1);
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/admin/media/assets?page=1&pageSize=20&keyword=logo&tags=a&tags=b",
      expect.objectContaining({
        method: "GET",
        headers: expect.objectContaining({
          "Content-Type": "application/json",
          "X-Tenant-UUID": "test-tenant",
        }),
      })
    );
  });

  it("buildResourcePath 应该生成受控资源路径", () => {
    const svc = useMediaAssetService();
    expect(svc.buildResourcePath("abc", "inline")).toBe(
      "/admin/media/assets/abc/resource?disposition=inline"
    );
  });
});

