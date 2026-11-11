
import { Badge } from "../host-ui";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "../host-ui";
import { Separator } from "../host-ui";
import type { AzureDevOpsPullRequestFeedback } from "./types";
import { useT } from "../i18n";

/** `vote` is Azure DevOps' numeric vote; only the catalog key is copy. */
function voteLabelKey(vote: number): string {
  if (vote >= 10) return "voteApproved";
  if (vote >= 5) return "voteApprovedWithSuggestions";
  if (vote <= -10) return "voteRejected";
  if (vote <= -5) return "voteWaitingForAuthor";
  return "voteNone";
}

function Summary({ feedback }: { feedback: AzureDevOpsPullRequestFeedback }) {
  const t = useT();
  return (
    <div className="flex flex-wrap gap-2">
      <Badge variant="outline">
        {t("reviewLabelled", {
          state: feedback.reviewState || t("pending"),
        })}
      </Badge>
      <Badge variant="outline">
        {t("policiesLabelled", {
          state: feedback.policyState || t("none"),
        })}
      </Badge>
      <Badge variant="secondary">
        {t("linkedItemsCount", { count: feedback.linkedWorkItems.length })}
      </Badge>
    </div>
  );
}

function Reviewers({ feedback }: { feedback: AzureDevOpsPullRequestFeedback }) {
  const t = useT();
  return (
    <section className="space-y-2">
      <h3 className="text-sm font-semibold">{t("reviewers")}</h3>
      {feedback.reviewers.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("noReviewers")}</p>
      ) : (
        <div className="space-y-2">
          {feedback.reviewers.map((reviewer) => (
            <div key={reviewer.id} className="flex items-center justify-between gap-3 text-sm">
              <span className="min-w-0 truncate">{reviewer.displayName}</span>
              <Badge variant="outline" className="shrink-0">
                {t(voteLabelKey(reviewer.vote))}
              </Badge>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function Policies({ feedback }: { feedback: AzureDevOpsPullRequestFeedback }) {
  const t = useT();
  return (
    <section className="space-y-2">
      <h3 className="text-sm font-semibold">{t("branchPolicies")}</h3>
      {feedback.policies.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("noPolicyEvaluations")}</p>
      ) : (
        <div className="space-y-2">
          {feedback.policies.map((policy) => (
            <div key={policy.id} className="flex items-center justify-between gap-3 text-sm">
              <span className="min-w-0 break-words">{policy.name}</span>
              <Badge variant="outline" className="shrink-0">
                {policy.status}
              </Badge>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function Threads({ feedback }: { feedback: AzureDevOpsPullRequestFeedback }) {
  const t = useT();
  const comments = feedback.threads.flatMap((thread) =>
    thread.comments.map((comment) => ({ ...comment, threadId: thread.id })),
  );
  return (
    <section className="space-y-2">
      <h3 className="text-sm font-semibold">{t("discussion")}</h3>
      {comments.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("noComments")}</p>
      ) : (
        <div className="space-y-3">
          {comments.map((comment) => (
            <div key={`${comment.threadId}:${comment.id}`} className="space-y-1 border-l-2 pl-3">
              <div className="text-xs font-medium">{comment.author.displayName}</div>
              <p className="whitespace-pre-wrap break-words text-sm text-muted-foreground">
                {comment.content}
              </p>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

export function AzureDevOpsFeedbackDialog({
  open,
  loading,
  error,
  feedback,
  onOpenChange,
}: {
  open: boolean;
  loading: boolean;
  error: string | null;
  feedback: AzureDevOpsPullRequestFeedback | null;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useT();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85dvh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="break-words">
            {feedback?.pullRequest.title ?? t("pullRequestFeedback")}
          </DialogTitle>
          <DialogDescription>
            {feedback
              ? // The repository name and id are provider data; the `PR` label
                // and the separator around them are not, so this is a message
                // with the data interpolated rather than a bare template.
                t("pullRequestSubtitle", {
                  repository: feedback.pullRequest.repositoryName,
                  id: feedback.pullRequest.id,
                })
              : t("azureDevopsReviewAndPolicyState")}
          </DialogDescription>
        </DialogHeader>
        {loading && (
          <p className="text-sm text-muted-foreground">{t("loadingFeedback")}</p>
        )}
        {error && (
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
        )}
        {feedback && (
          <div className="space-y-4" data-testid="azure-devops-feedback-detail">
            <Summary feedback={feedback} />
            <Separator />
            <Reviewers feedback={feedback} />
            <Separator />
            <Policies feedback={feedback} />
            <Separator />
            <Threads feedback={feedback} />
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
