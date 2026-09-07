package learning

import (
	"path/filepath"
	"strings"
	"testing"
)

/*
 * A subtitle is dialogue from a story, not a memory about anybody.
 *
 * Refused by shape rather than by extension, because subtitles arrive as .srt,
 * .sub, .vtt, .ass and as plain .txt — and only the last of those was ever
 * read at all. Taken from a real file on Petar's drive.
 */
func TestSubtitlesAreRecognisedWhateverTheyAreCalled(t *testing.T) {
	real := "1\n00:00:00,505 --> 00:00:03,806\n<i>Досега в \"Древните\"...</i>\n" +
		"- Искаш да кажеш, че не ме позна?\n\n" +
		"2\n00:00:03,956 --> 00:00:07,010\nМина много време, Фин.\n\n" +
		"3\n00:00:07,160 --> 00:00:09,225\nИлайджа ще остане тук.\n"

	if !LooksLikeSubtitles(real) {
		t.Error("a subtitle file was not recognised as one")
	}

	ssa := "Dialogue: 0,0:00:03.95,0:00:07.01,Default,,0,0,0,,Hello there\n" +
		"Dialogue: 0,0:00:07.16,0:00:09.22,Default,,0,0,0,,And again\n" +
		"Dialogue: 0,0:00:09.30,0:00:11.00,Default,,0,0,0,,And again\n"

	if !LooksLikeSubtitles(ssa) {
		t.Error("an SSA subtitle was not recognised")
	}

	// And it reaches the decision, so a subtitle saved as a .txt beside a film
	// is refused with the reason a person can read.
	said, why := FromDocumentContents(Document{
		Name: "The Originals.txt", Path: "/films/The Originals.txt", Kind: "text",
	}, real, "Petar")

	if len(said) != 0 {
		t.Errorf("remembered film dialogue: %+v", said)
	}

	if why != ASubtitle {
		t.Errorf("refused for the wrong reason: %q", why)
	}
}

// And ordinary writing that happens to mention a time is not a subtitle.
func TestWritingIsNotMistakenForASubtitle(t *testing.T) {
	for _, text := range []string{
		"The meeting is at 14:00 and should run until 15:30 at the latest.",
		"We agreed 09:00 --> the deployment window, then lunch.",
		"Разговорът е в 10:00 часа и ще продължи до 11:30.",
		"",
	} {
		if LooksLikeSubtitles(text) {
			t.Errorf("ordinary writing was thrown away as a subtitle: %q", text)
		}
	}
}

/*
 * Films are listed and never opened.
 *
 * "What films do I have" is a question about its owner, and the brain could
 * not answer it: a drive of 866 films was invisible to a scanner that only
 * knew about paperwork. Nothing here can watch one, so the name and the path
 * are the whole of what is worth keeping.
 */
func TestFilmsAreListedButNotOpened(t *testing.T) {
	for _, name := range []string{"a.mkv", "b.mp4", "c.avi", "d.mov"} {
		if !IsAFilm(filepath.Join("/films", name)) {
			t.Errorf("%s was not recognised as a film", name)
		}

		if CanRead(filepath.Join("/films", name)) {
			t.Errorf("%s would be opened and read", name)
		}
	}

	if IsAFilm("/docs/notes.md") {
		t.Error("a document was taken for a film")
	}

	// ContentsWanted is what asks for a file to be opened later; a film must
	// never be in that list, or every one produces "there is nothing in it".
	wanted := ContentsWanted([]Document{
		{Path: "/films/a.mkv", Name: "a.mkv", Kind: "film"},
		{Path: "/docs/notes.md", Name: "notes.md", Kind: "Markdown"},
	})

	for _, o := range wanted {
		if strings.Contains(o.Source, ".mkv") {
			t.Errorf("a film was queued to be read: %s", o.Source)
		}
	}

	if len(wanted) != 1 {
		t.Errorf("%d files queued for reading, want only the document", len(wanted))
	}
}
