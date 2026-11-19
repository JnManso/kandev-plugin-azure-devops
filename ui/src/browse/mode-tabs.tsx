import { Button } from "../host-ui";
import { useT } from "../i18n";

import type { AzureDevOpsBrowseMode } from "./filters";

type AzureDevOpsModeTabsProps = {
  mode: AzureDevOpsBrowseMode;
  onModeChange: (mode: AzureDevOpsBrowseMode) => void;
};

const modes: Array<{ mode: AzureDevOpsBrowseMode; labelKey: string; testId: string }> = [
  { mode: "board", labelKey: "board", testId: "azure-devops-board-mode" },
  { mode: "sprints", labelKey: "sprints", testId: "azure-devops-sprints-mode" },
  { mode: "work-items", labelKey: "workItems", testId: "azure-devops-work-items-mode" },
  {
    mode: "pull-requests",
    labelKey: "pullRequests",
    testId: "azure-devops-pull-requests-mode",
  },
];

export function AzureDevOpsModeTabs({ mode, onModeChange }: AzureDevOpsModeTabsProps) {
  const t = useT();
  return (
    <div className="flex flex-wrap items-center gap-1 border-b px-4 py-2">
      {modes.map((option) => (
        <Button
          key={option.mode}
          type="button"
          size="sm"
          variant={mode === option.mode ? "default" : "ghost"}
          className="cursor-pointer"
          onClick={() => onModeChange(option.mode)}
          data-testid={option.testId}
        >
          {t(option.labelKey)}
        </Button>
      ))}
    </div>
  );
}
