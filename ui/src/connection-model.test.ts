import { describe, expect, it, vi } from "vitest";
import type { PluginHostApi } from "@kdlbs/plugin-sdk";
import {
  type ConnectionView,
  EMPTY_FORM,
  connectionFormState,
  createConnectionApi,
  effectiveClientId,
  formFromConnection,
  healthFromConnection,
  isOrganizationUrl,
  personalAccessTokenUrl,
  saveRequest,
  storedTokenApplies,
} from "./connection-model";

const connected: ConnectionView = {
  configured: true,
  organizationUrl: "https://dev.azure.com/acme",
  defaultProjectId: "p1",
  defaultProjectName: "Platform",
  hasSecret: true,
  reauthRequired: false,
  lastCheckedAt: "2026-11-05T10:00:00Z",
  lastOk: true,
};

describe("organization URL rules", () => {
  it("accept only canonical dev.azure.com organizations", () => {
    expect(isOrganizationUrl("https://dev.azure.com/acme")).toBe(true);
    expect(isOrganizationUrl("https://dev.azure.com/acme/")).toBe(true);
    for (const value of [
      "",
      "http://dev.azure.com/acme",
      "https://acme.visualstudio.com",
      "https://dev.azure.com/acme/project",
      "https://user@dev.azure.com/acme",
      "https://dev.azure.com/-acme",
    ]) {
      expect(isOrganizationUrl(value), value).toBe(false);
    }
  });

  it("link to the organization's token page", () => {
    expect(personalAccessTokenUrl("https://dev.azure.com/acme/")).toBe(
      "https://dev.azure.com/acme/_usersSettings/tokens",
    );
    expect(personalAccessTokenUrl("https://example.com/acme")).toBeNull();
  });
});

describe("form and health", () => {
  it("never prefills the token", () => {
    expect(formFromConnection(connected)).toEqual({
      organizationUrl: "https://dev.azure.com/acme",
      defaultProjectId: "p1",
      defaultProjectName: "Platform",
      authMethod: "pat",
      pat: "",
      clientId: "",
    });
  });

  it("shows the application that signed in", () => {
    const microsoft = { ...connected, authMethod: "oauth", oauthClientId: "app-in-use" } as ConnectionView;
    expect(formFromConnection(microsoft).clientId).toBe("app-in-use");
  });

  it("keeps the application field empty for a token connection", () => {
    const token = { ...connected, oauthClientId: "stale" } as ConnectionView;
    expect(formFromConnection(token).clientId).toBe("");
  });

  it("resolves the application as the server does", () => {
    const form = formFromConnection(connected);
    const microsoft = { ...connected, authMethod: "oauth", oauthClientId: "app-in-use" } as ConnectionView;
    expect(effectiveClientId({ ...form, clientId: " typed " }, microsoft, "installation")).toBe("typed");
    expect(effectiveClientId(form, microsoft, "installation")).toBe("app-in-use");
    expect(effectiveClientId(form, connected, "installation")).toBe("installation");
    expect(effectiveClientId(form, null, "")).toBe("");
  });

  it("drops the default project when the organization changes", () => {
    const form = { ...formFromConnection(connected), organizationUrl: "https://dev.azure.com/other/" };
    expect(saveRequest(form, connected)).toEqual({
      organizationUrl: "https://dev.azure.com/other",
      defaultProjectId: "",
      defaultProjectName: "",
      authMethod: "pat",
      pat: "",
    });
  });

  it("reuses the stored token only for the stored organization", () => {
    expect(storedTokenApplies(connected, formFromConnection(connected))).toBe(true);
    expect(storedTokenApplies(connected, { ...formFromConnection(connected), organizationUrl: "https://dev.azure.com/x" })).toBe(false);
    expect(storedTokenApplies({ ...connected, reauthRequired: true }, formFromConnection(connected))).toBe(false);
  });

  it("reports health only for a usable credential", () => {
    expect(healthFromConnection(connected)).toEqual({ ok: true, error: "", checkedAt: new Date("2026-11-05T10:00:00Z") });
    expect(healthFromConnection({ ...connected, lastCheckedAt: undefined })?.checkedAt).toBeNull();
    expect(healthFromConnection({ ...connected, reauthRequired: true })).toBeNull();
    expect(healthFromConnection(null)).toBeNull();
  });
});

describe("connection API", () => {
  function hostWith(responses: Record<string, unknown>) {
    const invokeAction = vi.fn(async (key: string) => responses[key] ?? {});
    return { host: { api: { invokeAction } } as unknown as PluginHostApi, invokeAction };
  }

  it("scopes every action to the workspace", async () => {
    const { host, invokeAction } = hostWith({ "connection.get": { connection: connected } });
    const api = createConnectionApi(host, "ws-1");
    await expect(api.get()).resolves.toEqual(connected);
    await api.test({ ...EMPTY_FORM, organizationUrl: "https://dev.azure.com/acme/", pat: " t " });
    expect(invokeAction).toHaveBeenCalledWith(
      "connection.test",
      { workspaceId: "ws-1", body: { organizationUrl: "https://dev.azure.com/acme", pat: "t" } },
      { signal: undefined },
    );
  });

  it("tests a Microsoft account connection without a typed PAT", async () => {
    const { host, invokeAction } = hostWith({});
    await createConnectionApi(host, "ws-1").test({
      ...EMPTY_FORM,
      authMethod: "oauth",
      organizationUrl: "https://dev.azure.com/acme",
      pat: "leftover",
    });
    expect(invokeAction).toHaveBeenCalledWith(
      "connection.test",
      { workspaceId: "ws-1", body: { organizationUrl: "https://dev.azure.com/acme", pat: "" } },
      { signal: undefined },
    );
  });

  it("treats an unconfigured workspace as no connection", async () => {
    const { host } = hostWith({ "connection.get": { connection: { configured: false } } });
    await expect(createConnectionApi(host, "ws-1").get()).resolves.toBeNull();
  });

  it("offers only other configured workspaces as copy sources", async () => {
    const { host, invokeAction } = hostWith({
      "workspaces.describe": {
        workspaces: [
          { id: "ws-2", name: "Two", configured: true },
          { id: "ws-3", name: "Three", configured: false },
        ],
      },
    });
    const api = createConnectionApi(host, "ws-1");
    await expect(api.copySources(["ws-1", "ws-2", "ws-3"])).resolves.toEqual([{ id: "ws-2", name: "Two", configured: true }]);
    expect(invokeAction).toHaveBeenCalledWith(
      "workspaces.describe",
      { workspaceId: "ws-1", body: { workspaceIds: ["ws-2", "ws-3"] } },
      { signal: undefined },
    );
    await expect(api.copySources(["ws-1"])).resolves.toEqual([]);
  });
});

describe("connection form state", () => {
  const microsoft: ConnectionView = {
    ...connected,
    authMethod: "oauth",
    oauthAccount: "Ada",
    oauthClientId: "app-1",
  };
  const token: ConnectionView = { ...connected, authMethod: "pat" };

  it("is clean as loaded", () => {
    for (const connection of [microsoft, token]) {
      expect(connectionFormState(connection, formFromConnection(connection))).toEqual({ dirty: false, canSave: true });
    }
    expect(connectionFormState(null, EMPTY_FORM)).toEqual({ dirty: false, canSave: false });
  });

  it("saves a new default project for a Microsoft account", () => {
    const form = { ...formFromConnection(microsoft), defaultProjectId: "p2", defaultProjectName: "Data" };
    expect(connectionFormState(microsoft, form)).toEqual({ dirty: true, canSave: true });
  });

  it("asks for a new sign-in when the Microsoft account organization changes", () => {
    const form = { ...formFromConnection(microsoft), organizationUrl: "https://dev.azure.com/other" };
    expect(connectionFormState(microsoft, form)).toEqual({
      dirty: true,
      canSave: false,
      invalidReason: "oauthSignInForOrganization",
    });
  });

  it("asks for a new sign-in before saving an expired Microsoft account", () => {
    const expired = { ...microsoft, hasSecret: false, reauthRequired: true };
    const form = { ...formFromConnection(expired), defaultProjectId: "p2" };
    expect(connectionFormState(expired, form)).toEqual({
      dirty: true,
      canSave: false,
      invalidReason: "oauthReauthRequired",
    });
  });

  it("leaves Microsoft sign-in to its own buttons", () => {
    const fresh = { ...EMPTY_FORM, organizationUrl: "https://dev.azure.com/acme", authMethod: "oauth" as const };
    expect(connectionFormState(null, fresh).dirty).toBe(false);
    expect(connectionFormState(token, { ...formFromConnection(token), authMethod: "oauth" }).dirty).toBe(false);
    expect(connectionFormState(microsoft, { ...formFromConnection(microsoft), clientId: "app-2" }).dirty).toBe(false);
  });

  it("needs an organization and a token for a new token connection", () => {
    expect(connectionFormState(null, { ...EMPTY_FORM, organizationUrl: "https://dev.azure.com/acme" })).toEqual({
      dirty: true,
      canSave: false,
      invalidReason: "personalAccessTokenRequired",
    });
    expect(connectionFormState(null, { ...EMPTY_FORM, organizationUrl: "acme", pat: "secret" })).toEqual({
      dirty: true,
      canSave: false,
      invalidReason: "invalidOrganizationUrl",
    });
    expect(
      connectionFormState(null, { ...EMPTY_FORM, organizationUrl: "https://dev.azure.com/acme", pat: "secret" }),
    ).toEqual({ dirty: true, canSave: true });
  });

  it("reuses the stored token only for the same organization", () => {
    const project = { ...formFromConnection(token), defaultProjectId: "p2" };
    expect(connectionFormState(token, project)).toEqual({ dirty: true, canSave: true });
    const moved = { ...formFromConnection(token), organizationUrl: "https://dev.azure.com/other" };
    expect(connectionFormState(token, moved)).toEqual({
      dirty: true,
      canSave: false,
      invalidReason: "personalAccessTokenRequired",
    });
    expect(connectionFormState(token, { ...moved, pat: "secret" })).toEqual({ dirty: true, canSave: true });
  });

  it("replaces a Microsoft account with a token only once one is entered", () => {
    const switched = { ...formFromConnection(microsoft), authMethod: "pat" as const };
    expect(connectionFormState(microsoft, switched)).toEqual({
      dirty: true,
      canSave: false,
      invalidReason: "personalAccessTokenRequired",
    });
    expect(connectionFormState(microsoft, { ...switched, pat: "secret" })).toEqual({ dirty: true, canSave: true });
  });
});
