import { useMemo } from "../runtime";
import { Combobox } from "../host-ui";
import { useT } from "../i18n";
import { pickedValue, pickerFilter } from "./search-query";

export type PickerOption = { id: string; name: string; description?: string };

/**
 * Kandev's searchable picker, for Azure DevOps lists that can hold hundreds of
 * projects, teams, boards, or repositories.
 */
export function SearchablePicker({
  value,
  options,
  onChange,
  placeholder,
  testId,
  triggerId,
  className,
  disabled,
}: {
  value: string;
  options: PickerOption[];
  onChange: (value: string) => void;
  placeholder: string;
  testId?: string;
  triggerId?: string;
  className?: string;
  disabled?: boolean;
}) {
  const t = useT();
  const items = useMemo(
    () =>
      options.map((option) => ({
        value: option.id,
        label: option.name,
        ...(option.description
          ? {
              description: option.description,
              renderLabel: () => (
                <span className="flex min-w-0 flex-col">
                  <span className="truncate">{option.name}</span>
                  <span className="truncate text-xs text-muted-foreground">{option.description}</span>
                </span>
              ),
              renderTriggerLabel: () => <span className="truncate">{option.name}</span>,
            }
          : {}),
      })),
    [options],
  );
  return (
    <Combobox
      options={items}
      value={value}
      onValueChange={(next: string) => {
        const picked = pickedValue(value, next);
        if (picked !== value) onChange(picked);
      }}
      placeholder={placeholder}
      searchPlaceholder={t("commonSearch")}
      emptyMessage={t("noMatchingResults")}
      testId={testId}
      triggerId={triggerId}
      triggerClassName={className}
      disabled={disabled}
      filter={pickerFilter}
      showSearch
    />
  );
}
