# kandev-plugin-azure-devops

Azure DevOps Services integration for [Kandev](https://github.com/kdlbs/kandev),
delivered as a runtime plugin.

## Status

Feature-complete for its first release, not released yet. Until Kandev ships a
release without its built-in Azure DevOps integration, the built-in
integration remains the supported path.

- **Connection:** Settings > Integrations > Azure DevOps connects a workspace
  with a Personal Access Token or a Microsoft account. It tests the
  connection, picks a default project, copies settings from another workspace
  (never a token), and removes it. A health probe runs every 90 seconds.
- **Enable switch:** the integrations card and the settings header turn the
  integration on or off per workspace and per user. It only controls the
  settings menu badge and row; connections, watches, and credentials keep
  working while it is off.
- **Microsoft account:** sign-in runs in a popup with PKCE and returns to the
  settings page's own address; a paste panel completes it when the popup
  cannot return. Tokens renew under a per-workspace lock, a rejected request
  is retried once after renewal, and sign-out keeps the settings. The
  application (client) ID comes from the workspace form, then the application
  that signed in, then the operator setting `default_oauth_client_id`. Register
  the page origin (for example `http://localhost:38429`) as a redirect URI of
  that application.
- **Browse:** the Azure DevOps page offers Boards, Sprints, work-item search,
  and pull requests, with project, team, and board scopes, saved views, query
  presets, and quick actions, on desktop and phone. Board and sprint cards
  offer the same task actions as work-item results.
- **Teams and sprints:** team selectors list your teams first under **My
  teams** and remember the team you last chose in each project. Work items can
  be narrowed to a team's areas and one of its sprints. **Sprints** shows a
  sprint as a read-only taskboard: one row per backlog item with its tasks in
  the team's taskboard columns, past, current, and future sprints with their
  dates and working days left, and a person filter. "My teams" are the teams
  of the identity behind the connection, so with a shared Personal Access
  Token they are the token owner's teams.
- **Board card children:** a Board card shows how many of its child work
  items are done (an epic's features, a feature's stories, a story's tasks)
  and expands to list them, each opening its detail and offering task
  actions; a child with children expands the same way, down to the tasks.
  **Expand all** is remembered per board. The task backlog level is
  not offered as a board; its tasks are on the Sprints taskboard.
- **Tasks and links:** start a task from a work item or pull request in
  Kandev's task dialog and it is linked. A task's **Link** menu offers **Azure
  DevOps Pull Request** and **Azure DevOps Work Item**. Linked pull requests
  show their state, checks, and reviews in the task indicators and the review
  panel. **Create PR** opens an Azure Repos pull request from the task branch
  and links it, with the task's linked work items attached.
- **References and agent tool:** the composer offers `#` references to work
  items and pull requests. Agents get a `create_pull_request` tool that opens
  and links a pull request from a pushed branch, without the `az` CLI or a
  credential in their environment.
- **Watches:** work-item (WIQL) and pull-request watches create one task per
  matching item with the chosen workflow, step, agent and executor profiles,
  prompt, repository, and base branch. Watches can be run now, reset, and
  limited to a number of open tasks.
- **Repository provider:** `azure_devops` repositories are listed, inspected,
  and given branch lists through the plugin, including repositories recorded
  by the former built-in integration.
- **Git credentials:** task sessions clone, fetch, and push through the Kandev
  credential broker with the workspace token.

### Known differences from the built-in integration

These follow from the host contracts available to plugins today:

- Linked pull requests do not feed the task delivery ledger, and watch-created
  tasks are not classified as review-watch or issue-watch tasks in the sidebar.
- Watches can only launch with shared agent and executor profiles. When a
  profile stops being available, the watch is disabled and settings say why.
- Kandev does not let plugins delete tasks, so watches have no cleanup policy:
  deleting or resetting a watch keeps the tasks it created.
- The enable switch cannot remove the Azure DevOps entry from the main
  sidebar, because Kandev has no way for a plugin to hide its navigation item.
  With "Hide disabled integrations" on, only the settings menu row is hidden.
- Settings, watches, and saved views are not migrated from the built-in
  integration. Reconnect and recreate them after installing the plugin.

## Requirements

- Kandev with the plugin SDK this repository pins (see
  [Develop against the SDK](#develop-against-the-sdk)). The manifest's
  `min_kandev_version` is raised at release to the first Kandev release
  without the built-in integration.
- Executors other than local and worktree (Docker, remote Docker, SSH,
  Kubernetes, Sprites) obtain Git credentials from the credential broker. They
  require `githubCredentialBroker.publicBaseUrl` to be an HTTPS URL reachable
  from the executor, as for the Bitbucket plugin.

## Develop against the SDK

The plugin uses the published SDKs at exact versions: the Go module
`github.com/kdlbs/kandev/pluginsdk` `v0.2.0` and the npm package
`@kdlbs/plugin-sdk` `0.2.0`. Both require Kandev 0.100.0 or later, which is
the `min_kandev_version` floor in `manifest.yaml`. Upgrade them together, then
rerun the compatibility check, the tests, and a smoke test on the declared
minimum host:

```bash
go run github.com/kdlbs/kandev/pluginsdk/cmd/kandev-sdk-check@v0.2.0 \
  --manifest manifest.yaml --frontend-sdk node_modules/@kdlbs/plugin-sdk
```

CI, build, and release workflows run the same check after installing
dependencies.

Packaging and the packaged-host tests still use a sibling Kandev checkout for
`cmd/plugin-pack` and the disposable host:

```text
parent/
├── kandev/                        # https://github.com/kdlbs/kandev
│   └── apps/backend/              # provides cmd/plugin-pack
└── kandev-plugin-azure-devops/    # this repository
```

If your Kandev checkout has another name, a `kandev` symlink next to this
repository works. The build and release workflows check out `kdlbs/kandev` at
the commit in `.kandev-sdk-ref` for that tooling. Update that pin to an exact
reviewed commit, never a branch.

## Layout

```text
manifest.yaml               plugin contract (id, capabilities, UI bundle, icon)
server/main.go              pluginsdk.Serve(plugin.NewRuntime())
internal/azure/             Azure DevOps REST client (API 7.1), authorizers, remote URL parsing
internal/auth/              Microsoft Entra tenant discovery, authorization, and tokens
internal/store/             connection state and token secrets in Kandev Host storage
internal/plugin/            runtime, actions, health poller, repositories, Git credentials,
                            task links, review status, references, agent tool
internal/watches/           work-item and pull-request watches and their poller
internal/sourcecontrol/     provider-neutral repository, review, and reference boundary
ui/src/                     frontend sources, bundled to ui/bundle.js by esbuild
ui/e2e/                     packaged-host Playwright suite (desktop and phone)
assets/                     marketplace icon and its notice
scripts/                    package and release verifiers
```

## Build and test

Requires Go 1.26 and Node 20.19 or 22.12 and later.

```sh
npm ci
make test          # Go tests, UI typecheck/build/tests, verifier tests
make check-format vet
make audit         # npm audit --audit-level=high
```

### End-to-end tests

`make e2e` packages the plugin for the current platform and runs the
Playwright suite on a desktop and a phone project. It starts a disposable
Kandev host from the sibling checkout in mock mode, so it needs a built Kandev
backend (`make -C ../kandev/apps/backend build`) and web app, and a Chromium for
Playwright (`npx playwright install chromium`). Set `KANDEV_PLUGIN_E2E_URL` to
run against a host you started yourself; it must be fresh and run with
`KANDEV_E2E_MOCK=true` and `KANDEV_AZURE_DEVOPS_PLUGIN_MOCK=true`.

If `npm install` reports `ETARGET` for a version that exists, the npm cache is
stale. Retry with `npm install --prefer-online`.

## Package

```sh
make package          # all five platforms
make package-host     # current platform only
make verify-package   # package and verify the archive inventory and checksums
```

Packaging runs `cmd/plugin-pack` from the sibling Kandev backend and writes
`kandev-plugin-azure-devops-<version>.tar.gz`.

## Install into Kandev

Install through **Settings > Plugins > Install plugin** (file upload or URL),
or with the API:

```sh
curl -F package=@kandev-plugin-azure-devops-0.1.0.tar.gz \
  http://localhost:<kandev-port>/api/plugins/install
```

Kandev verifies the package checksums and manifest, extracts the package, and
starts the binary for the host platform. Reinstalling the same version returns
409; bump `version` in `manifest.yaml` and `VERSION` in the `Makefile`.

## Release

Pull requests run CI (tidy, format, vet, tests, audit) and full platform
packaging. The release workflow accepts a manual version bump from `main` or a
pushed `vX.Y.Z` tag. Both paths run the same checks, then publish the package
and its `checksums.txt` to a GitHub release.

## License

GNU Affero General Public License, version 3 only (AGPL-3.0-only), the same
license as Kandev and its plugin SDKs. See [LICENSE](LICENSE). Portions derived
from the MIT-licensed Kandev plugin template keep their notice in
[NOTICE.md](NOTICE.md). The marketplace icon is a Microsoft trademark; see
[assets/NOTICE.md](assets/NOTICE.md).
