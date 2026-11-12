
import type { ReactNode } from "../runtime";
import { useState } from "../runtime";
import { IconAdjustments, IconChevronDown, IconSearch } from "../icons";
import { Badge, Button } from "../host-ui";
import { Collapsible, CollapsibleContent } from "../host-ui";
import { Input } from "../host-ui";
import { Label } from "../host-ui";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../host-ui";
import { Textarea } from "../host-ui";
import { advancedFilterCount } from "./presets";
import { canSearch } from "./search-query";
import { SearchablePicker, type PickerOption } from "./searchable-picker";
import { ANY_OPTION } from "./sprints-model";
import { personPickerOptions } from "./pull-request-people";
import type {
  AzureDevOpsIdentity,
  AzureDevOpsProject,
  AzureDevOpsPullRequestPeople,
  AzureDevOpsRepository,
} from "./types";
import { cn } from "../cn";
import { useT } from "../i18n";

export type AzureDevOpsBrowseMode = "board" | "sprints" | "work-items" | "pull-requests";

export type AzureDevOpsFiltersState = {
  projectId: string;
  repositoryId: string;
  wiql: string;
  /** Work-item search box: a number or words in the title. */
  text: string;
  top: number;
  status: string;
  creator: string;
  reviewer: string;
  /** Optional work-item team filter: the team's areas. */
  teamId: string;
  /** Optional work-item sprint filter: one of the team's sprints. */
  iterationId: string;
};

export type FiltersProps = {
  idSuffix: "" | "-mobile";
  mode: AzureDevOpsBrowseMode;
  filters: AzureDevOpsFiltersState;
  projects: AzureDevOpsProject[];
  repositories: AzureDevOpsRepository[];
  loading: boolean;
  onChange: <K extends keyof AzureDevOpsFiltersState>(
    key: K,
    value: AzureDevOpsFiltersState[K],
  ) => void;
  onSearch: () => void;
  compact?: boolean;
  /** Work-item team filter options, the user's teams first. */
  teams?: PickerOption[];
  /** Work-item sprint filter options for the selected team. */
  sprints?: PickerOption[];
  /** The people the pull-request creator and reviewer filters can name. */
  people?: AzureDevOpsPullRequestPeople;
  /** The signed-in identity, named under the Me option. */
  me?: AzureDevOpsIdentity;
};

function Field({ compact, children }: { compact?: boolean; children: ReactNode }) {
  return <div className={cn("space-y-1.5", compact && "min-w-44 flex-1")}>{children}</div>;
}

function ProjectFilter({
  idSuffix,
  value,
  projects,
  onChange,
  compact,
}: {
  idSuffix: FiltersProps["idSuffix"];
  value: string;
  projects: AzureDevOpsProject[];
  onChange: (value: string) => void;
  compact?: boolean;
}) {
  const t = useT();
  const controlId = `azure-devops-filter-project${idSuffix}`;
  return (
    <Field compact={compact}>
      <Label htmlFor={controlId}>{t("project")}</Label>
      <SearchablePicker
        value={value}
        options={projects}
        onChange={onChange}
        placeholder={t("selectProject")}
        triggerId={controlId}
        className="w-full"
      />
    </Field>
  );
}

function PullRequestFilters({
  idSuffix,
  filters,
  repositories,
  onChange,
  compact,
  people,
  me,
}: Pick<FiltersProps, "idSuffix" | "filters" | "repositories" | "onChange" | "compact" | "people" | "me">) {
  const t = useT();
  const [advanced, setAdvanced] = useState(false);
  const meLabel = (name?: string) => (name ? t("meNamed", { name }) : t("me"));
  return (
    <>
      <Field compact={compact}>
        <Label htmlFor={`azure-devops-filter-repository${idSuffix}`}>
          {t("repository")}
        </Label>
        <SearchablePicker
          value={filters.repositoryId}
          options={repositories}
          onChange={(value) => onChange("repositoryId", value)}
          placeholder={t("selectRepository")}
          triggerId={`azure-devops-filter-repository${idSuffix}`}
          className="w-full"
          disabled={repositories.length === 0}
        />
      </Field>
      <Field compact={compact}>
        <Label htmlFor={`azure-devops-filter-status${idSuffix}`}>{t("status")}</Label>
        <Select value={filters.status} onValueChange={(value) => onChange("status", value)}>
          <SelectTrigger id={`azure-devops-filter-status${idSuffix}`} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="active">{t("statusActive")}</SelectItem>
            <SelectItem value="completed">{t("statusCompleted")}</SelectItem>
            <SelectItem value="abandoned">{t("statusAbandoned")}</SelectItem>
            <SelectItem value="all">{t("statusAll")}</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <AdvancedFilters
        open={advanced}
        onOpenChange={setAdvanced}
        compact={compact}
        activeCount={advancedFilterCount(filters)}
      >
        <OptionalPicker
          id={`azure-devops-filter-creator${idSuffix}`}
          label={t("pullRequestCreatedBy")}
          anyLabel={t("anyone")}
          value={filters.creator}
          options={personPickerOptions(people?.creators ?? [], meLabel, filters.creator, me)}
          onChange={(value) => onChange("creator", value)}
          compact={compact}
        />
        <OptionalPicker
          id={`azure-devops-filter-reviewer${idSuffix}`}
          label={t("reviewer")}
          anyLabel={t("anyone")}
          value={filters.reviewer}
          options={personPickerOptions(people?.reviewers ?? [], meLabel, filters.reviewer, me)}
          onChange={(value) => onChange("reviewer", value)}
          compact={compact}
        />
      </AdvancedFilters>
    </>
  );
}

function AdvancedFilters({
  open,
  onOpenChange,
  compact,
  activeCount = 0,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  compact?: boolean;
  /** Filters set under Advanced, shown on the button while it is closed. */
  activeCount?: number;
  children: ReactNode;
}) {
  const t = useT();
  return (
    <Collapsible open={open} onOpenChange={onOpenChange} className={cn(compact && "contents")}>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="cursor-pointer self-end"
        onClick={() => onOpenChange(!open)}
        aria-expanded={open}
      >
        <IconAdjustments className="h-4 w-4" />
        {t("advanced")}
        {activeCount > 0 && (
          <Badge variant="secondary" data-testid="azure-devops-advanced-count">
            {activeCount}
          </Badge>
        )}
        <IconChevronDown className={cn("h-3.5 w-3.5 transition-transform", open && "rotate-180")} />
      </Button>
      <CollapsibleContent className={cn(compact && "basis-full")}>
        <div className={cn("space-y-4 pt-3", compact && "flex flex-wrap gap-3 space-y-0")}>
          {children}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function OptionalPicker({
  id,
  label,
  anyLabel,
  value,
  options,
  onChange,
  disabled,
  compact,
}: {
  id: string;
  label: string;
  anyLabel: string;
  value: string;
  options: PickerOption[];
  onChange: (value: string) => void;
  disabled?: boolean;
  compact?: boolean;
}) {
  const withAny = [{ id: ANY_OPTION, name: anyLabel }, ...options];
  return (
    <Field compact={compact}>
      <Label htmlFor={id}>{label}</Label>
      <SearchablePicker
        value={value || ANY_OPTION}
        options={withAny}
        onChange={(next) => onChange(next === ANY_OPTION ? "" : next)}
        placeholder={anyLabel}
        triggerId={id}
        testId={id}
        className="w-full"
        disabled={disabled}
      />
    </Field>
  );
}

function WorkItemFilters({
  idSuffix,
  filters,
  onChange,
  onSearch,
  compact,
  teams = [],
  sprints = [],
}: Pick<FiltersProps, "idSuffix" | "filters" | "onChange" | "onSearch" | "compact" | "teams" | "sprints">) {
  const t = useT();
  const [advanced, setAdvanced] = useState(false);
  return (
    <>
      <OptionalPicker
        id={`azure-devops-filter-team${idSuffix}`}
        label={t("team")}
        anyLabel={t("allTeams")}
        value={filters.teamId}
        options={teams}
        onChange={(value) => onChange("teamId", value)}
        compact={compact}
      />
      <OptionalPicker
        id={`azure-devops-filter-sprint${idSuffix}`}
        label={t("sprint")}
        anyLabel={t("allSprints")}
        value={filters.iterationId}
        options={sprints}
        onChange={(value) => onChange("iterationId", value)}
        disabled={!filters.teamId}
        compact={compact}
      />
      <Field compact={compact}>
        <Label htmlFor={`azure-devops-filter-text${idSuffix}`}>{t("commonSearch")}</Label>
        <Input
          id={`azure-devops-filter-text${idSuffix}`}
          data-testid={`azure-devops-work-item-search${idSuffix}`}
          value={filters.text}
          placeholder={t("workItemSearchPlaceholder")}
          onChange={(event) => onChange("text", event.target.value)}
          onKeyDown={(event) => {
            if (event.key !== "Enter") return;
            event.preventDefault();
            if (canSearch("work-items", filters)) onSearch();
          }}
          className="w-full"
        />
      </Field>
      <Field compact={compact}>
        <Label htmlFor={`azure-devops-filter-limit${idSuffix}`}>
          {t("resultLimit")}
        </Label>
        <Select
          value={String(filters.top)}
          onValueChange={(value) => onChange("top", Number(value))}
        >
          <SelectTrigger id={`azure-devops-filter-limit${idSuffix}`} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {[25, 50, 100, 200].map((value) => (
              <SelectItem key={value} value={String(value)}>
                {value}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <AdvancedFilters open={advanced} onOpenChange={setAdvanced} compact={compact}>
        <div className="min-w-0 flex-1 space-y-1.5">
          <Label htmlFor={`azure-devops-filter-wiql${idSuffix}`}>WIQL</Label>
          <Textarea
            id={`azure-devops-filter-wiql${idSuffix}`}
            value={filters.wiql}
            onChange={(event) => onChange("wiql", event.target.value)}
            className="min-h-24 resize-y font-mono text-xs"
          />
        </div>
      </AdvancedFilters>
    </>
  );
}

export function AzureDevOpsFilters({
  idSuffix,
  mode,
  filters,
  projects,
  repositories,
  loading,
  onChange,
  onSearch,
  compact,
  teams,
  sprints,
  people,
  me,
}: FiltersProps) {
  const t = useT();
  const disabled = loading || !canSearch(mode, filters);
  return (
    <div
      className={cn("space-y-4", compact && "flex flex-wrap items-end gap-3 space-y-0")}
      data-testid={`azure-devops-filters${idSuffix}`}
    >
      <ProjectFilter
        idSuffix={idSuffix}
        value={filters.projectId}
        projects={projects}
        onChange={(value) => onChange("projectId", value)}
        compact={compact}
      />
      {mode === "work-items" ? (
        <WorkItemFilters
          idSuffix={idSuffix}
          filters={filters}
          onChange={onChange}
          onSearch={onSearch}
          compact={compact}
          teams={teams}
          sprints={sprints}
        />
      ) : (
        <PullRequestFilters
          idSuffix={idSuffix}
          filters={filters}
          repositories={repositories}
          onChange={onChange}
          compact={compact}
          people={people}
          me={me}
        />
      )}
      <Button
        type="button"
        onClick={onSearch}
        disabled={disabled}
        className={cn("w-full cursor-pointer", compact && "w-auto self-end")}
        data-testid={`azure-devops-search-button${idSuffix}`}
      >
        <IconSearch className="h-4 w-4" />
        {loading ? t("loading") : t("commonSearch")}
      </Button>
    </div>
  );
}
