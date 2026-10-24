package originality

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNamesCloseInSpellingSoundOrWholeWordAreRisky(t *testing.T) {
	cases := []struct {
		ours, theirs string
		want         Risk
	}{
		{"Tetriz", "Tetris", High},
		{"Pakman", "Pac-Man", High},
		{"Tetris Blast", "Tetris", High},
		{"Mineclaft", "Minecraft", High},
		{"Moonfold", "Tetris", None},
		{"Zorna", "Zelda", None},
	}

	for _, c := range cases {
		if got := NameRisk(NameSimilarity(c.ours, c.theirs)); got != c.want {
			t.Errorf("%s vs %s: %s (%.2f), want %s", c.ours, c.theirs, got, NameSimilarity(c.ours, c.theirs), c.want)
		}
	}

	hits := []Hit{
		{Title: "Tetris® - Apps on Google Play", URL: "https://play.google.com/x", Where: "app stores"},
		{Title: "Tetris | Official Site", URL: "https://tetris.com"},
		{Title: "Moon Pie recipes"},
	}

	got := CloseNames("Tetriz", hits)
	if len(got) != 1 || got[0].Name != "Tetris" || got[0].Risk != High {
		t.Errorf("close names: %+v", got)
	}

	if links := RegisterLinks("Ябълка Ко"); !strings.Contains(links[0], "%D0%AF") || !strings.Contains(links[0], "%20") {
		t.Errorf("register link not escaped: %s", links[0])
	}
}

func TestTheOneSentenceSaysWhatOnlyThisGameDoes(t *testing.T) {
	good := "You steer the tide itself, and every wave you raise rebuilds the island your villagers stand on."
	if err := UniquenessSentence(good); err != nil {
		t.Errorf("a real rule refused: %v", err)
	}

	for _, bad := range []string{
		"",
		"Falling blocks.",
		"It is like Tetris but with colours and a shop for upgrades.",
		"You build towers. Then enemies come along the road and you defend.",
	} {
		if UniquenessSentence(bad) == nil {
			t.Errorf("%q passed", bad)
		}
	}

	hits := []Hit{
		{Title: "Tetris - falling blocks", Snippet: "Rotate falling blocks to clear lines"},
		{Title: "Stardew Valley", Snippet: "Farm, fish and befriend villagers"},
		{Title: "Puyo Puyo", Snippet: "Match falling coloured blobs"},
		{Title: "Chess", Snippet: "Board game"},
	}

	got := ClosestTitles("Rotate falling blocks so they clear lines", hits)
	if len(got) != 3 || got[0].Title != "Tetris" || got[0].Risk != High {
		t.Errorf("closest: %+v", got)
	}
}

func pattern(w, h int, seed float64) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/float64(w), float64(y)/float64(h)
			v := 127 + 120*math.Sin(seed*fx*6+fy*3)*math.Cos(fy*seed*4-fx)
			// An off-centre blob, so mirrored is not the same picture.
			if (fx-0.25)*(fx-0.25)+(fy-0.4)*(fy-0.4) < 0.03 {
				v = 20
			}
			img.Set(x, y, color.RGBA{uint8(v), uint8(255 - v), uint8(v / 2), 255})
		}
	}

	return img
}

func save(t *testing.T, path string, img image.Image) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if strings.HasSuffix(path, ".jpg") {
		err = jpeg.Encode(f, img, &jpeg.Options{Quality: 60})
	} else {
		err = png.Encode(f, img)
	}

	if err != nil {
		t.Fatal(err)
	}
}

func mirror(img image.Image) image.Image {
	b := img.Bounds()
	out := image.NewRGBA(b)

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(b.Max.X-1-(x-b.Min.X), y, img.At(x, y))
		}
	}

	return out
}

func TestAPictureLikeAReferenceIsFoundResizedRecompressedOrMirrored(t *testing.T) {
	dir := t.TempDir()
	ref := filepath.Join(dir, "reference.png")
	save(t, ref, pattern(256, 256, 3))

	save(t, filepath.Join(dir, "small-copy.jpg"), pattern(97, 97, 3))
	save(t, filepath.Join(dir, "mirrored.png"), mirror(pattern(256, 256, 3)))
	save(t, filepath.Join(dir, "own.png"), pattern(256, 256, 11))

	ours := []string{filepath.Join(dir, "small-copy.jpg"), filepath.Join(dir, "mirrored.png"), filepath.Join(dir, "own.png")}

	matches, problems := ComparePictures(ours, []string{ref})
	if len(problems) > 0 {
		t.Fatal(problems)
	}

	found := map[string]PictureMatch{}
	for _, m := range matches {
		found[filepath.Base(m.Ours)] = m
	}

	if m, ok := found["small-copy.jpg"]; !ok || m.Risk != High {
		t.Errorf("resized copy: %+v", m)
	}

	if m, ok := found["mirrored.png"]; !ok || m.Risk != High || !m.Mirrored {
		t.Errorf("mirrored copy: %+v", m)
	}

	if m, ok := found["own.png"]; ok {
		t.Errorf("an own picture matched: %+v", m)
	}
}

func notes(names string) []int { return notesOf(names) }

func TestKnownThemesAreFoundInAnyKeyInCodeAndMIDI(t *testing.T) {
	// Ode to Joy a fourth up, written as note names in a script.
	code := `const tune = ["A4","A4","A#4","C5","C5","A#4","A4","G4","F4","F4","G4","A4","A4","G4","G4"];`

	tunes := NotesInCode(code)
	if len(tunes) == 0 {
		t.Fatal("no notes read from the code")
	}

	if m := CompareMelody("tune.js", tunes[0]); len(m) == 0 || m[0].Theme.Name != "Ode to Joy" || m[0].Risk != High {
		t.Errorf("Ode to Joy not found: %+v", m)
	}

	// Für Elise as frequencies.
	var freqs []string
	for _, n := range notes("E5 D#5 E5 D#5 E5 B4 D5 C5 A4") {
		freqs = append(freqs, fmt.Sprintf("%.2f", 440*math.Pow(2, float64(n-69)/12)))
	}

	tunes = NotesInCode("play([" + strings.Join(freqs, ", ") + "])")
	if len(tunes) == 0 || len(CompareMelody("sfx.gd", tunes[0])) == 0 || CompareMelody("sfx.gd", tunes[0])[0].Theme.Name != "Für Elise" {
		t.Errorf("Für Elise in frequencies not found: %v", tunes)
	}

	// Korobeiniki in a MIDI file, with a chord under it and drums.
	mid := midi(notes("E5 B4 C5 D5 C5 B4 A4 A4 C5 E5 D5 C5 B4"))

	tracks, err := NotesInMIDI(mid)
	if err != nil || len(tracks) != 1 {
		t.Fatalf("midi: %v %v", tracks, err)
	}

	m := CompareMelody("theme.mid", tracks[0])
	if len(m) == 0 || m[0].Theme.Name != "Korobeiniki" || !strings.Contains(m[0].Theme.Note, "Tetris") {
		t.Errorf("Korobeiniki not found: %+v", m)
	}

	// A scale and an own tune are nobody's.
	for _, own := range []string{
		"C4 D4 E4 F4 G4 A4 B4 C5 B4 A4 G4 F4 E4 D4 C4",
		"D4 A4 F4 C5 G4 E5 B4 D4 G#4 C#5 F#4 A#4",
	} {
		if m := CompareMelody("own", notes(own)); len(m) > 0 {
			t.Errorf("%s matched %+v", own, m)
		}
	}

	// Lists that are not notes are not read as notes.
	if got := NotesInCode("var pos = [0, 10, 200, 3000, 1, 2, 5, 7, 9]; // a1 e2 d3"); len(got) != 0 {
		t.Errorf("read notes from coordinates: %v", got)
	}
}

// midi is a format-0 file: the melody on channel 1 with a chord note under
// each, and a drum hit on channel 10.
func midi(melody []int) []byte {
	var tr bytes.Buffer

	tr.Write([]byte{0x00, 0xff, 0x51, 0x03, 0x07, 0xa1, 0x20}) // tempo

	for _, n := range melody {
		tr.Write([]byte{0x00, 0x90, byte(n), 0x60})
		tr.Write([]byte{0x00, byte(n - 12), 0x50}) // running status: a lower note
		tr.Write([]byte{0x00, 0x99, 36, 0x70})     // drums
		tr.Write([]byte{0x81, 0x40, 0x80, byte(n), 0x00})
		tr.Write([]byte{0x00, 0x80, byte(n - 12), 0x00})
	}

	tr.Write([]byte{0x00, 0xff, 0x2f, 0x00})

	var out bytes.Buffer

	out.WriteString("MThd")
	_ = binary.Write(&out, binary.BigEndian, uint32(6))
	_ = binary.Write(&out, binary.BigEndian, []uint16{0, 1, 96})
	out.WriteString("MTrk")
	_ = binary.Write(&out, binary.BigEndian, uint32(tr.Len()))
	out.Write(tr.Bytes())

	return out.Bytes()
}

func TestTheReportIsATableAndASheetAndNeverReplacesAnEarlierOne(t *testing.T) {
	dir := t.TempDir()
	when := time.Date(2026, 10, 24, 6, 0, 0, 0, time.UTC)

	f := []Finding{
		{Item: "B", Aspect: "sound", Finding: "clear", Risk: None},
		{Item: "A", Aspect: "name and trademarks", Finding: "\"Tetris\" | 95%", Risk: High, Action: "rename", Fixed: "renamed to Moonfold"},
	}

	md, sheet, err := Write(dir, "Catalogue", f, when)
	if err != nil {
		t.Fatal(err)
	}

	text, _ := os.ReadFile(md)
	if !strings.Contains(string(text), "| Item | Aspect | Finding | Risk | Action |") ||
		!strings.Contains(string(text), "Fixed: renamed to Moonfold") || !strings.Contains(string(text), `\|`) {
		t.Errorf("markdown:\n%s", text)
	}

	if strings.Index(string(text), "| A |") > strings.Index(string(text), "| B |") {
		t.Error("high risk is not first")
	}

	rows, _ := os.ReadFile(sheet)
	if !strings.HasPrefix(string(rows), "item,aspect,finding,risk,action,fixed") || strings.Count(string(rows), "\n") != 3 {
		t.Errorf("csv:\n%s", rows)
	}

	again, _, _ := Write(dir, "Catalogue", f, when)
	if again == md {
		t.Error("a second report replaced the first")
	}
}

func TestTheCatalogueAuditChecksEachItemForTheAspectsItHasOnly(t *testing.T) {
	root := t.TempDir()

	look := filepath.Join(root, "Tetriz")
	_ = os.MkdirAll(filepath.Join(look, "art"), 0o755)
	_ = os.WriteFile(filepath.Join(look, "project.godot"), []byte("[application]\nconfig/name=\"Tetriz\"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(look, "listing.json"), []byte(`{"pitch":"It is like Tetris but with colours."}`), 0o644)
	_ = os.WriteFile(filepath.Join(look, "music.gd"), []byte(`var theme = ["E5","B4","C5","D5","C5","B4","A4","A4","C5","E5","D5"]`), 0o644)
	save(t, filepath.Join(look, "art", "cover.png"), pattern(128, 128, 3))

	clean := filepath.Join(root, "Moonfold")
	_ = os.MkdirAll(clean, 0o755)
	_ = os.WriteFile(filepath.Join(clean, "README.md"), []byte("# Moonfold\n\nA calendar that folds your week into the phases of the moon.\n"), 0o644)

	refs := filepath.Join(t.TempDir(), "ref.png")
	save(t, refs, pattern(300, 300, 3))

	var asked []string

	search := func(q string) ([]Hit, error) {
		asked = append(asked, q)
		if strings.Contains(q, "Tetriz") || strings.Contains(q, "Tetris") {
			return []Hit{{Title: "Tetris® - Apps on Google Play", URL: "https://play.google.com/store/apps/details?id=x", Snippet: "falling blocks with colours"}}, nil
		}

		return []Hit{{Title: "Moon Phase Calendar", Snippet: "see the moon"}}, nil
	}

	findings, err := Audit(context.Background(), Options{Folder: root, References: []string{refs}, Search: search, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}

	by := map[string]map[string]Risk{}

	for _, f := range findings {
		if by[f.Item] == nil {
			by[f.Item] = map[string]Risk{}
		}

		if r, ok := by[f.Item][f.Aspect]; !ok || riskOrder[f.Risk] < riskOrder[r] {
			by[f.Item][f.Aspect] = f.Risk
		}
	}

	want := map[string]Risk{"name and trademarks": High, "game rules": Medium, "look": High, "sound": High}
	for aspect, risk := range want {
		if by["Tetriz"][aspect] != risk {
			t.Errorf("Tetriz %s: %s, want %s (%+v)", aspect, by["Tetriz"][aspect], risk, findings)
		}
	}

	if r := by["Moonfold"]["name and trademarks"]; r == High || r == Medium {
		t.Errorf("Moonfold name: %s", r)
	}

	for _, aspect := range []string{"look", "sound", "game rules"} {
		if _, ok := by["Moonfold"][aspect]; ok {
			t.Errorf("Moonfold has no %s and was checked for it", aspect)
		}
	}

	if findings[0].Risk != High {
		t.Error("findings not sorted by risk")
	}

	// Offline: the name is not passed, it is said to be unsearched.
	offline, _ := Audit(context.Background(), Options{Folder: clean})
	if len(offline) == 0 || offline[0].Risk != Low || !strings.Contains(offline[0].Finding, "not searched") {
		t.Errorf("offline: %+v", offline)
	}
}
