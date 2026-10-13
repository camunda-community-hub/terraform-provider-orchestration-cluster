// Command release holds the steps of the release workflow (.github/workflows/release.yml) as
// subcommands, so they can be tested and run locally instead of living as inline shell:
//
//	release require-main     fail unless GITHUB_REF is refs/heads/main
//	release next-version     compute the next version from the v* tags and RELEASE_TYPE
//	release check-changelog  require a dated CHANGELOG.md heading for VERSION
//	release create-tag       create and push the tag VERSION on GITHUB_SHA, idempotently
//
// Inputs are read from the environment, as set by the workflow: RELEASE_TYPE, DRY_RUN,
// VERSION, GITHUB_REF, GITHUB_SHA, GITHUB_OUTPUT and GITHUB_STEP_SUMMARY. Run it from the
// repository root, for example:
//
//	RELEASE_TYPE=minor go run ./.github/scripts/release next-version
//
// Failures are printed as GitHub error annotations and exit with status 1.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Getenv, git{}, os.Stdout); err != nil {
		fmt.Printf("::error::%v\n", err)
		os.Exit(1)
	}
}

// run executes one subcommand. It takes its inputs as parameters so tests can drive it.
func run(args []string, getenv func(string) string, g git, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: release require-main|next-version|check-changelog|create-tag")
	}

	switch args[0] {
	case "require-main":
		return requireMain(getenv("GITHUB_REF"))
	case "next-version":
		return runNextVersion(getenv, g, stdout)
	case "check-changelog":
		return runCheckChangelog(getenv, g, stdout)
	case "create-tag":
		return runCreateTag(getenv, g, stdout)
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func runNextVersion(getenv func(string) string, g git, stdout io.Writer) error {
	tags, err := g.tags()
	if err != nil {
		return err
	}

	releaseType := getenv("RELEASE_TYPE")
	latest, next, err := nextVersion(tags, releaseType)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "latest %s, release type %s, next %s\n", latest, releaseType, next)

	if err := appendTo(getenv("GITHUB_OUTPUT"), fmt.Sprintf("version=%s\n", next)); err != nil {
		return err
	}

	summary := strings.Join([]string{
		fmt.Sprintf("### Release %s", next),
		"",
		fmt.Sprintf("- Latest tag: `%s`", latest),
		fmt.Sprintf("- Release type: `%s`", releaseType),
		fmt.Sprintf("- Commit: `%s`", getenv("GITHUB_SHA")),
		fmt.Sprintf("- Dry run: `%s`", getenv("DRY_RUN")),
		"",
	}, "\n")

	return appendTo(getenv("GITHUB_STEP_SUMMARY"), summary)
}

func runCheckChangelog(getenv func(string) string, g git, stdout io.Writer) error {
	tag := getenv("VERSION")
	if tag == "" {
		return fmt.Errorf("VERSION is not set")
	}

	changelog, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		return err
	}

	if err := checkChangelog(string(changelog), tag); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "CHANGELOG.md has a dated entry for %s\n", tag)
	return nil
}

func runCreateTag(getenv func(string) string, g git, stdout io.Writer) error {
	tag, sha := getenv("VERSION"), getenv("GITHUB_SHA")
	if tag == "" || sha == "" {
		return fmt.Errorf("VERSION and GITHUB_SHA must be set")
	}

	created, err := g.createTag(tag, sha)
	if err != nil {
		return err
	}

	if created {
		fmt.Fprintf(stdout, "Created and pushed tag %s on %s\n", tag, sha)
	} else {
		fmt.Fprintf(stdout, "Tag %s already points at %s, reusing it\n", tag, sha)
	}
	return nil
}

// appendTo appends text to the file at path. An empty path, as when running outside GitHub
// Actions, is skipped.
func appendTo(path, text string) error {
	if path == "" {
		return nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}

	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return err
	}

	return f.Close()
}
