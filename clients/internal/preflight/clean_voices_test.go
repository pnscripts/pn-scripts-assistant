package preflight

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * The voices setup installs may be used by anybody, for anything.
 *
 * The three voices 0.13 installed (Jenny (Dioco), alba and
 * northern_english_male) were each "Finetuned from U.S. English lessac voice"
 * by their model cards, and the lessac data is licensed "exclusively for
 * Research Purposes only". A voice is only as free as everything it was
 * trained from, so the ones installed now were trained from scratch on
 * recordings in the public domain, and the old ones are withdrawn.
 */

// The voices installed now, by what each one is for, and the digests and
// sizes of their files as downloaded from the pinned commit on 2026-11-09.
var cleanVoices = map[string]struct {
	model, settings string
	size, json      int64
}{
	"en_GB-cori-medium": {
		"1899f98e5fb8310154f3c2973f4b8a929ba7245e722b3d3a85680b833d95f10d",
		"e262c16d7f192f69d4edd6b4ef8a5915379e67495fcc402f1ab15eeb33da3d36", 63531379, 4966},
	"en_US-ljspeech-medium": {
		"6f52a751e2349abe7a76735eb09dc1875298c77ea2342ffd2fef79ff81b87f22",
		"141d612cc0a95ed7efc1ca936b845c2364967f2e9217c5dbfcf69fc4d6c65860", 63531379, 4972},
	"en_US-norman-medium": {
		"b9739443232a80a59c7d18810dd856899bf16a7964725f5ab81ea49b1351cb71",
		"6c2db7f558a4a8deb9fe822583c1c5105f6c4e834dd0f9de8ad17a888ee9fe1d", 63531379, 4968},
}

// Every English Piper voice whose model card says it was fine-tuned from
// lessac, or whose data is non-commercial, at the pinned commit.
var notFreeToUse = []string{
	"en_GB-alba-medium", "en_GB-jenny_dioco-medium", "en_GB-northern_english_male-medium",
	"en_GB-alan-medium", "en_GB-aru-medium", "en_GB-semaine-medium", "en_GB-vctk-medium",
	"en_US-amy-medium", "en_US-lessac-medium", "en_US-ryan-medium", "en_US-joe-medium",
	"en_US-hfc_female-medium", "en_US-libritts_r-medium", "en_US-kathleen-low",
	"en_GB-southern_english_female-low",
}

func TestTheVoicesSetupInstallsAreFreeAllTheWayDown(t *testing.T) {
	got := map[string]Pinned{}

	for _, p := range PiperVoices() {
		got[p.Name] = p
	}

	if len(got) != 2*len(cleanVoices) {
		t.Errorf("%d voice files are pinned, want %d", len(got), 2*len(cleanVoices))
	}

	for name, want := range cleanVoices {
		model, settings := got[name+".onnx"], got[name+".onnx.json"]

		if model.Sum != want.model || model.Size != want.size {
			t.Errorf("%s.onnx is pinned as %s, %d bytes", name, model.Sum, model.Size)
		}

		if settings.Sum != want.settings || settings.Size != want.json {
			t.Errorf("%s.onnx.json is pinned as %s, %d bytes", name, settings.Sum, settings.Size)
		}

		// Said in words, for the page and the notices.
		if !strings.Contains(model.Licence, "public domain") {
			t.Errorf("%s says its licence is %q", name, model.Licence)
		}
	}

	for _, name := range notFreeToUse {
		if _, installed := got[name+".onnx"]; installed {
			t.Errorf("setup installs %s, which was trained from data that is not free to use", name)
		}
	}
}

// The three 0.13 installed are withdrawn by name, so a copy that still has
// them neither offers them nor counts them as a voice.
func TestTheVoicesOlderReleasesInstalledAreWithdrawn(t *testing.T) {
	for _, name := range []string{"en_GB-alba-medium", "en_GB-jenny_dioco-medium", "en_GB-northern_english_male-medium"} {
		if !Withdrawn(name) {
			t.Errorf("%s is not withdrawn", name)
		}
	}

	for name := range cleanVoices {
		if Withdrawn(name) {
			t.Errorf("%s, which setup installs, is withdrawn", name)
		}
	}
}

// voicesIn makes a piper install in a home of its own holding the voices
// named, a model and its settings file each.
func voicesIn(t *testing.T, names ...string) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".local", "share", "piper", "voices")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, name := range names {
		for _, ext := range []string{".onnx", ".onnx.json"} {
			if err := os.WriteFile(filepath.Join(dir, name+ext), []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	return dir
}

func voiceCheck(t *testing.T) (State, string) {
	t.Helper()

	for _, r := range Requirements() {
		if r.Name == "Voice (speaking)" {
			return r.Check()
		}
	}

	t.Fatal("there is no Voice (speaking) part")

	return Unknown, ""
}

/*
 * A copy installed by 0.13 is out of date, and says so.
 *
 * Its voices are on the disk and are no longer offered, so "installed" would
 * be a part that cannot say a word in the voices it promises; "out of date"
 * puts an Update button beside it.
 */
func TestAVoiceInstalledByAnOlderReleaseIsOutOfDate(t *testing.T) {
	voicesIn(t, "en_GB-alba-medium", "en_GB-jenny_dioco-medium", "en_GB-northern_english_male-medium")

	if state, detail := voiceCheck(t); state != Outdated {
		t.Errorf("only the withdrawn voices gave %v (%s), want out of date", state, detail)
	}
}

func TestAVoiceWithTheNewVoicesIsCurrentEvenWithTheOldOnesBeside(t *testing.T) {
	voicesIn(t, "en_GB-alba-medium", "en_GB-cori-medium", "en_US-ljspeech-medium", "en_US-norman-medium")

	if state, detail := voiceCheck(t); state != OK || !strings.Contains(detail, "piper") {
		t.Errorf("the new voices gave %v (%s), want piper", state, detail)
	}
}

// Somebody's own voice that was never ours is theirs: a piper with it is a
// working voice, not an old release.
func TestAVoiceSomebodyAddedThemselvesIsNotOutOfDate(t *testing.T) {
	voicesIn(t, "de_DE-thorsten-medium")

	if state, detail := voiceCheck(t); state != OK {
		t.Errorf("a voice somebody added gave %v (%s)", state, detail)
	}
}

/*
 * The new voices arrive by themselves after an update.
 *
 * Somebody who installed the voice agreed to its download once; the new
 * voices replace what that download brought, so they are fetched when the
 * program starts (as the voice recognition runtime is), and only then.
 */
func TestRefreshingFetchesTheNewVoicesForAnOlderInstall(t *testing.T) {
	dir := voicesIn(t, "en_GB-alba-medium", "en_GB-jenny_dioco-medium", "en_GB-northern_english_male-medium")

	var fetched []string

	swapVoiceFetch(t, func(p Pinned, to string, _ io.Writer) error {
		fetched = append(fetched, p.Name)

		return os.WriteFile(to, []byte(p.Name), 0o644)
	})

	replaced, err := RefreshVoices(io.Discard)
	if err != nil || !replaced {
		t.Fatalf("refreshing gave %v, %v", replaced, err)
	}

	if len(fetched) != 2*len(cleanVoices) {
		t.Errorf("fetched %v", fetched)
	}

	for name := range cleanVoices {
		for _, ext := range []string{".onnx", ".onnx.json"} {
			if _, err := os.Stat(filepath.Join(dir, name+ext)); err != nil {
				t.Errorf("%s%s is not there afterwards", name, ext)
			}
		}
	}

	// The old files stay where they were: removing them is the person's
	// choice (Remove a part), not something an update does behind them.
	if _, err := os.Stat(filepath.Join(dir, "en_GB-alba-medium.onnx")); err != nil {
		t.Error("the old voice was deleted")
	}

	// And a second start fetches nothing.
	fetched = nil

	if replaced, err := RefreshVoices(io.Discard); err != nil || replaced || len(fetched) > 0 {
		t.Errorf("a second start gave %v, %v and fetched %v", replaced, err, fetched)
	}
}

func TestRefreshingLeavesAMachineWithoutTheOldVoicesAlone(t *testing.T) {
	for _, tc := range []struct {
		name   string
		voices []string
	}{
		{"no voice at all", nil},
		{"only a voice somebody added", []string{"de_DE-thorsten-medium"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			voicesIn(t, tc.voices...)

			swapVoiceFetch(t, func(p Pinned, _ string, _ io.Writer) error {
				t.Errorf("fetched %s", p.Name)

				return nil
			})

			if replaced, err := RefreshVoices(io.Discard); err != nil || replaced {
				t.Errorf("gave %v, %v", replaced, err)
			}
		})
	}
}

// A failed download is reported, and what was there is untouched.
func TestARefreshThatCannotFetchSaysSo(t *testing.T) {
	voicesIn(t, "en_GB-alba-medium")

	swapVoiceFetch(t, func(p Pinned, _ string, _ io.Writer) error {
		return os.ErrDeadlineExceeded
	})

	if replaced, err := RefreshVoices(io.Discard); err == nil || replaced {
		t.Errorf("a failed download gave %v, %v", replaced, err)
	}
}

func swapVoiceFetch(t *testing.T, fake func(Pinned, string, io.Writer) error) {
	t.Helper()

	was := voiceFetch
	voiceFetch = fake

	t.Cleanup(func() { voiceFetch = was })
}
