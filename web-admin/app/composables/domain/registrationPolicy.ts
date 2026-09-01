export type RegistrationPolicyMode =
  | "closed"
  | "open"
  | "invite_only"
  | "waitlist"
  | "approval_required"
  | "allowlist"
  | "progressive_rollout";

export interface RegistrationPolicyEffective {
  mode: RegistrationPolicyMode;
  requires_verification: boolean;
  requires_invite_code: boolean;
  requires_request: boolean;
  requires_approval: boolean;
}

export const registrationPolicyModes: RegistrationPolicyMode[] = [
  "closed",
  "open",
  "invite_only",
  "waitlist",
  "approval_required",
  "allowlist",
  "progressive_rollout",
];

export const registrationPolicyModeState = (
  policy?: RegistrationPolicyEffective | null
) => {
  const mode = policy?.mode || "closed";
  switch (mode) {
    case "open":
      return {
        mode,
        canSignup: true,
        canRequest: false,
        color: "success" as const,
        titleKey: "registration.policy.mode.open",
        descriptionKey: "registration.public.open",
      };
    case "invite_only":
      return {
        mode,
        canSignup: true,
        canRequest: false,
        color: "primary" as const,
        titleKey: "registration.policy.mode.invite_only",
        descriptionKey: "registration.public.inviteOnly",
      };
    case "waitlist":
      return {
        mode,
        canSignup: false,
        canRequest: true,
        color: "warning" as const,
        titleKey: "registration.policy.mode.waitlist",
        descriptionKey: "registration.public.waitlist",
      };
    case "approval_required":
      return {
        mode,
        canSignup: false,
        canRequest: true,
        color: "warning" as const,
        titleKey: "registration.policy.mode.approval_required",
        descriptionKey: "registration.public.approvalRequired",
      };
    case "allowlist":
      return {
        mode,
        canSignup: true,
        canRequest: false,
        color: "primary" as const,
        titleKey: "registration.policy.mode.allowlist",
        descriptionKey: "registration.public.allowlist",
      };
    case "progressive_rollout":
      return {
        mode,
        canSignup: true,
        canRequest: false,
        color: "primary" as const,
        titleKey: "registration.policy.mode.progressive_rollout",
        descriptionKey: "registration.public.progressiveRollout",
      };
    default:
      return {
        mode: "closed" as const,
        canSignup: false,
        canRequest: false,
        color: "error" as const,
        titleKey: "registration.policy.mode.closed",
        descriptionKey: "registration.public.closed",
      };
  }
};
