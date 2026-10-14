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

// createTag makes sure the annotated tag exists on sha locally and on origin. It is safe to run
// again after a later step failed: an existing annotated tag is reused if it points at sha, and
// rejected if it points anywhere else or is a lightweight tag. The tag is pushed in both cases:
// pushing a tag that origin already has with the same object does nothing, a missing one is
// created, and a different one is rejected. It reports whether it created the tag locally.
func (g git) createTag(tag, sha string) (created bool, err error) {
	ref := "refs/tags/" + tag

	if _, err := g.run("rev-parse", "-q", "--verify", ref); err != nil {
		for _, args := range [][]string{
			{"config", "user.name", "github-actions[bot]"},
			{"config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com"},
			{"tag", "-a", tag, "-m", tag, sha},
		} {
			if _, err := g.run(args...); err != nil {
				return false, err
			}
		}
		created = true
	} else if err := g.checkExistingTag(tag, ref, sha); err != nil {
		return false, err
	}

	if _, err := g.run("push", "origin", ref); err != nil {
		return created, err
	}

	return created, nil
}

// checkExistingTag requires the existing tag to be an annotated tag that points at sha.
func (g git) checkExistingTag(tag, ref, sha string) error {
	// rev-list peels lightweight and annotated tags alike, so check the kind separately.
	kind, err := g.run("cat-file", "-t", ref)
	if err != nil {
		return err
	}
	if kind != "tag" {
		return fmt.Errorf("tag %s already exists as a lightweight tag, expected an annotated tag", tag)
	}

	existing, err := g.run("rev-list", "-n", "1", ref)
	if err != nil {
		return err
	}
	if existing != sha {
		return fmt.Errorf("tag %s already exists on %s, not on %s", tag, existing, sha)
	}

	return nil
}
