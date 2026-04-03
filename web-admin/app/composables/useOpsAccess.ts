import { computed } from "vue";
import { useUserContext } from "~/composables/api/services/meService";

export const useOpsAccess = () => {
  const userCtx = useUserContext();

  const canView = computed(() => userCtx.isRoot.value || userCtx.isCurrentTenantAdmin.value);
  const canExecute = computed(() => userCtx.isRoot.value || userCtx.isCurrentTenantAdmin.value);
  const canApprove = computed(() => userCtx.isRoot.value);
  const permissionHint = computed(() => {
    if (canExecute.value) return "";
    return "当前账号缺少 Ops 执行权限（需 root 或租户管理员）";
  });

  return {
    ...userCtx,
    canView,
    canExecute,
    canApprove,
    permissionHint,
  };
};

