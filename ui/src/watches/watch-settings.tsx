import { currentHost, useCallback, useEffect, useState } from "../runtime";
import { useT, type TFunction } from "../i18n";
import { IconPlus, IconRefresh, IconTrash } from "../icons";
import {
  Alert,
  AlertDescription,
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  SettingsSection,
} from "../host-ui";
import {
  createWatchesApi,
  emptyWatchInput,
  inputFromWatch,
  optionName,
  type Watch,
  type WatchInput,
  type WatchKind,
  type WatchOptions,
} from "./api";
import { WatchEditor } from "./watch-editor";

const EMPTY_OPTIONS: WatchOptions = { workflows: [], agentProfiles: [], executorProfiles: [], repositories: [] };

type Editor = { kind: WatchKind; watch?: Watch };
type Confirm = { action: "reset" | "delete"; watch: Watch; count?: number };

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

/** Localized reason a watch was disabled, naming the missing profile (UI-03). */
export function disabledReasonText(t: TFunction, watch: Watch, options: WatchOptions): string | null {
  switch (watch.disabledReason) {
    case "agent_profile_unavailable":
      return t("watchDisabledAgentProfile", { name: optionName(options.agentProfiles, watch.agentProfileId) });
    case "executor_profile_unavailable":
      return t("watchDisabledExecutorProfile", { name: optionName(options.executorProfiles, watch.executorProfileId) });
    default:
      return null;
  }
}

function lastChecked(t: TFunction, value?: string): string {
  if (!value) return t("notCheckedYet");
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? t("notCheckedYet") : t("checkedAt", { at: date.toLocaleString() });
}

function WatchCard({
  t,
  watch,
  options,
  onEdit,
  onToggle,
  onRun,
  onReset,
  onDelete,
}: {
  t: TFunction;
  watch: Watch;
  options: WatchOptions;
  onEdit: () => void;
  onToggle: () => void;
  onRun: () => void;
  onReset: () => void;
  onDelete: () => void;
}) {
  const reason = disabledReasonText(t, watch, options);
  const testKind = watch.kind === "work_item" ? "work-item" : "pull-request";
  return (
    <Card data-testid={`azure-${testKind}-watch-${watch.id}`}>
      <CardHeader className="space-y-2">
        <CardTitle className="flex flex-wrap items-center justify-between gap-2 text-sm">
          <span>{t(watch.kind === "work_item" ? "workItemQuery" : "pullRequestFilter")}</span>
          <Badge variant={watch.enabled ? "default" : "secondary"}>{t(watch.enabled ? "enabled" : "disabled")}</Badge>
        </CardTitle>
        <div className="text-xs text-muted-foreground">
          {t("watchSchedule", { projectId: watch.projectId, count: watch.pollIntervalSeconds })}
          {" · "}
          {optionName(options.workflows, watch.workflowId)}
        </div>
        <div className="text-xs text-muted-foreground">{lastChecked(t, watch.lastPolledAt)}</div>
      </CardHeader>
      <CardContent className="space-y-3">
        {watch.kind === "work_item" ? (
          <pre className="max-h-20 overflow-auto whitespace-pre-wrap break-words rounded bg-muted/40 p-2 text-xs">{watch.wiql}</pre>
        ) : (
          <div className="text-xs text-muted-foreground">{t("statusLine", { status: watch.status || t("pullRequestAny") })}</div>
        )}
        {reason && (
          <Alert variant="destructive" data-testid={`azure-watch-disabled-reason-${watch.id}`}>
            <AlertDescription>{reason}</AlertDescription>
          </Alert>
        )}
        {!reason && watch.lastError && (
          <Alert variant="destructive">
            <AlertDescription>{watch.lastError}</AlertDescription>
          </Alert>
        )}
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" className="cursor-pointer" onClick={onEdit} data-testid={`azure-watch-edit-${watch.id}`}>
            {t("edit")}
          </Button>
          <Button type="button" variant="outline" className="cursor-pointer" onClick={onToggle} data-testid={`azure-watch-toggle-${watch.id}`}>
            {t(watch.enabled ? "disable" : "enable")}
          </Button>
          <Button type="button" variant="outline" className="cursor-pointer" onClick={onRun} data-testid={`azure-watch-run-${watch.id}`}>
            {t("runNow")}
          </Button>
          <Button type="button" variant="outline" className="cursor-pointer" onClick={onReset} data-testid={`azure-watch-reset-${watch.id}`}>
            <IconRefresh className="h-4 w-4" /> {t("commonReset")}
          </Button>
          <Button type="button" variant="outline" className="cursor-pointer" onClick={onDelete} data-testid={`azure-watch-delete-${watch.id}`}>
            <IconTrash className="h-4 w-4" /> {t("delete")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function ConfirmDialog({ t, confirm, onClose, onConfirm }: { t: TFunction; confirm: Confirm; onClose: () => void; onConfirm: () => void }) {
  const reset = confirm.action === "reset";
  return (
    <Dialog open onOpenChange={(open: boolean) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t(reset ? "resetWatchTitle" : "deleteWatchConfirm")}</DialogTitle>
          <DialogDescription>
            {reset ? t("resetWatchDescription", { count: confirm.count ?? 0 }) : t("deleteWatchDescription")}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" className="cursor-pointer" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button type="button" variant={reset ? "default" : "destructive"} className="cursor-pointer" onClick={onConfirm} data-testid="azure-watch-confirm">
            {t(reset ? "commonReset" : "delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function useWatches(workspaceId: string) {
  const api = createWatchesApi(currentHost(), workspaceId);
  const [watches, setWatches] = useState<Watch[]>([]);
  const [options, setOptions] = useState<WatchOptions>(EMPTY_OPTIONS);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const reload = useCallback(async () => {
    try {
      setWatches(await api.list());
      setError(null);
    } catch (cause) {
      setError(errorText(cause));
    } finally {
      setLoading(false);
    }
  }, [workspaceId]);
  useEffect(() => {
    void reload();
    api.options().then(setOptions, (cause: unknown) => setError(errorText(cause)));
  }, [workspaceId]);
  return { api, watches, options, loading, error, reload };
}

/** Work-item and pull-request watches of one workspace. */
export function AzureDevOpsWatchSettings({ workspaceId, defaultProject }: { workspaceId: string; defaultProject?: string }) {
  const t = useT();
  const { api, watches, options, loading, error, reload } = useWatches(workspaceId);
  const [editor, setEditor] = useState<Editor | null>(null);
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const act = async (operation: () => Promise<string>) => {
    try {
      setMessage(await operation());
    } catch (cause) {
      setMessage(errorText(cause));
    }
    await reload();
  };
  const run = (watch: Watch) =>
    act(async () => {
      const result = await api.trigger(watch.id);
      return result.error ? t("watchRunFailed", { error: result.error }) : t("newMatches", { count: result.matched ?? 0 });
    });
  const toggle = (watch: Watch) =>
    act(async () => {
      await api.update(watch.id, { enabled: !watch.enabled });
      return t(watch.enabled ? "watchDisabled" : "watchEnabled");
    });
  const askReset = async (watch: Watch) => {
    try {
      setConfirm({ action: "reset", watch, count: await api.previewReset(watch.id) });
    } catch (cause) {
      setMessage(errorText(cause));
    }
  };
  const confirmed = (current: Confirm) => {
    setConfirm(null);
    void act(async () => {
      if (current.action === "reset") {
        await api.reset(current.watch.id);
        return t("watchReset");
      }
      await api.remove(current.watch.id);
      return t("watchDeleted");
    });
  };
  const submit = async (input: WatchInput) => {
    if (editor?.watch) await api.update(editor.watch.id, input);
    else await api.create(input);
    await reload();
  };

  const section = (kind: WatchKind) => {
    const list = watches.filter((watch) => watch.kind === kind);
    const isWorkItem = kind === "work_item";
    return (
      <SettingsSection
        title={t(isWorkItem ? "workItemWatches" : "pullRequestWatches")}
        description={t(isWorkItem ? "workItemWatchesDescription" : "pullRequestWatchesDescription")}
        action={
          <Button type="button" className="w-full cursor-pointer sm:w-auto" onClick={() => setEditor({ kind })} data-testid={`azure-add-${isWorkItem ? "work-item" : "pull-request"}-watch`}>
            <IconPlus className="h-4 w-4" /> {t(isWorkItem ? "addWorkItemWatch" : "addPrWatch")}
          </Button>
        }
      >
        {!loading && list.length === 0 && (
          <Card>
            <CardContent className="p-6 text-sm text-muted-foreground">{t(isWorkItem ? "noWorkItemWatches" : "noPullRequestWatches")}</CardContent>
          </Card>
        )}
        <div className="grid gap-4 xl:grid-cols-2">
          {list.map((watch) => (
            <WatchCard
              key={watch.id}
              t={t}
              watch={watch}
              options={options}
              onEdit={() => setEditor({ kind, watch })}
              onToggle={() => void toggle(watch)}
              onRun={() => void run(watch)}
              onReset={() => void askReset(watch)}
              onDelete={() => setConfirm({ action: "delete", watch })}
            />
          ))}
        </div>
      </SettingsSection>
    );
  };

  return (
    <div className="space-y-6" data-testid="azure-devops-watch-settings">
      {message && (
        <Alert role="status">
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      )}
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {loading && <p className="text-sm text-muted-foreground">{t("loadingWatchers")}</p>}
      {section("pull_request")}
      {section("work_item")}
      {editor && (
        <WatchEditor
          key={`${editor.kind}:${editor.watch?.id ?? "new"}`}
          workspaceId={workspaceId}
          initial={editor.watch ? inputFromWatch(editor.watch) : emptyWatchInput(editor.kind, defaultProject)}
          editing={Boolean(editor.watch)}
          options={options}
          onClose={() => setEditor(null)}
          onSubmit={submit}
        />
      )}
      {confirm && <ConfirmDialog t={t} confirm={confirm} onClose={() => setConfirm(null)} onConfirm={() => confirmed(confirm)} />}
    </div>
  );
}
