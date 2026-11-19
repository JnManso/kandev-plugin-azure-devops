import type { Component, PluginHostApi } from "@kdlbs/plugin-sdk";
import {
  type ConnectionApi,
  type ConnectionForm,
  type ConnectionView,
  EMPTY_FORM,
  type Project,
  type WorkspaceSummary,
  createConnectionApi,
  effectiveClientId,
  errorMessage,
  type AuthMethod,
  connectionFormState,
  formFromConnection,
  hasMicrosoftSignIn,
  healthFromConnection,
  isOrganizationUrl,
  microsoftSettingsSavable,
  personalAccessTokenUrl,
  storedTokenApplies,
} from "./connection-model";
import { type Translate, usePluginTranslation } from "./i18n";
import { type MicrosoftSignIn, useMicrosoftSignIn } from "./microsoft-sign-in";
import { AzureDevOpsDefaultQueriesSection } from "./browse/default-queries";
import { AzureDevOpsQuickActionsSection } from "./browse/quick-actions";
import { AzureDevOpsWatchSettings } from "./watches/watch-settings";

/** Health refresh while the settings page is open, matching built-in integrations. */
export const HEALTH_REFRESH_MS = 30_000;
const NO_PROJECT = "__none__";

type Busy = "save" | "test" | "remove" | "copy" | "signOut" | null;

type FieldProps = {
  host: PluginHostApi;
  t: Translate;
  form: ConnectionForm;
  setForm(update: (form: ConnectionForm) => ConnectionForm): void;
  disabled: boolean;
};

function help(host: PluginHostApi, text: string, testId?: string) {
  return host.jsx(
    "p",
    { className: "text-xs text-muted-foreground", ...(testId ? { "data-testid": testId } : {}) },
    text,
  );
}

function field(host: PluginHostApi, ...children: unknown[]) {
  return host.jsx("div", { className: "space-y-1.5" }, ...children);
}

function OrganizationField({ host, t, form, setForm, disabled }: FieldProps) {
  const invalid = form.organizationUrl.trim() !== "" && !isOrganizationUrl(form.organizationUrl);
  return field(
    host,
    host.jsx(host.ui.Label, { htmlFor: "azure-devops-organization-url" }, t("organizationUrl")),
    host.jsx(host.ui.Input, {
      id: "azure-devops-organization-url",
      "data-testid": "azure-devops-organization-url",
      value: form.organizationUrl,
      placeholder: "https://dev.azure.com/contoso",
      autoComplete: "off",
      disabled,
      "aria-invalid": invalid || undefined,
      onChange: (event: { target: { value: string } }) =>
        setForm((current) => ({ ...current, organizationUrl: event.target.value })),
    }),
    invalid
      ? host.jsx(
          "p",
          { className: "text-xs text-destructive", "data-testid": "azure-devops-organization-url-error" },
          t("invalidOrganizationUrl"),
        )
      : help(host, t("organizationUrlHelp")),
  );
}

function TokenField({ host, t, form, setForm, disabled, connection }: FieldProps & { connection: ConnectionView | null }) {
  const tokenUrl = personalAccessTokenUrl(form.organizationUrl);
  return field(
    host,
    host.jsx(host.ui.Label, { htmlFor: "azure-devops-pat" }, t("personalAccessToken")),
    host.jsx(host.ui.Input, {
      id: "azure-devops-pat",
      "data-testid": "azure-devops-pat",
      type: "password",
      autoComplete: "new-password",
      value: form.pat,
      disabled,
      placeholder: storedTokenApplies(connection, form) ? t("personalAccessTokenSaved") : "",
      onChange: (event: { target: { value: string } }) =>
        setForm((current) => ({ ...current, pat: event.target.value })),
    }),
    help(host, t("personalAccessTokenHelp")),
    tokenUrl
      ? host.jsx(
          "a",
          {
            href: tokenUrl,
            target: "_blank",
            rel: "noopener noreferrer",
            className: "text-xs underline underline-offset-2",
            "data-testid": "azure-devops-create-token",
          },
          t("createToken"),
        )
      : null,
  );
}

function ProjectField({ host, t, form, setForm, disabled, projects }: FieldProps & { projects: Project[] }) {
  const unavailable = projects.length === 0;
  const selected = projects.some((project) => project.id === form.defaultProjectId)
    ? form.defaultProjectId
    : NO_PROJECT;
  return field(
    host,
    host.jsx(host.ui.Label, { htmlFor: "azure-devops-default-project" }, t("defaultProject")),
    host.jsx(
      host.ui.Select,
      {
        value: selected,
        disabled: disabled || unavailable,
        onValueChange: (value: string) => {
          const project = projects.find((candidate) => candidate.id === value);
          setForm((current) => ({
            ...current,
            defaultProjectId: project?.id ?? "",
            defaultProjectName: project?.name ?? "",
          }));
        },
      },
      host.jsx(
        host.ui.SelectTrigger,
        { id: "azure-devops-default-project", "data-testid": "azure-devops-default-project", className: "w-full" },
        host.jsx(host.ui.SelectValue, {}),
      ),
      host.jsx(
        host.ui.SelectContent,
        {},
        host.jsx(host.ui.SelectItem, { value: NO_PROJECT }, t("noDefaultProject")),
        ...projects.map((project) => host.jsx(host.ui.SelectItem, { key: project.id, value: project.id }, project.name)),
      ),
    ),
    unavailable ? help(host, t("defaultProjectUnavailable")) : null,
  );
}

function AuthMethodField({ host, t, form, setForm, disabled }: FieldProps) {
  return field(
    host,
    host.jsx(host.ui.Label, { htmlFor: "azure-devops-auth-method" }, t("authMethod")),
    host.jsx(
      host.ui.Select,
      {
        value: form.authMethod,
        disabled,
        onValueChange: (value: AuthMethod) => setForm((current) => ({ ...current, authMethod: value })),
      },
      host.jsx(
        host.ui.SelectTrigger,
        { id: "azure-devops-auth-method", "data-testid": "azure-devops-auth-method", className: "w-full" },
        host.jsx(host.ui.SelectValue, {}),
      ),
      host.jsx(
        host.ui.SelectContent,
        {},
        host.jsx(host.ui.SelectItem, { value: "pat" }, t("personalAccessToken")),
        host.jsx(host.ui.SelectItem, { value: "oauth", "data-testid": "azure-devops-auth-oauth" }, t("authMethodMicrosoft")),
      ),
    ),
  );
}

function ClientIdField({
  host,
  t,
  form,
  setForm,
  disabled,
  defaultClientId,
  connection,
}: FieldProps & { defaultClientId: string; connection: ConnectionView | null }) {
  const clientInUse = connection?.oauthClientId;
  const next = effectiveClientId(form, connection, defaultClientId);
  const missing = !next;
  const pending = !!clientInUse && next.toLowerCase() !== clientInUse.toLowerCase();
  return field(
    host,
    host.jsx(host.ui.Label, { htmlFor: "azure-devops-oauth-client-id" }, t("oauthClientId")),
    host.jsx(host.ui.Input, {
      id: "azure-devops-oauth-client-id",
      "data-testid": "azure-devops-oauth-client-id",
      value: form.clientId,
      placeholder: defaultClientId,
      autoComplete: "off",
      spellCheck: false,
      className: "font-mono",
      disabled,
      onChange: (event: { target: { value: string } }) =>
        setForm((current) => ({ ...current, clientId: event.target.value })),
    }),
    missing
      ? host.jsx(
          "p",
          { className: "text-xs text-destructive", "data-testid": "azure-devops-oauth-client-required" },
          t("oauthClientIdRequired"),
        )
      : help(host, t("oauthApplicationHelp")),
    pending
      ? host.jsx(
          "p",
          { className: "text-xs text-amber-600", role: "status", "data-testid": "azure-devops-oauth-app-pending" },
          t("oauthApplicationChangePending"),
        )
      : null,
  );
}

function MicrosoftStatus({ host, t, connection }: { host: PluginHostApi; t: Translate; connection: ConnectionView | null }) {
  if (!hasMicrosoftSignIn(connection)) return help(host, t("oauthDescription"));
  if (connection?.reauthRequired) {
    return host.jsx(
      "p",
      { className: "text-sm text-destructive", role: "status", "data-testid": "azure-devops-oauth-reauth" },
      t("oauthReauthRequired"),
    );
  }
  return host.jsx(
    "div",
    { className: "space-y-1" },
    host.jsx(
      "p",
      { className: "text-sm font-medium", "data-testid": "azure-devops-oauth-account" },
      connection?.oauthAccount
        ? t("oauthSignedInAs", { values: { name: connection.oauthAccount } })
        : t("oauthSignedIn"),
    ),
    connection?.oauthClientId
      ? host.jsx(
          "p",
          { className: "break-all text-xs text-muted-foreground", "data-testid": "azure-devops-oauth-app-in-use" },
          t("oauthApplicationInUse", { values: { clientId: connection.oauthClientId } }),
        )
      : null,
  );
}

function PastePanel({ host, t, signIn, mobile }: { host: PluginHostApi; t: Translate; signIn: MicrosoftSignIn; mobile: boolean }) {
  if (!signIn.pasteOpen) return null;
  const fullWidth = mobile ? { width: "100%" } : undefined;
  return host.jsx(
    host.ui.Alert,
    { "data-testid": "azure-devops-oauth-paste" },
    host.jsx(
      host.ui.AlertDescription,
      { className: "space-y-3" },
      host.jsx("p", { className: "font-medium" }, t("oauthPasteTitle")),
      host.jsx("p", { className: "text-xs" }, t("oauthPasteHelp")),
      host.jsx(host.ui.Input, {
        type: "url",
        value: signIn.pasteUrl,
        disabled: signIn.completing,
        autoComplete: "off",
        "aria-label": t("oauthPasteTitle"),
        "data-testid": "azure-devops-oauth-paste-url",
        onChange: (event: { target: { value: string } }) => signIn.setPasteUrl(event.target.value),
      }),
      host.jsx(
        "div",
        { style: { display: "flex", flexDirection: mobile ? "column" : "row", gap: 8 } },
        host.jsx(
          host.ui.Button,
          {
            "data-testid": "azure-devops-oauth-paste-complete",
            disabled: signIn.completing || !signIn.pasteUrl,
            onClick: signIn.completeFromPaste,
            style: fullWidth,
          },
          t("oauthPasteComplete"),
        ),
        host.jsx(
          host.ui.Button,
          { variant: "outline", "data-testid": "azure-devops-oauth-paste-cancel", onClick: signIn.cancel, style: fullWidth },
          t("cancel"),
        ),
      ),
    ),
  );
}

function MicrosoftAccount({
  host,
  t,
  controller,
  mobile,
}: {
  host: PluginHostApi;
  t: Translate;
  controller: Controller;
  mobile: boolean;
}) {
  const { connection, form, setForm, busy, signIn, defaultClientId } = controller;
  const signedIn = hasMicrosoftSignIn(connection) && !connection?.reauthRequired;
  const canSignIn = isOrganizationUrl(form.organizationUrl) && !!effectiveClientId(form, connection, defaultClientId);
  const fullWidth = mobile ? { width: "100%" } : undefined;
  const progress = signIn.connecting
    ? t("oauthSigningIn")
    : signIn.completing
      ? t("oauthCompleting")
      : busy === "signOut"
        ? t("oauthSigningOut")
        : "";
  return host.jsx(
    "div",
    { className: "space-y-3 rounded-md border p-3", "data-testid": "azure-devops-microsoft-account" },
    host.jsx(MicrosoftStatus, { host, t, connection }),
    connectionFormState(connection, form).invalidReason === "oauthSignInForOrganization"
      ? host.jsx(
          "p",
          { className: "text-sm text-destructive", role: "status", "data-testid": "azure-devops-oauth-organization-changed" },
          t("oauthSignInForOrganization"),
        )
      : null,
    host.jsx(ClientIdField, {
      host,
      t,
      form,
      setForm,
      disabled: busy !== null || signIn.completing,
      defaultClientId,
      connection,
    }),
    host.jsx(
      "div",
      { style: { display: "flex", flexDirection: mobile ? "column" : "row", gap: 8 } },
      host.jsx(
        host.ui.Button,
        {
          variant: signedIn ? "outline" : "default",
          "data-testid": "azure-devops-oauth-connect",
          disabled: !canSignIn || signIn.connecting || signIn.completing || busy !== null,
          onClick: signIn.connect,
          style: fullWidth,
        },
        t(signedIn ? "oauthSignInAgain" : "oauthSignIn"),
      ),
      hasMicrosoftSignIn(connection)
        ? host.jsx(
            host.ui.Button,
            {
              variant: "ghost",
              "data-testid": "azure-devops-oauth-sign-out",
              disabled: busy !== null || signIn.connecting,
              onClick: controller.signOut,
              style: fullWidth,
            },
            t("oauthSignOut"),
          )
        : null,
    ),
    host.jsx("p", { className: "sr-only", role: "status", "aria-live": "polite" }, progress),
    host.jsx(PastePanel, { host, t, signIn, mobile }),
  );
}

function confirmRemove(host: PluginHostApi, t: Translate, mobile: boolean, onConfirm: () => void) {
  let handle: { close(): void } | undefined;
  const fullWidth = mobile ? { width: "100%" } : undefined;
  function RemoveConfirmation() {
    return host.jsx(
      "div",
      { style: { display: "flex", flexDirection: mobile ? "column-reverse" : "row", justifyContent: "flex-end", gap: 8 } },
      host.jsx(host.ui.Button, { variant: "outline", onClick: () => handle?.close(), style: fullWidth }, t("cancel")),
      host.jsx(
        host.ui.Button,
        {
          variant: "destructive",
          "data-testid": "azure-devops-remove-confirm",
          style: fullWidth,
          onClick: () => {
            handle?.close();
            onConfirm();
          },
        },
        t("remove"),
      ),
    );
  }
  handle = host.openModal({
    title: t("removeTitle"),
    description: t("removeDescription"),
    content: RemoveConfirmation,
    size: "sm",
    presentation: mobile ? "drawer" : "dialog",
  });
}

function CopySection({
  host,
  t,
  sources,
  busy,
  onCopy,
  mobile,
}: {
  host: PluginHostApi;
  t: Translate;
  sources: WorkspaceSummary[];
  busy: Busy;
  onCopy(sourceWorkspaceId: string): void;
  mobile: boolean;
}) {
  const [source, setSource] = host.React.useState("");
  if (sources.length === 0) return null;
  return host.jsx(
    "div",
    { className: "space-y-1.5", "data-testid": "azure-devops-copy-section" },
    host.jsx(host.ui.Label, { htmlFor: "azure-devops-copy-source" }, t("copyTitle")),
    host.jsx(
      "div",
      { style: { display: "flex", flexDirection: mobile ? "column" : "row", gap: 8 } },
      host.jsx(
        host.ui.Select,
        { value: source, onValueChange: setSource, disabled: busy !== null },
        host.jsx(
          host.ui.SelectTrigger,
          { id: "azure-devops-copy-source", "data-testid": "azure-devops-copy-source", className: "w-full" },
          host.jsx(host.ui.SelectValue, { placeholder: t("chooseWorkspace") }),
        ),
        host.jsx(
          host.ui.SelectContent,
          {},
          ...sources.map((workspace) =>
            host.jsx(host.ui.SelectItem, { key: workspace.id, value: workspace.id }, workspace.name),
          ),
        ),
      ),
      host.jsx(
        host.ui.Button,
        {
          variant: "outline",
          "data-testid": "azure-devops-copy",
          disabled: !source || busy !== null,
          onClick: () => onCopy(source),
          style: mobile ? { width: "100%" } : undefined,
        },
        t("copy"),
      ),
    ),
    help(host, t("copyHelp")),
  );
}

type Controller = ReturnType<typeof useConnectionController>;

function useConnectionController(host: PluginHostApi, workspaceId: string, t: Translate) {
  const React = host.React;
  const api: ConnectionApi = React.useMemo(() => createConnectionApi(host, workspaceId), [workspaceId]);
  const [connection, setConnection] = React.useState<ConnectionView | null>(null);
  const [form, replaceForm] = React.useState<ConnectionForm>(EMPTY_FORM);
  // Bumped on every form change; the Save changes bar compares revisions.
  const [revision, setRevision] = React.useState(0);
  const setForm = React.useCallback((next: ConnectionForm | ((current: ConnectionForm) => ConnectionForm)) => {
    replaceForm(next);
    setRevision((current) => current + 1);
  }, []);
  const [projects, setProjects] = React.useState<Project[]>([]);
  const [sources, setSources] = React.useState<WorkspaceSummary[]>([]);
  const [busy, setBusy] = React.useState<Busy>(null);
  const [defaultClientId, setDefaultClientId] = React.useState("");
  const usable = !!connection && connection.hasSecret && !connection.reauthRequired;
  const adopt = React.useCallback((next: ConnectionView | null) => {
    setConnection(next);
    setForm(formFromConnection(next));
  }, []);
  const signIn = useMicrosoftSignIn({ host, api, t, form, connection, onConnected: adopt });

  React.useEffect(() => {
    const controller = new AbortController();
    api
      .defaultClientId(controller.signal)
      .then(setDefaultClientId)
      .catch(() => setDefaultClientId(""));
    return () => controller.abort();
  }, [api]);

  React.useEffect(() => {
    const controller = new AbortController();
    setConnection(null);
    setForm(EMPTY_FORM);
    api
      .get(controller.signal)
      .then((loaded) => {
        setConnection(loaded);
        setForm(formFromConnection(loaded));
      })
      .catch(() => undefined);
    const refresh = setInterval(() => {
      api
        .get(controller.signal)
        .then(setConnection)
        .catch(() => undefined);
    }, HEALTH_REFRESH_MS);
    return () => {
      controller.abort();
      clearInterval(refresh);
    };
  }, [api]);

  React.useEffect(() => {
    if (!usable) {
      setProjects([]);
      return;
    }
    const controller = new AbortController();
    api
      .projects(controller.signal)
      .then(setProjects)
      .catch(() => setProjects([]));
    return () => controller.abort();
  }, [api, usable, connection?.organizationUrl]);

  React.useEffect(() => {
    const controller = new AbortController();
    const load = (ids: readonly string[]) =>
      api
        .copySources(ids, controller.signal)
        .then(setSources)
        .catch(() => setSources([]));
    load(host.context.getWorkspaceIds());
    const unsubscribe = host.context.subscribeWorkspaces(load);
    return () => {
      controller.abort();
      unsubscribe();
    };
  }, [api]);

  async function run(kind: Exclude<Busy, null>, operation: () => Promise<void>) {
    setBusy(kind);
    try {
      await operation();
    } catch (error) {
      host.toast.error(t("requestFailed", { values: { error: errorMessage(error) } }));
    } finally {
      setBusy(null);
    }
  }

  return {
    connection,
    form,
    setForm,
    revision,
    projects,
    sources,
    busy,
    signIn,
    defaultClientId,
    signOut: () =>
      run("signOut", async () => {
        try {
          adopt(await api.signOut());
          host.toast.success(t("oauthSignedOut"));
        } catch (error) {
          host.toast.error(t("oauthSignOutFailed", { values: { error: errorMessage(error) } }));
        }
      }),
    // Called by the Save changes bar, which keeps the form dirty on failure.
    saveChanges: async () => {
      setBusy("save");
      try {
        adopt(await api.save(form, connection));
        host.toast.success(t("saved"));
      } catch (error) {
        host.toast.error(t("requestFailed", { values: { error: errorMessage(error) } }));
        throw error;
      } finally {
        setBusy(null);
      }
    },
    discardChanges: () => setForm(formFromConnection(connection)),
    test: () =>
      run("test", async () => {
        const result = await api.test(form);
        if (result.ok) {
          host.toast.success(t("connectedAs", { values: { name: result.displayName || result.email || "" } }));
        } else {
          host.toast.error(t("connectionFailed", { values: { error: result.error ?? "" } }));
        }
      }),
    remove: () =>
      run("remove", async () => {
        await api.remove();
        setConnection(null);
        setForm(EMPTY_FORM);
        host.toast.success(t("removed"));
      }),
    copy: (sourceWorkspaceId: string) =>
      run("copy", async () => {
        const copied = await api.copy(sourceWorkspaceId);
        setConnection(copied);
        setForm(formFromConnection(copied));
        host.toast.success(t(copied.reauthRequired ? "copiedReauth" : "copied"));
      }),
  };
}

function Actions({ host, t, controller, mobile }: { host: PluginHostApi; t: Translate; controller: Controller; mobile: boolean }) {
  const { connection, form, busy } = controller;
  const validOrganization = isOrganizationUrl(form.organizationUrl);
  const microsoft = form.authMethod === "oauth";
  const hasToken = microsoft
    ? microsoftSettingsSavable(connection, form)
    : form.pat.trim() !== "" || storedTokenApplies(connection, form);
  const fullWidth = mobile ? { width: "100%" } : undefined;
  const test = host.jsx(
    host.ui.Button,
    {
      variant: "outline",
      "data-testid": "azure-devops-test",
      disabled: busy !== null || !validOrganization || !hasToken,
      onClick: controller.test,
      style: fullWidth,
    },
    busy === "test" ? t("testing") : t("testConnection"),
  );
  const remove = connection
    ? host.jsx(
        host.ui.Button,
        {
          variant: "ghost",
          className: "text-destructive",
          "data-testid": "azure-devops-remove",
          disabled: busy !== null,
          onClick: () => confirmRemove(host, t, mobile, controller.remove),
          style: fullWidth,
        },
        t("remove"),
      )
    : null;
  if (mobile) {
    return host.jsx("div", { style: { display: "flex", flexDirection: "column", gap: 8 } }, test, remove);
  }
  return host.jsx(
    "div",
    { style: { display: "flex", alignItems: "center", gap: 8 } },
    test,
    host.jsx("div", { style: { flex: 1 } }),
    remove,
  );
}

function WorkspaceConnection({ host, workspaceId }: { host: PluginHostApi; workspaceId: string }) {
  const t = usePluginTranslation(host);
  const mobile = host.useResponsiveBreakpoint().isMobile;
  const controller = useConnectionController(host, workspaceId, t);
  const { connection, form, setForm, busy } = controller;
  const disabled = busy !== null;
  const state = connectionFormState(connection, form);
  host.useSettingsSaveContributor({
    id: `azure-devops-connection:${workspaceId}`,
    revision: controller.revision,
    isDirty: state.dirty,
    canSave: state.canSave && !controller.signIn.connecting && !controller.signIn.completing,
    invalidReason: state.invalidReason ? t(state.invalidReason) : undefined,
    save: controller.saveChanges,
    discard: controller.discardChanges,
  });
  return host.jsx(
    "div",
    { className: "space-y-4", "data-testid": "azure-devops-settings" },
    host.jsx(host.ui.IntegrationAuthStatusBanner, { health: healthFromConnection(connection) }),
    connection?.reauthRequired && connection.authMethod !== "oauth"
      ? host.jsx(
          host.ui.Alert,
          { "data-testid": "azure-devops-reauth-required" },
          host.jsx(host.ui.AlertDescription, {}, t("reauthRequired")),
        )
      : null,
    host.jsx(
      host.ui.Card,
      {},
      host.jsx(
        host.ui.CardContent,
        { className: "space-y-4 pt-6" },
        host.jsx(OrganizationField, { host, t, form, setForm, disabled }),
        host.jsx(AuthMethodField, { host, t, form, setForm, disabled: disabled || controller.signIn.completing }),
        form.authMethod === "oauth"
          ? host.jsx(MicrosoftAccount, { host, t, controller, mobile })
          : host.jsx(TokenField, { host, t, form, setForm, disabled, connection }),
        host.jsx(ProjectField, { host, t, form, setForm, disabled, projects: controller.projects }),
        host.jsx(Actions, { host, t, controller, mobile }),
      ),
    ),
    host.jsx(CopySection, { host, t, sources: controller.sources, busy, onCopy: controller.copy, mobile }),
    connection ? host.jsx(AzureDevOpsDefaultQueriesSection, { workspaceId }) : null,
    connection ? host.jsx(AzureDevOpsQuickActionsSection, { workspaceId }) : null,
    connection
      ? host.jsx(AzureDevOpsWatchSettings, { workspaceId, defaultProject: connection.defaultProjectName ?? "" })
      : null,
  );
}

/** Settings > Integrations > Azure DevOps for the routed or active workspace. */
export function makeConnectionSettings(host: PluginHostApi): Component<{ workspaceId?: string }> {
  return function AzureDevOpsSettings({ workspaceId }: { workspaceId?: string } = {}) {
    const t = usePluginTranslation(host);
    const [activeWorkspaceId, setActiveWorkspaceId] = host.React.useState(
      () => host.context.getActiveWorkspaceId(),
    );
    host.React.useEffect(() => host.context.subscribeActiveWorkspace(setActiveWorkspaceId), []);
    const scoped = workspaceId || activeWorkspaceId;
    if (!scoped) {
      return host.jsx("p", { className: "text-sm text-muted-foreground" }, t("openWorkspace"));
    }
    return host.jsx(WorkspaceConnection, { key: scoped, host, workspaceId: scoped });
  };
}
