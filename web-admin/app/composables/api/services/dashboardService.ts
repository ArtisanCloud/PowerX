import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

/**
 * 仪表板相关类型定义
 */
export interface DashboardStats {
  totalUsers: number;
  totalOrders: number;
  totalProducts: number;
  totalRevenue: number;
  userGrowthRate: number;
  orderGrowthRate: number;
  revenueGrowthRate: number;
}

export interface ChartData {
  labels: string[];
  datasets: {
    label: string;
    data: number[];
    backgroundColor?: string;
    borderColor?: string;
    borderWidth?: number;
  }[];
}

export interface RecentActivity {
  id: string;
  type: "order" | "user" | "product" | "system";
  title: string;
  description: string;
  userId?: string;
  userName?: string;
  createdAt: string;
}

export interface TopProduct {
  id: string;
  name: string;
  image?: string;
  sales: number;
  revenue: number;
  category: string;
}

export interface TopCustomer {
  id: string;
  name: string;
  email: string;
  avatar?: string;
  totalOrders: number;
  totalSpent: number;
}

export interface DashboardData {
  stats: DashboardStats;
  salesChart: ChartData;
  userChart: ChartData;
  recentActivities: RecentActivity[];
  topProducts: TopProduct[];
  topCustomers: TopCustomer[];
}

export interface DashboardFilters {
  dateRange?: {
    start: string;
    end: string;
  };
  period?: "today" | "week" | "month" | "quarter" | "year";
}

/**
 * 仪表板服务 API
 */
export const useDashboardService = () => {
  const apiClient = useApiClient();
  const baseUrl = "/dashboard";

  return {
    /**
     * 获取仪表板概览数据
     */
    getDashboardData: (filters?: DashboardFilters) => {
      return apiClient.get<ApiResponse<DashboardData>>(baseUrl, {
        params: filters,
      });
    },

    /**
     * 获取统计数据
     */
    getStats: (filters?: DashboardFilters) => {
      return apiClient.get<ApiResponse<DashboardStats>>(`${baseUrl}/stats`, {
        params: filters,
      });
    },

    /**
     * 获取销售图表数据
     */
    getSalesChart: (filters?: DashboardFilters) => {
      return apiClient.get<ApiResponse<ChartData>>(`${baseUrl}/sales-chart`, {
        params: filters,
      });
    },

    /**
     * 获取用户增长图表数据
     */
    getUserChart: (filters?: DashboardFilters) => {
      return apiClient.get<ApiResponse<ChartData>>(`${baseUrl}/user-chart`, {
        params: filters,
      });
    },

    /**
     * 获取最近活动
     */
    getRecentActivities: (limit?: number) => {
      return apiClient.get<ApiResponse<RecentActivity[]>>(
        `${baseUrl}/activities`,
        {
          params: { limit },
        }
      );
    },

    /**
     * 获取热销产品
     */
    getTopProducts: (limit?: number, filters?: DashboardFilters) => {
      return apiClient.get<ApiResponse<TopProduct[]>>(
        `${baseUrl}/top-products`,
        {
          params: { limit, ...filters },
        }
      );
    },

    /**
     * 获取优质客户
     */
    getTopCustomers: (limit?: number, filters?: DashboardFilters) => {
      return apiClient.get<ApiResponse<TopCustomer[]>>(
        `${baseUrl}/top-customers`,
        {
          params: { limit, ...filters },
        }
      );
    },

    /**
     * 导出仪表板数据
     */
    exportDashboardData: (
      format: "excel" | "pdf",
      filters?: DashboardFilters
    ) => {
      return apiClient.get<Blob>(`${baseUrl}/export`, {
        params: { format, ...filters },
      });
    },
  };
};
