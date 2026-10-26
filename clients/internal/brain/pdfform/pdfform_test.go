package pdfform

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// formObjects is a small form: a page with a name field, an e-mail field of
// at most 30 characters, a read-only reference, a checkbox, a radio group
// of two buttons, a choice list, and a parent "address" with a "city" in it.
func formObjects() map[int]string {
	ap := func(on string) string {
		return fmt.Sprintf("/AP << /N << /%s 20 0 R /Off 21 0 R >> >>", on)
	}

	return map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /AcroForm 4 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Annots [5 0 R 6 0 R 7 0 R 8 0 R 10 0 R 11 0 R 12 0 R 14 0 R] /Resources << >> >>",
		4:  "<< /Fields [5 0 R 6 0 R 7 0 R 8 0 R 9 0 R 12 0 R 13 0 R] /DA (/Helv 10 Tf 0 g) /DR << /Font << /Helv 22 0 R >> >> >>",
		5:  "<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /Rect [50 700 300 720] /P 3 0 R /DA (/Helv 10 Tf 0 g) /AP << /N 20 0 R >> >>",
		6:  "<< /Type /Annot /Subtype /Widget /FT /Tx /T (email) /MaxLen 30 /Rect [50 670 300 690] /P 3 0 R /DA (/Helv 10 Tf 0 g) >>",
		7:  "<< /Type /Annot /Subtype /Widget /FT /Tx /T (reference) /Ff 1 /V (A-17) /Rect [50 640 300 660] /P 3 0 R >>",
		8:  "<< /Type /Annot /Subtype /Widget /FT /Btn /T (agree) /V /Off /AS /Off /Rect [50 610 64 624] /P 3 0 R " + ap("Yes") + " >>",
		9:  "<< /FT /Btn /Ff 32768 /T (size) /V /Off /Kids [10 0 R 11 0 R] >>",
		10: "<< /Type /Annot /Subtype /Widget /Parent 9 0 R /AS /Off /Rect [50 580 64 594] /P 3 0 R " + ap("Small") + " >>",
		11: "<< /Type /Annot /Subtype /Widget /Parent 9 0 R /AS /Off /Rect [80 580 94 594] /P 3 0 R " + ap("Large") + " >>",
		12: "<< /Type /Annot /Subtype /Widget /FT /Ch /Ff 131072 /T (country) /Opt [(Bulgaria) (Greece) [(RO) (Romania)]] /Rect [50 550 300 570] /P 3 0 R /DA (/Helv 10 Tf 0 g) >>",
		13: "<< /T (address) /Kids [14 0 R] >>",
		14: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (city) /Parent 13 0 R /Rect [50 520 300 540] /P 3 0 R /DA (/Helv 10 Tf 0 g) >>",
		20: "<< /Type /XObject /Subtype /Form /BBox [0 0 14 14] /Length 0 >>\nstream\n\nendstream",
		21: "<< /Type /XObject /Subtype /Form /BBox [0 0 14 14] /Length 0 >>\nstream\n\nendstream",
		22: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
}

// classic writes the objects with a cross-reference table.
func classic(objs map[int]string) []byte {
	var b bytes.Buffer

	b.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	nums := make([]int, 0, len(objs))
	size := 0

	for n := range objs {
		nums = append(nums, n)
		size = max(size, n+1)
	}

	sort.Ints(nums)

	offsets := map[int]int{}

	for _, n := range nums {
		offsets[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, objs[n])
	}

	at := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", size)

	for n := 1; n < size; n++ {
		if o, ok := offsets[n]; ok {
			fmt.Fprintf(&b, "%010d 00000 n \n", o)
		} else {
			b.WriteString("0000000000 65535 f \n")
		}
	}

	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", size, at)

	return b.Bytes()
}

// compressed writes the same objects the way PDF 1.5 writers do: every
// object that is not a stream inside one compressed object stream, and a
// compressed cross-reference stream with the PNG Up predictor.
func compressed(objs map[int]string) []byte {
	var b bytes.Buffer

	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")

	var inStream, plain []int

	size := 0

	for n, o := range objs {
		size = max(size, n+1)

		if strings.Contains(o, "stream") {
			plain = append(plain, n)
		} else {
			inStream = append(inStream, n)
		}
	}

	sort.Ints(inStream)
	sort.Ints(plain)

	stm := size
	xrefNum := size + 1
	size += 2

	type at struct{ kind, a, b int }

	where := map[int]at{}

	for _, n := range plain {
		where[n] = at{1, b.Len(), 0}
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, objs[n])
	}

	var head, body bytes.Buffer

	for i, n := range inStream {
		fmt.Fprintf(&head, "%d %d ", n, body.Len())
		body.WriteString(objs[n] + "\n")
		where[n] = at{2, stm, i}
	}

	data := deflate(append(head.Bytes(), body.Bytes()...))
	where[stm] = at{1, b.Len(), 0}
	fmt.Fprintf(&b, "%d 0 obj\n<< /Type /ObjStm /N %d /First %d /Filter /FlateDecode /Length %d >>\nstream\n", stm, len(inStream), head.Len(), len(data))
	b.Write(data)
	b.WriteString("\nendstream\nendobj\n")

	xrefAt := b.Len()
	where[xrefNum] = at{1, xrefAt, 0}

	var rows []byte

	prev := make([]byte, 7)

	for n := 0; n < size; n++ {
		w := where[n]
		row := []byte{byte(w.kind), byte(w.a >> 24), byte(w.a >> 16), byte(w.a >> 8), byte(w.a), byte(w.b >> 8), byte(w.b)}

		if n == 0 {
			row = []byte{0, 0, 0, 0, 0, 0xFF, 0xFF}
		}

		rows = append(rows, 2) // PNG Up

		for i := range row {
			rows = append(rows, row[i]-prev[i])
		}

		prev = row
	}

	xdata := deflate(rows)
	fmt.Fprintf(&b, "%d 0 obj\n<< /Type /XRef /Size %d /W [1 4 2] /Root 1 0 R /Filter /FlateDecode /DecodeParms << /Predictor 12 /Columns 7 >> /Length %d >>\nstream\n",
		xrefNum, size, len(xdata))
	b.Write(xdata)
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", xrefAt)

	return b.Bytes()
}

func names(fields []Field) string {
	var out []string
	for _, f := range fields {
		out = append(out, f.Name+":"+f.Kind)
	}

	return strings.Join(out, " ")
}

func TestTheFieldsAreReadInBothLayouts(t *testing.T) {
	for layout, pdf := range map[string][]byte{"classic": classic(formObjects()), "compressed": compressed(formObjects())} {
		d, err := Open(pdf)
		if err != nil {
			t.Fatalf("%s: %v", layout, err)
		}

		fields, err := d.Fields()
		if err != nil {
			t.Fatalf("%s: %v", layout, err)
		}

		want := "name:text email:text reference:text agree:checkbox size:radio country:choice address.city:text"
		if names(fields) != want {
			t.Errorf("%s: %s", layout, names(fields))
		}

		by := map[string]Field{}
		for _, f := range fields {
			by[f.Name] = f
		}

		if by["email"].MaxLen != 30 || !by["reference"].ReadOnly || by["reference"].Value != "A-17" ||
			strings.Join(by["size"].Options, ",") != "Large,Small" || strings.Join(by["country"].Options, ",") != "Bulgaria,Greece,RO" ||
			strings.Join(by["agree"].Options, ",") != "Yes" {
			t.Errorf("%s: %+v", layout, by)
		}
	}
}

func TestAFormIsFilledAndReadsBack(t *testing.T) {
	values := map[string]string{
		"name": "Иван Петров (ИП)", "email": "ivan@example.com", "agree": "да", "size": "large", "country": "Greece",
		"Address.City": "Пловдив", "reference": "B-1", "nickname": "x",
	}

	for layout, pdf := range map[string][]byte{"classic": classic(formObjects()), "compressed": compressed(formObjects())} {
		d, _ := Open(pdf)

		out, err := d.Fill(values)
		if err != nil {
			t.Fatalf("%s: %v", layout, err)
		}

		if !bytes.HasPrefix(out.PDF, pdf) {
			t.Errorf("%s: the original bytes were changed", layout)
		}

		var why []string
		for _, p := range out.Problems {
			why = append(why, p.Field+":"+p.Key)
		}

		sort.Strings(why)

		if strings.Join(why, " ") != "nickname:noField reference:readOnly" {
			t.Errorf("%s: problems %q", layout, why)
		}

		if problems, err := Check(out.PDF, values, out.Set); err != nil || len(problems) != 0 {
			t.Errorf("%s: read back %v %+v", layout, err, problems)
		}

		again, _ := Open(out.PDF)
		fields, _ := again.Fields()

		by := map[string]Field{}
		for _, f := range fields {
			by[f.Name] = f
		}

		if by["name"].Value != "Иван Петров (ИП)" || by["agree"].Value != "Yes" || by["size"].Value != "Large" ||
			by["country"].Value != "Greece" || by["address.city"].Value != "Пловдив" || by["reference"].Value != "A-17" {
			t.Errorf("%s: %+v", layout, by)
		}

		// The widgets show it: the checkbox and the right radio button on.
		if as := again.dict(Ref{8, 0})["AS"]; as != Name("Yes") {
			t.Errorf("%s: the checkbox shows %v", layout, as)
		}

		if a, b := again.dict(Ref{10, 0})["AS"], again.dict(Ref{11, 0})["AS"]; a != Name("Off") || b != Name("Large") {
			t.Errorf("%s: the radio buttons show %v %v", layout, a, b)
		}

		// Every filled value has a picture, so readers are not told to
		// redraw them from the form's font (which has no Cyrillic) — unless
		// this machine had no font to draw "Иван" with.
		if need := again.dict(again.catalog()["AcroForm"])["NeedAppearances"]; need != (unicodeFont([]string{"Иван"}) == nil) {
			t.Errorf("%s: NeedAppearances is %v", layout, need)
		}

		// The name has a new picture, drawn in an embedded Unicode font
		// (when this machine has one), and the e-mail one in Helvetica.
		pic := func(widget int) (Stream, Dict) {
			ap := again.dict(again.dict(Ref{widget, 0})["AP"])
			s, _ := again.resolve(ap["N"]).(Stream)
			font := again.dict(again.dict(again.dict(s.Dict["Resources"])["Font"])["F1"])

			return s, font
		}

		if s, font := pic(6); !bytes.Contains(s.Raw, []byte("(ivan@example.com) Tj")) || font["BaseFont"] != Name("Helvetica") {
			t.Errorf("%s: the e-mail's picture: %s %v", layout, s.Raw, font)
		}

		if s, font := pic(5); unicodeFont([]string{"Иван"}) != nil {
			if ap, _ := again.dict(Ref{5, 0})["AP"].(Dict); ap["N"] == (Ref{20, 0}) || font["Subtype"] != Name("Type0") || !bytes.Contains(s.Raw, []byte("> Tj")) {
				t.Errorf("%s: the name's picture: %s %v", layout, s.Raw, font)
			}
		}

		// Filled again, on top of the update.
		twice, err := again.Fill(map[string]string{"name": "Second"})
		if err != nil {
			t.Fatal(err)
		}

		if d3, _ := Open(twice.PDF); d3 != nil {
			if f, _ := d3.Fields(); f[0].Value != "Second" || f[6].Value != "Пловдив" {
				t.Errorf("%s: a second fill: %+v", layout, f)
			}
		}

		if poppler(t, layout, out.PDF) == "" {
			continue
		}
	}
}

// poppler has an independent reader check the filled file, when it is on
// this machine.
func poppler(t *testing.T, layout string, pdf []byte) string {
	bin, err := exec.LookPath("pdfinfo")
	if err != nil {
		return ""
	}

	path := filepath.Join(t.TempDir(), layout+".pdf")
	_ = os.WriteFile(path, pdf, 0o600)

	out, err := exec.Command(bin, path).CombinedOutput()
	if err != nil || strings.Contains(string(out), "Error") || !strings.Contains(string(out), "Form:") {
		t.Errorf("%s: pdfinfo: %v\n%s", layout, err, out)
	}

	return string(out)
}

func TestValuesAFieldCannotTakeAreSaid(t *testing.T) {
	d, _ := Open(classic(formObjects()))

	out, err := d.Fill(map[string]string{
		"email": strings.Repeat("a", 31), "agree": "maybe", "size": "medium", "country": "Spain", "name": "ok",
	})
	if err != nil {
		t.Fatal(err)
	}

	var why []string
	for _, p := range out.Problems {
		why = append(why, p.Field+":"+p.Key)
	}

	sort.Strings(why)

	if strings.Join(why, " ") != "agree:notYesNo country:notAnOption email:tooLong size:notAnOption" || len(out.Set) != 1 {
		t.Errorf("%q %q", why, out.Set)
	}

	if _, err := d.Fill(map[string]string{"nickname": "x"}); !errors.Is(err, ErrNothingToFill) {
		t.Errorf("%v", err)
	}
}

func TestEncryptedSignedAndFormlessPDFsAreLeftAlone(t *testing.T) {
	objs := formObjects()
	pdf := classic(objs)
	pdf = bytes.Replace(pdf, []byte("/Root 1 0 R >>"), []byte("/Root 1 0 R /Encrypt 30 0 R >>"), 1)

	d, _ := Open(pdf)
	if _, err := d.Fields(); !errors.Is(err, ErrEncrypted) {
		t.Errorf("encrypted: %v", err)
	}

	signed := formObjects()
	signed[4] = strings.Replace(signed[4], "/Fields [", "/SigFlags 3 /Fields [", 1)

	d, _ = Open(classic(signed))
	if _, err := d.Fill(map[string]string{"name": "x"}); !errors.Is(err, ErrSigned) {
		t.Errorf("signed: %v", err)
	}

	plain := map[int]string{1: "<< /Type /Catalog /Pages 2 0 R >>", 2: "<< /Type /Pages /Kids [] /Count 0 >>"}

	d, _ = Open(classic(plain))
	if _, err := d.Fields(); !errors.Is(err, ErrNoForm) {
		t.Errorf("formless: %v", err)
	}

	if _, err := Open([]byte("hello")); !errors.Is(err, ErrNotPDF) {
		t.Errorf("not a pdf: %v", err)
	}
}

func TestADamagedTableIsRebuiltAndTheUpdateIsWhole(t *testing.T) {
	pdf := classic(formObjects())

	// Every offset in the table off by seven bytes, as after a careless edit.
	broken := append([]byte("%junk!\n"), pdf...)

	d, err := Open(broken)
	if err != nil {
		t.Fatal(err)
	}

	out, err := d.Fill(map[string]string{"name": "Mended"})
	if err != nil {
		t.Fatal(err)
	}

	d2, err := Open(out.PDF)
	if err != nil {
		t.Fatal(err)
	}

	if f, _ := d2.Fields(); len(f) != 7 || f[0].Value != "Mended" {
		t.Errorf("%+v", f)
	}
}

func TestTextStringsCarryAnyLetter(t *testing.T) {
	for _, s := range []string{"plain", "Пловдив", "€ 12,50", "a (b) c\\d"} {
		got := TextString(s)

		var b bytes.Buffer
		writeObject(&b, got)

		l := &lexer{b: b.Bytes()}
		back, err := l.object()

		if err != nil || back.(String).Text() != s {
			t.Errorf("%q: %v %q", s, err, b.String())
		}
	}
}
