#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
verify_script=$repo_dir/scripts/verify-package.sh
tmp_root=${TMPDIR:-"$repo_dir/.build/tmp"}
mkdir -p "$tmp_root"
test_dir=$(mktemp -d "$tmp_root/azure-devops-package-test.XXXXXX")
trap 'rm -rf "$test_dir"' EXIT

create_fixture() {
	fixture=$1
	mkdir -p "$fixture/assets" "$fixture/server" "$fixture/ui"
	cat > "$fixture/manifest.yaml" <<'EOF'
id: "kandev-plugin-azure-devops"
api_version: 2
version: "0.1.0"
icon: "assets/icon.svg"
runtime:
  type: binary
  executables:
    linux-amd64: "server/plugin-linux-amd64"
    linux-arm64: "server/plugin-linux-arm64"
    darwin-amd64: "server/plugin-darwin-amd64"
    darwin-arm64: "server/plugin-darwin-arm64"
    windows-amd64: "server/plugin-windows-amd64.exe"
EOF
	printf 'Fixture license.\n' > "$fixture/LICENSE"
	printf 'Fixture program notice.\n' > "$fixture/NOTICE.md"
	printf '<svg/>\n' > "$fixture/assets/icon.svg"
	printf 'Fixture notice.\n' > "$fixture/assets/NOTICE.md"
	printf 'export default {}\n' > "$fixture/ui/bundle.js"
	printf '/* fixture */\n' > "$fixture/ui/plugin.css"
	for executable in \
		plugin-linux-amd64 plugin-linux-arm64 \
		plugin-darwin-amd64 plugin-darwin-arm64 \
		plugin-windows-amd64.exe; do
		printf 'fixture binary: %s\n' "$executable" > "$fixture/server/$executable"
	done
	write_checksums "$fixture"
}

write_checksums() {
	fixture=$1
	(
		cd "$fixture"
		find . -type f ! -name checksums.txt -print | sed 's#^\./##' | LC_ALL=C sort |
		while IFS= read -r path; do
			if command -v sha256sum >/dev/null 2>&1; then
				sha256sum "$path"
			else
				shasum -a 256 "$path"
			fi
		done
	) > "$fixture/checksums.txt"
}

copy_fixture() {
	name=$1
	fixture=$test_dir/$name
	mkdir -p "$fixture"
	cp -R "$test_dir/valid/." "$fixture/"
}

expect_failure() {
	name=$1
	fixture=$2
	mode=${3-full}
	platform=${4-}
	if [ "$mode" = host ]; then
		if sh "$verify_script" "$fixture" host "$platform" >/dev/null 2>&1; then
			printf 'expected package verification to reject %s\n' "$name" >&2
			exit 1
		fi
	elif sh "$verify_script" "$fixture" full >/dev/null 2>&1; then
		printf 'expected package verification to reject %s\n' "$name" >&2
		exit 1
	fi
}

mkdir -p "$test_dir/valid"
create_fixture "$test_dir/valid"
sh "$verify_script" "$test_dir/valid" full >/dev/null
host_platform=$(go env GOOS)-$(go env GOARCH)
copy_fixture host-valid
host_fixture=$test_dir/host-valid
host_executable=$(awk -v platform="$host_platform" '$0 ~ "    " platform ":" { gsub(/"/, "", $2); print $2 }' "$host_fixture/manifest.yaml")
[ -n "$host_executable" ] || { printf 'fixture does not declare host platform %s\n' "$host_platform" >&2; exit 1; }
for executable in "$host_fixture"/server/*; do
	[ "$executable" = "$host_fixture/$host_executable" ] || rm "$executable"
done
write_checksums "$host_fixture"
sh "$verify_script" "$host_fixture" host "$host_platform" >/dev/null

# plugin-pack -platform-only re-serializes the manifest without quotes and
# keeps only the host executable.
write_platform_only_manifest() {
	fixture=$1
	executables=$2
	cat > "$fixture/manifest.yaml" <<EOF
id: kandev-plugin-azure-devops
api_version: 2
version: 0.1.0
icon: assets/icon.svg
runtime:
    type: binary
    executables:
$executables
min_kandev_version: 0.99.0
EOF
	write_checksums "$fixture"
}

copy_fixture platform-only
platform_only=$test_dir/platform-only
for executable in "$platform_only"/server/*; do
	[ "$executable" = "$platform_only/$host_executable" ] || rm "$executable"
done
write_platform_only_manifest "$platform_only" "        $host_platform: $host_executable"
sh "$verify_script" "$platform_only" host "$host_platform" >/dev/null
expect_failure 'a platform-only package in full mode' "$platform_only"

copy_fixture platform-only-unsupported
unsupported=$test_dir/platform-only-unsupported
write_platform_only_manifest "$unsupported" "        $host_platform: $host_executable
        plan9-amd64: server/plugin-plan9-amd64"
expect_failure 'an unsupported executable in a platform-only manifest' "$unsupported" host "$host_platform"

for required in manifest.yaml LICENSE NOTICE.md assets/icon.svg assets/NOTICE.md ui/bundle.js ui/plugin.css \
	server/plugin-linux-amd64 server/plugin-linux-arm64 \
	server/plugin-darwin-amd64 server/plugin-darwin-arm64 \
	server/plugin-windows-amd64.exe; do
	name=missing-$(printf '%s' "$required" | tr / -)
	copy_fixture "$name"
	fixture=$test_dir/$name
	rm "$fixture/$required"
	write_checksums "$fixture"
	expect_failure "missing $required" "$fixture"
done

copy_fixture missing-checksums
rm "$test_dir/missing-checksums/checksums.txt"
expect_failure 'missing checksums.txt' "$test_dir/missing-checksums"

copy_fixture unchecksummed-license
sed '/  LICENSE$/d' "$test_dir/unchecksummed-license/checksums.txt" > "$test_dir/unchecksummed-license/checksums.next"
mv "$test_dir/unchecksummed-license/checksums.next" "$test_dir/unchecksummed-license/checksums.txt"
expect_failure 'a license without a checksum entry' "$test_dir/unchecksummed-license"

copy_fixture corrupt-ui
printf '// changed after checksum generation\n' >> "$test_dir/corrupt-ui/ui/bundle.js"
expect_failure 'corrupt UI contents' "$test_dir/corrupt-ui"

copy_fixture wrong-id
sed 's/^id: "kandev-plugin-azure-devops"$/id: "other-plugin"/' "$test_dir/wrong-id/manifest.yaml" > "$test_dir/wrong-id/manifest.next"
mv "$test_dir/wrong-id/manifest.next" "$test_dir/wrong-id/manifest.yaml"
write_checksums "$test_dir/wrong-id"
expect_failure 'a manifest for a different plugin id' "$test_dir/wrong-id"

copy_fixture wrong-api-version
sed 's/^api_version: 2$/api_version: 1/' "$test_dir/wrong-api-version/manifest.yaml" > "$test_dir/wrong-api-version/manifest.next"
mv "$test_dir/wrong-api-version/manifest.next" "$test_dir/wrong-api-version/manifest.yaml"
write_checksums "$test_dir/wrong-api-version"
expect_failure 'an unsupported manifest API version' "$test_dir/wrong-api-version"

copy_fixture incomplete-checksums
sed '/  ui\/bundle.js$/d' "$test_dir/incomplete-checksums/checksums.txt" > "$test_dir/incomplete-checksums/checksums.next"
mv "$test_dir/incomplete-checksums/checksums.next" "$test_dir/incomplete-checksums/checksums.txt"
expect_failure 'an omitted checksum entry' "$test_dir/incomplete-checksums"

copy_fixture unexpected-file
printf 'unexpected\n' > "$test_dir/unexpected-file/extra.txt"
write_checksums "$test_dir/unexpected-file"
expect_failure 'a checksummed unexpected file' "$test_dir/unexpected-file"

copy_fixture unexpected-directory
mkdir -p "$test_dir/unexpected-directory/extra"
expect_failure 'an unexpected empty directory' "$test_dir/unexpected-directory"

copy_fixture wrong-host-platform
expect_failure 'a host platform missing from the manifest' "$test_dir/wrong-host-platform" host freebsd-amd64

printf 'package verifier negative tests passed\n'
