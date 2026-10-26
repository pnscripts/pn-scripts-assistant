// Package pdfform reads the fields of a PDF form and fills them in.
//
// No library: a PDF is a file of numbered objects and a table saying where
// each one starts, and a form is a few of those objects. This reads as much
// of the format as finding and filling form fields needs — the classic
// cross-reference table and the compressed cross-reference streams and
// object streams of PDF 1.5 and later — and writes the filled values as an
// incremental update: the original bytes are kept exactly as they were and
// the changed objects are added after them, which is how the format itself
// says a document is changed. A PDF that is encrypted or signed is not
// touched.
package pdfform

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// The values a PDF object can be.
type (
	// Name is /Name.
	Name string

	// Ref is "12 0 R", a pointer to another object.
	Ref struct{ Num, Gen int }

	// Dict is << /Key value … >>.
	Dict map[Name]any

	// Array is [ … ].
	Array []any

	// String is (text) or <hex>.
	String struct {
		B   []byte
		Hex bool
	}

	// Stream is a dictionary followed by bytes, as they are in the file.
	Stream struct {
		Dict Dict
		Raw  []byte
	}
)

// Text is a PDF text string decoded: UTF-16 with its byte order mark, or
// PDFDocEncoding, which is Latin-1 for every letter a form uses.
func (s String) Text() string {
	b := s.B
	if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		var u []uint16

		for i := 2; i+1 < len(b); i += 2 {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		}

		return string(utf16.Decode(u))
	}

	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return string(b[3:])
	}

	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}

	return string(r)
}

// TextString encodes text for a PDF: plain bytes when it is all ASCII,
// otherwise UTF-16 with its byte order mark, which every reader since PDF
// 1.2 understands and which carries Cyrillic and everything else.
func TextString(s string) String {
	ascii := true

	for _, r := range s {
		if r > 126 || (r < 32 && r != '\n' && r != '\r' && r != '\t') {
			ascii = false

			break
		}
	}

	if ascii {
		return String{B: []byte(s)}
	}

	b := []byte{0xFE, 0xFF}
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u>>8), byte(u))
	}

	return String{B: b, Hex: true}
}

/* ---------- Reading objects ---------- */

type lexer struct {
	b   []byte
	pos int
}

func isWhite(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' || c == 0
}

func isDelim(c byte) bool {
	return strings.IndexByte("()<>[]{}/%", c) >= 0
}

func (l *lexer) skip() {
	for l.pos < len(l.b) {
		c := l.b[l.pos]

		switch {
		case isWhite(c):
			l.pos++
		case c == '%':
			for l.pos < len(l.b) && l.b[l.pos] != '\n' && l.b[l.pos] != '\r' {
				l.pos++
			}
		default:
			return
		}
	}
}

// token is the next bare word: a number, a keyword, true, false, null.
func (l *lexer) token() string {
	l.skip()
	start := l.pos

	for l.pos < len(l.b) && !isWhite(l.b[l.pos]) && !isDelim(l.b[l.pos]) {
		l.pos++
	}

	return string(l.b[start:l.pos])
}

var errSyntax = errors.New("the PDF could not be read here")

// object reads one object, turning "n g R" into a Ref.
func (l *lexer) object() (any, error) {
	l.skip()

	if l.pos >= len(l.b) {
		return nil, io.ErrUnexpectedEOF
	}

	switch c := l.b[l.pos]; {
	case c == '/':
		l.pos++
		start := l.pos

		for l.pos < len(l.b) && !isWhite(l.b[l.pos]) && !isDelim(l.b[l.pos]) {
			l.pos++
		}

		return Name(unescapeName(string(l.b[start:l.pos]))), nil
	case c == '(':
		return l.literal()
	case c == '<' && l.pos+1 < len(l.b) && l.b[l.pos+1] == '<':
		l.pos += 2

		d := Dict{}

		for {
			l.skip()

			if l.pos+1 < len(l.b) && l.b[l.pos] == '>' && l.b[l.pos+1] == '>' {
				l.pos += 2

				return d, nil
			}

			k, err := l.object()
			if err != nil {
				return nil, err
			}

			name, ok := k.(Name)
			if !ok {
				return nil, errSyntax
			}

			v, err := l.object()
			if err != nil {
				return nil, err
			}

			d[name] = v
		}
	case c == '<':
		l.pos++
		start := l.pos

		for l.pos < len(l.b) && l.b[l.pos] != '>' {
			l.pos++
		}

		hex := strings.Map(func(r rune) rune {
			if isWhite(byte(r)) {
				return -1
			}

			return r
		}, string(l.b[start:l.pos]))
		l.pos++

		if len(hex)%2 == 1 {
			hex += "0"
		}

		out := make([]byte, len(hex)/2)

		for i := range out {
			v, err := strconv.ParseUint(hex[2*i:2*i+2], 16, 8)
			if err != nil {
				return nil, errSyntax
			}

			out[i] = byte(v)
		}

		return String{B: out, Hex: true}, nil
	case c == '[':
		l.pos++

		var a Array

		for {
			l.skip()

			if l.pos < len(l.b) && l.b[l.pos] == ']' {
				l.pos++

				return a, nil
			}

			v, err := l.object()
			if err != nil {
				return nil, err
			}

			a = append(a, v)
		}
	case c == ']' || c == '>' || c == ')' || c == '{' || c == '}':
		return nil, errSyntax
	}

	word := l.token()

	switch word {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	case "":
		return nil, errSyntax
	}

	if n, err := strconv.ParseInt(word, 10, 64); err == nil {
		// "12 0 R": look ahead without consuming unless it is one.
		save := l.pos

		if g, err := strconv.Atoi(l.token()); err == nil && g >= 0 {
			if l.token() == "R" {
				return Ref{int(n), g}, nil
			}
		}

		l.pos = save

		return n, nil
	}

	if f, err := strconv.ParseFloat(word, 64); err == nil {
		return f, nil
	}

	// A keyword where an object was expected (endobj, stream …) is the
	// caller's to deal with.
	return keyword(word), nil
}

type keyword string

func (l *lexer) literal() (String, error) {
	l.pos++ // (

	var out []byte

	depth := 1

	for l.pos < len(l.b) {
		c := l.b[l.pos]
		l.pos++

		switch c {
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return String{B: out}, nil
			}

			out = append(out, c)
		case '\\':
			if l.pos >= len(l.b) {
				return String{}, errSyntax
			}

			e := l.b[l.pos]
			l.pos++

			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r':
				if l.pos < len(l.b) && l.b[l.pos] == '\n' {
					l.pos++
				}
			case '\n':
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')

					for i := 0; i < 2 && l.pos < len(l.b) && l.b[l.pos] >= '0' && l.b[l.pos] <= '7'; i++ {
						v = v*8 + int(l.b[l.pos]-'0')
						l.pos++
					}

					out = append(out, byte(v))
				} else {
					out = append(out, e)
				}
			}
		default:
			out = append(out, c)
		}
	}

	return String{}, errSyntax
}

func unescapeName(s string) string {
	if !strings.Contains(s, "#") {
		return s
	}

	var b strings.Builder

	for i := 0; i < len(s); i++ {
		if s[i] == '#' && i+2 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 2

				continue
			}
		}

		b.WriteByte(s[i])
	}

	return b.String()
}

/* ---------- Writing objects ---------- */

func writeObject(w *bytes.Buffer, v any) {
	switch x := v.(type) {
	case nil:
		w.WriteString("null")
	case bool:
		w.WriteString(strconv.FormatBool(x))
	case int64:
		w.WriteString(strconv.FormatInt(x, 10))
	case int:
		w.WriteString(strconv.Itoa(x))
	case float64:
		w.WriteString(strconv.FormatFloat(x, 'f', -1, 64))
	case Name:
		w.WriteByte('/')

		for i := 0; i < len(x); i++ {
			c := x[i]
			if c < 33 || c > 126 || isDelim(c) || c == '#' {
				fmt.Fprintf(w, "#%02X", c)
			} else {
				w.WriteByte(c)
			}
		}
	case String:
		if x.Hex {
			fmt.Fprintf(w, "<%X>", x.B)

			return
		}

		w.WriteByte('(')

		for _, c := range x.B {
			switch c {
			case '(', ')', '\\':
				w.WriteByte('\\')
				w.WriteByte(c)
			case '\n':
				w.WriteString(`\n`)
			case '\r':
				w.WriteString(`\r`)
			default:
				w.WriteByte(c)
			}
		}

		w.WriteByte(')')
	case Ref:
		fmt.Fprintf(w, "%d %d R", x.Num, x.Gen)
	case Array:
		w.WriteByte('[')

		for i, e := range x {
			if i > 0 {
				w.WriteByte(' ')
			}

			writeObject(w, e)
		}

		w.WriteByte(']')
	case Dict:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, string(k))
		}

		sort.Strings(keys)

		w.WriteString("<<")

		for _, k := range keys {
			writeObject(w, Name(k))
			w.WriteByte(' ')
			writeObject(w, x[Name(k)])
		}

		w.WriteString(">>")
	case Stream:
		writeObject(w, x.Dict)
		w.WriteString("\nstream\n")
		w.Write(x.Raw)
		w.WriteString("\nendstream")
	case keyword:
		w.WriteString(string(x))
	default:
		fmt.Fprintf(w, "null")
	}
}

/* ---------- Streams ---------- */

// deflate is data compressed as FlateDecode.
func deflate(data []byte) []byte {
	var z bytes.Buffer

	w := zlib.NewWriter(&z)
	_, _ = w.Write(data)
	_ = w.Close()

	return z.Bytes()
}

// decode is a stream's bytes with its filters undone: FlateDecode, with the
// PNG and TIFF predictors cross-reference streams use. Any other filter is
// not a stream a form needs.
func decode(s Stream) ([]byte, error) {
	filters := []Name{}

	switch f := s.Dict["Filter"].(type) {
	case Name:
		filters = append(filters, f)
	case Array:
		for _, x := range f {
			if n, ok := x.(Name); ok {
				filters = append(filters, n)
			}
		}
	}

	data := s.Raw

	for i, f := range filters {
		if f != "FlateDecode" && f != "Fl" {
			return nil, fmt.Errorf("a stream uses the %s filter, which this does not read", f)
		}

		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}

		out, err := io.ReadAll(io.LimitReader(r, 256<<20))
		r.Close()

		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}

		data = out

		var parms Dict

		switch p := s.Dict["DecodeParms"].(type) {
		case Dict:
			parms = p
		case Array:
			if i < len(p) {
				parms, _ = p[i].(Dict)
			}
		}

		if parms != nil {
			if data, err = unpredict(data, parms); err != nil {
				return nil, err
			}
		}
	}

	return data, nil
}

func intOf(v any, def int) int {
	switch x := v.(type) {
	case int64:
		return int(x)
	case float64:
		return int(x)
	}

	return def
}

// unpredict undoes a PNG predictor (10–15), row by row.
func unpredict(data []byte, parms Dict) ([]byte, error) {
	predictor := intOf(parms["Predictor"], 1)
	if predictor < 10 {
		if predictor == 1 {
			return data, nil
		}

		return nil, fmt.Errorf("predictor %d is not read", predictor)
	}

	colors := intOf(parms["Colors"], 1)
	bits := intOf(parms["BitsPerComponent"], 8)
	columns := intOf(parms["Columns"], 1)

	bpp := max(1, colors*bits/8)
	rowLen := (colors*bits*columns + 7) / 8

	var out []byte

	prev := make([]byte, rowLen)

	for i := 0; i+rowLen < len(data)+1 && i < len(data); i += rowLen + 1 {
		kind := data[i]

		end := min(i+1+rowLen, len(data))
		row := append([]byte(nil), data[i+1:end]...)

		for len(row) < rowLen {
			row = append(row, 0)
		}

		for j := range row {
			var left, up, ul byte

			if j >= bpp {
				left, ul = row[j-bpp], prev[j-bpp]
			}

			up = prev[j]

			switch kind {
			case 1:
				row[j] += left
			case 2:
				row[j] += up
			case 3:
				row[j] += byte((int(left) + int(up)) / 2)
			case 4:
				row[j] += paeth(left, up, ul)
			}
		}

		out = append(out, row...)
		prev = row
	}

	return out, nil
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))

	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}

	return c
}

func abs(x int) int {
	if x < 0 {
		return -x
	}

	return x
}
