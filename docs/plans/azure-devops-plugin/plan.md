---
created: 2026-11-02
status: draft
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001
system_design:
  - ../../specs/system-design/azure-devops-plugin.md
  - ../../specs/system-design/azure-devops-plugin-sprints.md
  - ../../specs/system-design/azure-devops-plugin-board-children.md
---

# Implementation Plan: Azure DevOps Connector Plugin

## Overview

This repository owns the Azure DevOps connector plugin that replaces Kandev's
first-party integration. It reaches parity with the built-in integration and
adds Microsoft account sign-in (ported from the closed kdlbs/kandev#4915),
teams and sprints, and board card children.

The Kandev side of the extraction is planned separately, in the
[Kandev extraction plan](https://github.com/kdlbs/kandev/blob/main/docs/plans/azure-devops-plugin-extraction/plan.md).
That plan covers the extraction contract, the removal of the built-in
integration, and the registry entry. This plan covers the plugin's own
behavior, and the documents it lists are maintained here.

| Document | Location |
| --- | --- |
| Requirements | [azure-devops-plugin](../../specs/requirements/azure-devops-plugin.md), [sprints](../../specs/requirements/azure-devops-plugin-sprints.md), [board children](../../specs/requirements/azure-devops-plugin-board-children.md) |
| System design | [azure-devops-plugin](../../specs/system-design/azure-devops-plugin.md), [sprints](../../specs/system-design/azure-devops-plugin-sprints.md), [board children](../../specs/system-design/azure-devops-plugin-board-children.md) |
| Extraction contract (Kandev) | [requirements](https://github.com/kdlbs/kandev/blob/main/docs/specs/plugins/requirements/azure-devops-extraction.md), [system design](https://github.com/kdlbs/kandev/blob/main/docs/specs/plugins/system-design/azure-devops-extraction.md) |

## Order of work

1. **Plugin to parity (tasks 01 to 06),** developed against a Kandev checkout
   that already carries the removal (Kandev task 08), because the web plugin
   registry rejects `azure_devops` while core owns it.
2. **Packaged E2E (task 07).**
3. **Teams and sprints (task 10) and board card children (task 11).**
4. After the Kandev release that contains the removal, the plugin pins
   `min_kandev_version` to it and publishes `v0.1.0`. Kandev then lists it in
   the registry.

All plugin work orders ship in one pull request on one branch. The release tag
follows its merge.

## Technical approach

- **Bootstrap (task 01):** template identity rename; the published SDKs
  `github.com/kdlbs/kandev/pluginsdk` and `@kdlbs/plugin-sdk`;
  `recipes/source-control/` adopted as one slice; the Bitbucket plugin
  toolchain (`ui/src/*.ts` with esbuild, Vitest, Playwright packaged host,
  `internal/` packages); README, CHANGELOG, LICENSE, icon notice, PR template;
  CI and release workflows.
- **Connection (task 02):** REST client and models, PAT authentication,
  organization URL validation, remote parsing, and the mock client;
  `connection` state and secrets; the integration settings form; health poll;
  repository provider; Git credential handler.
- **Microsoft sign-in (task 03):** Entra sign-in, token renewal, the
  per-workspace application, the sign-in UI, and the concurrency tests.
- **Browse (task 04):** the browse screens on the host integration
  components; route and navigation registration.
- **Tasks and links (task 05):** task creation from work items, task actions,
  review provider, Create PR, the agent pull-request tool, and entity
  references.
- **Watches (task 06):** the watch service and poller on Host state and
  `Tasks().Create`.
- **Release (task 07):** packaged E2E, release workflow, `v0.1.0`.
- **Teams and sprints (task 10):** my-teams grouping and remembered team,
  work-item Team and Sprint filters, and the read-only Sprints taskboard.
- **Board card children (task 11):** child counts, nested expansion down to
  tasks, and child actions on board cards.

### Executor compatibility matrix

| Executor | Plugin Git credentials | Requirement |
| --- | --- | --- |
| local, worktree | broker lease on loopback | none |
| local Docker, remote Docker, SSH, Kubernetes, Sprites | broker lease | `githubCredentialBroker.publicBaseUrl` (HTTPS) reachable from the executor; documented in the README, as for Bitbucket |

## ASCII UI preview

Spacing is illustrative. Control order, grouping, and the primary actions are
structural. Copy is localized. Browse screens keep the built-in composition;
the built-in desktop and mobile E2E specs are their parity reference.

UI-01: Plugin settings, Microsoft account, signed in. Desktop. Settings >
Integrations > Azure DevOps (AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.1 to
002.7).

```text
+--------------------------------------------------------------------------+
| Azure DevOps integration                                          [on]   |
| +----------------------------------------------------------------------+ |
| | v Authenticated . checked just now                                   | |
| | Organization URL   [https://dev.azure.com/org                     ]  | |
| | Authentication     [Microsoft account                          v]    | |
| | +------------------------------------------------------------------+ | |
| | | (v) Signed in as Ana Silva          [Sign in again] [Sign out]   | | |
| | |     Application in use: 04b0...6                                 | | |
| | | Application (client) ID [                                     ]  | | |
| | | Leave blank to use this installation's default.                  | | |
| | +------------------------------------------------------------------+ | |
| | Default project    [Platform                     v]                  | |
| | [Test connection] [Update]                                [Remove]   | |
| +----------------------------------------------------------------------+ |
| Watches: work items | pull requests                                     |
+--------------------------------------------------------------------------+
```

UI-02: The same settings on a phone. Controls stack full width
(AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.10).

```text
+------------------------------+
| < Integrations  Azure DevOps |
+------------------------------+
| Organization URL             |
| [https://dev.azure.com/org]  |
| Authentication               |
| [Microsoft account       v]  |
| (v) Signed in as Ana Silva   |
| [ Sign in again           ]  |
| [ Sign out                ]  |
| Application (client) ID      |
| [                         ]  |
| Default project              |
| [Platform                v]  |
| [ Update                  ]  |
| [ Test connection         ]  |
+------------------------------+
```

UI-03: Watch disabled because its profile is no longer available. Desktop and
phone share the row anatomy (AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.3).

```text
| Work-item watch "Bugs to triage"                      [disabled] [Edit] |
| ! Executor "Ana's laptop" is no longer available to this plugin. Edit   |
|   the watch and choose another executor.                                |
```

UI-04: Sprints mode. Desktop. Browse > Sprints
(AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.1, 001.3 to 001.6).

```text
+---------------------------------------------------------------------------+
| Board | Sprints | Work items | Pull requests                              |
| [Contoso    v] [Offers         v] [Sprint 23 Current v] [Person: All v]   |
| Nov 2 - Nov 13 . 4 working days left                       [Collapse all] |
+-------------------------+------------+------------+-----------+-----------+
| Backlog item            | New        | Active     | Resolved  | Closed    |
+-------------------------+------------+------------+-----------+-----------+
| v #1001 Send customer   |            | [#1003]    |           |           |
|   Active . A. Lovelace  |            |  Map ...   |           |           |
| v #1002 Send address    | [#1004]    |            |           |           |
| > Unparented tasks (2)  |                                                 |
+-------------------------+------------+------------+-----------+-----------+

Team selector, open:            Sprint selector, open:
+---------------------------+   +-------------------------------+
| [Filter teams           ] |   | [Filter sprints             ] |
| My teams                  |   | Sprint 22             Past    |
|   Platform                |   | Sprint 23             Current |
|   Offers                v |   | Sprint 24             Future  |
| All teams                 |   +-------------------------------+
|   Payments                |
|   ...                     |
+---------------------------+
```

UI-05: Sprints mode on a phone (AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.7).

```text
+------------------------------+
| [Offers                   v] |
| [Sprint 23 Current        v] |
| [Person: All              v] |
| Nov 2 - Nov 13 . 4 days left |
+------------------------------+
| v #1001 Send customer ..     |
|   Active (1)                 |
|   [#1003 Map the address ..] |
| > #1002 Send address ..      |
+------------------------------+
```

UI-06: Board card children. Desktop. Browse > Board, Features board
(AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.1 to 001.4, 001.6). The
phone shows the same card inside the column navigator (001.5).

```text
| [Contoso v] [Offers v] [Features v]                [Expand all]   |
| New                              2 | Active                         |
| +--------------------------------+ |                                |
| | :: Execution Details           | |                                |
| | #2005  Feature                 | |                                |
| | v User Story: 1/3     [+ Task] | |                                |
| |   #951201 Story  Closed  Ana   | |                                |
| |   #951202 Story  Active  Rui   | |                                |
| |   v Task: 1/2                  | |                                |
| |   | #951210 Task  Closed  Rui  | |                                |
| |   | #951211 Task  Active  Ana  | |                                |
| |   #951203 Story  New     -     | |                                |
| +--------------------------------+ |                                |
| | :: Status Workflow ...         | |                                |
| | #2006  Feature                 | |                                |
| | > User Story: 0/2     [+ Task] | |                                |
| +--------------------------------+ |                                |
```

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| PLUGIN-001.1, 001.4 | `scripts/test-verify-package.sh`, `test-verify-release-version.sh` |
| PLUGIN-002, 003 | `internal/auth/*_test.go`, `internal/plugin/connections_test.go`, the sign-in concurrency tests; `ui/test/*.test.ts` |
| PLUGIN-004 | `ui/test/browse*.test.ts`; packaged E2E |
| PLUGIN-005 | `internal/plugin/{links,review,agent_tools,references}_test.go` |
| PLUGIN-006 | `internal/plugin/credentials_test.go`, `provider_test.go` |
| PLUGIN-007 | `internal/watches/*_test.go` |
| PLUGIN-SPRINTS-001 | `internal/azure/sprints_test.go`, `internal/plugin/actions_browse_test.go`; `ui/src/browse/sprint-board.test.ts`, `search-query.test.ts` |
| PLUGIN-CHILDREN-001 | `internal/azure/children_test.go`, `rest_client_children_test.go`, `internal/plugin/browse_children_test.go`; `ui/src/browse/board-children.test.ts` |

## E2E tests

| Flow | AC | File |
| --- | --- | --- |
| Connect with PAT; Microsoft sign-in via mock authority; renewal; sign-out | PLUGIN-002, 003 | `ui/e2e/desktop-packaged-plugin.spec.ts`, `mobile-packaged-plugin.spec.ts` |
| Browse boards, work items, PRs; saved views | PLUGIN-004 | same files |
| Task from work item; Link; status indicators; Create PR; agent tool | PLUGIN-005 | same files |
| Repository and branch pickers; private clone on local executor | PLUGIN-006 | same files |
| Watch creates one task; disabled reason | PLUGIN-007 | same files |
| My teams first; Team and Sprint filters; Sprints taskboard, person filter, collapse, empty states | PLUGIN-SPRINTS-001 | same files |
| Board card child counts, expand and collapse, child detail and task actions, expand all remembered, unavailable notice | PLUGIN-CHILDREN-001 | same files |

## Work orders

- [ ] [Task 01: Plugin repository bootstrap](task-01-plugin-bootstrap.md)
- [ ] [Task 02: Connection with PAT, repositories, Git credentials](task-02-plugin-connection.md)
- [ ] [Task 03: Microsoft sign-in](task-03-plugin-microsoft-sign-in.md)
- [ ] [Task 04: Browse screens](task-04-plugin-browse-screens.md)
- [ ] [Task 05: Tasks, links, PR status, Create PR, agent tool](task-05-plugin-tasks-links.md)
- [ ] [Task 06: Watches](task-06-plugin-watches.md)
- [ ] [Task 07: Packaged E2E and release](task-07-plugin-release.md)
- [ ] [Task 10: Teams and sprints](task-10-plugin-teams-sprints.md)
- [x] [Task 11: Board card children](task-11-plugin-board-children.md)

## Risks

- **Entra redirect.** The popup return to the plugin route origin must be
  revalidated against a real organization in task 03.
- **Release ordering.** `min_kandev_version` follows the SDK floor (0.100.0)
  until the Kandev release with the removal exists; it is raised to that
  release before `v0.1.0`. Until the registry lists the plugin, users install
  it from the built package.
- **Executors.** Docker, SSH, and Kubernetes need the public HTTPS broker
  address for private Azure repos, as for Bitbucket. This is documented, not
  solved.
- **Packaging tooling.** The plugin builds against the published SDK pins.
  `.kandev-sdk-ref` pins only the Kandev checkout that provides `plugin-pack`
  and the packaged-host tests.
