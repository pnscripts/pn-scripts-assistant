package pdfform

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

/*
 * Drawing a filled value.
 *
 * Marking the form NeedAppearances asks the reader to draw each field
 * itself, and some do; others show whatever picture the field carries, and
 * the ones that draw use the form's font, which often cannot show the
 * letters. So every filled text field is given its own picture: in
 * Helvetica when the value is plain ASCII, as most forms expect, and
 * otherwise in a Unicode font embedded for it, at the size, colour and
 * alignment the field asks for, wrapped on several lines when the field is
 * a multi-line one.
 */

// helvetica is the width of each ASCII letter of Helvetica, in thousandths
// of the size (Adobe's metrics for the standard font).
var helvetica = [95]int{
	278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278, // space … /
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, // 0 … 9
	278, 278, 584, 584, 584, 556, 1015, // : … @
	667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778, 667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, // A … Z
	278, 278, 278, 469, 556, 333, // [ … `
	556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556, 556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, // a … z
	334, 260, 334, 584, // { … ~
}

func ascii(s string) bool {
	for _, r := range s {
		if (r < 32 || r > 126) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}

	return true
}

// face is what a value is drawn in.
type face struct {
	ref     Ref
	unicode *ttf // nil: Helvetica
}

func (f face) width(text string, size float64) float64 {
	if f.unicode != nil {
		return f.unicode.width(text, size)
	}

	w := 0

	for _, r := range text {
		if r >= 32 && r <= 126 {
			w += helvetica[r-32]
		} else {
			w += 556
		}
	}

	return float64(w) * size / 1000
}

func (f face) show(text string) string {
	if f.unicode == nil {
		var b bytes.Buffer
		writeObject(&b, String{B: []byte(text)})

		return b.String() + " Tj"
	}

	var b strings.Builder

	b.WriteByte('<')

	for _, r := range text {
		fmt.Fprintf(&b, "%04X", f.unicode.cmap[r])
	}

	b.WriteString("> Tj")

	return b.String()
}

// ascentRatio is how far above the baseline the letters reach, as a share
// of the size.
func (f face) ascentRatio() float64 {
	if f.unicode != nil {
		return float64(f.unicode.ascent) / float64(f.unicode.unitsPerEm)
	}

	return 0.718
}

func (f face) descentRatio() float64 {
	if f.unicode != nil {
		return -float64(f.unicode.descent) / float64(f.unicode.unitsPerEm)
	}

	return 0.207
}

// number is a PDF number as a float.
func number(v any) float64 {
	switch x := v.(type) {
	case int64:
		return float64(x)
	case float64:
		return x
	}

	return 0
}

// look is the size and colour a field's /DA asks for: "/Helv 10 Tf 0 g".
func look(da string) (float64, string) {
	words := strings.Fields(da)
	size, colour := 0.0, "0 g"

	for i, w := range words {
		switch w {
		case "Tf":
			if i >= 1 {
				size, _ = strconv.ParseFloat(words[i-1], 64)
			}
		case "g", "G":
			if i >= 1 {
				colour = words[i-1] + " g"
			}
		case "rg", "RG":
			if i >= 3 {
				colour = strings.Join(words[i-3:i], " ") + " rg"
			}
		case "k", "K":
			if i >= 4 {
				colour = strings.Join(words[i-4:i], " ") + " k"
			}
		}
	}

	return size, colour
}

// wrap breaks text into lines no wider than width, at spaces where it can.
func wrap(f face, text string, size, width float64) []string {
	var lines []string

	for _, para := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := ""

		for _, word := range strings.Fields(para) {
			try := word
			if line != "" {
				try = line + " " + word
			}

			if line != "" && f.width(try, size) > width {
				lines = append(lines, line)
				line = word
			} else {
				line = try
			}
		}

		lines = append(lines, line)
	}

	return lines
}

// picture is the appearance stream of one widget showing text.
func picture(f face, text string, rect [4]float64, da string, align int, multiline bool) Stream {
	w, h := math.Abs(rect[2]-rect[0]), math.Abs(rect[3]-rect[1])
	size, colour := look(da)

	var lines []string

	if multiline {
		if size <= 0 {
			size = 10
		}

		lines = wrap(f, text, size, w-4)
	} else {
		text = strings.Join(strings.Fields(text), " ")

		if size <= 0 {
			// Auto: as large as the height allows, then smaller until it fits.
			size = math.Min(12, (h-2)*0.75)

			for size > 4 && f.width(text, size) > w-4 {
				size -= 0.5
			}
		}

		lines = []string{text}
	}

	var c bytes.Buffer

	fmt.Fprintf(&c, "/Tx BMC\nq\n1 1 %.2f %.2f re W n\nBT\n/F1 %.2f Tf\n%s\n", math.Max(0, w-2), math.Max(0, h-2), size, colour)

	leading := size * 1.15

	y := (h-size*(f.ascentRatio()+f.descentRatio()))/2 + size*f.descentRatio()
	if multiline {
		y = h - 2 - size*f.ascentRatio()
	}

	for i, line := range lines {
		x := 2.0

		switch align {
		case 1:
			x = (w - f.width(line, size)) / 2
		case 2:
			x = w - 2 - f.width(line, size)
		}

		fmt.Fprintf(&c, "1 0 0 1 %.2f %.2f Tm\n%s\n", x, y-float64(i)*leading, f.show(line))
	}

	c.WriteString("ET\nQ\nEMC")

	return Stream{
		Dict: Dict{
			"Type": Name("XObject"), "Subtype": Name("Form"),
			"BBox":      Array{int64(0), int64(0), w, h},
			"Resources": Dict{"Font": Dict{"F1": f.ref}},
			"Length":    int64(c.Len()),
		},
		Raw: c.Bytes(),
	}
}

// embed puts a Unicode font in the file, subset to the letters used: the
// font program, its descriptor, the CID font, the composite font readers
// use, and the map back to Unicode so the text can be copied and searched.
// It returns the composite font's reference.
func embed(f *ttf, texts []string, alloc func(any) Ref) Ref {
	used := map[int]bool{}
	letters := map[int]rune{}

	for _, t := range texts {
		for _, r := range t {
			if g, ok := f.cmap[r]; ok {
				used[g] = true
				letters[g] = r
			}
		}
	}

	sub := f.subset(used)
	packed := deflate(sub)

	file := alloc(Stream{Dict: Dict{"Length": int64(len(packed)), "Length1": int64(len(sub)), "Filter": Name("FlateDecode")}, Raw: packed})

	scale := func(v int) int64 { return int64(math.Round(float64(v) * 1000 / float64(f.unitsPerEm))) }

	name := Name("PNSAAA+" + f.name)

	desc := alloc(Dict{
		"Type": Name("FontDescriptor"), "FontName": name, "Flags": int64(32),
		"FontBBox":    Array{scale(f.bbox[0]), scale(f.bbox[1]), scale(f.bbox[2]), scale(f.bbox[3])},
		"ItalicAngle": int64(0), "Ascent": scale(f.ascent), "Descent": scale(f.descent),
		"CapHeight": scale(f.ascent), "StemV": int64(80), "FontFile2": file,
	})

	glyphs := make([]int, 0, len(used))
	for g := range used {
		glyphs = append(glyphs, g)
	}

	sort.Ints(glyphs)

	var widths Array
	for _, g := range glyphs {
		widths = append(widths, int64(g), Array{scale(f.advances[g])})
	}

	cid := alloc(Dict{
		"Type": Name("Font"), "Subtype": Name("CIDFontType2"), "BaseFont": name,
		"CIDSystemInfo":  Dict{"Registry": String{B: []byte("Adobe")}, "Ordering": String{B: []byte("Identity")}, "Supplement": int64(0)},
		"FontDescriptor": desc, "CIDToGIDMap": Name("Identity"), "DW": int64(1000), "W": widths,
	})

	var cmap bytes.Buffer

	cmap.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n" +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n" +
		"1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")

	for i := 0; i < len(glyphs); i += 100 {
		part := glyphs[i:min(i+100, len(glyphs))]
		fmt.Fprintf(&cmap, "%d beginbfchar\n", len(part))

		for _, g := range part {
			u := ""
			for _, x := range encodeUTF16(letters[g]) {
				u += fmt.Sprintf("%04X", x)
			}

			fmt.Fprintf(&cmap, "<%04X> <%s>\n", g, u)
		}

		cmap.WriteString("endbfchar\n")
	}

	cmap.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend")

	toUnicode := alloc(Stream{Dict: Dict{"Length": int64(cmap.Len())}, Raw: cmap.Bytes()})

	return alloc(Dict{
		"Type": Name("Font"), "Subtype": Name("Type0"), "BaseFont": name, "Encoding": Name("Identity-H"),
		"DescendantFonts": Array{cid}, "ToUnicode": toUnicode,
	})
}

func encodeUTF16(r rune) []uint16 {
	if r < 0x10000 {
		return []uint16{uint16(r)}
	}

	r -= 0x10000

	return []uint16{uint16(0xD800 + (r >> 10)), uint16(0xDC00 + (r & 0x3FF))}
}
