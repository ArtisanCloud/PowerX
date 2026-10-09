import { describe, expect, it } from "vitest";
import { parseApiKeyPluginOwners } from "~/utils/apiKeyPluginOwners";

describe("API Key plugin owner input", () => {
  it("keeps CRM and SCRM distinct, accepts multiple exact owners", () => {
    expect(parseApiKeyPluginOwners("com.powerx.plugins.scrm\ncom.powerx.plugins.crm,com.powerx.plugins.crm"))
      .toEqual(["com.powerx.plugins.crm", "com.powerx.plugins.scrm"]);
  });
  it("uses an empty list to explicitly revoke owner access", () => {
    expect(parseApiKeyPluginOwners(" \n ")).toEqual([]);
  });
  it.each(["*", "com.powerx.*", "../crm", "com.PowerX.crm"])("rejects non-exact owner %s", (text) => {
    expect(() => parseApiKeyPluginOwners(text)).toThrow();
  });
});
