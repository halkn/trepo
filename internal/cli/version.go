package cli

import "runtime/debug"

// versionLine describes the running build in one line.
//
// Read out of the build information rather than stamped in with -ldflags:
// `go install <module>@<version>` is how trepo is installed, and that records
// the version on its own. A build script that had to be run for the answer to
// be right would be a second way to build trepo.
//
// Nothing is added to what go records. A build from a checkout is already
// stamped with a pseudo-version carrying the commit and a +dirty marker
// (observed with go 1.26.6), so naming the revision again would only repeat it.
func versionLine(info *debug.BuildInfo) string {
	if info == nil || info.Main.Version == "" {
		return "trepo unknown"
	}
	return "trepo " + info.Main.Version
}
