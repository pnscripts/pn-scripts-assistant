package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func call(t *testing.T, tool Tool, args map[string]any) (string, error) {
	t.Helper()

	raw, _ := json.Marshal(args)

	return tool.Execute(context.Background(), raw)
}

func TestProofreadFindsSlipsAndGivesTheCorrectedText(t *testing.T) {
	out, err := call(t, Proofread{}, map[string]any{"text": "Живея в Варна. Мисля че е прав."})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"Language: Bulgarian", `"в" → "във"`, `in "Мисля че"`, "Живея във Варна. Мисля, че е прав."} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestProofreadChecksATone(t *testing.T) {
	out, err := call(t, Proofread{}, map[string]any{"text": "Hey, we can't do it!", "tone": "formal", "max_words": 3})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"Tone and length", "full", "exclamation", "more than the 3 asked for"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestProofreadReadsFiles(t *testing.T) {
	dir := t.TempDir()

	plain := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(plain, []byte("It was a honest mistake."), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := call(t, Proofread{}, map[string]any{"path": plain})
	if err != nil || !strings.Contains(out, "an honest") {
		t.Fatalf("%v\n%s", err, out)
	}

	word := filepath.Join(dir, "letter.docx")
	if _, err := call(t, WriteDocument{}, map[string]any{"path": word, "text": "Дадох й книгата."}); err != nil {
		t.Fatal(err)
	}

	out, err = call(t, Proofread{}, map[string]any{"path": word})
	if err != nil || !strings.Contains(out, "Дадох ѝ книгата.") {
		t.Fatalf("%v\n%s", err, out)
	}

	if _, err := call(t, Proofread{}, map[string]any{"path": "note.txt"}); err == nil {
		t.Error("a relative path was read")
	}

	if _, err := call(t, Proofread{}, map[string]any{}); err == nil {
		t.Error("nothing to check did not say so")
	}
}

func TestCleanTextSaysNothingWasFound(t *testing.T) {
	out, err := call(t, Proofread{}, map[string]any{"text": "Everything here is fine."})
	if err != nil || !strings.Contains(out, "No slips found") {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestCompareVersionsShowsWhatChanged(t *testing.T) {
	out, err := call(t, CompareVersions{}, map[string]any{"before": "The quick brown fox.", "after": "The quick red fox."})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "1 words taken out, 1 put in") || !strings.Contains(out, "[-brown-]{+red+}") {
		t.Errorf("%s", out)
	}

	out, _ = call(t, CompareVersions{}, map[string]any{"before": "Same.", "after": "Same."})
	if out != "The two versions are the same." {
		t.Errorf("%s", out)
	}

	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	_ = os.WriteFile(a, []byte("Първа версия."), 0o600)
	_ = os.WriteFile(b, []byte("Втора версия."), 0o600)

	out, err = call(t, CompareVersions{}, map[string]any{"before_path": a, "after_path": b})
	if err != nil || !strings.Contains(out, "[-Първа-]{+Втора+}") {
		t.Errorf("%v %s", err, out)
	}
}
