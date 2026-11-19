import type { Component, IntegrationSettingsActionProps, PluginHostApi } from "@kdlbs/plugin-sdk";
import { IntegrationEnabledControl } from "./host-ui";
import { useT } from "./i18n";
import { useCallback, useEffect, useState } from "./runtime";

/** Per-user, per-workspace `host.storage` key holding the switch value. */
export const ENABLED_STORAGE_KEY = "integration.enabled";

export type IntegrationEnabledStore = {
  /** The durable value, or undefined until it has been read. */
  get(workspaceId: string): boolean | undefined;
  subscribe(listener: () => void): () => void;
  /** Reads the durable value; rejects when storage cannot be read. */
  load(workspaceId: string): Promise<void>;
  /** Saves the value first, so a failed write leaves the switch unchanged. */
  setEnabled(workspaceId: string, enabled: boolean): Promise<void>;
  start(): void;
  destroy(): void;
};

/**
 * Durable per-workspace on/off state for the integration. Kandev only keeps
 * `setIntegrationEnabled` in memory, so every value is read back from
 * `host.storage` and republished when the plugin loads. Integrations are on
 * until turned off, as the built-in integrations are.
 */
export function createIntegrationEnabledStore(host: PluginHostApi, integrationId: string): IntegrationEnabledStore {
  const values = new Map<string, boolean>();
  const versions = new Map<string, number>();
  const listeners = new Set<() => void>();
  let unsubscribeWorkspaces: (() => void) | undefined;
  let destroyed = false;

  const bumpVersion = (workspaceId: string) => {
    const version = (versions.get(workspaceId) ?? 0) + 1;
    versions.set(workspaceId, version);
    return version;
  };

  const apply = (workspaceId: string, enabled: boolean) => {
    values.set(workspaceId, enabled);
    host.setIntegrationEnabled(integrationId, workspaceId, enabled);
    listeners.forEach((listener) => listener());
  };

  const load = async (workspaceId: string) => {
    const version = bumpVersion(workspaceId);
    const entry = await host.storage.get("workspace", workspaceId, ENABLED_STORAGE_KEY);
    if (destroyed || versions.get(workspaceId) !== version) return;
    apply(workspaceId, entry?.value !== false);
  };

  // A failed background read stays unknown; the switch retries when it mounts.
  const loadQuietly = (workspaceId: string) => void load(workspaceId).catch(() => undefined);

  const syncWorkspaces = (workspaceIds: readonly string[]) => {
    const current = new Set(workspaceIds);
    for (const workspaceId of values.keys()) {
      if (!current.has(workspaceId)) values.delete(workspaceId);
    }
    workspaceIds.filter((workspaceId) => !values.has(workspaceId)).forEach(loadQuietly);
  };

  return {
    get: (workspaceId) => values.get(workspaceId),
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    load,
    async setEnabled(workspaceId, enabled) {
      bumpVersion(workspaceId);
      await host.storage.set("workspace", workspaceId, ENABLED_STORAGE_KEY, enabled);
      if (!destroyed) apply(workspaceId, enabled);
    },
    start() {
      syncWorkspaces(host.context.getWorkspaceIds());
      unsubscribeWorkspaces = host.context.subscribeWorkspaces(syncWorkspaces);
    },
    destroy() {
      destroyed = true;
      unsubscribeWorkspaces?.();
      listeners.clear();
    },
  };
}

/** Native enable/disable switch for the integrations card and detail header. */
export function makeIntegrationEnabledAction(
  store: IntegrationEnabledStore,
  integrationId: string,
): Component<IntegrationSettingsActionProps> {
  return function AzureDevOpsEnabledAction({ workspaceId }: IntegrationSettingsActionProps) {
    const t = useT();
    const [, setRevision] = useState(0);

    useEffect(() => store.subscribe(() => setRevision((revision) => revision + 1)), []);
    useEffect(() => {
      if (workspaceId) void store.load(workspaceId).catch(() => undefined);
    }, [workspaceId]);

    const persist = useCallback(
      async (enabled: boolean) => {
        if (workspaceId) await store.setEnabled(workspaceId, enabled);
      },
      [workspaceId],
    );

    const enabled = workspaceId ? store.get(workspaceId) : undefined;
    if (enabled === undefined) return null;
    return <IntegrationEnabledControl id={integrationId} name={t("azureDevOps")} enabled={enabled} persist={persist} />;
  } as Component<IntegrationSettingsActionProps>;
}
