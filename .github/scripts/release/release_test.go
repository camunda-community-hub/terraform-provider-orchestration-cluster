package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVersionTag(t *testing.T) {
	tests := []struct {
		tag  string
		want version
		ok   bool
	}{
		{"v0.1.1", version{0, 1, 1}, true},
		{"v10.20.30", version{10, 20, 30}, true},
		{"v0.0.0", version{0, 0, 0}, true},
		{"v1.2", version{}, false},
		{"v1", version{}, false},
		{"1.2.3", version{}, false},
		{"v1.2.3-rc.1", version{}, false},
		{"v1.2.3+build", version{}, false},
		{"release-1.2.3", version{}, false},
		{"", version{}, false},
	}

	for _, tt := range tests {
		got, ok := parseVersionTag(tt.tag)
		if ok != tt.ok || got != tt.want {
			t.Errorf("parseVersionTag(%q) = %v, %v; want %v, %v", tt.tag, got, ok, tt.want, tt.ok)
		}
	}
}

func TestNextVersion(t *testing.T) {
	tests := []struct {
		name        string
		tags        []string
		releaseType string
		wantLatest  string
		wantNext    string
		wantErr     bool
	}{
		{"patch", []string{"v0.1.0", "v0.1.1"}, "patch", "v0.1.1", "v0.1.2", false},
		{"minor resets patch", []string{"v0.1.0", "v0.1.1"}, "minor", "v0.1.1", "v0.2.0", false},
		{"major resets minor and patch", []string{"v0.1.0", "v0.1.1"}, "major", "v0.1.1", "v1.0.0", false},
		{"no tags, minor", nil, "minor", "v0.0.0", "v0.1.0", false},
		{"no tags, patch", nil, "patch", "v0.0.0", "v0.0.1", false},
		{"numeric, not textual, ordering", []string{"v0.9.0", "v0.10.0", "v0.2.0"}, "patch", "v0.10.0", "v0.10.1", false},
		{"pre-releases and other tags are ignored", []string{"v0.1.0", "v0.2.0-rc.1", "latest", "v1"}, "patch", "v0.1.0", "v0.1.1", false},
		{"unsorted input", []string{"v1.0.0", "v0.3.0", "v1.0.1"}, "minor", "v1.0.1", "v1.1.0", false},
		{"unknown release type", []string{"v0.1.0"}, "bogus", "", "", true},
		{"empty release type", []string{"v0.1.0"}, "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			latest, next, err := nextVersion(tt.tags, tt.releaseType)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if latest.String() != tt.wantLatest || next.String() != tt.wantNext {
				t.Errorf("got latest %s, next %s; want %s, %s", latest, next, tt.wantLatest, tt.wantNext)
			}
		})
	}
}

func TestRequireMain(t *testing.T) {
	if err := requireMain("refs/heads/main"); err != nil {
		t.Errorf("main rejected: %v", err)
	}

	for _, ref := range []string{"refs/heads/feature", "refs/tags/v1.0.0", "refs/heads/main2", ""} {
		if err := requireMain(ref); err == nil {
			t.Errorf("requireMain(%q) = nil, want an error", ref)
		}
	}
}

func TestCheckChangelog(t *testing.T) {
	changelog := strings.Join([]string{
		"## 0.3.0 (Unreleased)",
		"",
		"## 0.2.0 (October 13, 2026)",
		"",
		"## 0.1.1 (October 3, 2026)",
		"",
		"## 0.1.0 (October 8, 2026) extra",
		"",
		"## 0x2x0 (October 13, 2026)",
		"",
		"## 0.5.0 (Octember 99, 2026)",
		"",
		"## 0.5.1 (February 30, 2026)",
		"",
		"## 0.5.2 (October 0, 2026)",
		"",
		"## 0.5.3 (February 28, 2027)",
	}, "\n")

	tests := []struct {
		name    string
		tag     string
		wantErr bool
	}{
		{"dated entry", "v0.2.0", false},
		{"single digit day", "v0.1.1", false},
		{"undated entry", "v0.3.0", true},
		{"missing entry", "v0.4.0", true},
		{"trailing text after the date", "v0.1.0", true},
		{"dots are literal, not wildcards", "v0.2.0x", true},
		{"version with a similar prefix", "v0.2", true},
		{"month that does not exist", "v0.5.0", true},
		{"day that does not exist in the month", "v0.5.1", true},
		{"day zero", "v0.5.2", true},
		{"valid date in another year", "v0.5.3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkChangelog(changelog, tt.tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkChangelog(%q) error = %v, wantErr %v", tt.tag, err, tt.wantErr)
			}
		})
	}
}

// testRepo is a work tree with two commits and a bare repository as origin.
type testRepo struct {
	g       git
	remote  git
	first   string
	second  string
	workDir string
}

func newTestRepo(t *testing.T) testRepo {
	t.Helper()

	// Keep the developer's own git configuration (signing, templates, hooks) out of the tests.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	root := t.TempDir()
	workDir := filepath.Join(root, "work")
	remoteDir := filepath.Join(root, "remote.git")
	for _, dir := range []string{workDir, remoteDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	remote := git{dir: remoteDir}
	mustRun(t, remote, "init", "--bare", "-b", "main")

	g := git{dir: workDir}
	mustRun(t, g, "init", "-b", "main")
	mustRun(t, g, "remote", "add", "origin", remoteDir)

	commit := func(message string) string {
		mustRun(t, g, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", message)
		return mustRun(t, g, "rev-parse", "HEAD")
	}

	first := commit("first")
	second := commit("second")

	return testRepo{g: g, remote: remote, first: first, second: second, workDir: workDir}
}

func mustRun(t *testing.T, g git, args ...string) string {
	t.Helper()

	out, err := g.run(args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGitTags(t *testing.T) {
	r := newTestRepo(t)

	got, err := r.g.tags()
	if err != nil || len(got) != 0 {
		t.Fatalf("tags() in a repository without tags = %v, %v; want none", got, err)
	}

	for _, tag := range []string{"v0.1.0", "v1", "release-1", "v0.2.0-rc.1"} {
		mustRun(t, r.g, "tag", tag)
	}

	got, err = r.g.tags()
	if err != nil {
		t.Fatal(err)
	}
	if want := "v0.1.0,v0.2.0-rc.1,v1"; strings.Join(got, ",") != want {
		t.Errorf("tags() = %v, want %s", got, want)
	}
}

func TestCreateTag(t *testing.T) {
	r := newTestRepo(t)

	t.Run("creates and pushes an annotated tag when absent", func(t *testing.T) {
		created, err := r.g.createTag("v1.2.3", r.second)
		if err != nil || !created {
			t.Fatalf("createTag = %v, %v; want created", created, err)
		}

		if got := mustRun(t, r.remote, "rev-list", "-n", "1", "refs/tags/v1.2.3"); got != r.second {
			t.Errorf("remote tag points at %s, want %s", got, r.second)
		}
		if got := mustRun(t, r.g, "cat-file", "-t", "refs/tags/v1.2.3"); got != "tag" {
			t.Errorf("tag object type = %s, want an annotated tag", got)
		}
	})

	t.Run("reuses an existing tag on the same commit", func(t *testing.T) {
		created, err := r.g.createTag("v1.2.3", r.second)
		if err != nil || created {
			t.Fatalf("createTag = %v, %v; want reused without error", created, err)
		}
	})

	t.Run("rejects an existing lightweight tag, even on the same commit", func(t *testing.T) {
		mustRun(t, r.g, "tag", "v2.0.0", r.second)

		_, err := r.g.createTag("v2.0.0", r.second)
		if err == nil || !strings.Contains(err.Error(), "lightweight") {
			t.Fatalf("createTag = %v, want a lightweight tag error", err)
		}
	})

	t.Run("pushes an existing local tag that origin does not have", func(t *testing.T) {
		mustRun(t, r.g, "-c", "user.name=test", "-c", "user.email=test@example.com", "tag", "-a", "v4.0.0", "-m", "v4.0.0", r.second)

		created, err := r.g.createTag("v4.0.0", r.second)
		if err != nil || created {
			t.Fatalf("createTag = %v, %v; want reused without error", created, err)
		}
		if got := mustRun(t, r.remote, "rev-list", "-n", "1", "refs/tags/v4.0.0"); got != r.second {
			t.Errorf("remote tag points at %s, want %s", got, r.second)
		}
	})

	t.Run("rejects an existing local tag when origin has a different one", func(t *testing.T) {
		tagger := []string{"-c", "user.name=test", "-c", "user.email=test@example.com"}
		mustRun(t, r.g, append(tagger, "tag", "-a", "v3.0.0", "-m", "first", r.first)...)
		mustRun(t, r.g, "push", "origin", "refs/tags/v3.0.0")
		mustRun(t, r.g, "tag", "-d", "v3.0.0")
		mustRun(t, r.g, append(tagger, "tag", "-a", "v3.0.0", "-m", "second", r.second)...)

		if _, err := r.g.createTag("v3.0.0", r.second); err == nil {
			t.Fatal("createTag succeeded although origin has a different tag with that name")
		}
		if got := mustRun(t, r.remote, "rev-list", "-n", "1", "refs/tags/v3.0.0"); got != r.first {
			t.Errorf("remote tag moved to %s, want it to stay on %s", got, r.first)
		}
	})

	t.Run("rejects an existing tag on a different commit", func(t *testing.T) {
		_, err := r.g.createTag("v1.2.3", r.first)
		if err == nil {
			t.Fatal("createTag succeeded for a tag on another commit")
		}
		if !strings.Contains(err.Error(), r.second) || !strings.Contains(err.Error(), r.first) {
			t.Errorf("error %q should name both commits", err)
		}
		if got := mustRun(t, r.remote, "rev-list", "-n", "1", "refs/tags/v1.2.3"); got != r.second {
			t.Errorf("remote tag moved to %s", got)
		}
	})
}

func fakeEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestRunNextVersionWritesOutputs(t *testing.T) {
	r := newTestRepo(t)
	for _, tag := range []string{"v0.1.0", "v0.1.1"} {
		mustRun(t, r.g, "tag", tag)
	}

	dir := t.TempDir()
	env := map[string]string{
		"RELEASE_TYPE":        "minor",
		"DRY_RUN":             "true",
		"GITHUB_SHA":          r.second,
		"GITHUB_OUTPUT":       filepath.Join(dir, "output"),
		"GITHUB_STEP_SUMMARY": filepath.Join(dir, "summary"),
	}

	var stdout bytes.Buffer
	if err := run([]string{"next-version"}, fakeEnv(env), r.g, &stdout); err != nil {
		t.Fatal(err)
	}

	output, _ := os.ReadFile(env["GITHUB_OUTPUT"])
	if string(output) != "version=v0.2.0\n" {
		t.Errorf("GITHUB_OUTPUT = %q", output)
	}

	summary, _ := os.ReadFile(env["GITHUB_STEP_SUMMARY"])
	for _, want := range []string{"### Release v0.2.0", "`v0.1.1`", "`minor`", "`true`"} {
		if !strings.Contains(string(summary), want) {
			t.Errorf("summary is missing %q:\n%s", want, summary)
		}
	}
}

func TestRunNextVersionWithoutGitHubFiles(t *testing.T) {
	r := newTestRepo(t)
	mustRun(t, r.g, "tag", "v0.1.1")

	var stdout bytes.Buffer
	env := map[string]string{"RELEASE_TYPE": "patch"}
	if err := run([]string{"next-version"}, fakeEnv(env), r.g, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "next v0.1.2") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRunCheckChangelog(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("CHANGELOG.md", []byte("## 0.2.0 (October 13, 2026)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"check-changelog"}, fakeEnv(map[string]string{"VERSION": "v0.2.0"}), git{}, &stdout); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"check-changelog"}, fakeEnv(map[string]string{"VERSION": "v0.3.0"}), git{}, &stdout); err == nil {
		t.Error("an undated or missing entry was accepted")
	}
	if err := run([]string{"check-changelog"}, fakeEnv(nil), git{}, &stdout); err == nil {
		t.Error("a missing VERSION was accepted")
	}
}

func TestRunCreateTagRequiresInputs(t *testing.T) {
	var stdout bytes.Buffer
	for _, env := range []map[string]string{nil, {"VERSION": "v1.0.0"}, {"GITHUB_SHA": "abc"}} {
		if err := run([]string{"create-tag"}, fakeEnv(env), git{}, &stdout); err == nil {
			t.Errorf("create-tag with %v succeeded", env)
		}
	}
}

func TestRunRejectsBadUsage(t *testing.T) {
	var stdout bytes.Buffer
	for _, args := range [][]string{nil, {"unknown"}, {"next-version", "extra"}} {
		if err := run(args, fakeEnv(nil), git{}, &stdout); err == nil {
			t.Errorf("run(%v) succeeded", args)
		}
	}

	if err := run([]string{"require-main"}, fakeEnv(map[string]string{"GITHUB_REF": "refs/heads/main"}), git{}, &stdout); err != nil {
		t.Errorf("require-main on main failed: %v", err)
	}
	if err := run([]string{"require-main"}, fakeEnv(map[string]string{"GITHUB_REF": "refs/heads/dev"}), git{}, &stdout); err == nil {
		t.Error("require-main accepted another branch")
	}
}
