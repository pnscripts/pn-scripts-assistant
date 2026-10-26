package server

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func formPDF() []byte {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /DA (/Helv 10 Tf 0 g) >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Annots [4 0 R] >>",
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (city) /Rect [50 700 300 720] /P 3 0 R >>",
	}

	var b bytes.Buffer

	b.WriteString("%PDF-1.4\n")

	var offsets []int

	for i, o := range objs {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}

	at := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)

	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}

	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, at)

	return b.Bytes()
}

// The PDF forms page: fields read, the form filled into a new file once.
func TestThePDFFormsPage(t *testing.T) {
	ts, _, _ := newServer(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "form.pdf")
	_ = os.WriteFile(path, formPDF(), 0o600)

	var fields struct {
		Fields []struct{ Name, Kind string } `json:"fields"`
		Out    string                        `json:"out"`
	}

	if code := sendJSON(t, ts.URL+"/api/forms/fields", map[string]any{"path": path}, &fields); code != http.StatusOK ||
		len(fields.Fields) != 1 || fields.Fields[0].Name != "city" || fields.Out != filepath.Join(dir, "form (filled).pdf") {
		t.Fatalf("%d %+v", code, fields)
	}

	notPDF := filepath.Join(dir, "x.pdf")
	_ = os.WriteFile(notPDF, []byte("hello"), 0o600)

	if code := sendJSON(t, ts.URL+"/api/forms/fields", map[string]any{"path": notPDF}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("a file that is not a PDF answered %d", code)
	}

	var done struct {
		Path string   `json:"path"`
		Set  []string `json:"set"`
	}

	if code := sendJSON(t, ts.URL+"/api/forms/fill", map[string]any{"path": path, "values": map[string]string{"city": "Варна"}}, &done); code != http.StatusOK ||
		done.Path != fields.Out || len(done.Set) != 1 {
		t.Fatalf("%d %+v", code, done)
	}

	if code := sendJSON(t, ts.URL+"/api/forms/fill", map[string]any{"path": path, "values": map[string]string{"city": "x"}, "out": done.Path}, nil); code != http.StatusConflict {
		t.Errorf("filling over a file answered %d", code)
	}

	if code := sendJSON(t, ts.URL+"/api/forms/fill", map[string]any{"path": path, "values": map[string]string{"nope": "x"}}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("a fill with no field answered %d", code)
	}
}

// Every kind of problem filling a form can report is said in both
// languages: the keys are read from the package itself.
func TestEveryFormProblemIsTranslated(t *testing.T) {
	js, _ := os.ReadFile(filepath.Join("assets", "js", "i18n.js"))

	files, _ := filepath.Glob(filepath.Join("..", "pdfform", "*.go"))

	found := 0

	for _, f := range files {
		src, _ := os.ReadFile(f)

		for _, m := range regexp.MustCompile(`(?:problem\("|Key: ")([a-zA-Z]+)"`).FindAllSubmatch(src, -1) {
			found++

			if !bytes.Contains(js, []byte("'forms.p."+string(m[1])+"': [")) {
				t.Errorf("forms.p.%s is not in i18n.js", m[1])
			}
		}
	}

	if found < 8 {
		t.Errorf("only %d problem keys were found; the pattern no longer matches the code", found)
	}
}
