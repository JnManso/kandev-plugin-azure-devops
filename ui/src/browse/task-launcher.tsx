import type { TaskCreationContext } from "@kdlbs/plugin-sdk";
import { currentHost, useEffect, useMemo, useState } from "../runtime";
import { TaskCreateDialog } from "../host-ui";
import { useT } from "../i18n";
import { createTaskLinksApi } from "../tasks/api";
import {
  hasLaunchWorkflow,
  launchRepository,
  launchText,
  organizationFromUrl,
  type AzureDevOpsLaunchPayload,
} from "./task-launch";

function useTaskCreationContext(workspaceId?: string): TaskCreationContext | null {
  const host = currentHost();
  const [context, setContext] = useState<TaskCreationContext | null>(() =>
    workspaceId ? host.context.getTaskCreationContext(workspaceId) : null,
  );
  useEffect(() => {
    if (!workspaceId) {
      setContext(null);
      return undefined;
    }
    setContext(host.context.getTaskCreationContext(workspaceId));
    return host.context.subscribeTaskCreationContext(workspaceId, setContext);
  }, [workspaceId]);
  return context;
}

type CreatedTask = { id?: unknown };

/** Links a task created from a work item or pull request to that item. */
export async function linkLaunchedTask(workspaceId: string, taskId: string, payload: AzureDevOpsLaunchPayload) {
  const api = createTaskLinksApi(currentHost());
  if (payload.kind === "work-item") {
    await api.linkWorkItem(
      { workspaceId, taskId },
      { workItemId: payload.item.id, ...(payload.item.project ? { projectId: payload.item.project } : {}) },
    );
    return;
  }
  const pr = payload.pullRequest;
  await api.linkPullRequest(
    { workspaceId, taskId },
    { projectId: pr.projectId, repositoryId: pr.repositoryId, pullRequestId: pr.id },
  );
}

/** Kandev's native task dialog, prefilled from an Azure DevOps item. */
export function AzureDevOpsTaskLauncher({
  workspaceId,
  organizationUrl,
  payload,
  onClose,
}: {
  workspaceId?: string;
  organizationUrl: string;
  payload: AzureDevOpsLaunchPayload | null;
  onClose: () => void;
}) {
  const t = useT();
  const host = currentHost();
  const context = useTaskCreationContext(workspaceId);
  const initialValues = useMemo(() => {
    if (!payload || !workspaceId) return null;
    const organization = organizationFromUrl(organizationUrl);
    const repository = launchRepository(
      payload,
      (providerScope, providerRepositoryId) =>
        host.context.resolveRepositoryId({ workspaceId, providerId: "azure_devops", providerScope, providerRepositoryId }),
      organization,
    );
    return { ...launchText(payload, t("descriptionFallback")), ...repository };
  }, [payload, workspaceId, organizationUrl]);

  if (!workspaceId || !payload || !initialValues || !hasLaunchWorkflow(context)) return null;

  const onSuccess = async (task: CreatedTask, _mode?: unknown, meta?: { autoFocus?: boolean }) => {
    const taskId = typeof task?.id === "string" ? task.id : "";
    if (taskId) {
      await linkLaunchedTask(workspaceId, taskId, payload).catch((error: unknown) => {
        host.toast.error(t(payload.kind === "work-item" ? "failedToLinkWorkItem" : "failedToLinkPullRequest"), {
          description: error instanceof Error ? error.message : undefined,
        });
      });
    }
    onClose();
    if (taskId && meta?.autoFocus !== false) host.navigate(`/t/${encodeURIComponent(taskId)}`);
  };

  return (
    <TaskCreateDialog
      open
      onOpenChange={(open: boolean) => !open && onClose()}
      mode="create"
      workspaceId={workspaceId}
      workflowId={context.workflowId}
      defaultStepId={context.defaultStepId}
      steps={context.steps}
      initialValues={initialValues}
      onSuccess={onSuccess}
    />
  );
}
