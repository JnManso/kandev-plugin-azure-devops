
import { useEffect, useState } from "../runtime";
import {
  DEFAULT_AZURE_PULL_REQUEST_ACTIONS,
  DEFAULT_AZURE_PULL_REQUEST_QUERIES,
  DEFAULT_AZURE_WORK_ITEM_ACTIONS,
  DEFAULT_AZURE_WORK_ITEM_QUERIES,
} from "./workspace-defaults";
import { getAzureDevOpsWorkspaceSettings } from "./rest-compat";
import type { AzureDevOpsWorkspaceSettings } from "./types";

function defaults(_workspaceId = ""): AzureDevOpsWorkspaceSettings {
  return {
    workItemQueries: DEFAULT_AZURE_WORK_ITEM_QUERIES,
    pullRequestQueries: DEFAULT_AZURE_PULL_REQUEST_QUERIES,
    workItemActions: DEFAULT_AZURE_WORK_ITEM_ACTIONS,
    pullRequestActions: DEFAULT_AZURE_PULL_REQUEST_ACTIONS,
  };
}

export function useAzureDevOpsWorkspaceSettings(workspaceId?: string) {
  const [settings, setSettings] = useState<AzureDevOpsWorkspaceSettings>(() =>
    defaults(workspaceId),
  );
  const [loading, setLoading] = useState(Boolean(workspaceId));

  useEffect(() => {
    if (!workspaceId) {
      setSettings(defaults());
      setLoading(false);
      return;
    }
    let current = true;
    setLoading(true);
    void getAzureDevOpsWorkspaceSettings(workspaceId)
      .then((next) => current && setSettings(next))
      .catch(() => current && setSettings(defaults(workspaceId)))
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [workspaceId]);

  return { settings, loading };
}
