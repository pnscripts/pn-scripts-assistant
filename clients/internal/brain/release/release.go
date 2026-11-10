/*
 * Package release is what this copy of the program is.
 *
 * There was no answer to "which version is this?" — the build wrote one
 * number into the package name, the program reported the first seven
 * characters of a commit hash, and the desktop entry said nothing at all.
 * Three answers, none of them the same, and none of them a version somebody
 * could quote back when something went wrong.
 *
 * One source now: the VERSION file at the top of the repository, read by the
 * build and written in here. A build that did not go through the release
 * script says so rather than inventing a number, because "built from source"
 * is true and "0.1.0" would not be.
 */
package release

import (
	"runtime/debug"
	"strings"
)

/*
 * Set at build time with -ldflags "-X …/release.Version=…".
 *
 * Variables rather than constants for that reason, and unexported behind
 * functions so that nothing can accidentally compare against the placeholder.
 */
var (
	Version = ""
	Commit  = ""
	Built   = ""

	// Mark is the version again, as one string a file can be searched for
	// without running it: "PNSA-RELEASE[1.0.1]". The build record cannot
	// say it, because -trimpath leaves -ldflags out of the record, and a
	// program built for another system cannot be asked. 1.0.0 published its
	// standalone Linux program without a version; scripts/check-versions.sh
	// looks for this in every file a release publishes.
	Mark = ""
)

// Is the version of this copy: the released number when there is one, the
// commit when it was built by hand, and an honest phrase when there is
// neither.
func Is() string {
	if Version != "" {
		return Version
	}

	if commit := Revision(); commit != "" {
		return "built from source (" + commit + ")"
	}

	return "built from source"
}

// Revision is the commit this was built from, short, or empty.
func Revision() string {
	if Commit != "" {
		return short(Commit)
	}

	info, readable := debug.ReadBuildInfo()
	if !readable {
		return ""
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return short(setting.Value)
		}
	}

	return ""
}

// When it was built, or empty. Set by the release build so that two copies of
// the same version can still be told apart.
func When() string { return Built }

/*
 * Full is the line a person quotes back when something went wrong.
 *
 * Everything that is known, in one line: the version, what it was built from,
 * and when. Nothing invented — a part that is not known is left out rather
 * than filled in.
 */
func Full(product string) string {
	line := product + " " + Is()

	if commit := Revision(); commit != "" && Version != "" {
		line += " (" + commit + ")"
	}

	if Built != "" {
		line += ", built " + Built
	}

	// Two stamps that disagree mean the build was put together by hand from
	// two places; say so rather than trust either. (Reading Mark here is
	// also what keeps it in the program for the release check to find.)
	if Version != "" && Mark != "" && !strings.HasSuffix(Mark, "["+Version+"]") {
		line += ", stamped inconsistently (" + Mark + ")"
	}

	return line
}

func short(commit string) string {
	if commit = strings.TrimSpace(commit); len(commit) >= 7 {
		return commit[:7]
	}

	return commit
}
