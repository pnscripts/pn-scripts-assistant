/*
 * Package seed is the knowledge a new brain starts with, carried inside the
 * program.
 *
 * Only what every user needs (owner 2026-10-23): how to start, the portable
 * copy and USB, the scam guard, how experts get their models and tools, and
 * where missing models and programs may come from. The playbooks themselves
 * ship in the skills package; nothing about any one person or company is
 * here.
 *
 * Install writes the pack into the brain's knowledge folder, which is always
 * learned and travels with the brain. A manifest records the hash of every
 * file as it was shipped, which is what makes an upgrade safe: a file still
 * holding what was shipped is replaced by the new version, a file its owner
 * edited is left alone, and a file the owner added is never looked at.
 */
package seed

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

//go:embed pack
var pack embed.FS

// Version is the pack's version; raise it when a file in pack/ changes.
const Version = 13

// ManifestName is the record of what was shipped, inside the knowledge folder.
const ManifestName = ".seed.json"

type manifest struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
}

// Result is what one Install did, by path relative to the knowledge folder.
type Result struct {
	Written []string
	Updated []string
	Kept    []string
}

// Changed reports whether anything was written.
func (r Result) Changed() bool { return len(r.Written)+len(r.Updated) > 0 }

// Files is every path in the pack, relative and slash-separated.
func Files() ([]string, error) {
	var out []string

	err := fs.WalkDir(pack, "pack", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		out = append(out, p[len("pack/"):])

		return nil
	})

	sort.Strings(out)

	return out, err
}

// Install writes the pack into the knowledge folder dir.
func Install(dir string) (Result, error) {
	var res Result

	old := manifest{Files: map[string]string{}}

	if raw, err := os.ReadFile(filepath.Join(dir, ManifestName)); err == nil {
		if err := json.Unmarshal(raw, &old); err != nil || old.Files == nil {
			old = manifest{Files: map[string]string{}}
		}
	}

	files, err := Files()
	if err != nil {
		return res, err
	}

	next := manifest{Version: Version, Files: map[string]string{}}

	for _, rel := range files {
		body, err := pack.ReadFile(path.Join("pack", rel))
		if err != nil {
			return res, err
		}

		shipped := sum(body)
		target := filepath.Join(dir, filepath.FromSlash(rel))

		current, err := os.ReadFile(target)

		switch {
		case errors.Is(err, fs.ErrNotExist):
			if _, wasShipped := old.Files[rel]; wasShipped {
				// Deleted by its owner after it was shipped: stays deleted.
				res.Kept = append(res.Kept, rel)
				next.Files[rel] = old.Files[rel]

				continue
			}

			if err := write(target, body); err != nil {
				return res, err
			}

			res.Written = append(res.Written, rel)

		case err != nil:
			return res, err

		case sum(current) == shipped:
			// Already the shipped text.

		case old.Files[rel] != "" && sum(current) == old.Files[rel]:
			// Untouched since an older pack shipped it: the new text replaces it.
			if err := write(target, body); err != nil {
				return res, err
			}

			res.Updated = append(res.Updated, rel)

		default:
			// Edited by its owner, or theirs from the start: never overwritten.
			res.Kept = append(res.Kept, rel)

			if prev := old.Files[rel]; prev != "" {
				next.Files[rel] = prev

				continue
			}
		}

		next.Files[rel] = shipped
	}

	if !res.Changed() && old.Version == Version && len(old.Files) == len(next.Files) {
		return res, nil
	}

	raw, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return res, err
	}

	return res, write(filepath.Join(dir, ManifestName), raw)
}

func write(target string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(target), err)
	}

	tmp := target + ".seed-tmp"

	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, target)
}

func sum(b []byte) string {
	h := sha256.Sum256(b)

	return hex.EncodeToString(h[:])
}
