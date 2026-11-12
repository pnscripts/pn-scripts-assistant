package tools

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/aidisclosure"
)

/*
 * A document the assistant writes says in its properties that an AI wrote
 * it (EU AI Act article 50(2), machine-readable marking), and the text the
 * owner asked for is left as it is: no line is added to the page.
 */
func TestAWrittenPDFIsMarkedAsWrittenByAnAI(t *testing.T) {
	body, err := buildPDF("Offer", "Two rooms, three nights.")
	if err != nil {
		t.Fatal(err)
	}

	pdf := string(body)

	for _, want := range []string{
		"/Producer(" + aidisclosure.Producer + ")",
		"/Keywords(" + aidisclosure.DocumentKeywords + ")",
		"/Subject(" + aidisclosure.DocumentNote + ")",
	} {
		if !strings.Contains(pdf, want) {
			t.Errorf("the PDF has no %s", want)
		}
	}

	if !strings.Contains(pdf, "/Info ") {
		t.Error("the trailer does not point at the document information")
	}

	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Log("pdfinfo is not installed here; the properties were checked as text only")

		return
	}

	path := filepath.Join(t.TempDir(), "offer.pdf")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	// A reader that trusts the cross-reference table finds them too.
	out, err := exec.Command("pdfinfo", path).CombinedOutput()
	if err != nil {
		t.Fatalf("pdfinfo could not read it: %v\n%s", err, out)
	}

	for _, want := range []string{
		"Producer:        " + aidisclosure.Producer,
		"Keywords:        " + aidisclosure.DocumentKeywords,
		"Subject:         " + aidisclosure.DocumentNote,
		"Pages:           1",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("pdfinfo does not show %q:\n%s", want, out)
		}
	}

	if strings.Contains(string(out), "Syntax Error") || strings.Contains(string(out), "Syntax Warning") {
		t.Errorf("pdfinfo complained:\n%s", out)
	}
}

func TestAWrittenWordDocumentIsMarkedAsWrittenByAnAI(t *testing.T) {
	body, err := buildDocx("Offer", "Two rooms, three nights.")
	if err != nil {
		t.Fatal(err)
	}

	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}

	parts := map[string]string{}

	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}

		b, _ := io.ReadAll(r)
		r.Close()

		parts[f.Name] = string(b)
	}

	core, ok := parts["docProps/core.xml"]
	if !ok {
		t.Fatalf("there are no core properties; parts: %v", keysOf(parts))
	}

	var props struct {
		Description string `xml:"description"`
		Keywords    string `xml:"keywords"`
		Creator     string `xml:"creator"`
	}

	if err := xml.Unmarshal([]byte(core), &props); err != nil {
		t.Fatalf("core.xml is not XML: %v", err)
	}

	if props.Description != aidisclosure.DocumentNote || props.Keywords != aidisclosure.DocumentKeywords {
		t.Errorf("core properties are %+v", props)
	}

	if props.Creator != "" {
		t.Errorf("the author is the owner's to fill in, not %q", props.Creator)
	}

	for _, part := range []string{"[Content_Types].xml", "_rels/.rels"} {
		var parsed struct{}
		if err := xml.Unmarshal([]byte(parts[part]), &parsed); err != nil {
			t.Errorf("%s is not XML: %v", part, err)
		}
	}

	if !strings.Contains(parts["[Content_Types].xml"], `PartName="/docProps/core.xml"`) {
		t.Error("the content types do not name the core properties")
	}

	if !strings.Contains(parts["_rels/.rels"], `Target="docProps/core.xml"`) {
		t.Error("the package does not point at the core properties")
	}

	if strings.Contains(parts["word/document.xml"], "AI") {
		t.Error("words were added to the document itself")
	}
}

func keysOf(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}

	return out
}
