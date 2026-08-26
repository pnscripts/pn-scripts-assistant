package store

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeVector packs an embedding into the form stored in the embedding column.
//
// float32 rather than float64 halves the size for no meaningful loss: the
// models producing these vectors emit float32 to begin with, so the extra
// precision would be storing noise.
func EncodeVector(v []float32) []byte {
	out := make([]byte, len(v)*4)

	for i, f := range v {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(f))
	}

	return out
}

// DecodeVector reverses EncodeVector.
func DecodeVector(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("embedding blob is %d bytes, not a whole number of float32", len(b))
	}

	out := make([]float32, len(b)/4)

	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}

	return out, nil
}

// Cosine returns the cosine similarity of two vectors, from -1 to 1.
//
// Mismatched lengths return 0 rather than an error or a panic. That case means
// two different embedding models have been used against the same store, and the
// honest answer to "how similar are these" is then "unknown"; scoring it zero
// keeps such a pair out of results instead of inventing a relationship. The
// caller that cares about the difference checks dimensions explicitly.
func Cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, na, nb float64

	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}

	if na == 0 || nb == 0 {
		return 0
	}

	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// Normalise scales a vector to unit length, returning a copy.
//
// Unused by Cosine, which normalises as it goes, but needed where many
// comparisons are made against one stored set: pre-normalising turns each
// comparison into a plain dot product.
func Normalise(v []float32) []float32 {
	var sum float64

	for _, f := range v {
		sum += float64(f) * float64(f)
	}

	if sum == 0 {
		return append([]float32(nil), v...)
	}

	inv := float32(1 / math.Sqrt(sum))
	out := make([]float32, len(v))

	for i, f := range v {
		out[i] = f * inv
	}

	return out
}
