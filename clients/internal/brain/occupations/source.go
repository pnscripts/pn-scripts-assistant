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

/*
 * Source is a folder, a zip, or a folder of zips, read by file name.
 *
 * A folder of zips because that is how ESCO actually arrives: one zip per
 * language, the English one and the Bulgarian one side by side, and the
 * Bulgarian names are what make a job findable here.
 */
type Source struct {
	path  string
	zips  []*zip.ReadCloser
	files map[string]func() (io.ReadCloser, error)
	names []string
}

// OpenSource opens a folder, a .zip, or a folder holding zips.
func OpenSource(path string) (*Source, error) {
	path = strings.TrimSpace(path)

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("there is nothing at %s", path)
	}

	src := &Source{path: path, files: map[string]func() (io.ReadCloser, error){}}

	if !info.IsDir() {
		if err := src.addZip(path, ""); err != nil {
			return nil, fmt.Errorf("%s is neither a folder nor a zip that can be read", filepath.Base(path))
		}

		return src, nil
	}

	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(path, p)

		if strings.EqualFold(filepath.Ext(p), ".zip") {
			// A zip that will not open is skipped, and the files beside it
			// are still read.
			src.addZip(p, rel+"/")

			return nil
		}

		full := p
		src.add(rel, func() (io.ReadCloser, error) { return os.Open(full) })

		return nil
	})

	sort.Strings(src.names)

	return src, err
}

func (s *Source) add(name string, open func() (io.ReadCloser, error)) {
	s.files[name] = open
	s.names = append(s.names, name)
}

func (s *Source) addZip(path, prefix string) error {
	z, err := zip.OpenReader(path)
	if err != nil {
		return err
	}

	s.zips = append(s.zips, z)

	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}

		f := f
		s.add(prefix+f.Name, f.Open)
	}

	return nil
}

// Close lets the zips go.
func (s *Source) Close() error {
	for _, z := range s.zips {
		z.Close()
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
	open, ok := s.files[name]
	if !ok {
		return nil, fmt.Errorf("there is no %s", name)
	}

	return open()
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
 * Two ways to recognise a row, strictest first. By the source's own code,
 * which is what makes running an import twice find every row it wrote rather
 * than reasoning about names again. And by the arriving title, against every
 * name a row answers to — ESCO's "software developer" is the software engineer
 * that ships, which already answers to that name.
 *
 * Never by the arriving job's other names. Those are what somebody might also
 * call it, not what it is: ESCO's business economics researcher is also called
 * a business analyst, and matching on that folded economists into the business
 * analyst that ships — 201 skills and a dozen titles that were not its own.
 * Before that, other name against other name chained a machine-tool programmer
 * into software engineer.
 *
 * A name that two different rows already answer to is ambiguous, and nothing
 * is merged into either on the strength of it: the import makes its own row
 * and says so in its notes.
 */
type index struct {
	byName    map[string]string
	byTitle   map[string]string
	bySource  map[string]string
	ambiguous map[string]bool
}

func newIndex(rows []store.Occupation) *index {
	ix := &index{byName: map[string]string{}, byTitle: map[string]string{},
		bySource: map[string]string{}, ambiguous: map[string]bool{}}

	for _, row := range rows {
		for _, name := range append([]string{row.Title}, row.Aliases...) {
			ix.add(name, row.ID)
		}

		if key := fold(row.Title); key != "" {
			if _, taken := ix.byTitle[key]; !taken {
				ix.byTitle[key] = row.ID
			}
		}

		for _, source := range row.Sources {
			ix.bySource[source] = row.ID
		}
	}

	return ix
}

func fold(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

func (ix *index) add(name, id string) {
	key := fold(name)

	if key == "" {
		return
	}

	if have, ok := ix.byName[key]; ok && have != id {
		ix.ambiguous[key] = true

		return
	}

	ix.byName[key] = id
}

// match is the row an arriving job is, or nothing — and the name that was
// ambiguous, when that is why.
func (ix *index) match(source, title string, aliases []string) (id string, clash string) {
	if found, ok := ix.bySource[source]; ok && source != "" {
		return found, ""
	}

	/*
	 * The title as written, and then as one of them.
	 *
	 * O*NET names every occupation in the plural — "Software Developers",
	 * "Chief Executives" — where everything else names one person. Without
	 * this, every O*NET job missed the job already here and arrived as a
	 * near-duplicate of it: nine hundred of them.
	 */
	for _, key := range []string{fold(title), singular(fold(title))} {
		if ix.ambiguous[key] {
			return "", title
		}

		if found, ok := ix.byName[key]; ok {
			return found, ""
		}
	}

	return "", ""
}

/*
 * named is the row whose own name this is, and only that.
 *
 * For capabilities, which carry no source code to be recognised by: matching
 * a skill's name against other skills' other names moved thirty links to a
 * different capability every time the same release was imported again.
 */
func (ix *index) named(name string) string {
	return ix.byTitle[fold(name)]
}

// singular is a plural job title as one person: the last word only, since
// "Sales Representatives" is a sales representative and not a sale one.
func singular(title string) string {
	words := strings.Fields(title)

	if len(words) == 0 {
		return title
	}

	last := words[len(words)-1]

	switch {
	case strings.HasSuffix(last, "ies") && len(last) > 4:
		last = strings.TrimSuffix(last, "ies") + "y"
	case strings.HasSuffix(last, "ches"), strings.HasSuffix(last, "shes"),
		strings.HasSuffix(last, "sses"), strings.HasSuffix(last, "xes"):
		last = strings.TrimSuffix(last, "es")
	case strings.HasSuffix(last, "s") && !strings.HasSuffix(last, "ss") && len(last) > 3:
		last = strings.TrimSuffix(last, "s")
	}

	words[len(words)-1] = last

	return strings.Join(words, " ")
}

// remember makes a row just decided on findable by the rest of the same run,
// so two arriving rows naming one job land on it together.
func (ix *index) remember(source, id string) {
	if source != "" {
		ix.bySource[source] = id
	}
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
