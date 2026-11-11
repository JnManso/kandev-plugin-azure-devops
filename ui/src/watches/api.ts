import type { PluginHostApi } from "@kdlbs/plugin-sdk";

export type WatchKind = "work_item" | "pull_request";

export type Watch = {
  id: string;
  kind: WatchKind;
  workspaceId: string;
  workflowId: string;
  workflowStepId: string;
  projectId: string;
  wiql?: string;
  azureRepositoryId?: string;
  status?: string;
  creatorId?: string;
  reviewerId?: string;
  repositoryId?: string;
  baseBranch?: string;
  agentProfileId: string;
  executorProfileId: string;
  prompt: string;
  enabled: boolean;
  pollIntervalSeconds: number;
  maxInflightTasks?: number;
  generation: number;
  disabledReason?: string;
  lastError?: string;
  lastPolledAt?: string;
};

export type WatchInput = Omit<
  Watch,
  "id" | "workspaceId" | "enabled" | "generation" | "disabledReason" | "lastError" | "lastPolledAt"
> & { enabled?: boolean };

export type Option = { id: string; name: string; steps?: Option[] };

export type WatchOptions = {
  workflows: Option[];
  agentProfiles: Option[];
  executorProfiles: Option[];
  repositories: Option[];
};

/** Typed wrappers over the plugin's watch actions. */
export function createWatchesApi(host: PluginHostApi, workspaceId: string) {
  const invoke = <T>(key: string, body: unknown = {}, signal?: AbortSignal) =>
    host.api.invokeAction<T>(key, { workspaceId, body }, { signal });
  return {
    list: async (signal?: AbortSignal) => (await invoke<{ watches?: Watch[] }>("watches.list", {}, signal)).watches ?? [],
    options: async (signal?: AbortSignal): Promise<WatchOptions> => {
      const response = await invoke<Partial<WatchOptions>>("watches.options", {}, signal);
      return {
        workflows: response.workflows ?? [],
        agentProfiles: response.agentProfiles ?? [],
        executorProfiles: response.executorProfiles ?? [],
        repositories: response.repositories ?? [],
      };
    },
    create: async (input: WatchInput) => (await invoke<{ watch: Watch }>("watches.create", input)).watch,
    update: async (id: string, input: Partial<WatchInput>) =>
      (await invoke<{ watch: Watch }>("watches.update", { id, ...input })).watch,
    remove: (id: string) => invoke("watches.delete", { id }),
    trigger: (id: string) => invoke<{ matched?: number; error?: string }>("watches.trigger", { id }),
    previewReset: async (id: string) => (await invoke<{ count?: number }>("watches.preview_reset", { id })).count ?? 0,
    reset: (id: string) => invoke("watches.reset", { id }),
  };
}

export type WatchesApi = ReturnType<typeof createWatchesApi>;

// The default prompts are persisted on the watch and sent to the agent; their
// placeholders are substituted by the plugin, so they stay untranslated.
export const DEFAULT_PROMPTS: Record<WatchKind, string> = {
  work_item: "Implement {{work_item.title}}", // i18n-exempt: persisted agent prompt
  pull_request: "Review {{pull_request.title}}", // i18n-exempt: persisted agent prompt
};

export function emptyWatchInput(kind: WatchKind, projectId = ""): WatchInput {
  return {
    kind,
    workflowId: "",
    workflowStepId: "",
    projectId,
    wiql: "",
    azureRepositoryId: "",
    status: "active",
    creatorId: "",
    reviewerId: "",
    repositoryId: "",
    baseBranch: "",
    agentProfileId: "",
    executorProfileId: "",
    prompt: DEFAULT_PROMPTS[kind],
    pollIntervalSeconds: 300,
    maxInflightTasks: 0,
  };
}

/** Fields the editor must have before it can save. */
export function missingWatchFields(input: WatchInput): string[] {
  const required: Array<keyof WatchInput> = ["projectId", "workflowId", "workflowStepId", "agentProfileId", "executorProfileId"];
  if (input.kind === "work_item") required.push("wiql");
  return required.filter((key) => !String(input[key] ?? "").trim());
}

/** Name of an option, or its ID when the host no longer lists it. */
export function optionName(options: Option[], id: string): string {
  return options.find((option) => option.id === id)?.name ?? id;
}

export function inputFromWatch(watch: Watch): WatchInput {
  const { id: _id, workspaceId: _workspace, generation: _generation, disabledReason: _reason, lastError: _error, lastPolledAt: _polled, ...input } = watch;
  return { ...emptyWatchInput(watch.kind), ...input };
}
