package setup

import "testing"

/*
 * The bar read 100% for every download.
 *
 * Not a rounding problem or an off-by-one: the unit was never applied, so
 * "659MB of 1.3GB" was read as 659 over 1 and clamped down from 65,900. It
 * showed a full bar beside the words "231MB of 1.3GB" and nothing about that
 * was subtle — which is the argument for the test, since the same wrongness
 * looked exactly like a finished download.
 */
func TestPercentOfCountsUnits(t *testing.T) {
	for _, c := range []struct {
		name string
		log  string
		want int
	}{
		{"half way through a download", "Downloading\n659MB of 1.3GB\n", 49},
		{"barely started", "231MB of 1.3GB\n", 17},
		{"finished", "1.3GB of 1.3GB\n", 100},
		{"mixed units", "512KB of 1MB\n", 50},
		{"the binary units ollama writes", "1.6 GiB of 4.7 GiB\n", 34},
		{"a percentage when there is no byte count", "[ 39%] Building CXX\n", 39},
		{"nothing to go on", "Unpacking.\n", -1},
		{"the newest line wins", "100MB of 1.3GB\n300MB of 1.3GB\n", 22},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := percentOf(c.log); got != c.want {
				t.Fatalf("percentOf(%q) = %d, want %d", c.log, got, c.want)
			}
		})
	}
}
