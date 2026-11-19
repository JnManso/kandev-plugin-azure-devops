---
id: "12-plugin-board-person-filter"
title: "Board person filter"
status: done
wave: 4
depends_on:
  - "10-plugin-teams-sprints"
  - "11-plugin-board-children"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-002
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-002.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-002.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-002.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-002.4
system_design:
  - ../../specs/system-design/azure-devops-plugin-board-children.md
---

# Task 12: Board person filter

## Summary

Requested by the owner on 2026-11-19, after the `v0.1.1` release. The Board
gains the Sprints person selector after its board selector, so a user can see
only their own cards. A card matches when it or one of its direct children is
assigned to the person; expanded cards keep their full child list. The choice
is remembered per board. UI only: no backend change.

Decisions confirmed by the owner on 2026-11-19:

- Options and labels are those of the Sprints selector (**All people**,
  **Unassigned**, people by name). No **Me** option.
- Direct children count; grandchildren do not, so the result does not depend
  on expansion. Expanded cards list all their children.

## In scope

- `ui/src/browse/sprints-model.ts`: export `isAssignedTo`, add
  `personFilterOptions` and `knownPerson`, used by Sprints and the Board.
- `ui/src/browse/board-people.ts` (new): `boardPeople`, `filterCardsByPerson`.
- `ui/src/browse/board.tsx`: person selector, filtered columns and counts,
  empty state, phone selectors stacked full width.
- `ui/src/browse/use-board-children.ts`: report when the first child level has
  settled.
- `ui/src/browse/use-preferences.ts`, `page.tsx`: `personByBoard` per board.
- `ui/src/browse/sprints.tsx`, `use-sprints.ts`: use the shared helpers.
- Locale key `noBoardCardsForPerson` in all seven plugin locales.
- Packaged E2E on desktop and phone; CHANGELOG and README.

## Out of scope

- A **Me** option, several people at once, filtering by grandchildren.
- Backend and Kandev host changes.

## Acceptance

- Every CHILDREN-002 criterion has a unit or packaged E2E test that failed
  before its change.

## Verification

Commands run from the plugin repository root; the Kandev checkout
(>= 0.102.0) is its sibling `../kandev`.

```bash
npx vitest run ui/src/browse/board-people.test.ts ui/src/browse/sprints-model.test.ts
make check-format vet test
make e2e
```

## Files likely touched

- `ui/src/browse/` new `board-people.ts`, `board-people.test.ts`;
  `sprints-model.ts`, `sprints-model.test.ts`, `board.tsx`,
  `use-board-children.ts`, `use-preferences.ts`, `page.tsx`, `sprints.tsx`,
  `use-sprints.ts`
- `ui/src/locales/*.json`
- `ui/e2e/desktop-packaged-plugin.spec.ts`, `mobile-packaged-plugin.spec.ts`
- `CHANGELOG.md`, `README.md`

## Dependencies

Tasks 10 (person selector) and 11 (children index read for every card).

## Risks

- The people list grows when the children arrive. The selector keeps the
  stored person until the first child level settles, so a person found only
  on children is not dropped while it loads.
- Assignees are matched by display name, as in Sprints; two people with the
  same display name are one option.

## Results

- 2026-11-19: implemented on branch `feature/azure-devops-board-f-p4x`.
  - Shared model (`sprints-model.ts`): `isAssignedTo` (exported from the
    private sprint helper), `personFilterOptions`, and `knownPerson`. Sprints
    now builds its options and its fallback with them.
  - `board-people.ts`: `boardPeople`, `filterCardsByPerson`, and
    `boardPersonFilter` (keeps a remembered person until the first child
    level settles, then shows **All people** if the board does not name them,
    without overwriting the stored value).
  - `use-board-children.ts` reports `settled`; `board.tsx` filters the cards
    before grouping them (column badges and the phone column navigator follow),
    computes **Expand all** over the cards shown, shows
    `noBoardCardsForPerson` when no card matches, and stacks the selectors full
    width on a phone. `personByBoard` is stored per `boardChildrenKey` in the
    browse preference, written only when the user picks a person.
  - Locale key `noBoardCardsForPerson` in all seven locales.
- Tests (red first):
  - Vitest: `board-people.test.ts` (10 cases) failed with the module missing;
    the 4 new `sprints-model.test.ts` cases failed with `isAssignedTo`,
    `personFilterOptions`, and `knownPerson` not being functions.
  - Packaged E2E, desktop: "filters the board by person, matching a card or
    its direct children" (options, Bruno Costa kept by child #302, full child
    list and `Task: 1/2` while filtered, Unassigned empty state and a 0 column
    count), "lists the people of a board's cards and direct children, not
    grandchildren" (Features board omits Bruno Costa, who is only on
    grandchild #302), and "remembers the person filter per board after a
    reload". Phone: "filters the board by person on a phone without
    horizontal scrolling" (full-width selector, column navigator count).
  - Red run against the previous UI code (new specs, package rebuilt from
    `1a71572`'s `ui/src`, `npm run e2e -- --grep "person|people of a board"`):
    the desktop and phone cases failed waiting for
    `azure-board-person-select` / `azure-board-selectors`; the other desktop
    cases did not run after the first failure in the serial suite.
- Verification: `make check-format vet test` passes (187 Vitest tests,
  typecheck, Go, package scripts); `make e2e` passes 44/44 against the local
  Kandev `v0.102.0-1-g4cdec16377` build (`KANDEV_APPS_DIR` and
  `KANDEV_BACKEND` pointed at that checkout, as it is not the worktree's
  sibling).
- 2026-11-19, review follow-up (PR #5): the card level is read in batches
  of 1,000 (`nextChildReadIds`), so boards with more than 1,000 cards read
  every card's children; the remembered person falls back to **All people**
  only when that level is complete (`cardChildrenComplete`: every card read,
  no failed or truncated read); the E2E helper waits for the preference
  write's response. `board-children.test.ts` replaces the `childParentIds`
  case with 4 cases that failed before the change.
- Lifecycle evidence: not applicable; the change is a UI filter and touches no
  scheduling, background delivery, or task/session lifecycle.
