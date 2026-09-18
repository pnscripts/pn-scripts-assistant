package preflight

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"runtime"
	"strings"
)

/*
 * Releases fixed by version and by checksum.
 *
 * Asking a project which release is current and fetching whatever it names is
 * convenient and it is also trusting two things at once: the answer, and the
 * file. A release pinned here is trusted once, when somebody reads this file —
 * the version is the one written down, and the bytes are the ones whose hash is
 * written down beside it. A download that does not match is deleted, not used.
 *
 * The hashes are the projects' own, copied from the checksum file each one
 * publishes with its release (Godot's SHA512-SUMS.txt, Node's SHASUMS256.txt).
 * Moving to a newer release is an edit to this file, which is exactly the
 * review a new version of something that will run on this machine deserves.
 */

// Pinned is one file of one release.
type Pinned struct {
	// What it is, in words: "Godot 4.7.2".
	Name    string `json:"name"`
	Version string `json:"version"`

	URL string `json:"url"`

	// Sum is the hex digest, and Algorithm which one: sha256 or sha512.
	Sum       string `json:"sum"`
	Algorithm string `json:"algorithm"`

	// Size is the download in bytes, from the release itself.
	Size int64 `json:"size"`

	// Licence is what the project releases it under, said plainly.
	Licence string `json:"licence"`
}

// GodotVersion is the engine this program installs.
const GodotVersion = "4.7.2-stable"

var godotBuilds = map[string]struct {
	asset string
	sum   string
	size  int64
}{
	"amd64": {"Godot_v4.7.2-stable_linux.x86_64.zip",
		"9aa00f7a605200940bce3027a567b782f49bd8e940dd06ae9e987bd65aee1b1467edd56ed84fcdcbdd44354bf613bdbb4e5d2913e925850368e150c59ed54c65",
		77860424},
	"arm64": {"Godot_v4.7.2-stable_linux.arm64.zip",
		"dd59918da086bd49bde2f5450b5e567ff8650cbde9abbd7b8f4ca1197ff8c609baa38834666d032deafb47099078d7822279e2a0e06e5665745468f26533e7e2",
		77008699},
	"386": {"Godot_v4.7.2-stable_linux.x86_32.zip",
		"3023ce1e8eeb6cc7fb90dc39566cf071b93727d0ead66c0df2edb1e97c19b805a477d2117bb2b4df0f876b334aadf68fc6076513231c05e004f4e7e062b7acfe",
		78111204},
	"arm": {"Godot_v4.7.2-stable_linux.arm32.zip",
		"7fb547a95f8e99d928dd210c568269e79f0d5346cdfa5c1f421da2696228e79284411361c80648a731d6b1e305942b561b0d45e59a533c275b9804be2d5e66ce",
		74780990},
}

const godotReleases = "https://github.com/godotengine/godot/releases/download/" + GodotVersion + "/"

// GodotEngine is the build of the pinned engine for this machine.
func GodotEngine() (Pinned, error) {
	build, ok := godotBuilds[runtime.GOARCH]
	if !ok || runtime.GOOS != "linux" {
		return Pinned{}, fmt.Errorf("there is no pinned Godot build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	return Pinned{
		Name: "Godot " + strings.TrimSuffix(GodotVersion, "-stable"), Version: GodotVersion,
		URL: godotReleases + build.asset, Sum: build.sum, Algorithm: "sha512", Size: build.size,
		Licence: "MIT — free, no account, no fee",
	}, nil
}

/*
 * GodotTemplates is what exporting a game needs beside the engine.
 *
 * Every platform's templates in one archive, which is why it is well over a
 * gigabyte: the engine can run a project without them, and cannot write a
 * finished game for anybody else to run.
 */
func GodotTemplates() Pinned {
	return Pinned{
		Name: "Godot " + strings.TrimSuffix(GodotVersion, "-stable") + " export templates", Version: GodotVersion,
		URL:       godotReleases + "Godot_v4.7.2-stable_export_templates.tpz",
		Sum:       "ca4d71c4d7b81dfc15d1a98baa07534aa95b03fdda78a0075b06672e1648d2e5f40980c9adc28d23e1b92e732ee7bf3461997aa804af74ec2fcd7a93ccb84079",
		Algorithm: "sha512", Size: 1281349702,
		Licence: "MIT — free, no account, no fee",
	}
}

// NodeVersion is the Node.js this program installs: the long-term release.
const NodeVersion = "v24.21.0"

var nodeBuilds = map[string]struct {
	asset string
	sum   string
	size  int64
}{
	"amd64": {"node-v24.21.0-linux-x64.tar.gz",
		"6e1db87ef58b8819e5d5402eff1536491b18edd8eb7bee5ef7897876e88dc5ff", 58088022},
	"arm64": {"node-v24.21.0-linux-arm64.tar.gz",
		"724282c3b43aec998aa9527380465b45d229e021b58035f5f4f63095eabfe5d5", 0},
}

// Node is the pinned Node.js for this machine.
func Node() (Pinned, error) {
	build, ok := nodeBuilds[runtime.GOARCH]
	if !ok || runtime.GOOS != "linux" {
		return Pinned{}, fmt.Errorf("there is no pinned Node.js build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	return Pinned{
		Name: "Node.js " + NodeVersion, Version: NodeVersion,
		URL: "https://nodejs.org/dist/" + NodeVersion + "/" + build.asset,
		Sum: build.sum, Algorithm: "sha256", Size: build.size,
		Licence: "MIT — free, no account, no fee",
	}, nil
}

/*
 * Fetch downloads a pinned file and proves it is the one that was pinned.
 *
 * Through the same door as every other download — the allowed hosts, the size
 * bound, the part file — and then hashed. A mismatch removes the file: a
 * download that is not what was pinned is not "probably fine", it is exactly
 * the case pinning exists to catch.
 */
func Fetch(p Pinned, to string, w io.Writer) error {
	if p.Sum == "" {
		return fmt.Errorf("%s has no checksum written down, so it is not fetched", p.Name)
	}

	if err := download(p.URL, to, w); err != nil {
		return err
	}

	if err := Matches(to, p.Algorithm, p.Sum); err != nil {
		os.Remove(to)

		return fmt.Errorf("%s: %w — the download was deleted", p.Name, err)
	}

	fmt.Fprintf(w, "Checked: the %s of %s matches the release\n", p.Algorithm, p.Name)

	return nil
}

// Matches reports whether a file's digest is the one given.
func Matches(path, algorithm, want string) error {
	var h hash.Hash

	switch strings.ToLower(algorithm) {
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		return fmt.Errorf("%q is not a checksum this program checks", algorithm)
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}

	defer f.Close()

	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	got := hex.EncodeToString(h.Sum(nil))

	if !strings.EqualFold(got, strings.TrimSpace(want)) {
		return fmt.Errorf("its %s is %s…, and the release says %s…", algorithm, got[:16], want[:min(16, len(want))])
	}

	return nil
}

// The unpackers and the places things go, for the provisioners that build on
// this package rather than beside it.

func UnpackZip(archive, into string, strip int, w io.Writer) error {
	return unpackZip(archive, into, strip, w)
}

func UnpackTarGz(archive, into string, strip int, w io.Writer) error {
	return unpackTarGz(archive, into, strip, w)
}

// LocalBin is ~/.local/bin, where a program installed for one person goes.
func LocalBin() string { return localBin() }

// LocalShare is ~/.local/share/<name>.
func LocalShare(name string) string { return localShare(name) }
