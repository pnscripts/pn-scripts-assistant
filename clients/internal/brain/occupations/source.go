package occupations

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Where an import reads from: a folder somebody downloaded, or the zip it
 * came in.
 *
 * Never the network. In the strictest setting there is no web at all, and a
 * button that quietly fetched forty megabytes would be the program lying about
 * that; and an offline machine should not be a lesser one. So the download is
 * somebody's, and this reads what they downloaded — unzipped or not, since
 * both classifications arrive as a zip and making a person unpack one first
 * is a step for no reason.
 */

// Source is a folder or a zip, read by file name.
type Source struct {
	path  string
	zip   *zip.ReadCloser
	names []string
}

// OpenSource opens a folder or a .zip.
func OpenSource(path string) (*Source, error) {
	path = strings.TrimSpace(path)

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("there is nothing at %s", path)
	}

	src := &Source{path: path}

	if !info.IsDir() {
		if src.zip, err = zip.OpenReader(path); err != nil {
			return nil, fmt.Errorf("%s is neither a folder nor a zip that can be read", filepath.Base(path))
		}

		for _, f := range src.zip.File {
			if !f.FileInfo().IsDir() {
				src.names = append(src.names, f.Name)
			}
		}

		return src, nil
	}

	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(path, p)
			src.names = append(src.names, rel)
		}

		return nil
	})

	sort.Strings(src.names)

	return src, err
}

// Close lets the zip go, when there is one.
func (s *Source) Close() error {
	if s.zip != nil {
		return s.zip.Close()
	}

	return nil
}

/*
 * Find is the file whose name ends with one of these, ignoring case and
 * folders — the classifications nest their files differently in every release,
 * and the name is the only thing they keep.
 */
func (s *Source) Find(endings ...string) (string, bool) {
	for _, ending := range endings {
		ending = strings.ToLower(ending)

		for _, name := range s.names {
			if strings.HasSuffix(strings.ToLower(filepath.ToSlash(name)), ending) {
				return name, true
			}
		}
	}

	return "", false
}

// All is every file matching a pattern on its base name, for the ones that
// come once per language.
func (s *Source) All(match func(base string) bool) []string {
	out := []string{}

	for _, name := range s.names {
		if match(strings.ToLower(filepath.Base(name))) {
			out = append(out, name)
		}
	}

	return out
}

// Open reads one file.
func (s *Source) Open(name string) (io.ReadCloser, error) {
	if s.zip != nil {
		return s.zip.Open(filepath.ToSlash(name))
	}

	return os.Open(filepath.Join(s.path, name))
}

/*
 * table reads a delimited file a row at a time, by column name.
 *
 * By name and never by position: both classifications have added columns
 * between releases, and a reader counting commas would read a job's
 * description out of its modification date without noticing. A file missing
 * a column this needs is refused by name, which says exactly what changed.
 */
func (s *Source) table(ctx context.Context, name string, comma rune, needs []string, row func(get func(string) string) error) error {
	file, err := s.Open(name)
	if err != nil {
		return err
	}

	defer file.Close()

	r := csv.NewReader(file)
	r.Comma = comma
	r.FieldsPerRecord = -1

	// Loose about quotes, because O*NET's text files use none and a quote in a
	// task statement is a quote, and ESCO's descriptions contain every kind.
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("%s has no header: %w", filepath.Base(name), err)
	}

	at := map[string]int{}

	for i, h := range header {
		at[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF")))] = i
	}

	for _, need := range needs {
		if _, ok := at[strings.ToLower(need)]; !ok {
			return fmt.Errorf("%s has no %q column — this is not the file it looks like, "+
				"or its format has changed", filepath.Base(name), need)
		}
	}

	for n := 0; ; n++ {
		if n%500 == 0 && ctx.Err() != nil {
			return ctx.Err()
		}

		record, err := r.Read()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			// One malformed line is skipped, not the file: a stray quote in
			// row twelve thousand must not cost the other thirteen.
			continue
		}

		get := func(column string) string {
			i, ok := at[strings.ToLower(column)]
			if !ok || i >= len(record) {
				return ""
			}

			return strings.TrimSpace(record[i])
		}

		if err := row(get); err != nil {
			return err
		}
	}
}

/*
 * index is what the catalogue already calls things, so an import enriches a
 * job it has under another source's spelling instead of adding a second one.
 *
 * A name that two different rows already answer to is ambiguous, and nothing
 * is merged into either on the strength of it: the import makes its own row
 * and says so in its notes, because guessing which of two jobs somebody meant
 * is how two careers end up sharing one description.
 */
type index struct {
	byName    map[string]string
	ambiguous map[string]bool
}

func newIndex(rows []store.Occupation) *index {
	ix := &index{byName: map[string]string{}, ambiguous: map[string]bool{}}

	for _, row := range rows {
		for _, name := range append([]string{row.Title}, row.Aliases...) {
			ix.add(name, row.ID)
		}
	}

	return ix
}

func (ix *index) add(name, id string) {
	key := strings.ToLower(strings.TrimSpace(name))

	if key == "" {
		return
	}

	if have, ok := ix.byName[key]; ok && have != id {
		ix.ambiguous[key] = true

		return
	}

	ix.byName[key] = id
}

// match is the one row these names all agree on, or nothing.
func (ix *index) match(names ...string) (id string, clash string) {
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))

		if ix.ambiguous[key] {
			return "", name
		}

		if found, ok := ix.byName[key]; ok {
			return found, ""
		}
	}

	return "", ""
}

// Progress is how far an import has got, for whoever is watching.
type Progress struct {
	Stage string
	Done  int
	Of    int
	So    store.Imported
	Notes []string
}

func (p *Progress) add(done store.Imported) {
	p.So.Jobs += done.Jobs
	p.So.Capabilities += done.Capabilities
	p.So.Links += done.Links
	p.So.Filled += done.Filled
	p.So.Left += done.Left
}

func (p *Progress) note(line string) {
	// Enough to see the shape of a problem, not every instance of it.
	if len(p.Notes) < 25 {
		p.Notes = append(p.Notes, line)
	}
}
