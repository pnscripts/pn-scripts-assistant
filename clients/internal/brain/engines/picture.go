package engines

import (
	"image"
	_ "image/png" // pictures of runs are PNG
	"math"
	"os"
)

/*
 * Flat is whether a picture is one colour, near enough — which is what a game
 * that draws nothing looks like.
 *
 * A picture was taken as proof that a game runs, and an untouched template
 * photographed as a clean dark rectangle passed every check a finished game
 * would. A game shows something: a board, a piece, a score. One colour edge
 * to edge is a background and nothing on it.
 */
func Flat(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}

	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return false, err
	}

	b := img.Bounds()

	stepX := max(1, b.Dx()/64)
	stepY := max(1, b.Dy()/64)

	var n, sum, sumSq [3]float64

	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			r, g, bl, _ := img.At(x, y).RGBA()

			for i, v := range []uint32{r, g, bl} {
				c := float64(v >> 8)
				n[i]++
				sum[i] += c
				sumSq[i] += c * c
			}
		}
	}

	for i := range 3 {
		if n[i] == 0 {
			return false, nil
		}

		mean := sum[i] / n[i]

		if math.Sqrt(math.Max(0, sumSq[i]/n[i]-mean*mean)) >= 2 {
			return false, nil
		}
	}

	return true, nil
}

// flatPicture is the diagnostic for a picture with nothing drawn on it.
func flatPicture(path string) (Diagnostic, bool) {
	flat, err := Flat(path)
	if err != nil || !flat {
		return Diagnostic{}, false
	}

	return Diagnostic{Severity: "error",
		Message: "the picture is one flat colour: the game ran but nothing visible was drawn"}, true
}
