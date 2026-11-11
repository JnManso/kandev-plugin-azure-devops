# Changelog

## [Unreleased]

### Changed

- License changed from MIT to AGPL-3.0-only, matching Kandev and its plugin
  SDKs. Template-derived portions keep their MIT notice in `NOTICE.md`.

### Added

- Workspace connection with a Personal Access Token: save through Kandev's
  Save changes bar, test, copy without the token, remove, default project, and
  a 90-second health probe, in all Kandev locales.
- Microsoft account sign-in: popup with paste fallback, tenant discovery, PKCE,
  token renewal with one retry after a 401, sign-out, switching between a
  token and a Microsoft account, and the `default_oauth_client_id` setting.
- `azure_devops` repository provider: repository listing across the
  organization, URL inspection of the four Azure Repos remote forms, and branch
  listing, including repositories recorded before the plugin.
- Git credentials for task sessions through the Kandev credential broker,
  bound to the connection generation.
- Browse page with Boards, work-item search by number or title words (and
  WIQL), pull requests, saved views, query presets, and quick actions, on
  desktop and phone. Project, team, board, and repository pickers are
  searchable and list every project and team, beyond Azure DevOps' 100-record
  pages.
- Task links: tasks started from work items and pull requests are linked, the
  task Link menu links pull requests and work items, and linked pull requests
  report state, checks, and reviews through the review provider.
- Native Create PR, `#` composer references, and the `create_pull_request`
  agent tool.
- Work-item and pull-request watches with run now, reset, an open-task limit,
  and a localized reason when a watch is disabled. Task titles fit Kandev's
  60-character limit.
- Teams and sprints: team selectors list the connected identity's teams first
  and remember the last team per project; work items filter by team and
  sprint, kept in saved views; a Sprints mode shows a sprint as a read-only
  taskboard with sprint, person, and collapsible rows, on desktop and phone.
  Board and sprint cards offer task actions. A taskboard the team never
  customized uses the task type's states as columns, as Azure Boards does.
- Board card children: cards show a done/total count of their child work
  items and expand in place to list them, level by level down to the tasks,
  with detail and task actions; expand all is remembered per board. The board selector no longer offers the task
  backlog level, which has no Kanban board.
- The scope bar follows the browse mode and shows the shortcut that matches
  the current filters; **Advanced** shows how many of its pull-request
  filters are set. The pull-request creator and reviewer filters pick people
  by name (anyone, me, or the creators and reviewers of the repository's
  recent pull requests) instead of identity IDs, in browse and in
  pull-request watches; **Me** shows the signed-in user's name.
- Builds against the published SDKs (`pluginsdk` v0.2.0 and
  `@kdlbs/plugin-sdk` 0.2.0) instead of a sibling checkout, with the
  `kandev-sdk-check` compatibility check in CI; `min_kandev_version` is 0.100.0.
- Per-workspace enable switch on the integrations card and settings header.
- Packaged-host Playwright suite for desktop and phone (`make e2e`).

- Plugin skeleton: manifest, runtime, frontend registration, provider-neutral
  source-control boundary, marketplace icon, and build, package, and release
  tooling on the standalone Kandev plugin SDK paths.
