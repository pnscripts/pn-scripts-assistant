package learning

import (
	"path/filepath"
	"strings"
	"sync"
)

/*
 * Paths inside the brain are remembered relative to the brain.
 *
 * The knowledge folder travels with the brain — a USB stick, another
 * computer, Remote — so its absolute path changes while its files do not.
 * Stored as "/media/stick/BRAIN/knowledge/guide.md", every fact learned
 * from it was tied to where the brain happened to be: moved, the folder was
 * learned again under the new path and the old facts stayed as duplicates.
 * Stored as "@brain/knowledge/guide.md", it is the same file wherever the
 * brain is.
 */

// BrainPrefix starts a path that is relative to the brain's own folder.
const BrainPrefix = "@brain/"

var (
	rootMu    sync.RWMutex
	brainRoot string
)

// SetBrainRoot says where the brain is for this run.
func SetBrainRoot(root string) {
	rootMu.Lock()
	defer rootMu.Unlock()

	if root != "" {
		root = filepath.Clean(root)
	}

	brainRoot = root
}

func currentRoot() string {
	rootMu.RLock()
	defer rootMu.RUnlock()

	return brainRoot
}

// Portable is path as it is remembered: relative to the brain when it is
// inside it, unchanged otherwise.
func Portable(path string) string {
	root := currentRoot()
	if root == "" || path == "" || strings.HasPrefix(path, BrainPrefix) {
		return path
	}

	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}

	return BrainPrefix + filepath.ToSlash(rel)
}

// Resolve is a remembered path as it is on this machine now.
func Resolve(path string) string {
	rest, ok := strings.CutPrefix(path, BrainPrefix)
	if !ok {
		return path
	}

	root := currentRoot()
	if root == "" {
		return path
	}

	return filepath.Join(root, filepath.FromSlash(rest))
}
