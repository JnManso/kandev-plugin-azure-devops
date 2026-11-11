/** Wire types of the plugin's browse actions, from the built-in integration. */

export type AzureDevOpsProject = {
  id: string;
  name: string;
  url: string;
};

export type AzureDevOpsRepository = {
  id: string;
  name: string;
  projectId: string;
  projectName: string;
  defaultBranch: string;
  webUrl: string;
};

export type AzureDevOpsSavedView = {
  id: string;
  kind: "work_item" | "pull_request";
  label: string;
  projectId: string;
  repositoryId?: string;
  wiql?: string;
  top?: number;
  status?: string;
  creator?: string;
  reviewer?: string;
  teamId?: string;
  iterationId?: string;
  createdAt: string;
};

export type AzureDevOpsIdentity = {
  id: string;
  displayName: string;
  uniqueName?: string;
};

/** The creators and reviewers of a repository's recent pull requests. */
export type AzureDevOpsPullRequestPeople = { creators: AzureDevOpsIdentity[]; reviewers: AzureDevOpsIdentity[] };

export type AzureDevOpsWorkItem = {
  id: number;
  revision: number;
  title: string;
  description?: string;
  state: string;
  type: string;
  project?: string;
  areaPath?: string;
  assignedTo?: string;
  tags?: string[];
  webUrl?: string;
  apiUrl?: string;
  fields?: Record<string, unknown>;
};

export type AzureDevOpsPlanningField = {
  referenceName: string;
  label: string;
  value: string;
};

export type AzureDevOpsWorkItemDetail = AzureDevOpsWorkItem & {
  planningFields: AzureDevOpsPlanningField[];
};

export type AzureDevOpsWorkItemComment = {
  id: number;
  content: string;
  author: AzureDevOpsIdentity;
  publishedAt?: string;
  updatedAt?: string;
};

export type AzureDevOpsWorkItemCommentPage = {
  comments: AzureDevOpsWorkItemComment[];
  continuationToken?: string;
};

export type AzureDevOpsWorkItemAssignmentUpdate = {
  revision: number;
  assigneeAction: "assign_current_user" | "unassign";
};

export type AzureDevOpsTeam = {
  id: string;
  name: string;
  projectId: string;
  projectName?: string;
};

export type AzureDevOpsBoardReference = { id: string; name: string };

export type AzureDevOpsTeamList = { teams: AzureDevOpsTeam[]; mineIds: string[] };

export type AzureDevOpsIteration = {
  id: string;
  name: string;
  path: string;
  start?: string;
  finish?: string;
  timeFrame?: string;
};

export type AzureDevOpsTeamSettings = {
  areaField: string;
  areas: Array<{ path: string; includeChildren: boolean }>;
  iterations: AzureDevOpsIteration[];
};

export type AzureDevOpsTaskboardColumn = { id: string; name: string };

/** A child work item of a board card, with its state's category. */
export type AzureDevOpsChildCard = AzureDevOpsWorkItem & { stateCategory?: string };

export type AzureDevOpsCardChildren = { parentId: number; items: AzureDevOpsChildCard[] };

export type AzureDevOpsBoardChildren = { children: AzureDevOpsCardChildren[]; truncated?: boolean };

export type AzureDevOpsSprintCard = AzureDevOpsWorkItem & { columnId: string };

export type AzureDevOpsSprintRow = { parent?: AzureDevOpsWorkItem; tasks: AzureDevOpsSprintCard[] };

export type AzureDevOpsSprintBoard = {
  iteration: AzureDevOpsIteration;
  workingDaysLeft: number;
  columns: AzureDevOpsTaskboardColumn[];
  rows: AzureDevOpsSprintRow[];
  people: string[];
  truncated?: boolean;
};

export type AzureDevOpsBoardColumn = {
  id: string;
  name: string;
  columnType?: string;
  description?: string;
  isSplit?: boolean;
  itemLimit?: number;
  stateMappings?: Record<string, string>;
};

export type AzureDevOpsBoard = {
  id: string;
  name: string;
  columns: AzureDevOpsBoardColumn[];
  fields: {
    columnField: { referenceName: string };
    doneField: { referenceName: string };
    rowField: { referenceName: string };
  };
  rows?: Array<{ id: string; name: string; color?: string }>;
};

export type AzureDevOpsBoardWorkItem = AzureDevOpsWorkItem & {
  columnId: string;
  columnDone: boolean;
};

export type AzureDevOpsBoardSnapshot = {
  board: AzureDevOpsBoard;
  items: AzureDevOpsBoardWorkItem[];
};

export type AzureDevOpsBoardWorkItemUpdate = {
  revision: number;
  assigneeAction?: "assign_current_user" | "unassign";
  columnId?: string;
  columnDone?: boolean;
};

export type AzureDevOpsWorkItemSearchResult = {
  items: AzureDevOpsWorkItem[];
  count: number;
};

export type AzureDevOpsPullRequest = {
  id: number;
  title: string;
  description?: string;
  status: string;
  isDraft: boolean;
  sourceBranch: string;
  targetBranch: string;
  mergeStatus?: string;
  creationDate?: string;
  closedDate?: string;
  author: AzureDevOpsIdentity;
  projectId: string;
  projectName: string;
  repositoryId: string;
  repositoryName: string;
  webUrl: string;
  apiUrl: string;
};

export type AzureDevOpsPullRequestPage = {
  items: AzureDevOpsPullRequest[];
  count: number;
  skip: number;
  top: number;
};

export type AzureDevOpsReviewer = AzureDevOpsIdentity & {
  vote: number;
  isRequired: boolean;
  hasDeclined: boolean;
};

export type AzureDevOpsComment = {
  id: number;
  content: string;
  author: AzureDevOpsIdentity;
  commentType: string;
  publishedAt?: string;
  updatedAt?: string;
};

export type AzureDevOpsThread = {
  id: number;
  status: string;
  comments: AzureDevOpsComment[];
};

export type AzureDevOpsWorkItemRef = { id: number; url: string };

export type AzureDevOpsPolicyEvaluation = {
  id: string;
  status: string;
  name: string;
  isBlocking: boolean;
};

export type AzureDevOpsPullRequestFeedback = {
  pullRequest: AzureDevOpsPullRequest;
  reviewers: AzureDevOpsReviewer[];
  threads: AzureDevOpsThread[];
  linkedWorkItems: AzureDevOpsWorkItemRef[];
  policies: AzureDevOpsPolicyEvaluation[];
  reviewState: "approved" | "rejected" | "waiting" | "";
  policyState: "success" | "pending" | "failure" | "";
};

export type AzureDevOpsQueryPreset = {
  id: string;
  label: string;
  group: string;
  filters: Record<string, unknown>;
};

export type AzureDevOpsActionPreset = {
  id: string;
  label: string;
  hint: string;
  icon: string;
  promptTemplate: string;
};

export type AzureDevOpsWorkspaceSettings = {
  workItemQueries: AzureDevOpsQueryPreset[];
  pullRequestQueries: AzureDevOpsQueryPreset[];
  workItemActions: AzureDevOpsActionPreset[];
  pullRequestActions: AzureDevOpsActionPreset[];
};

export type AzureDevOpsPullRequestListPage = {
  items: AzureDevOpsPullRequest[];
  nextCursor?: string;
};

/** The connection as the browse page needs it. */
export type AzureDevOpsConfig = {
  workspaceId: string;
  organizationUrl: string;
  defaultProjectId?: string;
  defaultProjectName?: string;
  /** True when the workspace has a usable credential. */
  hasSecret: boolean;
};

/** A Kandev task linked to an Azure Boards work item. */
export type AzureDevOpsTaskWorkItem = {
  id: string;
  taskId: string;
  workspaceId: string;
  projectId: string;
  workItemId: number;
  workItemUrl: string;
  title: string;
  state: string;
  type: string;
  createdAt: string;
  updatedAt: string;
};
