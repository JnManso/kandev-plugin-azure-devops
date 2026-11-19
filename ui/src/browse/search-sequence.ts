/**
 * Orders searches whose query is resolved asynchronously (a team-scoped
 * work-item search reads the team's settings first). Each search takes a
 * token; a newer search or unmount retires it, so a slow older search never
 * replaces newer results.
 */
export function createSearchSequence() {
  let current = 0;
  return {
    begin(): () => boolean {
      const token = ++current;
      return () => token === current;
    },
    retire(): void {
      current++;
    },
  };
}

export type SearchSequence = ReturnType<typeof createSearchSequence>;

/**
 * Runs the search with its resolved query only while it is still the current
 * search; a retired search drops both its results and its errors.
 */
export function settleScopedSearch<Q>(
  query: Promise<Q>,
  isCurrent: () => boolean,
  run: (query: Q) => unknown,
  fail: (error: unknown) => void,
): Promise<void> {
  return query.then(
    (resolved) => {
      if (isCurrent()) void run(resolved);
    },
    (error: unknown) => {
      if (isCurrent()) fail(error);
    },
  );
}
