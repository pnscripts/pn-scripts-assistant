package preflight

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
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
 * Ollama, the program the local models run in.
 *
 * It used to come from releases/latest with no checksum: whatever the project
 * had published that morning, run as a service on the person's machine. Now it
 * is one release, chosen when it was at least two weeks old, and the archive
 * for each platform has its SHA-256 written down here. The digests are the
 * ones the release publishes in its sha256sum.txt (and GitHub lists as each
 * asset's digest); both archives were downloaded on 2026-11-09 and hashed, and
 * matched.
 *
 * A person who already has a newer Ollama keeps it: installOllama uses that
 * one rather than putting this older release over it.
 */

// OllamaVersion is the Ollama release this program installs.
const OllamaVersion = "v0.40.11"

var ollamaBuilds = map[string]struct {
	asset string
	sum   string
	size  int64
}{
	"amd64": {"ollama-linux-amd64.tar.zst",
		"7bedab90dd122661be78798e9f0edf37fccf0cccf2699d2c58ea1a3afdc04b6d", 1508659610},
	"arm64": {"ollama-linux-arm64.tar.zst",
		"77b278837b9e62c172f77d703675be4c52c8054e9075639031a9a6d5a53f1924", 1564235434},
}

// OllamaProgram is the pinned Ollama archive for this machine.
func OllamaProgram() (Pinned, error) {
	build, ok := ollamaBuilds[runtime.GOARCH]
	if !ok || runtime.GOOS != "linux" {
		return Pinned{}, fmt.Errorf("there is no pinned Ollama build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	return Pinned{
		Name: "Ollama " + strings.TrimPrefix(OllamaVersion, "v"), Version: OllamaVersion,
		URL: "https://github.com/ollama/ollama/releases/download/" + OllamaVersion + "/" + build.asset,
		Sum: build.sum, Algorithm: "sha256", Size: build.size,
		Licence: "MIT, free, no account, no fee",
	}, nil
}

/*
 * The speech parts: the neural voice, its voices, and the speech model.
 *
 * Until 1.0 these came from moving targets: the voices and the model from the
 * main branch of their Hugging Face repositories, whatever was there on the
 * day, and the Piper program, which is run, with no checksum at all. Now each
 * is one file of one fixed commit or release, with its SHA-256 written down
 * here.
 *
 * The Hugging Face digests are the files' LFS object ids at that commit (the
 * API's tree listing, lfs.oid). The small .onnx.json files are not in LFS, so
 * theirs were computed from a download, whose git blob ids matched the ones
 * the API lists. Every value here was confirmed on 2026-11-09 by downloading
 * the file from the address below and hashing it.
 */

// PiperRelease is the Piper program this installs: the last release of the
// original project, whose archives contain the program and its libraries.
const PiperRelease = "2023.11.14-2"

var piperBuilds = map[string]struct {
	asset string
	sum   string
	size  int64
}{
	"amd64": {"piper_linux_x86_64.tar.gz",
		"a50cb45f355b7af1f6d758c1b360717877ba0a398cc8cbe6d2a7a3a26e225992", 26460462},
	"arm64": {"piper_linux_aarch64.tar.gz",
		"fea0fd2d87c54dbc7078d0f878289f404bd4d6eea6e7444a77835d1537ab88eb", 26004717},
}

// PiperProgram is the pinned Piper archive for this machine.
func PiperProgram() (Pinned, error) {
	build, ok := piperBuilds[runtime.GOARCH]
	if !ok || runtime.GOOS != "linux" {
		return Pinned{}, fmt.Errorf("there is no pinned build of the neural voice for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	return Pinned{
		Name: "Piper " + PiperRelease, Version: PiperRelease,
		URL: "https://github.com/rhasspy/piper/releases/download/" + PiperRelease + "/" + build.asset,
		Sum: build.sum, Algorithm: "sha256", Size: build.size,
		Licence: "MIT, free, no account, no fee",
	}, nil
}

// PiperVoicesRevision is the commit of rhasspy/piper-voices the voices come
// from: main on 2026-10-20, the newest that was at least two weeks old on the
// day it was pinned.
const PiperVoicesRevision = "5041a7a49ae5b3a0f30e6837769eb03fdbb231bd"

// A voice is a model and the settings file beside it; one without the other
// says nothing.
type voiceFile struct {
	path, sum string
	size      int64
}

var piperVoiceFiles = []struct {
	name    string
	licence string
	files   [2]voiceFile
}{
	{"en_GB-alba-medium", "CC BY 4.0 (the Alba dataset)", [2]voiceFile{
		{"en/en_GB/alba/medium/en_GB-alba-medium.onnx",
			"401369c4a81d09fdd86c32c5c864440811dbdcc66466cde2d64f7133a66ad03b", 63201294},
		{"en/en_GB/alba/medium/en_GB-alba-medium.onnx.json",
			"aa965a2f02ecced632c2694e1fc72bbff6d65f265fab567ca945918c73dd89f4", 4888},
	}},
	{"en_GB-northern_english_male-medium", "CC BY-SA 4.0 (OpenSLR 83)", [2]voiceFile{
		{"en/en_GB/northern_english_male/medium/en_GB-northern_english_male-medium.onnx",
			"57a219ae8e638873db7d18893304be5069c42868f392bb95c3ff17f0690d0689", 63201294},
		{"en/en_GB/northern_english_male/medium/en_GB-northern_english_male-medium.onnx.json",
			"69557ed3d974463453e9b0c09dd99a7ed0e52b8b87b64b357dbeeb2540a97d47", 4847},
	}},
	/*
	 * And the one the robot is built out of: a neural voice with the machine
	 * timbre put on afterwards, so its words are as clear as the others. A
	 * woman's voice of its own rather than alba, because the model the robot
	 * is made of is hidden from the list of voices, and building it out of
	 * alba would have taken the woman's voice off the menu.
	 */
	{"en_GB-jenny_dioco-medium", "the Jenny (Dioco) dataset terms: free use, the voice credited as Jenny", [2]voiceFile{
		{"en/en_GB/jenny_dioco/medium/en_GB-jenny_dioco-medium.onnx",
			"469c630d209e139dd392a66bf4abde4ab86390a0269c1e47b4e5d7ce81526b01", 63201294},
		{"en/en_GB/jenny_dioco/medium/en_GB-jenny_dioco-medium.onnx.json",
			"a9a7a93a317c9a3cb6563e37eb057df9ef09c06188a8a4341b0fcb58cba54dd4", 4895},
	}},
}

// PiperVoices is every voice file the neural voice is installed with, each
// pinned. The file name on disk is the last part of the path.
func PiperVoices() []Pinned {
	var out []Pinned

	for _, v := range piperVoiceFiles {
		for _, f := range v.files {
			out = append(out, Pinned{
				Name: path.Base(f.path), Version: PiperVoicesRevision,
				URL: "https://huggingface.co/rhasspy/piper-voices/resolve/" + PiperVoicesRevision + "/" + f.path,
				Sum: f.sum, Algorithm: "sha256", Size: f.size,
				Licence: v.licence,
			})
		}
	}

	return out
}

/*
 * The speech model whisper.cpp listens with, from one commit of
 * ggerganov/whisper.cpp on Hugging Face (main on 2026-11-09, unchanged since
 * 2024-10-29).
 */
const (
	SpeechModelRevision = "5359861c739e955e79d9a303bcbc70fb988958b1"
	SpeechModelSHA256   = "1be3a9b2063867b937e64e2ec7483364a79917e157fa98c5d94b5c1fffea987b"
	SpeechModelBytes    = 487601967
)

// SpeechModelPinned is the speech model as a pinned file.
func SpeechModelPinned() Pinned {
	return Pinned{
		Name: SpeechModel, Version: SpeechModelRevision,
		URL: SpeechModelFrom, Sum: SpeechModelSHA256, Algorithm: "sha256", Size: SpeechModelBytes,
		Licence: "MIT, free, no account, no fee",
	}
}

/*
 * whisper.cpp itself is built from source, so what is pinned is the commit.
 *
 * The tag is how it is fetched (a shallow clone of one tag is a few
 * megabytes; of a commit it is not something every git server allows), and
 * the commit is what is checked afterwards with git rev-parse: a tag can be
 * moved, a commit cannot be anything but itself. v1.9.5 was released on
 * 2026-10-06; built and run on a sample with the speech model above on
 * 2026-11-09.
 */
const (
	WhisperTag    = "v1.9.5"
	WhisperCommit = "d1be6fde11ac6e0407606b4e42fe72d34add8037"
	WhisperRepo   = "https://github.com/ggml-org/whisper.cpp"
)

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

	// Checked before it is given its name (downloadVia), so a file that is
	// not the pinned one never replaces what was there.
	if err := downloadExpecting(p.URL, to, expect{algorithm: p.Algorithm, sum: p.Sum, most: p.Size}, w); err != nil {
		return fmt.Errorf("%s: %w", p.Name, err)
	}

	fmt.Fprintf(w, "Checked: the %s of %s matches the release\n", p.Algorithm, p.Name)

	return nil
}

// hasher is the digest named, nil for none, and an error for one this program
// does not check (so a misspelt name cannot mean "unchecked").
func hasher(algorithm string) (hash.Hash, error) {
	switch strings.ToLower(algorithm) {
	case "":
		return nil, nil
	case "sha256":
		return sha256.New(), nil
	case "sha512":
		return sha512.New(), nil
	default:
		return nil, fmt.Errorf("%q is not a checksum this program checks", algorithm)
	}
}

// Matches reports whether a file's digest is the one given.
func Matches(path, algorithm, want string) error {
	h, err := hasher(algorithm)
	if err != nil {
		return err
	}

	if h == nil {
		return fmt.Errorf("no checksum was given to compare %s with", path)
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
