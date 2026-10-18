export interface ChatSessionScope {
  sessionType: "agent" | "team" | "legacy";
  teamId?: string;
}

export function sessionScopeParams(scope: ChatSessionScope) {
  return {
    session_type: scope.sessionType,
    ...(scope.sessionType === "team"
      ? { team_id: String(scope.teamId || "") }
      : {}),
  };
}

export function sessionWorkspaceKey(
  user: string,
  tenant: string,
  env: string,
  scope: ChatSessionScope,
) {
  return JSON.stringify([
    user,
    tenant,
    env,
    scope.sessionType,
    scope.sessionType === "team" ? String(scope.teamId || "") : "",
  ]);
}
