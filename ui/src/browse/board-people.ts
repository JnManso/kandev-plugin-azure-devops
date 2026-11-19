import { ANY_OPTION, UNASSIGNED_PERSON, isAssignedTo, knownPerson } from "./sprints-model";
import type { AzureDevOpsChildCard } from "./types";

type Assignable = { id: number; assignedTo?: string };

/** Case-folded name order, then exact order, as the server sorts sprint people. */
function compareNames(left: string, right: string): number {
  const a = left.toLowerCase();
  const b = right.toLowerCase();
  if (a !== b) return a < b ? -1 : 1;
  if (left === right) return 0;
  return left < right ? -1 : 1;
}

/** Every assignee of a card or of a card's direct child, once, by name. */
export function boardPeople(cards: Assignable[], byParent: Map<number, AzureDevOpsChildCard[]>): string[] {
  const people = new Set<string>();
  const add = (name: string | undefined) => {
    const trimmed = name?.trim();
    if (trimmed) people.add(trimmed);
  };
  for (const card of cards) {
    add(card.assignedTo);
    for (const child of byParent.get(card.id) ?? []) add(child.assignedTo);
  }
  return [...people].sort(compareNames);
}

/**
 * The cards assigned to the person or with a direct child assigned to them.
 * Grandchildren never count. ANY_OPTION keeps every card.
 */
export function filterCardsByPerson<T extends Assignable>(
  cards: T[],
  byParent: Map<number, AzureDevOpsChildCard[]>,
  person: string,
): T[] {
  if (!person || person === ANY_OPTION) return cards;
  return cards.filter(
    (card) =>
      isAssignedTo(card, person) || (byParent.get(card.id) ?? []).some((child) => isAssignedTo(child, person)),
  );
}

/**
 * The person the Board filters by and the people its selector lists. Until
 * every card's children are read in full, a remembered person stays selected
 * (and listed), since they may be assigned only to children not read; after
 * that, a person the board does not name shows all people. The remembered
 * value is kept.
 */
export function boardPersonFilter(
  remembered: string | undefined,
  people: string[],
  complete: boolean,
): { person: string; people: string[] } {
  if (complete) return { person: knownPerson(remembered, people), people };
  const person = remembered || ANY_OPTION;
  const pending = person !== ANY_OPTION && person !== UNASSIGNED_PERSON && !people.includes(person);
  return { person, people: pending ? [...people, person].sort(compareNames) : people };
}
