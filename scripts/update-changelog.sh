#!/bin/sh
# Adds a release section to CHANGELOG.md in the current directory.
#
# Usage: update-changelog.sh VERSION DATE [CHANGES_FILE]
#
# The notes under "## [Unreleased]" become the release section and that heading
# is removed, so no empty or stale Unreleased section is left behind. Without
# Unreleased notes, the section lists the lines of CHANGES_FILE (one commit per
# line) under "### Changed", or "- Initial release" when there are none.
set -eu

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
	echo "usage: $0 VERSION DATE [CHANGES_FILE]" >&2
	exit 2
fi
version=$1
date=$2
changes=${3:-/dev/null}

printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || {
	echo "version must be X.Y.Z, got: $version" >&2
	exit 1
}
printf '%s\n' "$date" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' || {
	echo "date must be YYYY-MM-DD, got: $date" >&2
	exit 1
}
[ -r "$changes" ] || {
	echo "cannot read changes file: $changes" >&2
	exit 1
}

[ -f CHANGELOG.md ] || printf '# Changelog\n' > CHANGELOG.md

awk -v version="$version" -v date="$date" -v changes="$changes" '
function blank(line) { return line ~ /^[[:space:]]*$/ }
NR == 1 { title = $0; next }
/^## / {
	# Markdown may escape the brackets: "## \[Unreleased\]".
	unreleased = ($0 ~ /^## \\?\[Unreleased\\?\][[:space:]]*$/)
	if (unreleased) next
}
unreleased { notes[++n] = $0; next }
{ rest[++m] = $0 }
END {
	first = 1; while (first <= n && blank(notes[first])) first++
	last = n; while (last >= first && blank(notes[last])) last--

	print title
	print ""
	printf "## [%s] - %s\n\n", version, date
	if (first <= last) {
		for (i = first; i <= last; i++) print notes[i]
	} else {
		print "### Changed"
		print ""
		listed = 0
		while ((getline line < changes) > 0) {
			if (!blank(line)) { print line; listed++ }
		}
		if (!listed) print "- Initial release"
	}

	start = 1; while (start <= m && blank(rest[start])) start++
	if (start <= m) {
		print ""
		for (i = start; i <= m; i++) print rest[i]
	}
}
' CHANGELOG.md > CHANGELOG.md.next
mv CHANGELOG.md.next CHANGELOG.md
