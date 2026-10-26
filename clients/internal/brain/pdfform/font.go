package pdfform

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

/*
 * A Unicode font to draw filled values with.
 *
 * A form names the font its fields are drawn in, and most name Helvetica,
 * which has no Cyrillic: a viewer that draws "Пловдив" with it draws
 * nothing. So a value that Helvetica cannot show is drawn with a TrueType
 * font found on this computer and put in the file — only the letters used,
 * every other glyph emptied, so a filled form grows by kilobytes, not
 * megabytes. A font whose licence does not allow it to be embedded is not
 * used.
 */

// fontPlaces are where a Unicode TrueType font usually is.
func fontPlaces() []string {
	switch runtime.GOOS {
	case "windows":
		dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		if os.Getenv("WINDIR") == "" {
			dir = `C:\Windows\Fonts`
		}

		return []string{filepath.Join(dir, "arial.ttf"), filepath.Join(dir, "segoeui.ttf"), filepath.Join(dir, "calibri.ttf")}
	case "darwin":
		return []string{"/System/Library/Fonts/Supplemental/Arial.ttf", "/Library/Fonts/Arial.ttf", "/System/Library/Fonts/Supplemental/Verdana.ttf"}
	}

	return []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
		"/usr/share/fonts/truetype/liberation2/LiberationSans-Regular.ttf",
		"/usr/share/fonts/liberation-sans/LiberationSans-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
		"/usr/share/fonts/noto/NotoSans-Regular.ttf",
	}
}

// FontPaths is replaceable in tests.
var FontPaths = fontPlaces

// ttf is a TrueType font, read as far as drawing and embedding need.
type ttf struct {
	data       []byte
	name       string
	tables     map[string][]byte
	unitsPerEm int
	ascent     int
	descent    int
	bbox       [4]int
	numGlyphs  int
	longLoca   bool
	advances   []int
	cmap       map[rune]int
}

var errFont = errors.New("not a usable TrueType font")

func be16(b []byte, at int) int  { return int(binary.BigEndian.Uint16(b[at:])) }
func bes16(b []byte, at int) int { return int(int16(binary.BigEndian.Uint16(b[at:]))) }
func be32(b []byte, at int) int  { return int(binary.BigEndian.Uint32(b[at:])) }

// readTTF reads a font and says whether it may be embedded.
func readTTF(path string) (*ttf, error) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 12 {
		return nil, errFont
	}

	f := &ttf{data: data, tables: map[string][]byte{}, cmap: map[rune]int{}, name: fontName(path)}

	n := be16(data, 4)
	for i := 0; i < n; i++ {
		rec := 12 + 16*i
		if rec+16 > len(data) {
			return nil, errFont
		}

		tag := string(data[rec : rec+4])
		off, length := be32(data, rec+8), be32(data, rec+12)

		if off+length > len(data) {
			return nil, errFont
		}

		f.tables[tag] = data[off : off+length]
	}

	for _, t := range []string{"head", "hhea", "hmtx", "maxp", "cmap", "loca", "glyf"} {
		if f.tables[t] == nil {
			return nil, errFont
		}
	}

	// Licence: "restricted" (2) forbids embedding, and so does a font
	// that is bitmaps only (0x200).
	if os2 := f.tables["OS/2"]; len(os2) > 10 {
		fs := be16(os2, 8)
		if fs&0x000F == 0x0002 || fs&0x0200 != 0 {
			return nil, errors.New("the font's licence does not allow it to be embedded")
		}
	}

	head := f.tables["head"]
	f.unitsPerEm = be16(head, 18)
	f.bbox = [4]int{bes16(head, 36), bes16(head, 38), bes16(head, 40), bes16(head, 42)}
	f.longLoca = bes16(head, 50) == 1

	hhea := f.tables["hhea"]
	f.ascent, f.descent = bes16(hhea, 4), bes16(hhea, 6)
	metrics := be16(hhea, 34)

	f.numGlyphs = be16(f.tables["maxp"], 4)

	hmtx := f.tables["hmtx"]
	f.advances = make([]int, f.numGlyphs)

	last := 0

	for g := 0; g < f.numGlyphs; g++ {
		if g < metrics && 4*g+2 <= len(hmtx) {
			last = be16(hmtx, 4*g)
		}

		f.advances[g] = last
	}

	if f.unitsPerEm == 0 || !f.readCmap() {
		return nil, errFont
	}

	return f, nil
}

func fontName(path string) string {
	base := filepath.Base(path)

	out := make([]byte, 0, len(base))

	for i := 0; i < len(base) && base[i] != '.'; i++ {
		c := base[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			out = append(out, c)
		}
	}

	return string(out)
}

// readCmap reads the Unicode character map: format 12 when there is one,
// otherwise format 4.
func (f *ttf) readCmap() bool {
	c := f.tables["cmap"]
	if len(c) < 4 {
		return false
	}

	best, bestScore := -1, 0

	for i := 0; i < be16(c, 2); i++ {
		rec := 4 + 8*i
		if rec+8 > len(c) {
			break
		}

		platform, encoding, off := be16(c, rec), be16(c, rec+2), be32(c, rec+4)
		if off+2 > len(c) {
			continue
		}

		format := be16(c, off)

		score := 0

		switch {
		case format == 12 && (platform == 3 && encoding == 10 || platform == 0):
			score = 3
		case format == 4 && (platform == 3 && encoding == 1 || platform == 0):
			score = 2
		}

		if score > bestScore {
			best, bestScore = off, score
		}
	}

	if best < 0 {
		return false
	}

	switch be16(c, best) {
	case 12:
		groups := be32(c, best+12)

		for i := 0; i < groups; i++ {
			g := best + 16 + 12*i
			if g+12 > len(c) {
				break
			}

			start, end, glyph := be32(c, g), be32(c, g+4), be32(c, g+8)
			for r := start; r <= end && r-start < 0x10000; r++ {
				f.cmap[rune(r)] = glyph + r - start
			}
		}
	case 4:
		segs := be16(c, best+6) / 2
		ends, starts := best+14, best+16+2*segs
		deltas, ranges := starts+2*segs, starts+4*segs

		for s := 0; s < segs; s++ {
			end, start := be16(c, ends+2*s), be16(c, starts+2*s)
			delta, rangeOff := be16(c, deltas+2*s), be16(c, ranges+2*s)

			for r := start; r <= end && r != 0xFFFF; r++ {
				var g int

				if rangeOff == 0 {
					g = (r + delta) & 0xFFFF
				} else {
					at := ranges + 2*s + rangeOff + 2*(r-start)
					if at+2 > len(c) {
						continue
					}

					if g = be16(c, at); g != 0 {
						g = (g + delta) & 0xFFFF
					}
				}

				if g != 0 {
					f.cmap[rune(r)] = g
				}
			}
		}
	}

	return len(f.cmap) > 0
}

// covers is whether the font has every letter of a text.
func (f *ttf) covers(text string) bool {
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}

		if _, ok := f.cmap[r]; !ok {
			return false
		}
	}

	return true
}

// width is a text's width at a size, in points.
func (f *ttf) width(text string, size float64) float64 {
	w := 0

	for _, r := range text {
		if g, ok := f.cmap[r]; ok && g < len(f.advances) {
			w += f.advances[g]
		}
	}

	return float64(w) * size / float64(f.unitsPerEm)
}

// glyphRange is where a glyph's outline is in glyf.
func (f *ttf) glyphRange(g int) (int, int) {
	loca := f.tables["loca"]

	if f.longLoca {
		if 4*g+8 > len(loca) {
			return 0, 0
		}

		return be32(loca, 4*g), be32(loca, 4*g+4)
	}

	if 2*g+4 > len(loca) {
		return 0, 0
	}

	return 2 * be16(loca, 2*g), 2 * be16(loca, 2*g+2)
}

/*
 * subset is the font with only the glyphs given (and the ones they are
 * built of) left in it. Every glyph keeps its number — the others are
 * emptied, not removed — so nothing that refers to a glyph by number has to
 * change. Tables a PDF reader does not need for drawing are left out.
 */
func (f *ttf) subset(used map[int]bool) []byte {
	keep := map[int]bool{0: true}

	var add func(g int, depth int)

	add = func(g int, depth int) {
		if keep[g] && g != 0 || depth > 8 || g >= f.numGlyphs {
			return
		}

		keep[g] = true

		start, end := f.glyphRange(g)
		glyf := f.tables["glyf"]

		if end <= start || end > len(glyf) || end-start < 10 || bes16(glyf, start) >= 0 {
			return
		}

		// A composite glyph: its parts are kept too.
		at := start + 10

		for at+4 <= end {
			flags, part := be16(glyf, at), be16(glyf, at+2)
			add(part, depth+1)

			at += 4
			if flags&0x0001 != 0 {
				at += 4
			} else {
				at += 2
			}

			switch {
			case flags&0x0008 != 0:
				at += 2
			case flags&0x0040 != 0:
				at += 4
			case flags&0x0080 != 0:
				at += 8
			}

			if flags&0x0020 == 0 {
				break
			}
		}
	}

	for g := range used {
		add(g, 0)
	}

	glyf := f.tables["glyf"]

	var newGlyf []byte

	loca := make([]byte, 4*(f.numGlyphs+1))

	for g := 0; g < f.numGlyphs; g++ {
		binary.BigEndian.PutUint32(loca[4*g:], uint32(len(newGlyf)))

		if keep[g] {
			if start, end := f.glyphRange(g); end > start && end <= len(glyf) {
				newGlyf = append(newGlyf, glyf[start:end]...)

				for len(newGlyf)%4 != 0 {
					newGlyf = append(newGlyf, 0)
				}
			}
		}
	}

	binary.BigEndian.PutUint32(loca[4*f.numGlyphs:], uint32(len(newGlyf)))

	head := append([]byte(nil), f.tables["head"]...)
	binary.BigEndian.PutUint16(head[50:], 1) // long loca
	binary.BigEndian.PutUint32(head[8:], 0)  // checksum adjustment, recomputed below

	tables := map[string][]byte{"glyf": newGlyf, "loca": loca, "head": head}

	// What PDF asks of an embedded TrueType font, and OS/2 for its metrics.
	// The character map, the glyph names and the name table are not read:
	// the PDF says which glyph each code is.
	for _, t := range []string{"cvt ", "fpgm", "hhea", "hmtx", "maxp", "OS/2", "prep"} {
		if b, ok := f.tables[t]; ok {
			tables[t] = b
		}
	}

	return writeTTF(tables)
}

func checksum(b []byte) uint32 {
	var sum uint32

	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:min(i+4, len(b))])
		sum += binary.BigEndian.Uint32(w[:])
	}

	return sum
}

func writeTTF(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}

	sort.Strings(tags)

	n := len(tags)

	search, selector := 1, 0
	for search*2 <= n {
		search *= 2
		selector++
	}

	out := make([]byte, 12+16*n)
	binary.BigEndian.PutUint32(out, 0x00010000)
	binary.BigEndian.PutUint16(out[4:], uint16(n))
	binary.BigEndian.PutUint16(out[6:], uint16(search*16))
	binary.BigEndian.PutUint16(out[8:], uint16(selector))
	binary.BigEndian.PutUint16(out[10:], uint16(n*16-search*16))

	headAt := 0

	for i, t := range tags {
		b := tables[t]
		rec := 12 + 16*i

		copy(out[rec:], t)
		binary.BigEndian.PutUint32(out[rec+4:], checksum(b))
		binary.BigEndian.PutUint32(out[rec+8:], uint32(len(out)))
		binary.BigEndian.PutUint32(out[rec+12:], uint32(len(b)))

		if t == "head" {
			headAt = len(out)
		}

		out = append(out, b...)

		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}

	if headAt > 0 {
		binary.BigEndian.PutUint32(out[headAt+8:], 0xB1B0AFBA-checksum(out))
	}

	return out
}

// unicodeFont is the first font on this computer that has every letter of
// the texts and may be embedded; nil when there is none.
func unicodeFont(texts []string) *ttf {
	for _, path := range FontPaths() {
		f, err := readTTF(path)
		if err != nil {
			continue
		}

		all := true

		for _, t := range texts {
			if !f.covers(t) {
				all = false

				break
			}
		}

		if all {
			return f
		}
	}

	return nil
}
