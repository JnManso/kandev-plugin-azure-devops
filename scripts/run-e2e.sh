#!/bin/sh
# Runs the packaged-host Playwright suite. Without KANDEV_PLUGIN_E2E_URL it
# starts a disposable Kandev host from the sibling checkout (../kandev) in mock
# mode: KANDEV_E2E_MOCK provides mock agents, and
# KANDEV_AZURE_DEVOPS_PLUGIN_MOCK an in-process Azure DevOps organization and
# Microsoft sign-in. The host is stopped and its data removed on exit.
set -eu

repo_dir=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
cd "$repo_dir"

if [ -n "${KANDEV_PLUGIN_E2E_URL:-}" ]; then
	exec npx playwright test "$@"
fi

kandev_apps=${KANDEV_APPS_DIR:-"$repo_dir/../kandev/apps"}
binary=$kandev_apps/backend/bin/kandev
[ -x "$binary" ] || { echo "run-e2e: build Kandev first: make -C $kandev_apps/backend build" >&2; exit 1; }
[ -d "$kandev_apps/web/dist" ] || { echo "run-e2e: build the Kandev web app first" >&2; exit 1; }

port=${KANDEV_PLUGIN_E2E_PORT:-18799}
data_dir=$(mktemp -d "${TMPDIR:-/tmp}/azure-devops-e2e.XXXXXX")
mkdir -p "$data_dir/home"
(
	cd "$kandev_apps"
	KANDEV_E2E_MOCK=true KANDEV_AZURE_DEVOPS_PLUGIN_MOCK=true \
	KANDEV_HOME_DIR="$data_dir/home" KANDEV_SERVER_PORT="$port" \
	KANDEV_WEB_DIST_DIR="$kandev_apps/web/dist" KANDEV_DATABASE_PATH="$data_dir/kandev.db" \
	exec "$binary" __backend
) >"$data_dir/backend.log" 2>&1 &
host_pid=$!
cleanup() {
	kill "$host_pid" 2>/dev/null || true
	wait "$host_pid" 2>/dev/null || true
	rm -rf "$data_dir"
}
trap cleanup EXIT INT TERM

base_url=http://127.0.0.1:$port
attempt=0
until curl -sf "$base_url/ready" >/dev/null 2>&1; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 90 ] || ! kill -0 "$host_pid" 2>/dev/null; then
		echo "run-e2e: the disposable host did not start; log follows" >&2
		tail -40 "$data_dir/backend.log" >&2
		exit 1
	fi
	sleep 1
done

KANDEV_PLUGIN_E2E_URL=$base_url npx playwright test "$@"
