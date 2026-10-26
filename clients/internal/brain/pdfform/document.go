package pdfform

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type entry struct {
	kind   int // 1 in the file at offset, 2 inside object stream stm at index
	offset int64
	stm    int
	index  int
	gen    int
}

// Doc is an opened PDF.
type Doc struct {
	data    []byte
	xref    map[int]entry
	trailer Dict

	// startxref is where the last cross-reference section starts, which an
	// update points back to; stream is whether that section is a stream.
	startxref int64
	stream    bool

	objstm map[int]*objStream
}

type objStream struct {
	first   int
	offsets map[int]int // object number → offset after First
	data    []byte
}

// Errors a form cannot be read or filled for.
var (
	ErrNotPDF    = errors.New("this is not a PDF file")
	ErrEncrypted = errors.New("this PDF is encrypted; open it with its password in a PDF program and save an unencrypted copy first")
	ErrNoForm    = errors.New("this PDF has no form fields to fill")
	ErrSigned    = errors.New("this PDF is signed; filling it would break the signature, so it is left as it is")
)

// Open reads a PDF's structure.
func Open(data []byte) (*Doc, error) {
	if !bytes.HasPrefix(bytes.TrimLeft(data[:min(len(data), 1024)], "\x00\r\n\t "), []byte("%PDF-")) && !bytes.Contains(data[:min(len(data), 1024)], []byte("%PDF-")) {
		return nil, ErrNotPDF
	}

	d := &Doc{data: data, xref: map[int]entry{}, objstm: map[int]*objStream{}}

	at := bytes.LastIndex(data, []byte("startxref"))
	if at < 0 {
		return d, d.rebuild()
	}

	l := &lexer{b: data, pos: at + len("startxref")}

	off, err := strconv.ParseInt(l.token(), 10, 64)
	if err != nil || off <= 0 || off >= int64(len(data)) {
		return d, d.rebuild()
	}

	d.startxref = off

	seen := map[int64]bool{}

	for first := true; off > 0 && !seen[off]; first = false {
		seen[off] = true

		trailer, stream, err := d.section(off)
		if err != nil {
			return d, d.rebuild()
		}

		if first {
			d.trailer, d.stream = trailer, stream
		}

		// A hybrid file keeps the stream part beside the table.
		if x, ok := trailer["XRefStm"]; ok {
			if _, _, err := d.section(int64(intOf(x, 0))); err != nil {
				return d, d.rebuild()
			}
		}

		off = int64(intOf(trailer["Prev"], 0))
	}

	if d.trailer["Root"] == nil {
		return d, d.rebuild()
	}

	return d, nil
}

// section reads one cross-reference section, keeping only entries a newer
// section has not already given.
func (d *Doc) section(off int64) (Dict, bool, error) {
	l := &lexer{b: d.data, pos: int(off)}
	l.skip()

	if bytes.HasPrefix(d.data[l.pos:], []byte("xref")) {
		l.pos += 4

		for {
			save := l.pos

			start, err1 := strconv.Atoi(l.token())
			count, err2 := strconv.Atoi(l.token())

			if err1 != nil || err2 != nil {
				l.pos = save

				break
			}

			for i := 0; i < count; i++ {
				o, _ := strconv.ParseInt(l.token(), 10, 64)
				g, _ := strconv.Atoi(l.token())
				kind := l.token()

				num := start + i
				if _, have := d.xref[num]; have {
					continue
				}

				if kind == "n" {
					d.xref[num] = entry{kind: 1, offset: o, gen: g}
				} else {
					d.xref[num] = entry{kind: 0}
				}
			}
		}

		if l.token() != "trailer" {
			return nil, false, errSyntax
		}

		t, err := l.object()
		if err != nil {
			return nil, false, err
		}

		dict, ok := t.(Dict)
		if !ok {
			return nil, false, errSyntax
		}

		return dict, false, nil
	}

	// A cross-reference stream: "n g obj << /Type /XRef … >> stream".
	obj, err := d.parseAt(off)
	if err != nil {
		return nil, false, err
	}

	s, ok := obj.(Stream)
	if !ok || s.Dict["Type"] != Name("XRef") {
		return nil, false, errSyntax
	}

	data, err := decode(s)
	if err != nil {
		return nil, false, err
	}

	w, _ := s.Dict["W"].(Array)
	if len(w) != 3 {
		return nil, false, errSyntax
	}

	widths := [3]int{intOf(w[0], 0), intOf(w[1], 0), intOf(w[2], 0)}
	rowLen := widths[0] + widths[1] + widths[2]

	if rowLen == 0 {
		return nil, false, errSyntax
	}

	index := []int{0, intOf(s.Dict["Size"], 0)}
	if ix, ok := s.Dict["Index"].(Array); ok {
		index = index[:0]
		for _, v := range ix {
			index = append(index, intOf(v, 0))
		}
	}

	field := func(b []byte) int64 {
		var v int64
		for _, c := range b {
			v = v<<8 | int64(c)
		}

		return v
	}

	pos := 0

	for i := 0; i+1 < len(index); i += 2 {
		for n := 0; n < index[i+1] && pos+rowLen <= len(data); n++ {
			row := data[pos : pos+rowLen]
			pos += rowLen

			kind := int64(1)
			if widths[0] > 0 {
				kind = field(row[:widths[0]])
			}

			a := field(row[widths[0] : widths[0]+widths[1]])
			b := field(row[widths[0]+widths[1]:])

			num := index[i] + n
			if _, have := d.xref[num]; have {
				continue
			}

			switch kind {
			case 1:
				d.xref[num] = entry{kind: 1, offset: a, gen: int(b)}
			case 2:
				d.xref[num] = entry{kind: 2, stm: int(a), index: int(b)}
			default:
				d.xref[num] = entry{kind: 0}
			}
		}
	}

	return s.Dict, true, nil
}

var objHeader = regexp.MustCompile(`(?m)(?:^|[\r\n\s])(\d+)\s+(\d+)\s+obj\b`)

// rebuild finds every object by scanning the file, for one whose
// cross-reference table is damaged — as many are after a careless edit.
func (d *Doc) rebuild() error {
	d.xref = map[int]entry{}

	for _, m := range objHeader.FindAllSubmatchIndex(d.data, -1) {
		num, _ := strconv.Atoi(string(d.data[m[2]:m[3]]))
		gen, _ := strconv.Atoi(string(d.data[m[4]:m[5]]))
		d.xref[num] = entry{kind: 1, offset: int64(m[2]), gen: gen}
	}

	// The trailer is the last one, or failing that the catalogue found by
	// its type.
	if at := bytes.LastIndex(d.data, []byte("trailer")); at >= 0 {
		l := &lexer{b: d.data, pos: at + len("trailer")}
		if t, err := l.object(); err == nil {
			d.trailer, _ = t.(Dict)
		}
	}

	if d.trailer == nil {
		d.trailer = Dict{}
	}

	if d.trailer["Root"] == nil {
		for num, e := range d.xref {
			if obj, err := d.parseAt(e.offset); err == nil {
				if dict, ok := obj.(Dict); ok && dict["Type"] == Name("Catalog") {
					d.trailer["Root"] = Ref{num, e.gen}
				}
			}
		}
	}

	if d.trailer["Root"] == nil {
		return errors.New("the PDF is damaged: its catalogue cannot be found")
	}

	// An update to a rebuilt file has nothing valid to point back to.
	d.startxref, d.stream = 0, false

	return nil
}

// parseAt reads "n g obj … endobj" at an offset.
func (d *Doc) parseAt(off int64) (any, error) {
	if off < 0 || off >= int64(len(d.data)) {
		return nil, errSyntax
	}

	l := &lexer{b: d.data, pos: int(off)}

	if _, err := strconv.Atoi(l.token()); err != nil {
		return nil, errSyntax
	}

	if _, err := strconv.Atoi(l.token()); err != nil {
		return nil, errSyntax
	}

	if l.token() != "obj" {
		return nil, errSyntax
	}

	obj, err := l.object()
	if err != nil {
		return nil, err
	}

	dict, isDict := obj.(Dict)
	if !isDict {
		return obj, nil
	}

	save := l.pos
	l.skip()

	if !bytes.HasPrefix(d.data[l.pos:], []byte("stream")) {
		l.pos = save

		return dict, nil
	}

	l.pos += len("stream")

	if l.pos < len(d.data) && d.data[l.pos] == '\r' {
		l.pos++
	}

	if l.pos < len(d.data) && d.data[l.pos] == '\n' {
		l.pos++
	}

	length := -1

	switch v := dict["Length"].(type) {
	case int64:
		length = int(v)
	case Ref:
		if lv, err := d.Get(v); err == nil {
			length = intOf(lv, -1)
		}
	}

	start := l.pos

	// A wrong /Length is common; "endstream" is the ground truth.
	if length < 0 || start+length > len(d.data) || !bytes.HasPrefix(bytes.TrimLeft(d.data[start+length:min(len(d.data), start+length+20)], "\r\n "), []byte("endstream")) {
		end := bytes.Index(d.data[start:], []byte("endstream"))
		if end < 0 {
			return nil, errSyntax
		}

		length = len(bytes.TrimRight(d.data[start:start+end], "\r\n"))
	}

	return Stream{Dict: dict, Raw: d.data[start : start+length]}, nil
}

// Get is the object a reference points to; nil when there is none.
func (d *Doc) Get(r Ref) (any, error) {
	e, ok := d.xref[r.Num]
	if !ok || e.kind == 0 {
		return nil, nil
	}

	if e.kind == 1 {
		return d.parseAt(e.offset)
	}

	os, err := d.objectStream(e.stm)
	if err != nil {
		return nil, err
	}

	off, ok := os.offsets[r.Num]
	if !ok {
		return nil, nil
	}

	l := &lexer{b: os.data, pos: os.first + off}

	return l.object()
}

func (d *Doc) objectStream(num int) (*objStream, error) {
	if os, ok := d.objstm[num]; ok {
		return os, nil
	}

	e := d.xref[num]
	if e.kind != 1 {
		return nil, fmt.Errorf("object stream %d is missing", num)
	}

	obj, err := d.parseAt(e.offset)
	if err != nil {
		return nil, err
	}

	s, ok := obj.(Stream)
	if !ok {
		return nil, fmt.Errorf("object stream %d is not a stream", num)
	}

	data, err := decode(s)
	if err != nil {
		return nil, err
	}

	os := &objStream{first: intOf(s.Dict["First"], 0), offsets: map[int]int{}, data: data}
	l := &lexer{b: data}

	for i := 0; i < intOf(s.Dict["N"], 0); i++ {
		n, err1 := strconv.Atoi(l.token())
		o, err2 := strconv.Atoi(l.token())

		if err1 != nil || err2 != nil {
			break
		}

		os.offsets[n] = o
	}

	d.objstm[num] = os

	return os, nil
}

// resolve follows a reference, if it is one.
func (d *Doc) resolve(v any) any {
	for i := 0; i < 8; i++ {
		r, ok := v.(Ref)
		if !ok {
			return v
		}

		v, _ = d.Get(r)
	}

	return v
}

func (d *Doc) dict(v any) Dict {
	x, _ := d.resolve(v).(Dict)
	if s, ok := d.resolve(v).(Stream); ok {
		return s.Dict
	}

	return x
}

// Field kinds.
const (
	Text      = "text"
	Checkbox  = "checkbox"
	Radio     = "radio"
	Choice    = "choice"
	Signature = "signature"
	Button    = "button"
)

// Field is one field of the form.
type Field struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Value    string   `json:"value"`
	Options  []string `json:"options,omitempty"` // a choice's items, a checkbox's or radio's on states
	ReadOnly bool     `json:"read_only,omitempty"`
	Required bool     `json:"required,omitempty"`
	MaxLen   int      `json:"max_len,omitempty"`
	Edit     bool     `json:"edit,omitempty"` // a choice that takes any text

	ref     Ref
	widgets []Ref
	merged  bool // the field and its one widget are the same object
	flags   int
}

func (d *Doc) catalog() Dict { return d.dict(d.trailer["Root"]) }

// Encrypted is whether the document is encrypted.
func (d *Doc) Encrypted() bool { return d.trailer["Encrypt"] != nil }

// Fields are the form's fields that hold a value, in the order the form
// lists them, with their full names ("address.street").
func (d *Doc) Fields() ([]Field, error) {
	if d.Encrypted() {
		return nil, ErrEncrypted
	}

	acro := d.dict(d.catalog()["AcroForm"])
	if acro == nil {
		return nil, ErrNoForm
	}

	top, _ := d.resolve(acro["Fields"]).(Array)

	var out []Field

	seen := map[int]bool{}

	var walk func(v any, prefix string, inherited Dict, depth int)

	walk = func(v any, prefix string, inherited Dict, depth int) {
		ref, isRef := v.(Ref)
		if depth > 32 || (isRef && seen[ref.Num]) {
			return
		}

		if isRef {
			seen[ref.Num] = true
		}

		node := d.dict(v)
		if node == nil {
			return
		}

		name := prefix
		if t, ok := d.resolve(node["T"]).(String); ok {
			if name != "" {
				name += "."
			}

			name += t.Text()
		}

		here := Dict{}
		for k, x := range inherited {
			here[k] = x
		}

		for _, k := range []Name{"FT", "Ff", "V", "Opt", "MaxLen"} {
			if x, ok := node[k]; ok {
				here[k] = x
			}
		}

		kids, _ := d.resolve(node["Kids"]).(Array)

		var fieldKids, widgets []Ref

		for _, k := range kids {
			kr, ok := k.(Ref)
			if !ok {
				continue
			}

			if kd := d.dict(kr); kd != nil && kd["T"] != nil {
				fieldKids = append(fieldKids, kr)
			} else {
				widgets = append(widgets, kr)
			}
		}

		for _, k := range fieldKids {
			walk(k, name, here, depth+1)
		}

		if len(fieldKids) > 0 || name == "" {
			return
		}

		f := Field{Name: name, ref: ref, widgets: widgets}

		if len(widgets) == 0 && isRef {
			f.widgets, f.merged = []Ref{ref}, true
		}

		flags := intOf(here["Ff"], 0)
		f.flags = flags
		f.ReadOnly = flags&1 != 0
		f.Required = flags&2 != 0

		switch here["FT"] {
		case Name("Tx"):
			f.Kind = Text
			f.MaxLen = intOf(d.resolve(here["MaxLen"]), 0)
		case Name("Btn"):
			switch {
			case flags&(1<<16) != 0:
				f.Kind = Button
			case flags&(1<<15) != 0:
				f.Kind = Radio
			default:
				f.Kind = Checkbox
			}

			f.Options = d.onStates(f.widgets)
		case Name("Ch"):
			f.Kind = Choice
			f.Edit = flags&(1<<18) != 0

			opts, _ := d.resolve(here["Opt"]).(Array)
			for _, o := range opts {
				switch x := d.resolve(o).(type) {
				case String:
					f.Options = append(f.Options, x.Text())
				case Array:
					if len(x) > 0 {
						if s, ok := d.resolve(x[0]).(String); ok {
							f.Options = append(f.Options, s.Text())
						}
					}
				}
			}
		case Name("Sig"):
			f.Kind = Signature
		default:
			return
		}

		switch v := d.resolve(here["V"]).(type) {
		case String:
			f.Value = v.Text()
		case Name:
			f.Value = string(v)
		case Array:
			var parts []string
			for _, x := range v {
				if s, ok := d.resolve(x).(String); ok {
					parts = append(parts, s.Text())
				}
			}

			f.Value = strings.Join(parts, ", ")
		case Dict:
			f.Value = "(signed)"
		}

		out = append(out, f)
	}

	for _, f := range top {
		walk(f, "", Dict{}, 0)
	}

	if len(out) == 0 {
		return nil, ErrNoForm
	}

	return out, nil
}

// onStates are the names a checkbox or radio button's widgets are "on" as.
func (d *Doc) onStates(widgets []Ref) []string {
	var out []string

	seen := map[string]bool{}

	for _, w := range widgets {
		ap := d.dict(d.dict(w)["AP"])
		normal := d.dict(ap["N"])

		for k := range normal {
			if k != "Off" && !seen[string(k)] {
				seen[string(k)] = true
				out = append(out, string(k))
			}
		}
	}

	sort.Strings(out)

	return out
}

// Signed is whether the document carries a signature, which filling in
// would break.
func (d *Doc) Signed() bool {
	acro := d.dict(d.catalog()["AcroForm"])
	if acro != nil && intOf(d.resolve(acro["SigFlags"]), 0)&1 != 0 {
		return true
	}

	fields, _ := d.Fields()
	for _, f := range fields {
		if f.Kind == Signature && f.Value != "" {
			return true
		}
	}

	return false
}
