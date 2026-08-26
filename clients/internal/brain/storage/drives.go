package storage

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	mounts, err := mountPoints()
	if err != nil {
		return nil, err
	}

	currentDevice := deviceOf(currentRoot)

	// One entry per filesystem, not per mount point. Bind mounts and snap
	// packages put the same disk under several paths — /var/snap/firefox/...
	// is the root filesystem again — and offering it twice invites somebody to
	// "move to another drive" that is the drive they are already on.
	byDevice := map[string]Drive{}

	for _, m := range mounts {
		total, free, err := spaceOn(m.Path)
		if err != nil {
			continue
		}

		d := Drive{
			MountPoint: m.Path,
			Filesystem: m.Type,
			TotalBytes: total,
			FreeBytes:  free,
			Removable:  isRemovable(m.Path),
			Current:    deviceOf(m.Path) == currentDevice && currentDevice != "",
			Writable:   canWrite(m.Path),
		}

		if d.TotalBytes < MinimumUsableBytes {
			continue
		}

		device := deviceOf(m.Path)

		// Keep the shallowest path for a filesystem: "/" is a more useful and
		// more honest answer than a snap's private bind mount of it.
		if existing, ok := byDevice[device]; ok {
			if len(existing.MountPoint) <= len(d.MountPoint) {
				continue
			}
		}

		byDevice[device] = d
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

// mount is one filesystem the system has mounted.
type mount struct {
	Path string
	Type string
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
