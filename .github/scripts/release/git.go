package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// git runs git commands in a working directory.
type git struct {
	dir string
}

// run runs git with the given arguments and returns its trimmed standard output. On failure
// the error includes git's standard error.
func (g git) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

// tags lists the tags that start with "v".
func (g git) tags() ([]string, error) {
	out, err := g.run("tag", "--list", "v*")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// createTag makes sure the annotated tag exists on sha and has been pushed to origin. It is
// safe to run again after a later step failed: an existing tag is reused if it points at sha
// and rejected if it points anywhere else. It reports whether it created and pushed the tag.
func (g git) createTag(tag, sha string) (created bool, err error) {
	ref := "refs/tags/" + tag

	if _, err := g.run("rev-parse", "-q", "--verify", ref); err == nil {
		existing, err := g.run("rev-list", "-n", "1", ref)
		if err != nil {
			return false, err
		}
		if existing != sha {
			return false, fmt.Errorf("tag %s already exists on %s, not on %s", tag, existing, sha)
		}
		return false, nil
	}

	for _, args := range [][]string{
		{"config", "user.name", "github-actions[bot]"},
		{"config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com"},
		{"tag", "-a", tag, "-m", tag, sha},
		{"push", "origin", ref},
	} {
		if _, err := g.run(args...); err != nil {
			return false, err
		}
	}

	return true, nil
}
