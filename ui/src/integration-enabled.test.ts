import { describe, expect, it, vi } from "vitest";
import type { PluginHostApi } from "@kdlbs/plugin-sdk";
import { createIntegrationEnabledStore, ENABLED_STORAGE_KEY } from "./integration-enabled";

const INTEGRATION_ID = "azure-devops";
const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

function storageKey(workspaceId: string) {
  return `workspace:${workspaceId}:${ENABLED_STORAGE_KEY}`;
}

function setup(stored: Record<string, unknown> = {}, workspaceIds: string[] = ["ws-1", "ws-2"]) {
  const entries = new Map(Object.entries(stored).map(([workspaceId, value]) => [storageKey(workspaceId), value]));
  let workspaceListener: ((ids: readonly string[]) => void) | undefined;
  const unsubscribeWorkspaces = vi.fn();
  const setIntegrationEnabled = vi.fn();
  const storage = {
    get: vi.fn(async (scope: string, scopeId: string, key: string) => {
      const id = `${scope}:${scopeId}:${key}`;
      return entries.has(id) ? { key, value: entries.get(id), updatedAt: "2026-11-09T00:00:00Z" } : undefined;
    }),
    set: vi.fn(async (scope: string, scopeId: string, key: string, value: unknown) => {
      entries.set(`${scope}:${scopeId}:${key}`, value);
      return { updatedAt: "2026-11-09T00:00:00Z" };
    }),
  };
  const host = {
    setIntegrationEnabled,
    storage,
    context: {
      getWorkspaceIds: () => workspaceIds,
      subscribeWorkspaces: (listener: (ids: readonly string[]) => void) => {
        workspaceListener = listener;
        return unsubscribeWorkspaces;
      },
    },
  } as unknown as PluginHostApi;
  const store = createIntegrationEnabledStore(host, INTEGRATION_ID);
  return { store, storage, setIntegrationEnabled, unsubscribeWorkspaces, entries, emitWorkspaces: (ids: string[]) => workspaceListener?.(ids) };
}

describe("integration enabled store", () => {
  it("defaults to enabled and publishes every workspace on start", async () => {
    const { store, setIntegrationEnabled } = setup();
    store.start();
    await flush();
    expect(store.get("ws-1")).toBe(true);
    expect(setIntegrationEnabled).toHaveBeenCalledWith(INTEGRATION_ID, "ws-1", true);
    expect(setIntegrationEnabled).toHaveBeenCalledWith(INTEGRATION_ID, "ws-2", true);
  });

  it("restores a stored disabled value for that workspace only", async () => {
    const { store, setIntegrationEnabled } = setup({ "ws-1": false });
    store.start();
    await flush();
    expect(store.get("ws-1")).toBe(false);
    expect(store.get("ws-2")).toBe(true);
    expect(setIntegrationEnabled).toHaveBeenCalledWith(INTEGRATION_ID, "ws-1", false);
    expect(setIntegrationEnabled).toHaveBeenCalledWith(INTEGRATION_ID, "ws-2", true);
  });

  it("persists a change, publishes it, and notifies subscribers", async () => {
    const { store, setIntegrationEnabled, entries } = setup();
    store.start();
    await flush();
    const listener = vi.fn();
    store.subscribe(listener);
    setIntegrationEnabled.mockClear();

    await store.setEnabled("ws-1", false);

    expect(entries.get(storageKey("ws-1"))).toBe(false);
    expect(store.get("ws-1")).toBe(false);
    expect(store.get("ws-2")).toBe(true);
    expect(setIntegrationEnabled.mock.calls).toEqual([[INTEGRATION_ID, "ws-1", false]]);
    expect(listener).toHaveBeenCalled();
  });

  it("leaves the value unknown after a failed read and recovers on retry", async () => {
    const { store, storage, setIntegrationEnabled } = setup({ "ws-1": false }, ["ws-1"]);
    storage.get.mockRejectedValueOnce(new Error("offline"));
    store.start();
    await flush();
    expect(store.get("ws-1")).toBeUndefined();
    expect(setIntegrationEnabled).not.toHaveBeenCalled();

    await store.load("ws-1");

    expect(store.get("ws-1")).toBe(false);
    expect(setIntegrationEnabled).toHaveBeenCalledWith(INTEGRATION_ID, "ws-1", false);
  });

  it("keeps the previous value when a write fails", async () => {
    const { store, storage, setIntegrationEnabled } = setup({}, ["ws-1"]);
    store.start();
    await flush();
    setIntegrationEnabled.mockClear();
    storage.set.mockRejectedValueOnce(new Error("offline"));

    await expect(store.setEnabled("ws-1", false)).rejects.toThrow("offline");

    expect(store.get("ws-1")).toBe(true);
    expect(setIntegrationEnabled).not.toHaveBeenCalled();
  });

  it("does not let a slower read overwrite a newer change", async () => {
    const { store, storage } = setup({}, ["ws-1"]);
    store.start();
    await flush();
    let resolveRead: (entry: undefined) => void = () => undefined;
    storage.get.mockImplementationOnce(() => new Promise((resolve) => { resolveRead = resolve; }));

    const read = store.load("ws-1");
    await store.setEnabled("ws-1", false);
    resolveRead(undefined);
    await read;

    expect(store.get("ws-1")).toBe(false);
  });

  it("loads workspaces that appear later and forgets removed ones", async () => {
    const { store, setIntegrationEnabled, emitWorkspaces } = setup({ "ws-3": false }, ["ws-1"]);
    store.start();
    await flush();

    emitWorkspaces(["ws-3"]);
    await flush();

    expect(store.get("ws-3")).toBe(false);
    expect(setIntegrationEnabled).toHaveBeenCalledWith(INTEGRATION_ID, "ws-3", false);
    expect(store.get("ws-1")).toBeUndefined();
  });

  it("stops publishing after destroy", async () => {
    const { store, storage, setIntegrationEnabled, unsubscribeWorkspaces } = setup({}, ["ws-1"]);
    let resolveRead: (entry: undefined) => void = () => undefined;
    storage.get.mockImplementationOnce(() => new Promise((resolve) => { resolveRead = resolve; }));
    store.start();

    store.destroy();
    resolveRead(undefined);
    await flush();

    expect(unsubscribeWorkspaces).toHaveBeenCalled();
    expect(setIntegrationEnabled).not.toHaveBeenCalled();
  });
});
