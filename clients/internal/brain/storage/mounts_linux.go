//go:build linux

package storage

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// pseudoFilesystems never hold data and must not be offered.
var pseudoFilesystems = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true, "tmpfs": true,
	"securityfs": true, "cgroup": true, "cgroup2": true, "pstore": true,
	"efivarfs": true, "bpf": true, "autofs": true, "mqueue": true, "hugetlbfs": true,
	"debugfs": true, "tracefs": true, "fusectl": true, "configfs": true,
	"ramfs": true, "binfmt_misc": true, "squashfs": true, "overlay": true,
	"nsfs": true, "fuse.portal": true, "fuse.gvfsd-fuse": true,
}

// mountPoints reads what the kernel has mounted.
func mountPoints() ([]mount, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, fmt.Errorf("could not read mounted filesystems: %w", err)
	}
	defer f.Close()

	seen := map[string]bool{}

	var out []mount

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		path := unescapeMount(fields[1])

		if pseudoFilesystems[fields[2]] || seen[path] {
			continue
		}

		seen[path] = true
		out = append(out, mount{Path: path, Type: fields[2]})
	}

	return out, scanner.Err()
}

// unescapeMount decodes the octal escapes /proc/mounts uses for spaces and
// other awkward characters in a path.
func unescapeMount(s string) string {
	return strings.NewReplacer(
		`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`,
	).Replace(s)
}
