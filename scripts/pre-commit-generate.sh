#!/usr/bin/env bash
# Runs `make generate` and fails when it leaves docs or examples changed or untracked, so
# the generated output is committed together with the sources that produced it. This mirrors
# the CI generate step, which fails on any diff after generation.
#
# The hook is always_run because pre-commit omits deleted paths from the file list it passes,
# and deleting an example or provider source also changes the generated docs. Relevance is
# decided here from the staged names, deletions and renames included. With nothing staged
# (CI, or `pre-commit run --all-files` on a clean index) generation always runs.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

relevant='^(internal/provider/.*\.go|examples/.*|tools/.*|main\.go)$'

if ! git diff --cached --quiet; then
	if ! git diff --cached --name-only --no-renames \
		| grep -E "$relevant" | grep -vE '_test\.go$' | grep -q .; then
		exit 0
	fi
fi

before=$(git diff -- docs examples | git hash-object --stdin)

make generate

after=$(git diff -- docs examples | git hash-object --stdin)
untracked=$(git ls-files --others --exclude-standard -- docs examples)

if [ "$before" != "$after" ] || [ -n "$untracked" ]; then
	echo "make generate changed docs/examples. Review, 'git add docs examples', and commit again." >&2
	[ -n "$untracked" ] && echo "New files:" >&2 && echo "$untracked" >&2
	exit 1
fi
