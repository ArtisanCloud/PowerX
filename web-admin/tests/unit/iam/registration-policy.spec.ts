import { describe, expect, it } from "vitest";
import { registrationPolicyModeState } from "~/composables/domain/registrationPolicy";

describe("registrationPolicyModeState", () => {
  it("shows closed policy as blocked", () => {
    const state = registrationPolicyModeState({ mode: "closed", requires_verification: false, requires_invite_code: false, requires_request: false, requires_approval: false });
    expect(state.canSignup).toBe(false);
    expect(state.canRequest).toBe(false);
    expect(state.titleKey).toBe("registration.policy.mode.closed");
  });

  it("shows invite-only policy as signup with invite", () => {
    const state = registrationPolicyModeState({ mode: "invite_only", requires_verification: true, requires_invite_code: true, requires_request: false, requires_approval: false });
    expect(state.canSignup).toBe(true);
    expect(state.canRequest).toBe(false);
    expect(state.descriptionKey).toBe("registration.public.inviteOnly");
  });

  it("shows waitlist and approval policies as request flows", () => {
    expect(registrationPolicyModeState({ mode: "waitlist", requires_verification: false, requires_invite_code: false, requires_request: true, requires_approval: false }).canRequest).toBe(true);
    expect(registrationPolicyModeState({ mode: "approval_required", requires_verification: false, requires_invite_code: false, requires_request: true, requires_approval: true }).canRequest).toBe(true);
  });
});
