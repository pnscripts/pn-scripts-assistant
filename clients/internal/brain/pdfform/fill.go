package pdfform

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Problem is a value that could not be put in a field, and why.
type Problem struct {
	Field string `json:"field"`
	Value string `json:"value"`
	Why   string `json:"why"`
	Key   string `json:"key"`
}

var (
	yes = map[string]bool{"yes": true, "true": true, "1": true, "on": true, "x": true, "✓": true, "checked": true, "да": true, "вярно": true}
	no  = map[string]bool{"no": true, "false": true, "0": true, "off": true, "": true, "unchecked": true, "не": true}
)

// value turns what somebody wants in a field into what the field takes:
// the text for a text field, the on or Off state for a checkbox, one of the
// on states for a radio group, one of the items for a choice.
func value(f Field, want string) (any, *Problem) {
	problem := func(key, why string) *Problem {
		return &Problem{Field: f.Name, Value: want, Why: why, Key: key}
	}

	if f.ReadOnly {
		return nil, problem("readOnly", "the field is read-only")
	}

	switch f.Kind {
	case Text:
		if f.MaxLen > 0 && utf8.RuneCountInString(want) > f.MaxLen {
			return nil, problem("tooLong", fmt.Sprintf("the field takes at most %d characters", f.MaxLen))
		}

		return TextString(want), nil
	case Checkbox:
		w := strings.ToLower(strings.TrimSpace(want))

		switch {
		case yes[w]:
			if len(f.Options) == 0 {
				return Name("Yes"), nil
			}

			return Name(f.Options[0]), nil
		case no[w]:
			return Name("Off"), nil
		}

		for _, o := range f.Options {
			if strings.EqualFold(o, want) {
				return Name(o), nil
			}
		}

		return nil, problem("notYesNo", "a checkbox is yes or no")
	case Radio:
		if no[strings.ToLower(strings.TrimSpace(want))] {
			return Name("Off"), nil
		}

		for _, o := range f.Options {
			if strings.EqualFold(o, strings.TrimSpace(want)) {
				return Name(o), nil
			}
		}

		return nil, problem("notAnOption", "the choices are: "+strings.Join(f.Options, ", "))
	case Choice:
		for _, o := range f.Options {
			if strings.EqualFold(o, strings.TrimSpace(want)) {
				return TextString(o), nil
			}
		}

		if f.Edit || len(f.Options) == 0 {
			return TextString(want), nil
		}

		return nil, problem("notAnOption", "the choices are: "+strings.Join(f.Options, ", "))
	case Signature:
		return nil, problem("signature", "a signature field is signed in a PDF program, not filled")
	}

	return nil, problem("button", "a push button holds no value")
}

// Filled is the outcome of filling a form.
type Filled struct {
	PDF      []byte    `json:"-"`
	Set      []string  `json:"set"`
	Problems []Problem `json:"problems,omitempty"`
}

// ErrNothingToFill is a fill with no value that could be put anywhere.
var ErrNothingToFill = errors.New("none of the values could be put in a field")

/*
 * Fill puts values in the form's fields, by their full names, and returns
 * the document with the changes added after its original bytes.
 *
 * A text field keeps its value as a text string (Unicode when it needs to
 * be) and loses its old appearance, so the reader draws the new value with
 * the form's own font — the form is marked NeedAppearances for that. A
 * checkbox or a radio button gets its state on the field and on each of its
 * widgets. A name that is not a field, a read-only field, a value a field
 * cannot take: each is said, and the rest is still filled.
 */
func (d *Doc) Fill(values map[string]string) (Filled, error) {
	if d.Encrypted() {
		return Filled{}, ErrEncrypted
	}

	if d.Signed() {
		return Filled{}, ErrSigned
	}

	fields, err := d.Fields()
	if err != nil {
		return Filled{}, err
	}

	byName := map[string]Field{}
	for _, f := range fields {
		byName[strings.ToLower(f.Name)] = f
	}

	changed := map[int]any{} // object number → its new version, or a new object
	gens := map[int]int{}

	edit := func(r Ref) Dict {
		if c, ok := changed[r.Num].(Dict); ok {
			return c
		}

		orig := d.dict(r)
		c := Dict{}

		for k, v := range orig {
			c[k] = v
		}

		changed[r.Num], gens[r.Num] = c, r.Gen

		return c
	}

	var (
		out  Filled
		jobs []drawJob
	)

	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}

	sort.Strings(names)

	for _, name := range names {
		want := values[name]

		f, ok := byName[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			out.Problems = append(out.Problems, Problem{Field: name, Value: want, Why: "there is no field of that name", Key: "noField"})

			continue
		}

		v, problem := value(f, want)
		if problem != nil {
			out.Problems = append(out.Problems, *problem)

			continue
		}

		if f.ref == (Ref{}) {
			out.Problems = append(out.Problems, Problem{Field: f.Name, Value: want, Why: "the field is not an object of its own and cannot be changed", Key: "inline"})

			continue
		}

		edit(f.ref)["V"] = v

		for _, w := range f.widgets {
			wd := edit(w)

			switch f.Kind {
			case Text, Choice:
				// The old picture of the old value would be shown over the
				// new one: it is replaced below, or taken away so the reader
				// draws the value itself.
				delete(wd, "AP")

				text := want
				if s, ok := v.(String); ok {
					text = s.Text()
				}

				jobs = append(jobs, drawJob{widget: w, field: f, text: text})
			case Checkbox, Radio:
				state := Name("Off")

				if n, ok := v.(Name); ok && n != "Off" {
					if normal := d.dict(d.dict(wd["AP"])["N"]); normal != nil {
						if _, has := normal[n]; has {
							state = n
						}
					} else {
						state = n
					}
				}

				wd["AS"] = state
			}
		}

		out.Set = append(out.Set, f.Name)
	}

	if len(out.Set) == 0 {
		return out, ErrNothingToFill
	}

	// Each filled text gets its own picture, with fonts that can draw it.
	next := intOf(d.trailer["Size"], 0)
	for n := range d.xref {
		next = max(next, n+1)
	}

	alloc := func(obj any) Ref {
		r := Ref{next, 0}
		changed[next], gens[next] = obj, 0
		next++

		return r
	}

	// A field the form left without a picture of its value is drawn too,
	// so no reader has to be asked to draw anything from the form's font.
	filled := map[string]bool{}
	for _, n := range out.Set {
		filled[n] = true
	}

	for _, f := range fields {
		if filled[f.Name] || f.Value == "" || (f.Kind != Text && f.Kind != Choice) {
			continue
		}

		for _, w := range f.widgets {
			if d.dict(w)["AP"] == nil {
				jobs = append(jobs, drawJob{widget: w, field: f, text: f.Value})
			}
		}
	}

	drawn := d.draw(jobs, edit, alloc)
	out.Problems = append(out.Problems, drawn...)

	/*
	 * NeedAppearances tells a reader to draw every field itself, from the
	 * form's own font — and a reader that does ignores the pictures drawn
	 * here, which is exactly what goes wrong with Cyrillic. So it is asked
	 * for only when something still has no picture: a value no font here
	 * could draw, or a field of the form that never had one.
	 */
	need := len(drawn) > 0

	if all, err := d.Fields(); err == nil {
		for _, f := range all {
			if f.Kind != Text && f.Kind != Choice {
				continue
			}

			for _, w := range f.widgets {
				wd, isNew := changed[w.Num].(Dict)
				if !isNew {
					wd = d.dict(w)
				}

				if wd["AP"] == nil && (f.Value != "" || isNew) {
					need = true
				}
			}
		}
	}

	cat := d.trailer["Root"].(Ref)

	if ar, ok := d.catalog()["AcroForm"].(Ref); ok {
		edit(ar)["NeedAppearances"] = need
	} else {
		c := edit(cat)
		acro := Dict{}

		for k, v := range d.dict(c["AcroForm"]) {
			acro[k] = v
		}

		acro["NeedAppearances"] = need
		c["AcroForm"] = acro
	}

	out.PDF = d.update(changed, gens)

	return out, nil
}

// update writes the changed objects after the original bytes, with a
// cross-reference section for them pointing back to the one before.
func (d *Doc) update(changed map[int]any, gens map[int]int) []byte {
	var b bytes.Buffer

	b.Write(d.data)

	if !bytes.HasSuffix(d.data, []byte("\n")) {
		b.WriteByte('\n')
	}

	nums := make([]int, 0, len(changed))
	for n := range changed {
		nums = append(nums, n)
	}

	sort.Ints(nums)

	offsets := map[int]int{}

	for _, n := range nums {
		offsets[n] = b.Len()
		fmt.Fprintf(&b, "%d %d obj\n", n, gens[n])
		writeObject(&b, changed[n])
		b.WriteString("\nendobj\n")
	}

	size := intOf(d.trailer["Size"], 0)
	for n := range d.xref {
		size = max(size, n+1)
	}

	for n := range changed {
		size = max(size, n+1)
	}

	trailer := Dict{"Size": int64(size), "Root": d.trailer["Root"]}

	for _, k := range []Name{"Info", "ID"} {
		if v, ok := d.trailer[k]; ok {
			trailer[k] = v
		}
	}

	if d.startxref > 0 {
		trailer["Prev"] = d.startxref
	}

	xrefAt := b.Len()

	if d.stream {
		// The section before is a stream, so this one is too: an
		// uncompressed one, three fields of 1, 4 and 2 bytes per object.
		num := size
		trailer["Size"] = int64(size + 1)
		trailer["Type"] = Name("XRef")
		trailer["W"] = Array{int64(1), int64(4), int64(2)}

		var rows bytes.Buffer

		var index Array

		all := append(append([]int(nil), nums...), num)
		offsets[num] = xrefAt

		for _, n := range all {
			index = append(index, int64(n), int64(1))
			o := offsets[n]
			rows.Write([]byte{1, byte(o >> 24), byte(o >> 16), byte(o >> 8), byte(o), byte(gens[n] >> 8), byte(gens[n])})
		}

		trailer["Index"] = index
		trailer["Length"] = int64(rows.Len())

		fmt.Fprintf(&b, "%d 0 obj\n", num)
		writeObject(&b, Stream{Dict: trailer, Raw: rows.Bytes()})
		b.WriteString("\nendobj\n")
	} else {
		b.WriteString("xref\n")

		// A file whose own table was damaged gets a whole new one: every
		// object, where it was found or where its new version is.
		listed := nums
		if d.startxref == 0 {
			listed = nil

			for n, e := range d.xref {
				if e.kind == 1 {
					if _, isNew := offsets[n]; !isNew {
						offsets[n], gens[n] = int(e.offset), e.gen
					}

					listed = append(listed, n)
				}
			}

			for _, n := range nums {
				if e, ok := d.xref[n]; !ok || e.kind != 1 {
					listed = append(listed, n)
				}
			}

			sort.Ints(listed)
		}

		for _, n := range listed {
			fmt.Fprintf(&b, "%d 1\n%010d %05d n \n", n, offsets[n], gens[n])
		}

		b.WriteString("trailer\n")
		writeObject(&b, trailer)
		b.WriteByte('\n')
	}

	fmt.Fprintf(&b, "startxref\n%d\n%%%%EOF\n", xrefAt)

	return b.Bytes()
}

// Check reads a filled document back and says which values are not what
// was asked for: the proof that the file says what was meant.
func Check(pdf []byte, values map[string]string, set []string) ([]Problem, error) {
	d, err := Open(pdf)
	if err != nil {
		return nil, err
	}

	fields, err := d.Fields()
	if err != nil {
		return nil, err
	}

	byName := map[string]Field{}
	for _, f := range fields {
		byName[strings.ToLower(f.Name)] = f
	}

	given := map[string]string{}
	for k, v := range values {
		given[strings.ToLower(strings.TrimSpace(k))] = v
	}

	var out []Problem

	for _, name := range set {
		f := byName[strings.ToLower(name)]
		want, _ := value(f, given[strings.ToLower(name)])

		var expect string

		switch w := want.(type) {
		case String:
			expect = w.Text()
		case Name:
			expect = string(w)
		}

		if f.Value != expect {
			out = append(out, Problem{Field: name, Value: f.Value, Why: fmt.Sprintf("reads back as %q, not %q", f.Value, expect), Key: "readBack"})
		}
	}

	return out, nil
}

// drawJob is one widget of a filled text field, to be drawn.
type drawJob struct {
	widget Ref
	field  Field
	text   string
}

// draw gives every filled text widget its own picture: Helvetica for
// plain ASCII, an embedded Unicode font for anything else. A value no font
// on this computer can draw is still filled, and said: readers that draw
// fields themselves will show it.
func (d *Doc) draw(jobs []drawJob, edit func(Ref) Dict, alloc func(any) Ref) []Problem {
	if len(jobs) == 0 {
		return nil
	}

	acro := d.dict(d.catalog()["AcroForm"])

	var (
		problems  []Problem
		wide      []string
		helv, uni *face
	)

	for _, j := range jobs {
		if !ascii(j.text) {
			wide = append(wide, j.text)
		}
	}

	if len(wide) > 0 {
		if f := unicodeFont(wide); f != nil {
			uni = &face{unicode: f}
			uni.ref = embed(f, wide, alloc)
		}
	}

	for _, j := range jobs {
		var fc *face

		switch {
		case ascii(j.text):
			if helv == nil {
				helv = &face{}
				helv.ref = alloc(Dict{"Type": Name("Font"), "Subtype": Name("Type1"), "BaseFont": Name("Helvetica"), "Encoding": Name("WinAnsiEncoding")})
			}

			fc = helv
		case uni != nil:
			fc = uni
		default:
			problems = append(problems, Problem{Field: j.field.Name, Value: j.text, Key: "noFont",
				Why: "filled, but no font on this computer can draw these letters; readers that draw fields themselves will show it"})

			continue
		}

		wd := d.dict(j.widget)

		var rect [4]float64
		if r, ok := d.resolve(wd["Rect"]).(Array); ok && len(r) == 4 {
			for i := range rect {
				rect[i] = number(d.resolve(r[i]))
			}
		}

		da := ""
		for _, src := range []Dict{wd, d.dict(j.field.ref), acro} {
			if s, ok := d.resolve(src["DA"]).(String); ok && da == "" {
				da = string(s.B)
			}
		}

		align := -1
		for _, src := range []Dict{wd, d.dict(j.field.ref), acro} {
			if q, ok := d.resolve(src["Q"]).(int64); ok && align < 0 {
				align = int(q)
			}
		}

		multiline := j.field.Kind == Text && j.field.flags&(1<<12) != 0

		ap := alloc(picture(*fc, j.text, rect, da, max(align, 0), multiline))
		edit(j.widget)["AP"] = Dict{"N": ap}
	}

	return problems
}
