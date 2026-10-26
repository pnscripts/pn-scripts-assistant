package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// smallForm is a one-page form with a name and a checkbox.
func smallForm() []byte {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R 5 0 R] /DA (/Helv 0 Tf 0 g) >> >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Annots [4 0 R 5 0 R] >>",
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (full_name) /Rect [50 700 300 720] /P 3 0 R >>",
		"<< /Type /Annot /Subtype /Widget /FT /Btn /T (married) /V /Off /AS /Off /Rect [50 670 64 684] /P 3 0 R /AP << /N << /On 6 0 R /Off 6 0 R >> >> >>",
		"<< /Length 0 >>\nstream\n\nendstream",
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

func TestAPDFFormIsFilledIntoANewFile(t *testing.T) {
	dir := t.TempDir()
	form := filepath.Join(dir, "заявление.pdf")
	_ = os.WriteFile(form, smallForm(), 0o600)

	list, err := PDFFormFields{}.Execute(context.Background(), json.RawMessage(`{"path":"`+form+`"}`))
	if err != nil || !strings.Contains(list, `"full_name": text`) || !strings.Contains(list, `"married": checkbox`) {
		t.Fatalf("%v\n%s", err, list)
	}

	out, err := FillPDFForm{}.Execute(context.Background(),
		json.RawMessage(`{"path":"`+form+`","values":{"full_name":"Мария Иванова","married":"да","pet":"cat"}}`))
	if err != nil || !strings.Contains(out, "Filled 2 fields") || !strings.Contains(out, "(filled).pdf") || !strings.Contains(out, "pet: there is no field") {
		t.Fatalf("%v\n%s", err, out)
	}

	if orig, _ := os.ReadFile(form); !bytes.Equal(orig, smallForm()) {
		t.Error("the original was changed")
	}

	fields, _ := FormFields(context.Background(), filepath.Join(dir, "заявление (filled).pdf"))
	if fields[0].Value != "Мария Иванова" || fields[1].Value != "On" {
		t.Errorf("%+v", fields)
	}

	// A second fill goes beside the first, never over it.
	if FilledName(form) != filepath.Join(dir, "заявление (filled 2).pdf") {
		t.Errorf("%s", FilledName(form))
	}

	if _, err := FillForm(context.Background(), form, map[string]string{"full_name": "x"}, filepath.Join(dir, "заявление (filled).pdf")); !errors.Is(err, ErrFileThere) {
		t.Errorf("a file was written over: %v", err)
	}

	if _, err := FillForm(context.Background(), form, map[string]string{"nobody": "x"}, ""); err == nil {
		t.Error("a fill with no field to fill succeeded")
	}

	if _, err := FormFields(context.Background(), "form.pdf"); err == nil {
		t.Error("a relative path was read")
	}
}
