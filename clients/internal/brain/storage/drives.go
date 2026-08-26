package storage

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// Drive is somewhere the brain could live.
type Drive struct {
	MountPoint string `json:"mount_point"`
	Filesystem string `json:"filesystem"`
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	Removable  bool   `json:"removable"`
	Current    bool   `json:"current"`
	Writable   bool   `json:"writable"`
}

// pseudoFilesystems never hold data and must not be offered.
var pseudoFilesystems = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true, "tmpfs": true,
	"securityfs": true, "cgroup": true, "cgroup2": true, "pstore": true,
	"efivarfs": true, "bpf": true, "autofs": true, "mqueue": true, "hugetlbfs": true,
	"debugfs": true, "tracefs": true, "fusectl": true, "configfs": true,
	"ramfs": true, "binfmt_misc": true, "squashfs": true, "overlay": true,
	"nsfs": true, "fuse.portal": true, "fuse.gvfsd-fuse": true,
}

// MinimumUsableBytes is the smallest destination worth offering.
//
// A brain starts small but grows with everything it reads, and moving it onto a
// 512MB EFI partition would succeed and then immediately fail. Offering only
// somewhere it can actually live is kinder than allowing a choice that breaks.
const MinimumUsableBytes = 5 << 30 // 5GB

// Drives lists the mounted filesystems the brain could be moved to.
//
// Only mounted ones. A drive that is plugged in but not mounted cannot be
// written to, and mounting it needs root — so it is left out rather than
// offered as something that will fail.
func Drives(currentRoot string) ([]Drive, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, fmt.Errorf("could not read mounted filesystems: %w", err)
	}
	defer f.Close()

	currentDevice := deviceOf(currentRoot)
	seen := map[string]bool{}

	// One entry per filesystem, not per mount point. Bind mounts and snap
	// packages put the same disk under several paths — /var/snap/firefox/...
	// is the root filesystem again — and offering it twice invites somebody to
	// "move to another drive" that is the drive they are already on.
	byDevice := map[string]Drive{}

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		mount := unescapeMount(fields[1])
		fstype := fields[2]

		if pseudoFilesystems[fstype] || seen[mount] {
			continue
		}

		seen[mount] = true

		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount, &stat); err != nil {
			continue
		}

		block := uint64(stat.Bsize)
		d := Drive{
			MountPoint: mount,
			Filesystem: fstype,
			TotalBytes: stat.Blocks * block,
			// Bavail, not Bfree: the difference is reserved for root and this
			// process cannot use it.
			FreeBytes: stat.Bavail * block,
			Removable: isRemovable(mount),
			Current:   deviceOf(mount) == currentDevice && currentDevice != "",
			Writable:  canWrite(mount),
		}

		if d.TotalBytes < MinimumUsableBytes {
			continue
		}

		device := deviceOf(mount)

		// Keep the shallowest path for a filesystem: "/" is a more useful and
		// more honest answer than a snap's private bind mount of it.
		if existing, ok := byDevice[device]; ok {
			if len(existing.MountPoint) <= len(d.MountPoint) {
				continue
			}
		}

		byDevice[device] = d
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	out := make([]Drive, 0, len(byDevice))

	for _, d := range byDevice {
		out = append(out, d)
	}

	// Most room first: the useful order when choosing where a growing thing
	// should go. The drive already holding the brain sorts first regardless, so
	// it is obvious which one that is.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Current != out[j].Current {
			return out[i].Current
		}

		return out[i].FreeBytes > out[j].FreeBytes
	})

	return out, nil
}

// isRemovable reports whether a mount point looks like external media.
//
// Judged by where the desktop mounts such things rather than by querying the
// kernel, because the distinction that matters here is "can be unplugged",
// which is a fact about how somebody uses a drive rather than about the device.
func isRemovable(mount string) bool {
	for _, prefix := range []string{"/media/", "/run/media/", "/mnt/"} {
		if strings.HasPrefix(mount, prefix) {
			return true
		}
	}

	return false
}

func canWrite(dir string) bool {
	probe := filepath.Join(dir, ".pn-brain-write-probe")

	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}

	f.Close()
	os.Remove(probe)

	return true
}

// deviceOf identifies the filesystem a path sits on, so two mount points on the
// same device are not mistaken for two places to put things.
func deviceOf(path string) string {
	var stat syscall.Stat_t

	if err := syscall.Stat(path, &stat); err != nil {
		return ""
	}

	return fmt.Sprintf("%d", stat.Dev)
}

// unescapeMount decodes the octal escapes /proc/mounts uses for spaces and
// other awkward characters in a path.
func unescapeMount(s string) string {
	replacer := strings.NewReplacer(
		`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`,
	)

	return replacer.Replace(s)
}
