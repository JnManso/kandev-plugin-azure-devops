import { useMemo } from "../runtime";
import { useT } from "../i18n";
import type { PickerOption } from "./searchable-picker";
import { iterationTimeFrameKey, sprintDateRange } from "./sprints-model";
import type { AzureDevOpsIteration } from "./types";

/** Sprint picker options, described by their past/current/future tag and dates. */
export function useSprintOptions(iterations: AzureDevOpsIteration[]): PickerOption[] {
  const t = useT();
  return useMemo(
    () =>
      iterations.map((iteration) => {
        const tagKey = iterationTimeFrameKey(iteration.timeFrame);
        const parts = [tagKey ? t(tagKey) : "", sprintDateRange(iteration.start, iteration.finish)].filter(Boolean);
        return { id: iteration.id, name: iteration.name, ...(parts.length ? { description: parts.join(" · ") } : {}) };
      }),
    [iterations, t],
  );
}
