package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/pdfform"
)

/*
 * PDF forms: the fields of a form read out, and the form filled in as a new
 * file beside it (package pdfform) — the original never changed, a file
 * that is there never written over, and every value read back from the new
 * file before it is called done.
 */

// FormFields reads the fields of a PDF form.
func FormFields(ctx context.Context, path string) ([]pdfform.Field, error) {
	data, err := readPDF(ctx, path)
	if err != nil {
		return nil, err
	}

	d, err := pdfform.Open(data)
	if err != nil {
		return nil, err
	}

	return d.Fields()
}

func readPDF(ctx context.Context, path string) ([]byte, error) {
	path = strings.TrimSpace(path)

	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("give the full path of the PDF, not %q", path)
	}

	if err := GuardSensitive(ctx, path); err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("there is no file at %s", path)
	}

	if info.Size() > 100<<20 {
		return nil, errors.New("the PDF is larger than 100 MB")
	}

	return os.ReadFile(path)
}

// FilledForm is a form filled in and saved.
type FilledForm struct {
	Path     string            `json:"path"`
	Set      []string          `json:"set"`
	Problems []pdfform.Problem `json:"problems,omitempty"`
}

// ErrFileThere is a new file whose name is already taken.
var ErrFileThere = errors.New("there is already a file at that path")

// FilledName is where a filled copy goes when nobody says: beside the form,
// "(filled)" after its name, a number after that when the name is taken.
func FilledName(path string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)

	for i := 1; ; i++ {
		try := base + " (filled)" + ext
		if i > 1 {
			try = fmt.Sprintf("%s (filled %d)%s", base, i, ext)
		}

		if _, err := os.Stat(try); err != nil {
			return try
		}
	}
}

// FillForm fills a PDF form and saves it as a new file, then reads every
// value back from the new file.
func FillForm(ctx context.Context, path string, values map[string]string, out string) (FilledForm, error) {
	data, err := readPDF(ctx, path)
	if err != nil {
		return FilledForm{}, err
	}

	out = strings.TrimSpace(out)
	if out == "" {
		out = FilledName(path)
	}

	if !filepath.IsAbs(out) || !strings.EqualFold(filepath.Ext(out), ".pdf") {
		return FilledForm{}, errors.New("give the full path of a new .pdf file to save the filled form to")
	}

	if err := GuardSensitive(ctx, out); err != nil {
		return FilledForm{}, err
	}

	d, err := pdfform.Open(data)
	if err != nil {
		return FilledForm{}, err
	}

	filled, err := d.Fill(values)
	if err != nil {
		return FilledForm{Problems: filled.Problems}, err
	}

	// The proof: the new file is read again and every value checked.
	back, err := pdfform.Check(filled.PDF, values, filled.Set)
	if err != nil {
		return FilledForm{}, fmt.Errorf("the filled form could not be read back: %w", err)
	}

	if len(back) > 0 {
		return FilledForm{Problems: back}, errors.New("the filled form did not read back as filled; nothing was saved")
	}

	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return FilledForm{}, fmt.Errorf("%w: %s", ErrFileThere, out)
		}

		return FilledForm{}, err
	}

	if _, err := f.Write(filled.PDF); err != nil {
		f.Close()
		_ = os.Remove(out)

		return FilledForm{}, err
	}

	if err := f.Close(); err != nil {
		return FilledForm{}, err
	}

	return FilledForm{Path: out, Set: filled.Set, Problems: filled.Problems}, nil
}

// formCues are the words of somebody with a form to fill.
var formCues = []string{"form", "fill", "pdf", "application", "формуляр", "бланк", "попълн", "заявлени", "декларация"}

// PDFFormFields lists a PDF form's fields.
type PDFFormFields struct{}

func (PDFFormFields) Name() string   { return "pdf_form_fields" }
func (PDFFormFields) Risk() Risk     { return Safe }
func (PDFFormFields) Cues() []string { return formCues }

func (PDFFormFields) Description() string {
	return "List the fields of a fillable PDF form: each field's full name, kind (text, checkbox, radio, choice, " +
		"signature), what it holds now, its choices, and whether it is read-only or required. Use the names " +
		"exactly as given with fill_pdf_form. Changes nothing."
}

func (PDFFormFields) Parameters() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {"path": {"type": "string", "description": "Absolute path to the PDF"}}, "required": ["path"]}`)
}

func (PDFFormFields) Summarize(raw json.RawMessage) string {
	var a struct{ Path string }

	_ = json.Unmarshal(raw, &a)

	return "Read the form fields of " + a.Path
}

func (PDFFormFields) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct{ Path string }

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	fields, err := FormFields(ctx, a.Path)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%d fields:\n", len(fields))

	for _, f := range fields {
		fmt.Fprintf(&b, "- %q: %s", f.Name, f.Kind)

		if f.Value != "" {
			fmt.Fprintf(&b, ", now %q", f.Value)
		}

		if len(f.Options) > 0 {
			fmt.Fprintf(&b, ", choices: %s", strings.Join(f.Options, " | "))
		}

		if f.MaxLen > 0 {
			fmt.Fprintf(&b, ", at most %d characters", f.MaxLen)
		}

		if f.ReadOnly {
			b.WriteString(", read-only")
		}

		if f.Required {
			b.WriteString(", required")
		}

		b.WriteString("\n")
	}

	return b.String(), nil
}

// FillPDFForm fills a PDF form and saves it as a new file.
type FillPDFForm struct{}

func (FillPDFForm) Name() string   { return "fill_pdf_form" }
func (FillPDFForm) Cues() []string { return formCues }

// Mutating: it writes a new file. It never changes the form itself and
// never writes over a file that is there.
func (FillPDFForm) Risk() Risk { return Mutating }

func (FillPDFForm) Description() string {
	return "Fill a PDF form and save it as a new file (beside the form, \"(filled)\" after its name, unless out " +
		"says where): values by the field names pdf_form_fields gives — text as it should read, yes/no for a " +
		"checkbox, one of the choices for a radio group or a list. Letters of any language are drawn with a font " +
		"that has them. Every value is read back from the new file before it is saved; a value a field cannot " +
		"take is said and the rest filled. The original is never changed. Encrypted and signed PDFs are refused."
}

func (FillPDFForm) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute path to the PDF form"},
			"values": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Field name → value"},
			"out": {"type": "string", "description": "Absolute path of the new PDF; beside the form when not given"}
		},
		"required": ["path", "values"]
	}`)
}

type fillArgs struct {
	Path   string            `json:"path"`
	Values map[string]string `json:"values"`
	Out    string            `json:"out"`
}

func (FillPDFForm) Summarize(raw json.RawMessage) string {
	var a fillArgs

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Fill %d fields of %s into a new file", len(a.Values), filepath.Base(a.Path))
}

func (FillPDFForm) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a fillArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	done, err := FillForm(ctx, a.Path, a.Values, a.Out)
	if err != nil {
		var b strings.Builder

		b.WriteString(err.Error())

		for _, p := range done.Problems {
			fmt.Fprintf(&b, "\n- %s: %s", p.Field, p.Why)
		}

		return "", errors.New(b.String())
	}

	var b strings.Builder

	fmt.Fprintf(&b, "Filled %d fields and saved the form as %s; every value read back from the new file. The original is unchanged.", len(done.Set), done.Path)

	for _, p := range done.Problems {
		fmt.Fprintf(&b, "\n- %s: %s", p.Field, p.Why)
	}

	return b.String(), nil
}
