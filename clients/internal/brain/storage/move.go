package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MoveReport describes what a move did.
type MoveReport struct {
	From       string
	To         string
	Files      int
	Bytes      int64
	Verified   int
	SourceKept bool
}

// Move relocates the brain's data to another drive.
//
// Moving rather than splitting is deliberate. The data could be spread across
// drives, but then the brain only works when every one of them is attached, and
// a missing drive is a corrupt brain rather than a smaller one. A brain that
// lives on exactly one drive at a time can be carried, backed up and reasoned
// about, and that is worth more than squeezing two drives together.
//
// The order is copy, verify, then remove — never rename or move-and-hope. A
// rename across filesystems is a copy and a delete with no verification
// between them, and the failure mode is a half-written brain and no original.
// Nothing is deleted from the source until every byte has been read back from
// the destination and checked.
//
// progress is called with each file as it is copied; nil is fine.
func Move(from, to string, keepSource bool, progress func(path string, done, total int)) (MoveReport, error) {
	rep := MoveReport{From: from, To: to, SourceKept: keepSource}

	if from == "" || to == "" {
		return rep, fmt.Errorf("both a source and a destination are needed")
	}

	absFrom, err := filepath.Abs(from)
	if err != nil {
		return rep, err
	}

	absTo, err := filepath.Abs(to)
	if err != nil {
		return rep, err
	}

	if absFrom == absTo {
		return rep, fmt.Errorf("the brain is already at %s", absTo)
	}

	// Copying a directory into itself would recurse until the disk filled.
	if strings.HasPrefix(absTo+string(filepath.Separator), absFrom+string(filepath.Separator)) {
		return rep, fmt.Errorf("cannot move %s into itself (%s)", absFrom, absTo)
	}

	files, total, err := inventory(absFrom)
	if err != nil {
		return rep, err
	}

	rep.Files = len(files)
	rep.Bytes = total

	// Refuse before starting rather than filling the destination and failing
	// part-way. The margin covers the filesystem's own overhead.
	var free uint64

	for _, d := range mustDrives(absTo) {
		free = d.FreeBytes

		break
	}

	if free > 0 && uint64(total)+(256<<20) > free {
		return rep, fmt.Errorf(
			"not enough room: %s needs %.1fGB and %s has %.1fGB free",
			absFrom, float64(total)/(1<<30), absTo, float64(free)/(1<<30))
	}

	if err := os.MkdirAll(absTo, 0o755); err != nil {
		return rep, fmt.Errorf("could not create %s: %w", absTo, err)
	}

	// Refuse to write into a directory that already holds a brain: merging two
	// would silently mix two histories.
	if _, err := os.Stat(filepath.Join(absTo, ".brain-root.json")); err == nil {
		return rep, fmt.Errorf("%s already holds a brain; choose an empty folder", absTo)
	}

	for i, rel := range files {
		src := filepath.Join(absFrom, rel)
		dst := filepath.Join(absTo, rel)

		if err := copyOne(src, dst); err != nil {
			return rep, fmt.Errorf("copying %s: %w", rel, err)
		}

		if progress != nil {
			progress(rel, i+1, len(files))
		}
	}

	// Verify by reading both copies back. A copy that reported success and
	// wrote nothing is exactly the failure this guards against, and it is
	// silent without this step.
	for _, rel := range files {
		same, err := identical(filepath.Join(absFrom, rel), filepath.Join(absTo, rel))
		if err != nil {
			return rep, fmt.Errorf("verifying %s: %w", rel, err)
		}

		if !same {
			return rep, fmt.Errorf(
				"%s did not copy correctly; nothing has been deleted from %s", rel, absFrom)
		}

		rep.Verified++
	}

	if keepSource {
		return rep, nil
	}

	if err := os.RemoveAll(absFrom); err != nil {
		// The move succeeded; only the cleanup failed. Say exactly that, so
		// nobody concludes their brain is gone.
		return rep, fmt.Errorf(
			"the brain is safely at %s, but %s could not be removed: %w", absTo, absFrom, err)
	}

	return rep, nil
}

// inventory lists every regular file under root, relative to it.
func inventory(root string) ([]string, int64, error) {
	var files []string
	var total int64

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Symlinks are not followed: a link pointing outside the brain would
		// drag unrelated data along, and one pointing inside would duplicate it.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		files = append(files, rel)
		total += info.Size()

		return nil
	})

	return files, total, err
}

func copyOne(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()

		return err
	}

	// Sync before reporting success. Without it the bytes may still be in the
	// kernel's cache, and a verification that reads them back from cache proves
	// nothing about what reached the disk.
	if err := out.Sync(); err != nil {
		out.Close()

		return err
	}

	return out.Close()
}

// identical compares two files by size and content hash.
func identical(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, err
	}

	bi, err := os.Stat(b)
	if err != nil {
		return false, err
	}

	if ai.Size() != bi.Size() {
		return false, nil
	}

	ah, err := hashFile(a)
	if err != nil {
		return false, err
	}

	bh, err := hashFile(b)
	if err != nil {
		return false, err
	}

	return ah == bh, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()

	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// mustDrives returns the drive holding path, or nothing if it cannot be read.
func mustDrives(path string) []Drive {
	all, err := Drives("")
	if err != nil {
		return nil
	}

	var best Drive
	var found bool

	for _, d := range all {
		if !strings.HasPrefix(path, d.MountPoint) {
			continue
		}

		// The longest matching mount point is the filesystem the path is on.
		if !found || len(d.MountPoint) > len(best.MountPoint) {
			best = d
			found = true
		}
	}

	if !found {
		return nil
	}

	return []Drive{best}
}
