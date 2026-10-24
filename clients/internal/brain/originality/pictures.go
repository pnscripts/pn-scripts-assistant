package originality

import (
	"fmt"
	"image"
	_ "image/gif" // decoders for the formats people keep references in
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

/*
 * The visual check: a perceptual hash of each picture, compared with the
 * reference pictures the user gives (the works ours must not look like).
 *
 * The hash is the classic DCT one — 32×32 greys, the 8×8 lowest frequencies,
 * one bit each against their median — so a resized, recompressed or slightly
 * recoloured copy keeps nearly the same 64 bits while a different picture
 * differs in about half. A mirrored copy is caught by hashing ours both ways.
 * It sees composition and shape, not style: a new drawing of a protected
 * character in another pose passes it, which is why the visual specialist
 * still looks and the report gives the reverse image search step.
 */

// Hash is a 64-bit perceptual hash.
type Hash uint64

// Distance is how many of the 64 bits differ: 0 is the same picture.
func Distance(a, b Hash) int { return bits.OnesCount64(uint64(a ^ b)) }

// PictureRisk turns a distance into what to do.
func PictureRisk(d int) Risk {
	switch {
	case d <= 8:
		return High
	case d <= 14:
		return Medium
	}

	return None
}

// HashImage hashes a decoded picture; mirror hashes it flipped left to right.
func HashImage(img image.Image, mirror bool) Hash {
	const n = 32

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	var grey [n][n]float64

	if w == 0 || h == 0 {
		return 0
	}

	// Box-averaged down to 32×32, so the hash does not depend on the size.
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			x0, x1 := x*w/n, max((x+1)*w/n, x*w/n+1)
			y0, y1 := y*h/n, max((y+1)*h/n, y*h/n+1)

			var sum float64

			count := 0

			for yy := y0; yy < y1 && yy < h; yy++ {
				for xx := x0; xx < x1 && xx < w; xx++ {
					r, g, bl, a := img.At(b.Min.X+xx, b.Min.Y+yy).RGBA()
					// Transparent counts as white, as it is usually shown.
					alpha := float64(a) / 65535
					lum := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)) / 65535
					sum += lum*alpha + (1 - alpha)
					count++
				}
			}

			col := x
			if mirror {
				col = n - 1 - x
			}

			grey[y][col] = sum / float64(max(count, 1))
		}
	}

	// The 8×8 lowest frequencies of the 2-D DCT.
	var low [64]float64

	for u := 0; u < 8; u++ {
		for v := 0; v < 8; v++ {
			var s float64

			for y := 0; y < n; y++ {
				for x := 0; x < n; x++ {
					s += grey[y][x] *
						math.Cos(float64(2*x+1)*float64(u)*math.Pi/(2*n)) *
						math.Cos(float64(2*y+1)*float64(v)*math.Pi/(2*n))
				}
			}

			low[v*8+u] = s
		}
	}

	// The median without the overall brightness (the first coefficient).
	sorted := append([]float64(nil), low[1:]...)
	sort.Float64s(sorted)
	median := (sorted[31] + sorted[32]) / 2

	var hash Hash

	for i, c := range low {
		if c > median {
			hash |= 1 << uint(i)
		}
	}

	return hash
}

// HashFile hashes a PNG, JPEG or GIF file, both ways round.
func HashFile(path string) (plain, mirrored Hash, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return 0, 0, fmt.Errorf("%s is not a picture this can read (PNG, JPEG or GIF): %w", filepath.Base(path), err)
	}

	return HashImage(img, false), HashImage(img, true), nil
}

var pictureExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true}

// IsPicture is whether a file is one HashFile can read.
func IsPicture(path string) bool { return pictureExt[strings.ToLower(filepath.Ext(path))] }

// Pictures are the readable pictures at the paths, folders walked.
func Pictures(paths ...string) []string {
	var out []string

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}

		if !info.IsDir() {
			if IsPicture(p) {
				out = append(out, p)
			}

			continue
		}

		_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && IsPicture(path) {
				out = append(out, path)
			}

			return nil
		})
	}

	return out
}

// PictureMatch is one of ours that looks like one of theirs.
type PictureMatch struct {
	Ours, Reference string
	Distance        int
	Mirrored        bool
	Risk            Risk
}

// ComparePictures compares every picture of ours with every reference.
// Unreadable files are reported, not fatal.
func ComparePictures(ours, references []string) ([]PictureMatch, []string) {
	type hashed struct {
		path        string
		plain, flip Hash
	}

	var problems []string

	hashAll := func(paths []string) []hashed {
		var out []hashed

		for _, p := range paths {
			plain, flip, err := HashFile(p)
			if err != nil {
				problems = append(problems, err.Error())

				continue
			}

			out = append(out, hashed{p, plain, flip})
		}

		return out
	}

	mine, theirs := hashAll(ours), hashAll(references)

	var out []PictureMatch

	for _, m := range mine {
		for _, t := range theirs {
			d, mirrored := Distance(m.plain, t.plain), false

			if f := Distance(m.flip, t.plain); f < d {
				d, mirrored = f, true
			}

			if risk := PictureRisk(d); risk != None {
				out = append(out, PictureMatch{Ours: m.path, Reference: t.path, Distance: d, Mirrored: mirrored, Risk: risk})
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Distance < out[j].Distance })

	return out, problems
}

// ReverseSearch is the step for what the hash cannot see: the picture
// searched against the whole web, by hand, in services free to use.
func ReverseSearch(path string) string {
	return "Search " + filepath.Base(path) + " by image at https://lens.google.com/ , https://tineye.com/ or " +
		"https://www.bing.com/visualsearch and look for characters, logos or scenes it resembles."
}
