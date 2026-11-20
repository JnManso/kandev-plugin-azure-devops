#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
update_script=$repo_dir/scripts/update-changelog.sh
tmp_root=${TMPDIR:-"$repo_dir/.build/tmp"}
mkdir -p "$tmp_root"
test_dir=$(mktemp -d "$tmp_root/azure-devops-changelog-test.XXXXXX")
trap 'rm -rf "$test_dir"' EXIT

printf '%s\n' '- fix: second change (bbb2222)' '- feat: first change (aaa1111)' > "$test_dir/changes.txt"
: > "$test_dir/no-changes.txt"

# expect NAME VERSION CHANGES_FILE: runs the script in fixture NAME (whose
# CHANGELOG.md, if any, is already written) and compares with NAME.expected.
expect() {
	name=$1
	(cd "$test_dir/$name" && sh "$update_script" "$2" 2026-01-02 "$3")
	if ! diff -u "$test_dir/$name.expected" "$test_dir/$name/CHANGELOG.md"; then
		printf 'unexpected changelog for %s\n' "$name" >&2
		exit 1
	fi
}

mkdir "$test_dir/unreleased"
cat > "$test_dir/unreleased/CHANGELOG.md" <<'EOF'
# Changelog

## [Unreleased]

### Added

- A curated note.

## [1.0.0] - 2026-01-01

### Changed

- Older note.
EOF
cat > "$test_dir/unreleased.expected" <<'EOF'
# Changelog

## [1.1.0] - 2026-01-02

### Added

- A curated note.

## [1.0.0] - 2026-01-01

### Changed

- Older note.
EOF
expect unreleased 1.1.0 "$test_dir/changes.txt"

mkdir "$test_dir/commits"
cat > "$test_dir/commits/CHANGELOG.md" <<'EOF'
# Changelog

## [1.0.0] - 2026-01-01

### Changed

- Older note.
EOF
cat > "$test_dir/commits.expected" <<'EOF'
# Changelog

## [1.0.1] - 2026-01-02

### Changed

- fix: second change (bbb2222)
- feat: first change (aaa1111)

## [1.0.0] - 2026-01-01

### Changed

- Older note.
EOF
expect commits 1.0.1 "$test_dir/changes.txt"

mkdir "$test_dir/empty-unreleased"
cat > "$test_dir/empty-unreleased/CHANGELOG.md" <<'EOF'
# Changelog

## [Unreleased]

## [1.0.0] - 2026-01-01

- Older note.
EOF
cat > "$test_dir/empty-unreleased.expected" <<'EOF'
# Changelog

## [1.0.1] - 2026-01-02

### Changed

- fix: second change (bbb2222)
- feat: first change (aaa1111)

## [1.0.0] - 2026-01-01

- Older note.
EOF
expect empty-unreleased 1.0.1 "$test_dir/changes.txt"

mkdir "$test_dir/missing"
cat > "$test_dir/missing.expected" <<'EOF'
# Changelog

## [0.1.0] - 2026-01-02

### Changed

- Initial release
EOF
expect missing 0.1.0 "$test_dir/no-changes.txt"

mkdir "$test_dir/bad-version"
printf '# Changelog\n' > "$test_dir/bad-version/CHANGELOG.md"
if (cd "$test_dir/bad-version" && sh "$update_script" v1.0.0 2026-01-02 "$test_dir/changes.txt") > /dev/null 2>&1; then
	echo 'expected update-changelog.sh to reject a version with a v prefix' >&2
	exit 1
fi
echo 'changelog update tests passed'
