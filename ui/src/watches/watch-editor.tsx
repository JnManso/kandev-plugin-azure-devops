import { currentHost, useEffect, useMemo, useResponsiveBreakpoint, useState } from "../runtime";
import type { ReactNode } from "../runtime";
import { useT, type TFunction } from "../i18n";
import { IconX } from "../icons";
import {
  Alert,
  AlertDescription,
  Button,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Textarea,
} from "../host-ui";
import { createBrowseApi } from "../browse/api";
import type {
  AzureDevOpsIdentity,
  AzureDevOpsProject,
  AzureDevOpsPullRequestPeople,
  AzureDevOpsRepository,
} from "../browse/types";
import { personPickerOptions } from "../browse/pull-request-people";
import { SearchablePicker, type PickerOption } from "../browse/searchable-picker";
import { ANY_OPTION } from "../browse/sprints-model";
import { useAzureIdentity, usePullRequestPeople } from "../browse/use-pull-request-people";
import { missingWatchFields, type Option, type WatchInput, type WatchKind, type WatchOptions } from "./api";

/** Select value that stands for "no value"; Radix Select rejects "". */
const NONE = "__none__";

const PLACEHOLDERS: Record<WatchKind, Array<[string, string]>> = {
  work_item: [
    ["{{work_item.title}}", "placeholderTitleDescription"],
    ["{{work_item.url}}", "placeholderUrlDescription"],
    ["{{work_item.id}}", "placeholderIdDescription"],
    ["{{work_item.description}}", "placeholderDescriptionBody"],
    ["{{work_item.state}}", "placeholderStateDescription"],
    ["{{work_item.type}}", "placeholderTypeDescription"],
    ["{{work_item.project}}", "placeholderProjectDescription"],
  ],
  pull_request: [
    ["{{pull_request.title}}", "placeholderTitleDescription"],
    ["{{pull_request.url}}", "placeholderUrlDescription"],
    ["{{pull_request.id}}", "placeholderIdDescription"],
    ["{{pull_request.description}}", "placeholderDescriptionBody"],
    ["{{pull_request.state}}", "placeholderStateDescription"],
    ["{{pull_request.source_branch}}", "placeholderSourceBranchDescription"],
    ["{{pull_request.target_branch}}", "placeholderTargetBranchDescription"],
    ["{{pull_request.project}}", "placeholderProjectDescription"],
  ],
};

type Set = <K extends keyof WatchInput>(key: K, value: WatchInput[K]) => void;

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      {children}
    </div>
  );
}

function OptionSelect({
  value,
  options,
  onChange,
  testId,
  allowNone,
  noneLabel,
}: {
  value: string;
  options: Option[];
  onChange: (value: string) => void;
  testId: string;
  allowNone?: boolean;
  noneLabel?: string;
}) {
  const t = useT();
  const known = options.some((option) => option.id === value);
  return (
    <Select value={value || (allowNone ? NONE : undefined)} onValueChange={(next) => onChange(next === NONE ? "" : next)}>
      <SelectTrigger className="w-full" data-testid={testId}>
        <SelectValue placeholder={t("watchSelectPlaceholder")} />
      </SelectTrigger>
      <SelectContent>
        {allowNone && <SelectItem value={NONE}>{noneLabel}</SelectItem>}
        {value && !known && <SelectItem value={value}>{value}</SelectItem>}
        {options.map((option) => (
          <SelectItem key={option.id} value={option.id}>
            {option.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function useAzureChoices(workspaceId: string, projectId: string) {
  const [projects, setProjects] = useState<AzureDevOpsProject[]>([]);
  const [repositories, setRepositories] = useState<AzureDevOpsRepository[]>([]);
  useEffect(() => {
    const controller = new AbortController();
    createBrowseApi(currentHost(), workspaceId).projects(controller.signal).then(setProjects, () => setProjects([]));
    return () => controller.abort();
  }, [workspaceId]);
  useEffect(() => {
    if (!projectId) {
      setRepositories([]);
      return undefined;
    }
    const controller = new AbortController();
    createBrowseApi(currentHost(), workspaceId)
      .repositories(projectId, controller.signal)
      .then(setRepositories, () => setRepositories([]));
    return () => controller.abort();
  }, [workspaceId, projectId]);
  return {
    projects: projects.map((project) => ({ id: project.name, name: project.name })),
    repositories: repositories.map((repository) => ({ id: repository.id, name: repository.name })),
  };
}

type PeopleChoices = { people?: AzureDevOpsPullRequestPeople; me?: AzureDevOpsIdentity };

function PersonField({
  label,
  id,
  value,
  options,
  onChange,
  anyLabel,
}: {
  label: string;
  id: string;
  value: string;
  options: PickerOption[];
  onChange: (value: string) => void;
  anyLabel: string;
}) {
  return (
    <Field label={label}>
      <SearchablePicker
        value={value || ANY_OPTION}
        options={[{ id: ANY_OPTION, name: anyLabel }, ...options]}
        onChange={(next) => onChange(next === ANY_OPTION ? "" : next)}
        placeholder={anyLabel}
        testId={id}
        className="w-full"
      />
    </Field>
  );
}

function QueryFields({
  t,
  input,
  set,
  azureRepositories,
  choices,
}: {
  t: TFunction;
  input: WatchInput;
  set: Set;
  azureRepositories: Option[];
  choices: PeopleChoices;
}) {
  const meLabel = (name?: string) => (name ? t("meNamed", { name }) : t("me"));
  if (input.kind === "work_item") {
    return (
      <Field label="WIQL">
        <Textarea
          className="min-h-28 font-mono text-xs"
          value={input.wiql ?? ""}
          onChange={(event) => set("wiql", event.target.value)}
          data-testid="azure-work-item-watch-wiql"
        />
      </Field>
    );
  }
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Field label={t("azureRepositoryIdOptional")}>
        <OptionSelect
          value={input.azureRepositoryId ?? ""}
          options={azureRepositories}
          onChange={(value) => set("azureRepositoryId", value)}
          testId="azure-pull-request-watch-azure-repository"
          allowNone
          noneLabel={t("pullRequestAny")}
        />
      </Field>
      <Field label={t("status")}>
        <Select value={input.status || "active"} onValueChange={(value) => set("status", value)}>
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="active">{t("statusActive")}</SelectItem>
            <SelectItem value="completed">{t("statusCompleted")}</SelectItem>
            <SelectItem value="abandoned">{t("statusAbandoned")}</SelectItem>
            <SelectItem value="all">{t("statusAny")}</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <PersonField
        label={t("pullRequestCreatedBy")}
        id="azure-pull-request-watch-creator"
        value={input.creatorId ?? ""}
        options={personPickerOptions(choices.people?.creators ?? [], meLabel, input.creatorId ?? "", choices.me)}
        onChange={(value) => set("creatorId", value)}
        anyLabel={t("anyone")}
      />
      <PersonField
        label={t("reviewer")}
        id="azure-pull-request-watch-reviewer"
        value={input.reviewerId ?? ""}
        options={personPickerOptions(choices.people?.reviewers ?? [], meLabel, input.reviewerId ?? "", choices.me)}
        onChange={(value) => set("reviewerId", value)}
        anyLabel={t("anyone")}
      />
    </div>
  );
}

function LaunchFields({ t, input, set, options }: { t: TFunction; input: WatchInput; set: Set; options: WatchOptions }) {
  const steps = options.workflows.find((workflow) => workflow.id === input.workflowId)?.steps ?? [];
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Field label={t("watchWorkflow")}>
        <OptionSelect
          value={input.workflowId}
          options={options.workflows}
          onChange={(value) => {
            set("workflowId", value);
            set("workflowStepId", options.workflows.find((workflow) => workflow.id === value)?.steps?.[0]?.id ?? "");
          }}
          testId={`azure-${input.kind}-watch-workflow`}
        />
      </Field>
      <Field label={t("watchWorkflowStep")}>
        <OptionSelect value={input.workflowStepId} options={steps} onChange={(value) => set("workflowStepId", value)} testId={`azure-${input.kind}-watch-step`} />
      </Field>
      <Field label={t("watchAgentProfile")}>
        <OptionSelect value={input.agentProfileId} options={options.agentProfiles} onChange={(value) => set("agentProfileId", value)} testId={`azure-${input.kind}-watch-agent`} />
      </Field>
      <Field label={t("watchExecutorProfile")}>
        <OptionSelect value={input.executorProfileId} options={options.executorProfiles} onChange={(value) => set("executorProfileId", value)} testId={`azure-${input.kind}-watch-executor`} />
      </Field>
      <Field label={t("watchRepository")}>
        <OptionSelect
          value={input.repositoryId ?? ""}
          options={options.repositories}
          onChange={(value) => set("repositoryId", value)}
          testId={`azure-${input.kind}-watch-repository`}
          allowNone
          noneLabel={t("watchNoRepository")}
        />
      </Field>
      <Field label={t("baseBranch")}>
        <Input
          value={input.baseBranch ?? ""}
          onChange={(event) => set("baseBranch", event.target.value)}
          placeholder="main"
          disabled={!input.repositoryId}
          data-testid={`azure-${input.kind}-watch-branch`}
        />
      </Field>
    </div>
  );
}

function PromptField({ t, input, set }: { t: TFunction; input: WatchInput; set: Set }) {
  return (
    <Field label={t("prompt")}>
      <Textarea value={input.prompt} onChange={(event) => set("prompt", event.target.value)} className="min-h-24" data-testid={`azure-${input.kind}-watch-prompt`} />
      <details className="text-xs text-muted-foreground">
        <summary className="cursor-pointer">{t("watchPromptPlaceholders")}</summary>
        <ul className="mt-1 space-y-0.5">
          {PLACEHOLDERS[input.kind].map(([token, key]) => (
            <li key={token}>
              <code>{token}</code>: {t(key)}
            </li>
          ))}
        </ul>
      </details>
    </Field>
  );
}

function ScheduleFields({ t, input, set }: { t: TFunction; input: WatchInput; set: Set }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Field label={t("pollIntervalSeconds")}>
        <Input type="number" min={60} value={input.pollIntervalSeconds} onChange={(event) => set("pollIntervalSeconds", Number(event.target.value))} />
      </Field>
      <Field label={t("maxInflightTasks")}>
        <Input type="number" min={0} value={input.maxInflightTasks ?? 0} onChange={(event) => set("maxInflightTasks", Number(event.target.value))} />
      </Field>
    </div>
  );
}

export type WatchEditorProps = {
  workspaceId: string;
  initial: WatchInput;
  editing: boolean;
  options: WatchOptions;
  onClose: () => void;
  onSubmit: (input: WatchInput) => Promise<unknown>;
};

/** Create or edit one watch; a dialog on desktop and a full-height drawer on phones. */
export function WatchEditor({ workspaceId, initial, editing, options, onClose, onSubmit }: WatchEditorProps) {
  const t = useT();
  const { isMobile } = useResponsiveBreakpoint();
  const [input, setInput] = useState<WatchInput>(initial);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const choices = useAzureChoices(workspaceId, input.projectId);
  const pullRequestWatch = input.kind !== "work_item";
  const people = usePullRequestPeople(
    workspaceId,
    pullRequestWatch ? input.projectId : "",
    input.azureRepositoryId ?? "",
  );
  const identity = useAzureIdentity(pullRequestWatch ? workspaceId : undefined);
  const set: Set = (key, value) => setInput((previous) => ({ ...previous, [key]: value }));
  const kindNoun = t(input.kind === "work_item" ? "workItemKindNoun" : "pullRequestKindNoun");
  const title = editing ? t("editWatchTitle", { kind: kindNoun }) : t("newWatchTitle", { kind: kindNoun });
  const projects = useMemo(() => choices.projects, [choices.projects]);

  const submit = async () => {
    if (missingWatchFields(input).length > 0) {
      setError(t(input.kind === "work_item" ? "workItemWatchFieldsRequired" : "pullRequestWatchFieldsRequired"));
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await onSubmit(input);
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSaving(false);
    }
  };

  const body = (
    <div className="min-h-0 space-y-4 overflow-y-auto p-4 pb-[env(safe-area-inset-bottom,0px)] sm:p-6" data-testid="azure-watch-editor">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <Field label={t("project")}>
        <OptionSelect value={input.projectId} options={projects} onChange={(value) => set("projectId", value)} testId={`azure-${input.kind}-watch-project`} />
      </Field>
      <QueryFields
        t={t}
        input={input}
        set={set}
        azureRepositories={choices.repositories}
        choices={{ people: people.data, me: identity.data }}
      />
      <LaunchFields t={t} input={input} set={set} options={options} />
      <PromptField t={t} input={input} set={set} />
      <ScheduleFields t={t} input={input} set={set} />
      <Button type="button" className="w-full cursor-pointer" onClick={() => void submit()} disabled={saving} data-testid="azure-watch-save">
        {saving ? t("savingEllipsis") : t(editing ? "updateWatch" : "createWatch")}
      </Button>
    </div>
  );
  const close = (
    <Button type="button" variant="ghost" size="icon" className="cursor-pointer" aria-label={t("closeWatchEditor")} data-testid="azure-watch-editor-close" onClick={onClose}>
      <IconX className="h-4 w-4" />
    </Button>
  );
  if (isMobile) {
    return (
      <Drawer open onOpenChange={(open: boolean) => !open && onClose()}>
        <DrawerContent className="h-[100dvh] max-h-[100dvh] overflow-hidden">
          <DrawerHeader className="flex items-center justify-between gap-3">
            <DrawerTitle>{title}</DrawerTitle>
            {close}
          </DrawerHeader>
          {body}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open onOpenChange={(open: boolean) => !open && onClose()}>
      <DialogContent className="flex max-h-[90dvh] flex-col overflow-hidden p-0 sm:max-w-2xl">
        <DialogHeader className="px-6 pt-6">
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        {body}
      </DialogContent>
    </Dialog>
  );
}
