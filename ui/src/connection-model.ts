import type { PluginHostApi } from "@kdlbs/plugin-sdk";

export type ConnectionView = {
  configured: boolean;
  organizationUrl?: string;
  defaultProjectId?: string;
  defaultProjectName?: string;
  authMethod?: string;
  hasSecret: boolean;
  reauthRequired: boolean;
  oauthAccount?: string;
  oauthClientId?: string;
  lastCheckedAt?: string;
  lastOk: boolean;
  lastError?: string;
};

export type TestResult = {
  ok: boolean;
  displayName?: string;
  email?: string;
  error?: string;
};

export type Project = { id: string; name: string };

export type WorkspaceSummary = { id: string; name: string; configured: boolean };

export type AuthMethod = "pat" | "oauth";

export type ConnectionForm = {
  organizationUrl: string;
  defaultProjectId: string;
  defaultProjectName: string;
  authMethod: AuthMethod;
  pat: string;
  /** Workspace Microsoft application (client) ID; blank uses the default. */
  clientId: string;
};

export type OAuthStart = { authUrl: string; state: string };

/** Shape of IntegrationAuthStatusBanner's `health` prop. */
export type AuthHealth = { ok: boolean; error: string; checkedAt: Date | null };

export const EMPTY_FORM: ConnectionForm = {
  organizationUrl: "",
  defaultProjectId: "",
  defaultProjectName: "",
  authMethod: "pat",
  pat: "",
  clientId: "",
};

export function formFromConnection(connection: ConnectionView | null): ConnectionForm {
  if (!connection?.configured) return EMPTY_FORM;
  return {
    organizationUrl: connection.organizationUrl ?? "",
    defaultProjectId: connection.defaultProjectId ?? "",
    defaultProjectName: connection.defaultProjectName ?? "",
    authMethod: connection.authMethod === "oauth" ? "oauth" : "pat",
    pat: "",
    clientId: connection.authMethod === "oauth" ? (connection.oauthClientId ?? "") : "",
  };
}

/** The application sign-in will use, in the order `oauth.start` resolves it. */
export function effectiveClientId(
  form: ConnectionForm,
  connection: ConnectionView | null,
  defaultClientId: string,
): string {
  return form.clientId.trim() || connection?.oauthClientId || defaultClientId;
}

/** True while the workspace holds a Microsoft account sign-in, valid or expired. */
export function hasMicrosoftSignIn(connection: ConnectionView | null): boolean {
  return connection?.authMethod === "oauth" && (connection.hasSecret || connection.reauthRequired);
}

/** True when Microsoft account settings can be saved without a new sign-in. */
export function microsoftSettingsSavable(connection: ConnectionView | null, form: ConnectionForm): boolean {
  return (
    connection?.authMethod === "oauth" &&
    connection.hasSecret &&
    !connection.reauthRequired &&
    normalizeOrganizationUrl(connection.organizationUrl ?? "") === normalizeOrganizationUrl(form.organizationUrl)
  );
}

/** Why the native Save changes bar cannot save the form; a translation key. */
export type ConnectionFormIssue =
  | "invalidOrganizationUrl"
  | "personalAccessTokenRequired"
  | "oauthSignInForOrganization"
  | "oauthReauthRequired";

export type ConnectionFormState = { dirty: boolean; canSave: boolean; invalidReason?: ConnectionFormIssue };

/**
 * What the Save changes bar offers for the connection form. A Microsoft
 * account is created and replaced by its own sign-in buttons, so only its
 * default project is saved here; a token connection saves everything.
 */
export function connectionFormState(connection: ConnectionView | null, form: ConnectionForm): ConnectionFormState {
  const sameOrganization =
    normalizeOrganizationUrl(connection?.organizationUrl ?? "") === normalizeOrganizationUrl(form.organizationUrl);
  const projectChanged = form.defaultProjectId !== (connection?.defaultProjectId ?? "");
  let dirty: boolean;
  let issue: ConnectionFormIssue | undefined;
  if (form.authMethod === "oauth") {
    if (!hasMicrosoftSignIn(connection)) return { dirty: false, canSave: false };
    dirty = !sameOrganization || projectChanged;
    if (!isOrganizationUrl(form.organizationUrl)) issue = "invalidOrganizationUrl";
    else if (!sameOrganization) issue = "oauthSignInForOrganization";
    else if (!microsoftSettingsSavable(connection, form)) issue = "oauthReauthRequired";
  } else {
    dirty = connection
      ? connection.authMethod === "oauth" || !sameOrganization || projectChanged || form.pat.trim() !== ""
      : form.organizationUrl.trim() !== "" || form.pat.trim() !== "";
    if (!isOrganizationUrl(form.organizationUrl)) issue = "invalidOrganizationUrl";
    else if (form.pat.trim() === "" && !storedTokenApplies(connection, form)) issue = "personalAccessTokenRequired";
  }
  if (!issue) return { dirty, canSave: true };
  return dirty ? { dirty, canSave: false, invalidReason: issue } : { dirty, canSave: false };
}

/** Reads code and state from the address Microsoft redirected to. */
export function parseOAuthRedirect(value: string): { code: string; state: string } | null {
  let url: URL;
  try {
    url = new URL(value.trim());
  } catch {
    return null;
  }
  const code = url.searchParams.get("code");
  const state = url.searchParams.get("state");
  return code && state ? { code, state } : null;
}

/** Banner health, or null while there is no usable credential to report on. */
export function healthFromConnection(connection: ConnectionView | null): AuthHealth | null {
  if (!connection?.configured || !connection.hasSecret || connection.reauthRequired) return null;
  return {
    ok: connection.lastOk,
    error: connection.lastError ?? "",
    checkedAt: connection.lastCheckedAt ? new Date(connection.lastCheckedAt) : null,
  };
}

export function normalizeOrganizationUrl(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

/** Mirrors the backend rule: https://dev.azure.com/<organization>. */
export function isOrganizationUrl(value: string): boolean {
  return organizationName(value) !== null;
}

function organizationName(value: string): string | null {
  let parsed: URL;
  try {
    parsed = new URL(normalizeOrganizationUrl(value));
  } catch {
    return null;
  }
  const segments = parsed.pathname.split("/").filter(Boolean);
  if (
    parsed.protocol !== "https:" ||
    parsed.hostname.toLowerCase() !== "dev.azure.com" ||
    parsed.port ||
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash ||
    segments.length !== 1 ||
    !/^[A-Za-z0-9](?:[A-Za-z0-9-]{0,48}[A-Za-z0-9])?$/.test(segments[0])
  ) {
    return null;
  }
  return segments[0];
}

/** Azure DevOps page where the user creates a personal access token. */
export function personalAccessTokenUrl(organizationUrl: string): string | null {
  const organization = organizationName(organizationUrl);
  return organization ? `https://dev.azure.com/${organization}/_usersSettings/tokens` : null;
}

/** True when the stored token can be reused for the organization in the form. */
export function storedTokenApplies(connection: ConnectionView | null, form: ConnectionForm): boolean {
  return (
    !!connection?.configured &&
    connection.authMethod !== "oauth" &&
    connection.hasSecret &&
    !connection.reauthRequired &&
    normalizeOrganizationUrl(connection.organizationUrl ?? "") ===
      normalizeOrganizationUrl(form.organizationUrl)
  );
}

/** The default project only survives while the organization is unchanged. */
export function saveRequest(form: ConnectionForm, connection: ConnectionView | null) {
  const sameOrganization =
    normalizeOrganizationUrl(connection?.organizationUrl ?? "") ===
    normalizeOrganizationUrl(form.organizationUrl);
  return {
    organizationUrl: normalizeOrganizationUrl(form.organizationUrl),
    defaultProjectId: sameOrganization ? form.defaultProjectId : "",
    defaultProjectName: sameOrganization ? form.defaultProjectName : "",
    authMethod: form.authMethod,
    pat: form.authMethod === "pat" ? form.pat.trim() : "",
  };
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  return String(error);
}

type Envelope<Key extends string, Value> = { [K in Key]?: Value };

/** Typed wrappers over the plugin's workspace-scoped actions. */
export function createConnectionApi(host: PluginHostApi, workspaceId: string) {
  const invoke = <T>(key: string, body?: unknown, signal?: AbortSignal) =>
    host.api.invokeAction<T>(key, { workspaceId, body: body ?? {} }, { signal });
  return {
    async get(signal?: AbortSignal): Promise<ConnectionView | null> {
      const response = await invoke<Envelope<"connection", ConnectionView>>("connection.get", {}, signal);
      return response.connection?.configured ? response.connection : null;
    },
    async save(form: ConnectionForm, connection: ConnectionView | null): Promise<ConnectionView> {
      const response = await invoke<Envelope<"connection", ConnectionView>>(
        "connection.save",
        saveRequest(form, connection),
      );
      if (!response.connection) throw new Error("connection.save returned no connection");
      return response.connection;
    },
    async test(form: ConnectionForm): Promise<TestResult> {
      const response = await invoke<Envelope<"result", TestResult>>("connection.test", {
        organizationUrl: normalizeOrganizationUrl(form.organizationUrl),
        pat: form.authMethod === "pat" ? form.pat.trim() : "",
      });
      return response.result ?? { ok: false };
    },
    async copy(sourceWorkspaceId: string): Promise<ConnectionView> {
      const response = await invoke<Envelope<"connection", ConnectionView>>("connection.copy", {
        sourceWorkspaceId,
      });
      if (!response.connection) throw new Error("connection.copy returned no connection");
      return response.connection;
    },
    async defaultClientId(signal?: AbortSignal): Promise<string> {
      const response = await invoke<{ defaultClientId?: string }>("oauth.availability", {}, signal);
      return response.defaultClientId ?? "";
    },
    async startOAuth(form: ConnectionForm, connection: ConnectionView | null, redirectUrl: string): Promise<OAuthStart> {
      const request = saveRequest(form, connection);
      const response = await invoke<Partial<OAuthStart>>("oauth.start", {
        organizationUrl: request.organizationUrl,
        defaultProjectId: request.defaultProjectId,
        defaultProjectName: request.defaultProjectName,
        clientId: form.clientId.trim(),
        redirectUrl,
      });
      if (!response.authUrl || !response.state) throw new Error("oauth.start returned no authorization URL");
      return { authUrl: response.authUrl, state: response.state };
    },
    async completeOAuth(code: string, state: string): Promise<ConnectionView> {
      const response = await invoke<Envelope<"connection", ConnectionView>>("oauth.complete", { code, state });
      if (!response.connection) throw new Error("oauth.complete returned no connection");
      return response.connection;
    },
    async signOut(): Promise<ConnectionView> {
      const response = await invoke<Envelope<"connection", ConnectionView>>("oauth.sign_out");
      if (!response.connection) throw new Error("oauth.sign_out returned no connection");
      return response.connection;
    },
    async remove(): Promise<void> {
      await invoke("connection.delete");
    },
    async projects(signal?: AbortSignal): Promise<Project[]> {
      const response = await invoke<Envelope<"projects", Project[]>>("projects.list", {}, signal);
      return (response.projects ?? []).filter((project) => project.id && project.name);
    },
    async copySources(workspaceIds: readonly string[], signal?: AbortSignal): Promise<WorkspaceSummary[]> {
      const others = workspaceIds.filter((id) => id !== workspaceId);
      if (others.length === 0) return [];
      const response = await invoke<Envelope<"workspaces", WorkspaceSummary[]>>(
        "workspaces.describe",
        { workspaceIds: others },
        signal,
      );
      return (response.workspaces ?? []).filter((workspace) => workspace.configured);
    },
  };
}

export type ConnectionApi = ReturnType<typeof createConnectionApi>;
