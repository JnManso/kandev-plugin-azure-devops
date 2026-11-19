import { describe, expect, it, vi } from "vitest";
import type { PluginHostApi } from "@kdlbs/plugin-sdk";
import {
  type ConnectionView,
  EMPTY_FORM,
  createConnectionApi,
  hasMicrosoftSignIn,
  microsoftSettingsSavable,
  parseOAuthRedirect,
  saveRequest,
  storedTokenApplies,
} from "./connection-model";
import { readSameOriginRedirect } from "./microsoft-sign-in";

const signedIn: ConnectionView = {
  configured: true,
  organizationUrl: "https://dev.azure.com/acme",
  authMethod: "oauth",
  hasSecret: true,
  reauthRequired: false,
  oauthAccount: "Ada",
  oauthClientId: "11111111-2222-3333-4444-555555555555",
  lastOk: true,
};

describe("Microsoft sign-in model", () => {
  it("parses the redirect address Microsoft returns", () => {
    expect(parseOAuthRedirect("http://localhost:38429/?code=abc&state=xyz")).toEqual({ code: "abc", state: "xyz" });
    expect(parseOAuthRedirect("http://localhost:38429/?state=xyz")).toBeNull();
    expect(parseOAuthRedirect("not a url")).toBeNull();
  });

  it("distinguishes signed-in, expired, and absent sign-ins", () => {
    expect(hasMicrosoftSignIn(signedIn)).toBe(true);
    expect(hasMicrosoftSignIn({ ...signedIn, hasSecret: false, reauthRequired: true })).toBe(true);
    expect(hasMicrosoftSignIn({ ...signedIn, authMethod: "pat" })).toBe(false);
    expect(hasMicrosoftSignIn(null)).toBe(false);
  });

  it("saves Microsoft settings only for the signed-in organization", () => {
    const form = { ...EMPTY_FORM, authMethod: "oauth" as const, organizationUrl: "https://dev.azure.com/acme/" };
    expect(microsoftSettingsSavable(signedIn, form)).toBe(true);
    expect(microsoftSettingsSavable(signedIn, { ...form, organizationUrl: "https://dev.azure.com/other" })).toBe(false);
    expect(microsoftSettingsSavable({ ...signedIn, reauthRequired: true }, form)).toBe(false);
    expect(storedTokenApplies(signedIn, { ...form, authMethod: "pat" })).toBe(false);
  });

  it("never sends a token with Microsoft settings", () => {
    const form = { ...EMPTY_FORM, authMethod: "oauth" as const, organizationUrl: "https://dev.azure.com/acme", pat: "stale" };
    expect(saveRequest(form, signedIn)).toMatchObject({ authMethod: "oauth", pat: "" });
  });

  it("starts sign-in with the workspace application and the page origin", async () => {
    const invokeAction = vi.fn(async () => ({ authUrl: "https://login.example/authorize", state: "s1" }));
    const api = createConnectionApi({ api: { invokeAction } } as unknown as PluginHostApi, "ws-1");
    const form = { ...EMPTY_FORM, authMethod: "oauth" as const, organizationUrl: "https://dev.azure.com/acme", clientId: " abc " };
    await expect(api.startOAuth(form, null, "http://localhost:38429")).resolves.toEqual({
      authUrl: "https://login.example/authorize",
      state: "s1",
    });
    expect(invokeAction).toHaveBeenCalledWith(
      "oauth.start",
      {
        workspaceId: "ws-1",
        body: {
          organizationUrl: "https://dev.azure.com/acme",
          defaultProjectId: "",
          defaultProjectName: "",
          clientId: "abc",
          redirectUrl: "http://localhost:38429",
        },
      },
      { signal: undefined },
    );
  });
});

describe("popup redirect watch", () => {
  function popupAt(origin: string, search: string): Window {
    return { location: { origin, search } } as unknown as Window;
  }

  it("reads the result once the popup is back on the page origin", () => {
    expect(readSameOriginRedirect(popupAt("http://localhost:1", "?code=c&state=s"), "http://localhost:1")).toEqual({
      code: "c",
      state: "s",
      error: "",
    });
    expect(readSameOriginRedirect(popupAt("http://localhost:1", "?error=access_denied&state=s"), "http://localhost:1")).toEqual({
      code: "",
      state: "s",
      error: "access_denied",
    });
  });

  it("ignores Microsoft pages and pages without a state", () => {
    expect(readSameOriginRedirect(popupAt("https://login.microsoftonline.com", "?code=c&state=s"), "http://localhost:1")).toBeNull();
    expect(readSameOriginRedirect(popupAt("http://localhost:1", "?code=c"), "http://localhost:1")).toBeNull();
    const crossOrigin = {
      get location(): Location {
        throw new DOMException("cross-origin", "SecurityError");
      },
    } as unknown as Window;
    expect(readSameOriginRedirect(crossOrigin, "http://localhost:1")).toBeNull();
  });
});
