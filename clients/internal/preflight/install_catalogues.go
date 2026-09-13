package preflight

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

/*
 * The job catalogues, as parts setup can fetch.
 *
 * The program ships 427 jobs written by hand, and the real classifications are
 * what turn that into thousands. Importing them was already a button; getting
 * them was a trip to two websites, one of which hides its files behind a
 * multi-step form. So they are on the same list as everything else setup
 * installs, downloaded from each project's own servers, into this machine's
 * folder for them — never into a brain, because a brain can live on a drive
 * and travel, and the brain reads them in itself the next time it opens.
 *
 * Machine-wide rather than per brain for the same reason the models are: a
 * second brain on this computer should not download them twice.
 */

// CatalogueFolder is where one classification's downloaded files are kept.
func CatalogueFolder(source string) string {
	return filepath.Join(localShare("pn-scripts-assistant"), "catalogues", source)
}

/*
 * The ESCO release, by the names its own file server gives it.
 *
 * The download page is a form, but the files it produces sit on ESCO's server
 * under names that follow one pattern — version, content, language, format —
 * and have for every release since 1.0. English for the labels and the links,
 * Bulgarian for the names this assistant is also spoken to in.
 */
const escoVersion = "v1.2.1"

var escoLanguages = []string{"en", "bg"}

func escoURL(version, language string) string {
	return "https://ec.europa.eu/esco/download/ESCO%20dataset%20-%20" + version +
		"%20-%20classification%20-%20" + language + "%20-%20csv.zip"
}

func installESCO(w io.Writer) error {
	into := CatalogueFolder("esco")

	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}

	for _, language := range escoLanguages {
		fmt.Fprintf(w, "ESCO %s, %s\n", escoVersion, language)

		to := filepath.Join(into, "esco-"+escoVersion+"-"+language+".zip")

		if err := download(escoURL(escoVersion, language), to, w); err != nil {
			return err
		}
	}

	fmt.Fprintln(w, "Downloaded. The assistant reads them in the next time it opens, "+
		"or when you open Organisation.")

	return nil
}

/*
 * O*NET, at whatever release is current.
 *
 * Its database page names the release in every link; the text files are no
 * longer linked from it but are still published beside the others under the
 * same name. So the version is read off the page and the text archive asked
 * for directly — and when the page cannot be read, the last release this was
 * checked against, which is still there.
 */
const onetKnownRelease = "31_0"

var onetRelease = regexp.MustCompile(`db_(\d+_\d+)_[a-z]+\.zip`)

func onetVersion(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.onetcenter.org/database.html", nil)
	if err != nil {
		return onetKnownRelease
	}

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return onetKnownRelease
	}

	defer resp.Body.Close()

	page, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return onetKnownRelease
	}

	best := onetKnownRelease

	for _, m := range onetRelease.FindAllStringSubmatch(string(page), -1) {
		if newerRelease(m[1], best) {
			best = m[1]
		}
	}

	return best
}

// newerRelease compares "31_0" with "30_2" as the numbers they are.
func newerRelease(a, b string) bool {
	var aMajor, aMinor, bMajor, bMinor int

	fmt.Sscanf(strings.ReplaceAll(a, "_", " "), "%d %d", &aMajor, &aMinor)
	fmt.Sscanf(strings.ReplaceAll(b, "_", " "), "%d %d", &bMajor, &bMinor)

	return aMajor > bMajor || (aMajor == bMajor && aMinor > bMinor)
}

func installONET(w io.Writer) error {
	into := CatalogueFolder("onet")

	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	version := onetVersion(ctx)

	cancel()

	fmt.Fprintf(w, "O*NET %s\n", strings.ReplaceAll(version, "_", "."))

	to := filepath.Join(into, "db_"+version+"_text.zip")

	if err := download("https://www.onetcenter.org/dl_files/database/db_"+version+"_text.zip", to, w); err != nil {
		return err
	}

	// An older release beside the new one would be read too; the new one is
	// the whole of it.
	old, _ := filepath.Glob(filepath.Join(into, "db_*_text.zip"))

	for _, path := range old {
		if path != to {
			os.Remove(path)
		}
	}

	fmt.Fprintln(w, "Downloaded. The assistant reads it in the next time it opens, "+
		"or when you open Organisation.")

	return nil
}

// catalogueCheck is whether a classification has been downloaded here.
func catalogueCheck(source string) func() (State, string) {
	return func() (State, string) {
		found, _ := filepath.Glob(filepath.Join(CatalogueFolder(source), "*.zip"))

		if len(found) == 0 {
			return Missing, ""
		}

		names := make([]string, 0, len(found))

		for _, path := range found {
			names = append(names, filepath.Base(path))
		}

		return OK, strings.Join(names, ", ")
	}
}

func removeCatalogue(source string) func(io.Writer) error {
	return func(w io.Writer) error {
		if err := removeAll(w, CatalogueFolder(source)); err != nil {
			return err
		}

		fmt.Fprintln(w, "The downloaded files are gone. Jobs already read into a brain stay there.")

		return nil
	}
}

func cataloguePaths(source string) func() []string {
	return func() []string { return []string{CatalogueFolder(source)} }
}

// catalogueRequirements are the two classifications, for the list setup shows.
func catalogueRequirements() []Requirement {
	return []Requirement{
		{
			Name: "Jobs to hire from (ESCO)",
			Why: "3,000 occupations and 14,000 skills from the European classification, " +
				"with their Bulgarian names — so somebody can be found or hired for almost any work",
			Consequence: "it hires from the 427 jobs it ships with",
			Size:        "20MB",
			Optional:    true,
			Check:       catalogueCheck("esco"),
			Where:       func() string { return CatalogueFolder("esco") },
			InstallFunc: installESCO,
			RemoveFunc:  removeCatalogue("esco"),
			Occupies:    cataloguePaths("esco"),
			ManualHint: "Download the classification as CSV, in English and Bulgarian, from " +
				"esco.ec.europa.eu, and import it under Organisation",
		},
		{
			Name: "What each job does (O*NET)",
			Why: "the day-to-day tasks, knowledge and other names of 1,000 occupations, " +
				"to tell each agent what its job actually consists of",
			Consequence: "a job's description is what the classification or the program wrote",
			Size:        "13MB",
			Optional:    true,
			Check:       catalogueCheck("onet"),
			Where:       func() string { return CatalogueFolder("onet") },
			InstallFunc: installONET,
			RemoveFunc:  removeCatalogue("onet"),
			Occupies:    cataloguePaths("onet"),
			ManualHint: "Download the database as text files from onetcenter.org, " +
				"and import it under Organisation",
		},
	}
}
