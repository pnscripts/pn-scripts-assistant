//go:build linux

package preflight

import "golang.org/x/sys/unix"

// The request that reads terminal attributes, which Linux and the BSDs spell
// differently.
const tcGetAttr = unix.TCGETS
