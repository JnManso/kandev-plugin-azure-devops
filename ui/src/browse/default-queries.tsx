
import type { ReactNode } from "../runtime";
import { useCallback, useEffect, useMemo, useState } from "../runtime";
import { useT } from "../i18n";
import { IconPlus, IconRefresh, IconTrash } from "../icons";
import { Button, SettingsCard, SettingsSection, Textarea } from "../host-ui";
import {
  ACTION_PRESET_ICON_CHOICES,
  controlSizingClassName,
  iconForActionPreset,
  settingsActionClassName,
  settingsControlClassName,
  stripTransMarkers,
  useSettingsSaveContributor,
  useToast,
} from "./settings-shim";
import { CardContent } from "../host-ui";
import { Input } from "../host-ui";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../host-ui";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../host-ui";
import {
  getAzureDevOpsWorkspaceSettings,
  updateAzureDevOpsWorkspaceSettings,
} from "./rest-compat";
import type { AzureDevOpsQueryPreset } from "./types";
import {
  editListDrafts,
  listDraftsDirty,
  listDraftsEditable,
  listDraftsFrom,
  listDraftsSaveBody,
  resetListDrafts,
  type ListPair,
} from "./settings-drafts";
import {
  DEFAULT_AZURE_PULL_REQUEST_QUERIES,
  DEFAULT_AZURE_WORK_ITEM_QUERIES,
} from "./workspace-defaults";

/**
 * Wire discriminant for the two query kinds. These used to be the English
 * phrases "Work item" / "Pull request", which made the same value both a
 * `===` comparand and display copy; they are now identifiers, and the labels
 * resolve through the key maps below.
 */
type QueryKind = "work-item" | "pull-request";

/** Title-cased kind noun, e.g. "Work item query label 1". */
const KIND_TITLE_KEYS: Record<QueryKind, string> = {
  "work-item": "workItemKindTitle",
  "pull-request": "pullRequestKindTitle",
};

/** Lower-cased kind noun for mid-sentence use, e.g. "Remove work item query 1". */
const KIND_LOWER_KEYS: Record<QueryKind, string> = {
  "work-item": "workItemKindLower",
  "pull-request": "pullRequestKindLower",
};

/** Azure DevOps identity shorthand accepted by the creator/reviewer filters. */
const AZURE_ME_TOKEN = "@me";

/** Route these presets drive; a path, so it is interpolated rather than translated. */
const AZURE_DEVOPS_ROUTE = "/azure-devops";

// `label` is PERSISTED as part of `AzureDevOpsQueryPreset` and is editable in
// the row below, so it must stay locale-neutral — the same contract as
// `newPreset` in components/github/default-queries-section.tsx. The seeded WIQL
// is Azure DevOps' query language, not copy.
function newQuery(kind: QueryKind): AzureDevOpsQueryPreset {
  // i18n-exempt: persisted, editable preset label. See the comment above.
  return {
    id: `query_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`,
    label: "New query",
    group: "inbox",
    filters:
      kind === "work-item"
        ? {
            wiql: "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project",
            top: 50,
          }
        : { status: "active", creator: "", reviewer: "" },
  };
}

function stringFilter(preset: AzureDevOpsQueryPreset, key: string, fallback = ""): string {
  return typeof preset.filters[key] === "string" ? preset.filters[key] : fallback;
}

function numberFilter(preset: AzureDevOpsQueryPreset, key: string, fallback: number): number {
  return typeof preset.filters[key] === "number" ? preset.filters[key] : fallback;
}

function QueryGroupSelect({
  kind,
  index,
  value,
  dirty,
  onChange,
}: {
  kind: QueryKind;
  index: number;
  value: string;
  dirty: boolean;
  onChange: (value: string) => void;
}) {
  const t = useT();
  return (
    <Select value={value === "created" ? "created" : "inbox"} onValueChange={onChange}>
      <SelectTrigger
        className={settingsControlClassName("w-full cursor-pointer")}
        aria-label={t("queryGroupAria", {
          kind: t(KIND_TITLE_KEYS[kind]),
          index: index + 1,
        })}
        data-settings-dirty={dirty}
      >
        <SelectValue />
      </SelectTrigger>
      {/* `value` is the persisted preset group; only the label is copy. */}
      <SelectContent>
        <SelectItem value="inbox" className="cursor-pointer">
          {t("groupInbox")}
        </SelectItem>
        <SelectItem value="created" className="cursor-pointer">
          {t("groupCreated")}
        </SelectItem>
      </SelectContent>
    </Select>
  );
}

function QueryField({
  label,
  className,
  children,
}: {
  label: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <label className={`flex min-w-0 flex-col gap-0.5 ${className ?? ""}`}>
      <span className="text-[10px] text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

type QueryRowProps = {
  preset: AzureDevOpsQueryPreset;
  baseline?: AzureDevOpsQueryPreset;
  index: number;
  onPatch: (patch: Partial<AzureDevOpsQueryPreset>) => void;
  onPatchFilters: (patch: Record<string, unknown>) => void;
  onRemove: () => void;
};

function RemoveQueryButton({
  kind,
  index,
  onRemove,
}: {
  kind: QueryKind;
  index: number;
  onRemove: () => void;
}) {
  const t = useT();
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className={controlSizingClassName("icon", "cursor-pointer text-destructive")}
      aria-label={t("removeQueryAria", {
        kind: t(KIND_LOWER_KEYS[kind]),
        index: index + 1,
      })}
      onClick={onRemove}
    >
      <IconTrash className="h-4 w-4" />
    </Button>
  );
}

function WorkItemQueryRow({
  preset,
  baseline,
  index,
  onPatch,
  onPatchFilters,
  onRemove,
}: QueryRowProps) {
  const t = useT();
  const kind = t(KIND_TITLE_KEYS["work-item"]);
  return (
    <div
      className="grid grid-cols-2 gap-2 rounded-md border p-3 sm:grid-cols-[10rem_minmax(0,1fr)_5rem_7rem_auto] sm:items-end sm:p-2"
      data-settings-dirty={JSON.stringify(preset) !== JSON.stringify(baseline)}
      data-settings-dirty-level="container"
    >
      <QueryField label={t("label")}>
        <Input
          className={settingsControlClassName("w-full")}
          value={preset.label}
          aria-label={t("queryLabelAria", { kind, index: index + 1 })}
          data-settings-dirty={preset.label !== baseline?.label}
          onChange={(event) => onPatch({ label: event.target.value })}
        />
      </QueryField>
      {/* WIQL is the Azure DevOps query language, not copy. */}
      <QueryField label="WIQL" className="col-span-2 row-start-2 sm:col-span-1 sm:row-auto">
        <Input
          className={settingsControlClassName("w-full font-mono text-xs")}
          value={stringFilter(preset, "wiql")}
          aria-label={t("queryWiqlAria", { kind, index: index + 1 })}
          data-settings-dirty={
            stringFilter(preset, "wiql") !== (baseline ? stringFilter(baseline, "wiql") : undefined)
          }
          onChange={(event) => onPatchFilters({ wiql: event.target.value })}
        />
      </QueryField>
      <QueryField label={t("top")}>
        <Input
          type="number"
          min={1}
          max={200}
          className={settingsControlClassName("w-full")}
          value={numberFilter(preset, "top", 50)}
          aria-label={t("queryTopAria", { kind, index: index + 1 })}
          onChange={(event) => onPatchFilters({ top: Number(event.target.value) || 1 })}
        />
      </QueryField>
      <QueryField label={t("group")}>
        <QueryGroupSelect
          kind="work-item"
          index={index}
          value={preset.group}
          dirty={preset.group !== baseline?.group}
          onChange={(group) => onPatch({ group })}
        />
      </QueryField>
      <RemoveQueryButton kind="work-item" index={index} onRemove={onRemove} />
    </div>
  );
}

function PullRequestQueryRow({
  preset,
  baseline,
  index,
  onPatch,
  onPatchFilters,
  onRemove,
}: QueryRowProps) {
  const t = useT();
  const kind = t(KIND_TITLE_KEYS["pull-request"]);
  const identityPlaceholder = t("identityIdPlaceholder", { token: AZURE_ME_TOKEN });
  return (
    <div
      className="grid grid-cols-2 gap-2 rounded-md border p-3 sm:grid-cols-[10rem_8rem_minmax(0,1fr)_minmax(0,1fr)_7rem_auto] sm:items-end sm:p-2"
      data-settings-dirty={JSON.stringify(preset) !== JSON.stringify(baseline)}
      data-settings-dirty-level="container"
    >
      <QueryField label={t("label")}>
        <Input
          className={settingsControlClassName("w-full")}
          value={preset.label}
          aria-label={t("queryLabelAria", { kind, index: index + 1 })}
          data-settings-dirty={preset.label !== baseline?.label}
          onChange={(event) => onPatch({ label: event.target.value })}
        />
      </QueryField>
      <QueryField label={t("status")}>
        <Select
          value={stringFilter(preset, "status", "active")}
          onValueChange={(status) => onPatchFilters({ status })}
        >
          <SelectTrigger
            className={settingsControlClassName("w-full cursor-pointer")}
            aria-label={t("queryStatusAria", { kind, index: index + 1 })}
          >
            <SelectValue />
          </SelectTrigger>
          {/* `value` is the Azure DevOps pull-request status sent on the wire;
              only the item label is copy. */}
          <SelectContent>
            <SelectItem value="active">{t("statusActive")}</SelectItem>
            <SelectItem value="completed">{t("statusCompleted")}</SelectItem>
            <SelectItem value="abandoned">{t("statusAbandoned")}</SelectItem>
            <SelectItem value="all">{t("statusAll")}</SelectItem>
          </SelectContent>
        </Select>
      </QueryField>
      <QueryField label={t("creator")} className="col-span-2 sm:col-span-1">
        <Input
          className={settingsControlClassName("w-full")}
          value={stringFilter(preset, "creator")}
          aria-label={t("queryCreatorAria", { kind, index: index + 1 })}
          placeholder={identityPlaceholder}
          onChange={(event) => onPatchFilters({ creator: event.target.value })}
        />
      </QueryField>
      <QueryField label={t("reviewer")} className="col-span-2 sm:col-span-1">
        <Input
          className={settingsControlClassName("w-full")}
          value={stringFilter(preset, "reviewer")}
          aria-label={t("queryReviewerAria", { kind, index: index + 1 })}
          placeholder={identityPlaceholder}
          onChange={(event) => onPatchFilters({ reviewer: event.target.value })}
        />
      </QueryField>
      <QueryField label={t("group")}>
        <QueryGroupSelect
          kind="pull-request"
          index={index}
          value={preset.group}
          dirty={preset.group !== baseline?.group}
          onChange={(group) => onPatch({ group })}
        />
      </QueryField>
      <RemoveQueryButton kind="pull-request" index={index} onRemove={onRemove} />
    </div>
  );
}

function QueryEditor({
  kind,
  queries,
  baseline,
  onChange,
}: {
  kind: QueryKind;
  queries: AzureDevOpsQueryPreset[];
  baseline: AzureDevOpsQueryPreset[];
  onChange: (queries: AzureDevOpsQueryPreset[]) => void;
}) {
  const t = useT();
  const patch = (index: number, values: Partial<AzureDevOpsQueryPreset>) =>
    onChange(
      queries.map((query, current) => (current === index ? { ...query, ...values } : query)),
    );
  const patchFilters = (index: number, values: Record<string, unknown>) =>
    patch(index, { filters: { ...queries[index].filters, ...values } });
  const Row = kind === "work-item" ? WorkItemQueryRow : PullRequestQueryRow;
  return (
    <div className="space-y-2">
      {queries.map((query, index) => (
        <Row
          key={query.id}
          preset={query}
          baseline={baseline.find((candidate) => candidate.id === query.id)}
          index={index}
          onPatch={(values) => patch(index, values)}
          onPatchFilters={(values) => patchFilters(index, values)}
          onRemove={() => onChange(queries.filter((_, current) => current !== index))}
        />
      ))}
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName("w-full cursor-pointer sm:w-auto")}
        onClick={() => onChange([...queries, newQuery(kind)])}
      >
        <IconPlus className="h-4 w-4" />{" "}
        {kind === "pull-request" ? t("addPrQuery") : t("addWorkItemQuery")}
      </Button>
    </div>
  );
}

type Translate = ReturnType<typeof useT>;

const DEFAULT_QUERY_LISTS: ListPair<AzureDevOpsQueryPreset> = {
  workItems: DEFAULT_AZURE_WORK_ITEM_QUERIES,
  pullRequests: DEFAULT_AZURE_PULL_REQUEST_QUERIES,
};

// eslint-disable-next-line max-lines-per-function -- one owner for the load, edit, reset, and save state of the query lists.
function useQueryDrafts(workspaceId: string, t: Translate) {
  const [drafts, setDrafts] = useState(() => listDraftsFrom(DEFAULT_QUERY_LISTS));
  const [baseline, setBaseline] = useState(DEFAULT_QUERY_LISTS);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const dirty = useMemo(() => listDraftsDirty(drafts, baseline), [baseline, drafts]);

  useEffect(() => {
    let current = true;
    setLoading(true);
    setLoadError(null);
    void getAzureDevOpsWorkspaceSettings(workspaceId)
      .then((settings) => {
        if (!current) return;
        const stored = { workItems: settings.workItemQueries, pullRequests: settings.pullRequestQueries };
        setDrafts(listDraftsFrom(stored));
        setBaseline(stored);
      })
      .catch((error: unknown) => {
        if (!current) return;
        setLoadError(error instanceof Error ? error.message : t("failedToLoadDefaultQueries"));
      })
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [t, workspaceId]);

  const save = useCallback(async () => {
    const body = listDraftsSaveBody(drafts);
    const response = await updateAzureDevOpsWorkspaceSettings(workspaceId, {
      pullRequestQueries: body.pullRequests,
      workItemQueries: body.workItems,
    });
    const stored = { workItems: response.workItemQueries, pullRequests: response.pullRequestQueries };
    setDrafts(listDraftsFrom(stored));
    setBaseline(stored);
  }, [drafts, workspaceId]);
  const reset = useCallback(() => setDrafts(resetListDrafts(DEFAULT_QUERY_LISTS)), []);
  const discard = useCallback(() => setDrafts(listDraftsFrom(baseline)), [baseline]);
  const setWorkItems = useCallback(
    (values: AzureDevOpsQueryPreset[]) => setDrafts((previous) => editListDrafts(previous, "workItems", values)),
    [],
  );
  const setPullRequests = useCallback(
    (values: AzureDevOpsQueryPreset[]) => setDrafts((previous) => editListDrafts(previous, "pullRequests", values)),
    [],
  );

  return {
    workItems: drafts.workItems,
    pullRequests: drafts.pullRequests,
    setWorkItems,
    setPullRequests,
    baseline,
    loading,
    loadError,
    editable: listDraftsEditable({ loading, loadError }),
    dirty,
    save,
    reset,
    discard,
  };
}

function validQueries(queries: AzureDevOpsQueryPreset[], kind: QueryKind): boolean {
  return (
    queries.length > 0 &&
    queries.every(
      (query) =>
        query.label.trim() &&
        (kind === "pull-request"
          ? stringFilter(query, "status", "active")
          : stringFilter(query, "wiql")),
    )
  );
}

export function AzureDevOpsDefaultQueriesSection({ workspaceId }: { workspaceId: string }) {
  const t = useT();
  const drafts = useQueryDrafts(workspaceId, t);
  const { toast } = useToast();
  const valid =
    validQueries(drafts.pullRequests, "pull-request") &&
    validQueries(drafts.workItems, "work-item");
  const save = useCallback(async () => {
    try {
      await drafts.save();
      toast({ description: t("defaultQueriesSaved"), variant: "success" });
    } catch (error) {
      toast({
        description:
          error instanceof Error ? error.message : t("failedToSaveDefaultQueries"),
        variant: "error",
      });
      throw error;
    }
  }, [drafts, t, toast]);

  useSettingsSaveContributor({
    id: `azure-devops-default-queries:${workspaceId}`,
    revision: JSON.stringify([drafts.pullRequests, drafts.workItems]),
    isDirty: drafts.dirty,
    canSave: drafts.editable && valid,
    invalidReason: drafts.loadError ?? (valid ? undefined : t("defaultQueryInvalid")),
    save,
    discard: drafts.discard,
  });

  return (
    <SettingsSection
      title={t("defaultQueries")}
      description={t("defaultQueriesDescription", { path: AZURE_DEVOPS_ROUTE })}
      action={
        <Button
          type="button"
          variant="outline"
          className={settingsActionClassName("w-full cursor-pointer sm:w-auto")}
          disabled={!drafts.editable}
          onClick={drafts.reset}
        >
          <IconRefresh className="h-4 w-4" /> {t("commonReset")}
        </Button>
      }
    >
      <SettingsCard isDirty={drafts.dirty} data-testid="azure-devops-default-queries-card">
        <CardContent className="pt-4 sm:pt-6">
          {drafts.loadError && (
            <p role="alert" className="mb-4 text-sm text-destructive">
              {t("couldNotLoadDefaultQueries", { error: drafts.loadError })}
            </p>
          )}
          <fieldset disabled={!drafts.editable} className="contents">
            <Tabs defaultValue="pull-request">
              <TabsList className="w-full sm:w-auto">
                <TabsTrigger value="pull-request" className="flex-1 cursor-pointer sm:flex-none">
                  {t("pullRequestsTab")}
                </TabsTrigger>
                <TabsTrigger value="work-item" className="flex-1 cursor-pointer sm:flex-none">
                  {t("workItemsTab")}
                </TabsTrigger>
              </TabsList>
              <TabsContent value="pull-request">
                <QueryEditor
                  kind="pull-request"
                  queries={drafts.pullRequests}
                  baseline={drafts.baseline.pullRequests}
                  onChange={drafts.setPullRequests}
                />
              </TabsContent>
              <TabsContent value="work-item">
                <QueryEditor
                  kind="work-item"
                  queries={drafts.workItems}
                  baseline={drafts.baseline.workItems}
                  onChange={drafts.setWorkItems}
                />
              </TabsContent>
            </Tabs>
          </fieldset>
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}
