import { useMeService, useUserContext } from "./api/services/meService";

/**
 * 用户上下文服务的简化访问
 * 提供对 meService 和 userContext 的统一访问
 */
export const useMe = () => {
  const meService = useMeService();
  const userContext = useUserContext();

  return {
    // 从 meService 获取的方法
    getUserContext: async () => {
      const response = await meService.getMyContext();
      return response.data;
    },
    switchTenant: async (tenantUuid: string) => {
      const response = await meService.switchTenant(tenantUuid);
      return response.data;
    },
    getMyTenants: async () => {
      const response = await meService.getMyTenants();
      return response.data;
    },
    updateMyProfile: async (data: any) => {
      const response = await meService.updateMyProfile(data);
      return response.data;
    },
    uploadAvatar: async (file: File) => {
      const response = await meService.uploadAvatar(file);
      return response.data;
    },
    checkPermission: async (permission: string, resource?: string) => {
      const response = await meService.checkPermission(permission, resource);
      return response.data;
    },

    // 从 userContext 获取的方法和属性
    ...userContext,
  };
};
