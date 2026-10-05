#!/usr/bin/env bash
# Runs `make generate` and fails when it leaves docs or examples changed or untracked, so
# the generated output is committed together with the sources that produced it. This mirrors
# the CI "generate" job, which fails on any diff after generation.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

before=$(git diff -- docs examples | git hash-object --stdin)

make generate

after=$(git diff -- docs examples | git hash-object --stdin)
untracked=$(git ls-files --others --exclude-standard -- docs examples)

if [ "$before" != "$after" ] || [ -n "$untracked" ]; then
	echo "make generate changed docs/examples. Review, 'git add docs examples', and commit again." >&2
	[ -n "$untracked" ] && echo "New files:" >&2 && echo "$untracked" >&2
	exit 1
fi
