/** The two editable lists of a workspace settings section. */
export type ListPair<T> = { workItems: T[]; pullRequests: T[] };

/**
 * The lists being edited. `resetRequested` holds only while the lists are the
 * untouched defaults from Reset: saving then clears the stored lists, and any
 * edit afterwards saves the edited lists instead.
 */
export type ListDrafts<T> = ListPair<T> & { resetRequested: boolean };

export type ListKind = keyof ListPair<unknown>;

export function listDraftsFrom<T>(lists: ListPair<T>): ListDrafts<T> {
  return { workItems: lists.workItems, pullRequests: lists.pullRequests, resetRequested: false };
}

export function resetListDrafts<T>(defaults: ListPair<T>): ListDrafts<T> {
  return { ...listDraftsFrom(defaults), resetRequested: true };
}

export function editListDrafts<T>(drafts: ListDrafts<T>, kind: ListKind, values: T[]): ListDrafts<T> {
  return { ...drafts, [kind]: values, resetRequested: false };
}

/** The lists to store; null restores the defaults. */
export function listDraftsSaveBody<T>(drafts: ListDrafts<T>): { workItems: T[] | null; pullRequests: T[] | null } {
  return drafts.resetRequested
    ? { workItems: null, pullRequests: null }
    : { workItems: drafts.workItems, pullRequests: drafts.pullRequests };
}

export function listDraftsDirty<T>(drafts: ListDrafts<T>, baseline: ListPair<T>): boolean {
  return (
    drafts.resetRequested ||
    JSON.stringify({ workItems: drafts.workItems, pullRequests: drafts.pullRequests }) !== JSON.stringify(baseline)
  );
}

/**
 * Editing, Reset, and Save need the stored lists as their baseline; a failed
 * read leaves only the defaults, and saving those would erase the stored lists.
 */
export function listDraftsEditable(state: { loading: boolean; loadError: string | null }): boolean {
  return !state.loading && !state.loadError;
}
