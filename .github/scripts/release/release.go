package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// version is a strict MAJOR.MINOR.PATCH release version.
type version struct {
	major, minor, patch int
}

var versionTag = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

// parseVersionTag parses a tag such as "v0.2.1". Tags that are not strict MAJOR.MINOR.PATCH
// versions, for example pre-releases, are reported as not ok.
func parseVersionTag(tag string) (version, bool) {
	m := versionTag.FindStringSubmatch(tag)
	if m == nil {
		return version{}, false
	}

	var v version
	for i, dst := range []*int{&v.major, &v.minor, &v.patch} {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return version{}, false
		}
		*dst = n
	}

	return v, true
}

func (v version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
}

// less reports whether v is an older version than o, comparing the numbers, not the text.
func (v version) less(o version) bool {
	if v.major != o.major {
		return v.major < o.major
	}
	if v.minor != o.minor {
		return v.minor < o.minor
	}
	return v.patch < o.patch
}

// bump returns the version that follows v for the given release type.
func (v version) bump(releaseType string) (version, error) {
	switch releaseType {
	case "major":
		return version{v.major + 1, 0, 0}, nil
	case "minor":
		return version{v.major, v.minor + 1, 0}, nil
	case "patch":
		return version{v.major, v.minor, v.patch + 1}, nil
	default:
		return version{}, fmt.Errorf("unknown release type %q, expected major, minor or patch", releaseType)
	}
}

// nextVersion computes the version that follows the latest strict version among tags. Tags
// that are not strict versions are ignored. Without any version tag the base is v0.0.0. The
// result is always greater than every existing version tag.
func nextVersion(tags []string, releaseType string) (latest, next version, err error) {
	for _, tag := range tags {
		v, ok := parseVersionTag(tag)
		if ok && latest.less(v) {
			latest = v
		}
	}

	next, err = latest.bump(releaseType)
	if err != nil {
		return version{}, version{}, err
	}

	return latest, next, nil
}

// requireMain fails unless ref is the main branch, the only branch releases may start from.
func requireMain(ref string) error {
	if ref != "refs/heads/main" {
		return fmt.Errorf("releases can only be started from main, not from %s", ref)
	}
	return nil
}

// changelogDateLayout is how release dates are written in CHANGELOG.md, for example
// "October 13, 2026".
const changelogDateLayout = "January 2, 2006"

// checkChangelog requires a heading of the form "## 0.2.0 (October 13, 2026)" for the given
// version tag, where the date is an actual calendar date. The release archives ship
// CHANGELOG.md, so a missing or undated entry such as "(Unreleased)" is an error.
func checkChangelog(changelog, tag string) error {
	plain := strings.TrimPrefix(tag, "v")
	heading := regexp.MustCompile(`(?m)^## ` + regexp.QuoteMeta(plain) + ` \(([A-Z][a-z]+ [0-9]{1,2}, [0-9]{4})\)$`)

	m := heading.FindStringSubmatch(changelog)
	if m == nil {
		return fmt.Errorf("CHANGELOG.md needs a dated heading for %s, for example '## %s (October 13, 2026)'", plain, plain)
	}

	if _, err := time.Parse(changelogDateLayout, m[1]); err != nil {
		return fmt.Errorf("CHANGELOG.md heading for %s has the date %q, which is not a valid date like 'October 13, 2026'", plain, m[1])
	}

	return nil
}
