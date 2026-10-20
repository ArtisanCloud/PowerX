/** Core derives this role from live root identity; it is never a business role. */
export const isCoreManagedRootRole = (
  role: { code?: string; scope?: string } | null | undefined,
): boolean => role?.scope === "system" && role.code === "root";
