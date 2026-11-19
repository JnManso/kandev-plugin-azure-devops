import type { KandevPlugin } from "@kdlbs/plugin-sdk";
import { createAzureDevOpsIcon } from "./azure-icon";
import { AzureDevOpsPage } from "./browse/page";
import { makeConnectionSettings } from "./connection-settings";
import { pluginTranslate, registerTranslations } from "./i18n";
import {
  createIntegrationEnabledStore,
  type IntegrationEnabledStore,
  makeIntegrationEnabledAction,
} from "./integration-enabled";
import { setHost } from "./runtime";
import { createRepositoryProvider } from "./source-control/register";
import { registerTaskIntegrations, type TaskIntegrations } from "./tasks/register";

/** Manifest id; it prefixes every host route served for this plugin. */
export const PLUGIN_ID = "kandev-plugin-azure-devops";
/** Kandev repository provider owned by this plugin. */
export const PROVIDER_ID = "azure_devops";
/** Settings > Integrations entry id, released by Kandev's built-in removal. */
export const INTEGRATION_SETTINGS_ID = "azure-devops";
/** Browse page route, the built-in integration's former path. */
export const BROWSE_PATH = "/azure-devops";

/** Cheap ownership hint; `repositories.inspect` stays the authority. */
export function matchesAzureReposURL(url: string): boolean {
  const value = url.trim().toLowerCase();
  if (value.startsWith("git@ssh.dev.azure.com:")) return true;
  try {
    const host = new URL(value).hostname;
    return host === "dev.azure.com" || host === "ssh.dev.azure.com" || host.endsWith(".visualstudio.com");
  } catch {
    return false;
  }
}

export function createAzureDevOpsPlugin(): KandevPlugin {
  let tasks: TaskIntegrations | null = null;
  let enabledStore: IntegrationEnabledStore | null = null;
  return {
    initialize(registry, host) {
      setHost(host);
      registerTranslations(registry);
      const t = pluginTranslate(host);
      const icon = createAzureDevOpsIcon(host);
      const enabled = createIntegrationEnabledStore(host, INTEGRATION_SETTINGS_ID);
      enabledStore = enabled;
      registry.registerIntegrationSettings({
        id: INTEGRATION_SETTINGS_ID,
        get label() {
          return t("azureDevOps");
        },
        get description() {
          return t("integrationDescription");
        },
        icon,
        Component: makeConnectionSettings(host),
        action: makeIntegrationEnabledAction(enabled, INTEGRATION_SETTINGS_ID),
      });
      enabled.start();
      registry.registerRoute(BROWSE_PATH, AzureDevOpsPage, {
        topbar: {
          title: "Azure DevOps",
          icon,
          get subtitle() {
            return t("boardsWorkItemsPullRequests");
          },
        },
      });
      registry.registerNavItem({
        id: "azure-devops",
        get label() {
          return t("azureDevOps");
        },
        path: BROWSE_PATH,
        icon,
        section: "integrations",
      });
      const taskIntegrations = registerTaskIntegrations(registry, host, icon);
      tasks = taskIntegrations;
      registry.registerRepositoryProvider({
        ...createRepositoryProvider(host, {
          providerId: PROVIDER_ID,
          label: "Azure DevOps",
          icon,
          matchesURL: matchesAzureReposURL,
          supportsDraft: true,
        }),
        async createChangeRequest({ workspaceId, taskId, sessionId, repositoryId, title, body, baseBranch, draft, signal }) {
          const result = await taskIntegrations.api.createPullRequest(
            { workspaceId, taskId, sessionId, repositoryId },
            { title, description: body, ...(baseBranch ? { baseBranch } : {}), draft },
            signal,
          );
          if (!result.url) throw new Error(t("createPullRequestNoUrl"));
          await taskIntegrations.refreshTask(workspaceId, taskId, signal);
          return {
            url: result.url,
            provider: PROVIDER_ID,
            ...(typeof result.linked === "boolean" ? { linked: result.linked } : {}),
            ...(result.associationError ? { associationError: result.associationError } : {}),
          };
        },
      });
    },
    destroy() {
      tasks?.destroy();
      tasks = null;
      enabledStore?.destroy();
      enabledStore = null;
    },
  };
}
