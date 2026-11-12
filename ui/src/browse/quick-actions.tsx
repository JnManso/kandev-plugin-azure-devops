
import { useCallback, useEffect, useMemo, useState } from "../runtime";
import type { ReactNode } from "../runtime";
import { useT } from "../i18n";
import { IconPlus, IconRefresh, IconTrash } from "../icons";
import { Button, SettingsCard, SettingsSection, Textarea } from "../host-ui";
import {
  ACTION_PRESET_ICON_CHOICES,
  type ScriptPlaceholder,
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
import type { AzureDevOpsActionPreset } from "./types";
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
  DEFAULT_AZURE_PULL_REQUEST_ACTIONS,
  DEFAULT_AZURE_WORK_ITEM_ACTIONS,
} from "./workspace-defaults";

type Translate = (key: string, values?: Record<string, unknown>) => string;

/**
 * Prompt-template placeholders offered by the ScriptEditor completions.
 *
 * A function rather than a module-scope constant: `t()` at module scope would
 * freeze the descriptions at the boot locale. `key` is the placeholder token
 * the backend substitutes and `example` is sample data — neither is copy.
 * Memoize at every call site so the shared editor does not rebuild the
 * completion provider on every keystroke.
 */
function actionPromptPlaceholders(t: Translate): ScriptPlaceholder[] {
  return [
    {
      key: "url",
      description: t("placeholderUrlDescription"),
      example: "https://dev.azure.com/acme/Platform/_workitems/edit/42",
      executor_types: [],
    },
    {
      key: "title",
      description: t("placeholderTitleDescription"),
      example: "Rotate integration credentials",
      executor_types: [],
    },
  ];
}

/**
 * Wire discriminant for the two action kinds. These used to be the English
 * phrases "Work item" / "Pull request", which made the same value both a
 * `===` comparand and display copy; they are now identifiers, and the labels
 * resolve through the key maps below.
 */
type ActionKind = "work-item" | "pull-request";

/** Title-cased kind noun, e.g. "Work item action label 1". */
const KIND_TITLE_KEYS: Record<ActionKind, string> = {
  "work-item": "workItemKindTitle",
  "pull-request": "pullRequestKindTitle",
};

/** Lower-cased kind noun for mid-sentence use, e.g. "Remove work item action 1". */
const KIND_LOWER_KEYS: Record<ActionKind, string> = {
  "work-item": "workItemKindLower",
  "pull-request": "pullRequestKindLower",
};

/** Route these presets drive; a path, so it is interpolated rather than translated. */
const AZURE_DEVOPS_ROUTE = "/azure-devops";

/** Prompt-template tokens, shown verbatim in the hint below the editor. */
const PROMPT_OPEN_BRACES = "{{";
const PROMPT_URL_TOKEN = "{{url}}";
const PROMPT_TITLE_TOKEN = "{{title}}";

// `label` is PERSISTED as part of `AzureDevOpsActionPreset` and is editable in
// the row below, so it must stay locale-neutral — the same contract as
// `newPreset` in components/github/action-presets-section.tsx.
function newAction(): AzureDevOpsActionPreset {
  // i18n-exempt: persisted, editable preset label. See the comment above.
  return {
    id: `preset_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`,
    label: "New action",
    hint: "",
    icon: "sparkle",
    promptTemplate: "",
  };
}

function ActionIconSelect({
  value,
  dirty,
  onChange,
}: {
  value: string;
  dirty: boolean;
  onChange: (value: string) => void;
}) {
  const t = useT();
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        className={settingsControlClassName("w-full cursor-pointer")}
        aria-label={t("icon")}
        data-settings-dirty={dirty}
      >
        <SelectValue>
          {(() => {
            const SelectedIcon = iconForActionPreset(value);
            return <SelectedIcon className="h-4 w-4" />;
          })()}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {ACTION_PRESET_ICON_CHOICES.map((choice) => {
          const ChoiceIcon = choice.icon;
          return (
            <SelectItem key={choice.key} value={choice.key} className="cursor-pointer">
              <span className="flex items-center gap-2">
                <ChoiceIcon className="h-3.5 w-3.5" />
                {t(choice.labelKey)}
              </span>
            </SelectItem>
          );
        })}
      </SelectContent>
    </Select>
  );
}

function Field({
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

function ActionPromptPanel({
  action,
  baseline,
  onPatch,
}: {
  action: AzureDevOpsActionPreset;
  baseline?: AzureDevOpsActionPreset;
  onPatch: (patch: Partial<AzureDevOpsActionPreset>) => void;
}) {
  const t = useT();
  const placeholders = useMemo(() => actionPromptPlaceholders(t), [t]);
  return (
    <div className="space-y-1 px-3 pb-3 sm:px-2 sm:pb-2">
      <Textarea
        value={action.promptTemplate}
        onChange={(event) => onPatch({ promptTemplate: event.target.value })}
        data-settings-dirty={action.promptTemplate !== baseline?.promptTemplate}
        data-testid={`azure-action-prompt-editor-${action.id}`}
        aria-label={t("prompt")}
        className="min-h-24 font-mono text-xs"
      />
      <p className="text-[11px] text-muted-foreground/60">
        {stripTransMarkers(
          t("promptPlaceholdersHint", {
            open: PROMPT_OPEN_BRACES,
            url: PROMPT_URL_TOKEN,
            title: PROMPT_TITLE_TOKEN,
          }),
        )}
      </p>
    </div>
  );
}

function ActionRow({
  kind,
  index,
  action,
  baseline,
  expanded,
  onToggle,
  onPatch,
  onRemove,
}: {
  kind: ActionKind;
  index: number;
  action: AzureDevOpsActionPreset;
  baseline?: AzureDevOpsActionPreset;
  expanded: boolean;
  onToggle: () => void;
  onPatch: (patch: Partial<AzureDevOpsActionPreset>) => void;
  onRemove: () => void;
}) {
  const t = useT();
  const kindTitle = t(KIND_TITLE_KEYS[kind]);
  return (
    <div
      className="rounded-md border"
      data-settings-dirty={JSON.stringify(action) !== JSON.stringify(baseline)}
      data-settings-dirty-level="container"
    >
      <div className="grid grid-cols-2 gap-2 p-3 sm:grid-cols-[5rem_10rem_minmax(0,1fr)_auto_auto] sm:items-end sm:p-2">
        <Field label={t("icon")}>
          <ActionIconSelect
            value={action.icon}
            dirty={action.icon !== baseline?.icon}
            onChange={(icon) => onPatch({ icon })}
          />
        </Field>
        <Field label={t("label")}>
          <Input
            className={settingsControlClassName("w-full")}
            value={action.label}
            aria-label={t("actionLabelAria", { kind: kindTitle, index: index + 1 })}
            data-settings-dirty={action.label !== baseline?.label}
            onChange={(event) => onPatch({ label: event.target.value })}
          />
        </Field>
        <Field label={t("hint")} className="col-span-2 sm:col-span-1">
          <Input
            className={settingsControlClassName("w-full")}
            value={action.hint}
            aria-label={t("actionHintAria", { kind: kindTitle, index: index + 1 })}
            placeholder={t("hintOptional")}
            data-settings-dirty={action.hint !== baseline?.hint}
            onChange={(event) => onPatch({ hint: event.target.value })}
          />
        </Field>
        <Button
          type="button"
          variant="outline"
          className={settingsActionClassName("cursor-pointer")}
          onClick={onToggle}
        >
          {expanded ? t("hidePrompt") : t("editPrompt")}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={controlSizingClassName("icon", "cursor-pointer text-destructive")}
          onClick={onRemove}
          aria-label={t("removeActionAria", {
            kind: t(KIND_LOWER_KEYS[kind]),
            index: index + 1,
          })}
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </div>
      {expanded && <ActionPromptPanel action={action} baseline={baseline} onPatch={onPatch} />}
    </div>
  );
}

function ActionEditor({
  kind,
  actions,
  baseline,
  onChange,
}: {
  kind: ActionKind;
  actions: AzureDevOpsActionPreset[];
  baseline: AzureDevOpsActionPreset[];
  onChange: (actions: AzureDevOpsActionPreset[]) => void;
}) {
  const t = useT();
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const patch = (index: number, values: Partial<AzureDevOpsActionPreset>) =>
    onChange(
      actions.map((action, current) => (current === index ? { ...action, ...values } : action)),
    );
  const add = () => {
    const action = newAction();
    onChange([...actions, action]);
    setExpandedId(action.id);
  };
  return (
    <div className="space-y-2">
      {actions.map((action, index) => (
        <ActionRow
          key={action.id}
          kind={kind}
          index={index}
          action={action}
          baseline={baseline.find((candidate) => candidate.id === action.id)}
          expanded={expandedId === action.id}
          onToggle={() => setExpandedId((id) => (id === action.id ? null : action.id))}
          onPatch={(values) => patch(index, values)}
          onRemove={() => onChange(actions.filter((_, current) => current !== index))}
        />
      ))}
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName("w-full cursor-pointer sm:w-auto")}
        onClick={add}
      >
        <IconPlus className="h-4 w-4" />{" "}
        {kind === "pull-request"
          ? t("addPrAction")
          : t("addWorkItemAction")}
      </Button>
    </div>
  );
}

const DEFAULT_ACTION_LISTS: ListPair<AzureDevOpsActionPreset> = {
  workItems: DEFAULT_AZURE_WORK_ITEM_ACTIONS,
  pullRequests: DEFAULT_AZURE_PULL_REQUEST_ACTIONS,
};

// eslint-disable-next-line max-lines-per-function -- one owner for the load, edit, reset, and save state of the action lists.
function useActionDrafts(workspaceId: string, t: Translate) {
  const [drafts, setDrafts] = useState(() => listDraftsFrom(DEFAULT_ACTION_LISTS));
  const [baseline, setBaseline] = useState(DEFAULT_ACTION_LISTS);
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
        const stored = { workItems: settings.workItemActions, pullRequests: settings.pullRequestActions };
        setDrafts(listDraftsFrom(stored));
        setBaseline(stored);
      })
      .catch((error: unknown) => {
        if (!current) return;
        setLoadError(
          error instanceof Error ? error.message : t("failedToLoadQuickActions"),
        );
      })
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [t, workspaceId]);

  const save = useCallback(async () => {
    const body = listDraftsSaveBody(drafts);
    const response = await updateAzureDevOpsWorkspaceSettings(workspaceId, {
      workItemActions: body.workItems,
      pullRequestActions: body.pullRequests,
    });
    const stored = { workItems: response.workItemActions, pullRequests: response.pullRequestActions };
    setDrafts(listDraftsFrom(stored));
    setBaseline(stored);
  }, [drafts, workspaceId]);
  const reset = useCallback(() => setDrafts(resetListDrafts(DEFAULT_ACTION_LISTS)), []);
  const discard = useCallback(() => setDrafts(listDraftsFrom(baseline)), [baseline]);
  const setWorkItems = useCallback(
    (values: AzureDevOpsActionPreset[]) => setDrafts((previous) => editListDrafts(previous, "workItems", values)),
    [],
  );
  const setPullRequests = useCallback(
    (values: AzureDevOpsActionPreset[]) =>
      setDrafts((previous) => editListDrafts(previous, "pullRequests", values)),
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

function validActions(actions: AzureDevOpsActionPreset[]): boolean {
  return (
    actions.length > 0 &&
    actions.every((action) => action.label.trim() && action.promptTemplate.trim())
  );
}

export function AzureDevOpsQuickActionsSection({ workspaceId }: { workspaceId: string }) {
  const t = useT();
  const drafts = useActionDrafts(workspaceId, t);
  const { toast } = useToast();
  const valid = validActions(drafts.workItems) && validActions(drafts.pullRequests);
  const save = useCallback(async () => {
    try {
      await drafts.save();
      toast({ description: t("quickActionsSaved"), variant: "success" });
    } catch (error) {
      toast({
        description:
          error instanceof Error ? error.message : t("failedToSaveQuickActions"),
        variant: "error",
      });
      throw error;
    }
  }, [drafts, t, toast]);

  useSettingsSaveContributor({
    id: `azure-devops-actions:${workspaceId}`,
    revision: JSON.stringify([drafts.workItems, drafts.pullRequests]),
    isDirty: drafts.dirty,
    canSave: drafts.editable && valid,
    invalidReason: drafts.loadError ?? (valid ? undefined : t("quickActionInvalid")),
    save,
    discard: drafts.discard,
  });

  return (
    <SettingsSection
      title={t("quickActions")}
      description={t("quickActionsDescription", { path: AZURE_DEVOPS_ROUTE })}
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
      <SettingsCard isDirty={drafts.dirty} data-testid="azure-devops-quick-actions-card">
        <CardContent className="pt-4 sm:pt-6">
          {drafts.loadError && (
            <p role="alert" className="mb-4 text-sm text-destructive">
              {t("couldNotLoadQuickActions", { error: drafts.loadError })}
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
                <ActionEditor
                  kind="pull-request"
                  actions={drafts.pullRequests}
                  baseline={drafts.baseline.pullRequests}
                  onChange={drafts.setPullRequests}
                />
              </TabsContent>
              <TabsContent value="work-item">
                <ActionEditor
                  kind="work-item"
                  actions={drafts.workItems}
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
