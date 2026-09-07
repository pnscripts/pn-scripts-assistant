package subtitles

import (
	"os"
	"path/filepath"
	"testing"
)

/*
 * Which films have none, on the shapes a real drive actually holds.
 *
 * Petar's has 866 films and 1,362 subtitle files, and they are not paired one
 * to one: some films have two languages beside them, some subtitles belong to
 * a film that is no longer there, and the language is in the name as often as
 * it is not.
 */
func TestFilmsWithSubtitlesBesideThemAreNotOffered(t *testing.T) {
	root := t.TempDir()

	write := func(name string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755)
		os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644)
	}

	write("Plain.mkv")
	write("Plain.srt")

	// The common Bulgarian case: the language is in the subtitle's name.
	write("WithLanguage.avi")
	write("WithLanguage.bg.srt")

	write("Nothing.mp4")

	write("Series/Episode1.mkv")
	write("Series/Episode1.sub")
	write("Series/Episode2.mkv")

	// A subtitle whose film has gone. It must not invent one.
	write("Orphan.srt")

	missing, err := Missing(root)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}

	for _, f := range missing {
		got[f.Name] = true
	}

	for _, want := range []string{"Nothing.mp4", "Episode2.mkv"} {
		if !got[want] {
			t.Errorf("%s has no subtitles and was not offered", want)
		}
	}

	for _, no := range []string{"Plain.mkv", "WithLanguage.avi", "Episode1.mkv"} {
		if got[no] {
			t.Errorf("%s already has subtitles and was offered anyway", no)
		}
	}

	if len(missing) != 2 {
		t.Errorf("offered %d films, want 2: %+v", len(missing), missing)
	}
}

// Where the subtitle goes: beside the film, with the film's name, which is
// where every player looks for it.
func TestTheSubtitleIsWrittenBesideTheFilm(t *testing.T) {
	for film, want := range map[string]string{
		"/films/The Matrix.mkv":    "/films/The Matrix.srt",
		"/films/Series/Ep.01.mp4":  "/films/Series/Ep.01.srt",
		"/films/no extension here": "/films/no extension here.srt",
	} {
		if got := Path(film); got != want {
			t.Errorf("Path(%q) = %q, want %q", film, got, want)
		}
	}
}

// It refuses rather than half-doing it when a piece is missing, and says which
// piece — "it did not work" about a three-hour job is not an answer.
func TestItSaysWhichPieceIsMissing(t *testing.T) {
	root := t.TempDir()
	film := filepath.Join(root, "a.mkv")
	os.WriteFile(film, []byte("not really a film"), 0o644)

	if _, err := Make(t.Context(), film, "", "", "", nil); err == nil {
		t.Error("claimed to work with no recogniser")
	} else if !contains(err.Error(), "recogniser") {
		t.Errorf("did not say what was missing: %v", err)
	}

	if _, err := Make(t.Context(), filepath.Join(root, "gone.mkv"), "w", "m", "", nil); err == nil {
		t.Error("claimed to work on a film that is not there")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}

			return false
		}())
}
