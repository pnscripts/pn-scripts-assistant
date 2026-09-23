// One module for every PN Scripts Assistant client so they can share the preflight logic
// instead of each carrying a copy. The desktop app needs to know what the
// machine is missing before the brain exists, which is exactly what the doctor
// knows — duplicating that would guarantee the two drift apart.
module pn-scripts-assistant

go 1.25.0

// The toolchain this is built with, pinned rather than left to whatever the
// machine happens to have.
//
// govulncheck against go1.26.0 reported twenty-eight vulnerabilities in the
// standard library alone — crypto/x509, crypto/tls and net/http among them,
// all three of which this program serves with. Raising this is how they are
// fixed: there is nothing to change in this code. The go command fetches the
// toolchain when it is not already here, so a build on a clean machine and a
// build on this one are the same build.
//
// Worth re-running govulncheck when this is raised again, and worth raising
// it when a scan says so.
toolchain go1.26.6

require (
	github.com/yalue/onnxruntime_go v1.36.0
	golang.org/x/sys v0.47.0
	modernc.org/sqlite v1.57.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
