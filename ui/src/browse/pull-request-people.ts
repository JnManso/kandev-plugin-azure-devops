import type { PickerOption } from "./searchable-picker";
import type { AzureDevOpsIdentity } from "./types";

/** The creator and reviewer filter value the server resolves to the signed-in identity. */
export const CURRENT_USER = "@me";

/**
 * Person filter options: the signed-in user first, labelled with their name
 * and email as the pull requests show them (else as the identity does), then
 * each other person by name. A selected value the list does not name (a saved
 * filter) stays selectable, and so does my own entry while selected.
 */
export function personPickerOptions(
  people: AzureDevOpsIdentity[],
  meLabel: (name?: string) => string,
  selected: string,
  me?: AzureDevOpsIdentity,
): PickerOption[] {
  const same = (left: string, right: string | undefined) => left.toLowerCase() === (right ?? "").toLowerCase();
  const mine = me ? (people.find((person) => same(person.id, me.id)) ?? me) : undefined;
  const others = people.filter((person) => !me || !same(person.id, me.id) || same(person.id, selected));
  const options: PickerOption[] = [
    mine
      ? { id: CURRENT_USER, name: meLabel(mine.displayName), description: mine.uniqueName ?? me?.uniqueName }
      : { id: CURRENT_USER, name: meLabel() },
    ...others.map((person) => ({ id: person.id, name: person.displayName, description: person.uniqueName })),
  ];
  if (selected && !options.some((option) => same(option.id, selected))) {
    options.push({ id: selected, name: selected });
  }
  return options;
}
