import type {
  AzureDevOpsIteration,
  AzureDevOpsSprintRow,
  AzureDevOpsTaskboardColumn,
  AzureDevOpsTeam,
  AzureDevOpsTeamSettings,
  AzureDevOpsWorkItem,
} from "./types";

/** Picker value for "any team" or "any sprint" (the host combobox needs one). */
export const ANY_OPTION = "__any__";

/** Person picker value for tasks without an assignee. */
export const UNASSIGNED_PERSON = "__unassigned__";

export type TeamPickerOption = { id: string; name: string; description?: string };

/** The project's teams with the connected identity's teams first, marked. */
export function teamPickerOptions(
  teams: AzureDevOpsTeam[],
  mineIds: string[],
  myTeamsLabel: string,
): TeamPickerOption[] {
  const mine = new Set(mineIds);
  const own = teams.filter((team) => mine.has(team.id));
  const others = teams.filter((team) => !mine.has(team.id));
  return [
    ...own.map((team) => ({ id: team.id, name: team.name, description: myTeamsLabel })),
    ...others.map((team) => ({ id: team.id, name: team.name })),
  ];
}

const quote = (value: string) => `'${value.replaceAll("'", "''")}'`;

/**
 * WIQL conditions that limit work items to the team's areas (ORed, with or
 * without sub-areas as the team defines them) and to one of its sprints.
 */
export function teamScopeConditions(
  settings: AzureDevOpsTeamSettings | undefined,
  iterationId: string,
): string[] {
  if (!settings) return [];
  const conditions: string[] = [];
  const field = settings.areaField || "System.AreaPath";
  if (settings.areas.length > 0) {
    const areas = settings.areas.map(
      (area) => `[${field}] ${area.includeChildren ? "UNDER" : "="} ${quote(area.path)}`,
    );
    conditions.push(`(${areas.join(" OR ")})`);
  }
  const iteration = settings.iterations.find((candidate) => candidate.id === iterationId);
  if (iteration) conditions.push(`[System.IterationPath] = ${quote(iteration.path)}`);
  return conditions;
}

/** The query with the conditions ANDed to its WHERE clause, before ORDER BY. */
export function wiqlWithConditions(wiql: string, conditions: string[]): string {
  if (conditions.length === 0) return wiql;
  const added = conditions.join(" AND ");
  const orderAt = wiql.search(/\sORDER\s+BY\s/i);
  const head = orderAt >= 0 ? wiql.slice(0, orderAt) : wiql;
  const tail = orderAt >= 0 ? wiql.slice(orderAt) : "";
  const whereAt = head.search(/\sWHERE\s/i);
  if (whereAt < 0) return `${head} WHERE ${added}${tail}`;
  const whereEnd = whereAt + (/\sWHERE\s/i.exec(head.slice(whereAt))?.[0].length ?? 0);
  return `${head.slice(0, whereEnd)}(${head.slice(whereEnd).trim()}) AND ${added}${tail}`;
}

/**
 * The sprint to open: the remembered one while it exists, else the current
 * sprint, else the latest past sprint, else the first.
 */
export function defaultIterationId(iterations: AzureDevOpsIteration[], remembered: string | undefined): string {
  if (remembered && iterations.some((iteration) => iteration.id === remembered)) return remembered;
  const current = iterations.find((iteration) => iteration.timeFrame?.toLowerCase() === "current");
  if (current) return current.id;
  const past = iterations
    .filter((iteration) => iteration.timeFrame?.toLowerCase() === "past")
    .sort((left, right) => (left.finish ?? "").localeCompare(right.finish ?? ""));
  return past.at(-1)?.id ?? iterations[0]?.id ?? "";
}

/** Locale key of a sprint's past, current, or future tag. */
export function iterationTimeFrameKey(timeFrame: string | undefined): string | undefined {
  switch (timeFrame?.toLowerCase()) {
    case "past":
      return "sprintPast";
    case "current":
      return "sprintCurrent";
    case "future":
      return "sprintFuture";
    default:
      return undefined;
  }
}

function assignedTo(item: AzureDevOpsWorkItem, person: string): boolean {
  const assignee = item.assignedTo?.trim() ?? "";
  return person === UNASSIGNED_PERSON ? assignee === "" : assignee === person;
}

/**
 * The rows for one person: their tasks, in the rows that contain them or
 * whose backlog item is theirs. ANY_OPTION keeps every row.
 */
export function filterRowsByPerson(rows: AzureDevOpsSprintRow[], person: string): AzureDevOpsSprintRow[] {
  if (!person || person === ANY_OPTION) return rows;
  return rows.flatMap((row) => {
    const tasks = row.tasks.filter((task) => assignedTo(task, person));
    const parentMatches = row.parent ? assignedTo(row.parent, person) : false;
    return tasks.length > 0 || parentMatches ? [{ ...row, tasks }] : [];
  });
}

/** "9 Nov – 20 Nov" in the given locale, or "" without valid dates. */
export function sprintDateRange(start: string | undefined, finish: string | undefined, locale?: string): string {
  if (!start || !finish) return "";
  const from = new Date(start);
  const to = new Date(finish);
  if (Number.isNaN(from.getTime()) || Number.isNaN(to.getTime())) return "";
  const format = new Intl.DateTimeFormat(locale, { day: "numeric", month: "short", timeZone: "UTC" });
  return `${format.format(from)} – ${format.format(to)}`;
}

/** A row's tasks in one column; tasks without a known column go to the first. */
export function tasksInColumn<T extends { columnId: string }>(
  row: { tasks: T[] },
  columns: AzureDevOpsTaskboardColumn[],
  columnId: string,
): T[] {
  const known = new Set(columns.map((column) => column.id));
  const first = columns[0]?.id;
  return row.tasks.filter((task) => task.columnId === columnId || (columnId === first && !known.has(task.columnId)));
}
