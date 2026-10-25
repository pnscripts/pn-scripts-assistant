package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/glossary"
	"pn-scripts-assistant/internal/brain/translate"
)

func translating(t *testing.T) Translating {
	t.Helper()

	root := t.TempDir()

	return Translating{Root: root, Translator: func() translate.Translator {
		terms, _ := glossary.Load(root)

		return translate.Translator{Glossary: terms, Engine: func(_ context.Context, _, text string) (string, error) {
			return strings.ReplaceAll(strings.ToUpper(text), "INVOICE", "ФАКТУРА"), nil
		}}
	}}
}

func TestTheGlossaryIsKeptAndUsedByTranslation(t *testing.T) {
	with := translating(t)

	out, err := call(t, ChangeGlossary{With: with}, map[string]any{"source": "invoice", "target": "фактура", "to": "bg"})
	if err != nil || !strings.Contains(out, "Added") {
		t.Fatal(out, err)
	}

	out, _ = call(t, Glossary{With: with}, map[string]any{"to": "bg"})
	if !strings.Contains(out, "invoice → фактура (any→bg)") {
		t.Errorf("%s", out)
	}

	out, err = call(t, Translate{With: with}, map[string]any{"text": "Pay the invoice by 15.", "to": "bg"})
	if err != nil || !strings.Contains(out, "PAY THE ФАКТУРА BY 15.") || !strings.Contains(out, "Glossary terms used: invoice → фактура") {
		t.Errorf("%v\n%s", err, out)
	}

	out, _ = call(t, Glossary{With: with}, map[string]any{"to": "bg", "original": "The invoice, 15 May.", "translation": "Сметката, 16 май."})
	if !strings.Contains(out, `"invoice" should be translated as "фактура"`) || !strings.Contains(out, `"15"`) {
		t.Errorf("%s", out)
	}

	out, _ = call(t, ChangeGlossary{With: with}, map[string]any{"source": "invoice", "to": "bg", "remove": true})
	if !strings.Contains(out, "Removed") {
		t.Errorf("%s", out)
	}

	out, _ = call(t, Glossary{With: with}, map[string]any{})
	if !strings.Contains(out, "no terms yet") {
		t.Errorf("%s", out)
	}

	if _, err := call(t, ChangeGlossary{With: with}, map[string]any{"source": "x", "target": "y", "to": "Bulgarian"}); err == nil {
		t.Error("a language name was accepted as a code")
	}
}

func TestADocumentIsTranslatedIntoANewFile(t *testing.T) {
	with := translating(t)
	dir := t.TempDir()

	in := filepath.Join(dir, "note.txt")
	_ = os.WriteFile(in, []byte("Hello.\n\nThe invoice."), 0o600)

	summary := TranslateDocument{}.Summarize([]byte(`{"path":"` + in + `","to":"bg"}`))
	if !strings.Contains(summary, filepath.Join(dir, "note.bg.txt")) || !strings.Contains(summary, "original is not changed") {
		t.Errorf("the approval does not say where it writes: %s", summary)
	}

	out, err := call(t, TranslateDocument{With: with}, map[string]any{"path": in, "to": "bg"})
	if err != nil || !strings.Contains(out, "Translated 2 parts") {
		t.Fatal(out, err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "note.bg.txt"))
	if string(got) != "HELLO.\n\nTHE ФАКТУРА." {
		t.Errorf("%q", got)
	}

	original, _ := os.ReadFile(in)
	if string(original) != "Hello.\n\nThe invoice." {
		t.Error("the original was changed")
	}

	if _, err := call(t, TranslateDocument{With: with}, map[string]any{"path": "note.txt", "to": "bg"}); err == nil {
		t.Error("a relative path was accepted")
	}
}
