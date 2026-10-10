#!/usr/bin/env bash
# Print the CHANGELOG.md section for a tag, i.e. the lines between "# <tag>..." and the next "# " heading.
# Usage: release-notes.sh v0.15.0 [CHANGELOG.md]
# Exits 1 (printing nothing) when there is no such section.
set -euo pipefail

tag="${1:?usage: $0 <tag> [changelog]}"
changelog="${2:-CHANGELOG.md}"

awk -v tag="$tag" '
	/^# / {
		if (found) exit
		split($0, words, /[ \t]+/)
		if (words[2] == tag) { found = 1; next }
	}
	found { print }
	END { exit !found }
' "$changelog"
