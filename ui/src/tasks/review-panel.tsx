import { useCallback, useEffect, useState } from "../runtime";
import { ChangeRequestDetail } from "../host-ui";
import { currentHost } from "../runtime";
import { useT } from "../i18n";
import { createBrowseApi } from "../browse/api";
import type { AzureDevOpsPullRequestFeedback } from "../browse/types";
import { toChangeRequestDetail } from "./review-model";

export type ReviewPanelProps = {
  presentation: "desktop" | "mobile";
  workspaceId: string;
  connectionScope: string;
  repositoryId: string;
  changeRequestNumber: string | number;
};

type PanelState = { feedback: AzureDevOpsPullRequestFeedback | null; loading: boolean; error: string | null };

/** Linked pull request detail on the host's native review surface. */
export function AzureDevOpsReviewPanel(props: ReviewPanelProps) {
  const t = useT();
  const [state, setState] = useState<PanelState>({ feedback: null, loading: true, error: null });
  const [generation, setGeneration] = useState(0);
  const number = Number(props.changeRequestNumber);
  const refresh = useCallback(() => setGeneration((value) => value + 1), []);

  useEffect(() => {
    const controller = new AbortController();
    setState((previous) => ({ ...previous, loading: previous.feedback === null, error: null }));
    createBrowseApi(currentHost(), props.workspaceId)
      .feedback(props.connectionScope, props.repositoryId, number, controller.signal)
      .then((feedback) => setState({ feedback, loading: false, error: null }))
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        const message = error instanceof Error ? error.message : String(error);
        setState((previous) => ({ ...previous, loading: false, error: t("requestFailed", { error: message }) }));
      });
    return () => controller.abort();
  }, [props.workspaceId, props.connectionScope, props.repositoryId, number, generation]);

  return (
    <ChangeRequestDetail
      detail={state.feedback ? toChangeRequestDetail(state.feedback, t("fallbackAuthor")) : null}
      presentation={props.presentation}
      loading={state.loading}
      error={state.error}
      onRefresh={refresh}
      onRetry={refresh}
    />
  );
}
