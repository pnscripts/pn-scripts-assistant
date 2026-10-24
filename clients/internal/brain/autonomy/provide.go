package autonomy

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/workspace"
)

const gb = 1 << 30

// Room is what the machine has for a download, in bytes.
type Room struct {
	// Disk is free space where models or tools are kept; Memory is what a
	// model may use while it runs: the graphics card's, or the processor's
	// memory less what the system and the program need.
	Disk, Memory uint64

	// Processor is a machine without graphics acceleration: a model runs on
	// the processor there, where size costs speed far more than memory.
	Processor bool
}

/*
 * ProcessorMost is the largest model chosen for a machine that runs models on
 * its processor. Found by the end-to-end goal: with 31 GB of memory and four
 * cores, "the largest that fits" was a 32-billion-parameter model that
 * would take minutes for every answer. Memory is not the limit there; speed is.
 */
const ProcessorMost = 6 * gb

// Option is a model that could be downloaded, with what it takes.
type Option struct {
	Name string
	// GB is roughly what it takes on disk and in memory (catalogue.MemoryFor).
	GB float64
	// Digest, when known, is what the download must turn out to be.
	Digest string
}

// diskMargin is kept free after any download: a full disk breaks everything
// else on the machine, which is worse than a smaller model.
const diskMargin = 4 * gb

/*
 * Choose is the largest option that fits this machine, and why — the model
 * that "pick the size that fits" means. Larger is better at the same work;
 * one that does not fit in memory swaps the machine to a standstill, and one
 * that does not fit on the disk fills it.
 */
func Choose(options []Option, room Room) (Option, string, error) {
	sorted := append([]Option(nil), options...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].GB > sorted[j].GB })

	var refused []string

	for _, o := range sorted {
		need := uint64(o.GB * gb)

		switch {
		case o.GB <= 0:
			refused = append(refused, o.Name+": size unknown")
		case need > room.Memory:
			refused = append(refused, fmt.Sprintf("%s: needs %.1f GB of memory", o.Name, o.GB))
		case room.Processor && need > ProcessorMost:
			refused = append(refused, fmt.Sprintf("%s: too slow on the processor alone", o.Name))
		case need+diskMargin > room.Disk:
			refused = append(refused, fmt.Sprintf("%s: needs %.1f GB of disk", o.Name, o.GB))
		default:
			why := fmt.Sprintf("%s fits: about %.1f GB, with %.1f GB of memory and %.1f GB of disk free",
				o.Name, o.GB, float64(room.Memory)/gb, float64(room.Disk)/gb)

			if room.Processor {
				why += ", the largest that answers in reasonable time on the processor"
			}

			return o, why, nil
		}
	}

	return Option{}, "", fmt.Errorf("no model fits this machine (%s)", strings.Join(refused, "; "))
}

// Puller is what downloads a model: Ollama, or a fake in tests.
type Puller interface {
	Pull(ctx context.Context, name string, onProgress func(string)) error
	// DigestOf is the digest of an installed model, or "" when it is not.
	DigestOf(ctx context.Context, name string) string
}

/*
 * PullModel downloads the model that fits, checks it, and records it.
 *
 * Checked twice over: Ollama verifies every layer's sha256 as it downloads
 * and says so, and a download that never said it verified is not accepted;
 * then the installed model's digest is read back and, when one was expected,
 * compared. A model that fails either is not used and the next that fits is
 * tried — so a broken mirror or a cut connection costs a fallback, not the
 * goal.
 */
func PullModel(ctx context.Context, p Puller, options []Option, room Room, ledger *Ledger, forWhom string, progress func(string)) (Entry, error) {
	remaining := append([]Option(nil), options...)

	var failures []string

	for len(remaining) > 0 {
		choice, why, err := Choose(remaining, room)
		if err != nil {
			if len(failures) > 0 {
				err = fmt.Errorf("%w; tried: %s", err, strings.Join(failures, "; "))
			}

			return Entry{}, err
		}

		// Out of the list whatever happens, so a failure moves on.
		for i, o := range remaining {
			if o.Name == choice.Name {
				remaining = append(remaining[:i], remaining[i+1:]...)

				break
			}
		}

		if have := p.DigestOf(ctx, choice.Name); have != "" {
			return Entry{Kind: "model", Name: choice.Name, Digest: have, For: forWhom}, nil
		}

		if progress != nil {
			progress(why)
		}

		verified := false

		err = p.Pull(ctx, choice.Name, func(line string) {
			if strings.Contains(line, "verifying sha256") {
				verified = true
			}

			if progress != nil {
				progress(line)
			}
		})

		if err == nil && !verified {
			err = fmt.Errorf("the download never said it verified its checksum")
		}

		digest := ""
		if err == nil {
			digest = p.DigestOf(ctx, choice.Name)

			switch {
			case digest == "":
				err = fmt.Errorf("it is not there after downloading")
			case choice.Digest != "" && !sameDigest(digest, choice.Digest):
				err = fmt.Errorf("its digest %s is not the expected %s", digest, choice.Digest)
			}
		}

		if err != nil {
			failures = append(failures, choice.Name+": "+err.Error())

			continue
		}

		e := Entry{Kind: "model", Name: choice.Name, Digest: digest, Bytes: int64(choice.GB * gb),
			Source: "the Ollama library", For: forWhom}

		if ledger != nil {
			if err := ledger.Add(e); err != nil {
				return e, err
			}
		}

		return e, nil
	}

	return Entry{}, fmt.Errorf("no model could be downloaded: %s", strings.Join(failures, "; "))
}

func sameDigest(a, b string) bool {
	a, b = strings.TrimPrefix(a, "sha256:"), strings.TrimPrefix(b, "sha256:")
	n := min(len(a), len(b))

	return n >= 12 && a[:n] == b[:n]
}

var sizeText = regexp.MustCompile(`(?i)([\d.]+)\s*(KB|MB|GB)`)

// bytesOf reads a recipe's "78MB".
func bytesOf(size string) uint64 {
	m := sizeText.FindStringSubmatch(size)
	if m == nil {
		return 0
	}

	n, _ := strconv.ParseFloat(m[1], 64)

	switch strings.ToUpper(m[2]) {
	case "KB":
		return uint64(n * (1 << 10))
	case "MB":
		return uint64(n * (1 << 20))
	}

	return uint64(n * gb)
}

/*
 * ProvideTool installs a recipe the goal needs, alone, when that is allowed:
 * a user-level install from the recipe's official source, which the recipe
 * checks against its published checksum and proves with a harmless run
 * (provision.Carry). One needing administrator rights is ErrNeedsOwner; one
 * that would leave the disk nearly full is refused before anything is
 * downloaded.
 */
func ProvideTool(ctx context.Context, book *provision.Book, id, want string, freeDisk uint64, ledger *Ledger, forWhom string, w io.Writer) (Entry, error) {
	r, ok := book.Get(id)
	if !ok {
		return Entry{}, fmt.Errorf("there is no recipe called %q", id)
	}

	if s := r.Check(want); s.Ready() {
		// Ours or not, it is in use again.
		if ledger != nil {
			ledger.Touch("tool", r.Title)
		}

		return Entry{Kind: "tool", ID: r.ID, Name: r.Title, Version: s.Version}, nil
	}

	if r.NeedsRoot {
		return Entry{}, fmt.Errorf("%s: %w", r.Title, ErrNeedsOwner)
	}

	if !r.Installable() {
		return Entry{}, fmt.Errorf("%s cannot be installed from here: %s", r.Title, r.Manual)
	}

	// Unpacked it takes about three times the download.
	if need := 3*bytesOf(r.Size) + diskMargin; freeDisk < need {
		return Entry{}, fmt.Errorf("%s needs about %.1f GB free and there is %.1f GB",
			r.Title, float64(need)/gb, float64(freeDisk)/gb)
	}

	if w == nil {
		w = io.Discard
	}

	s, err := provision.Carry(ctx, r, want, w)
	if err != nil {
		return Entry{}, err
	}

	e := Entry{Kind: "tool", ID: r.ID, Name: r.Title, Version: s.Version, Bytes: int64(bytesOf(r.Size)), Source: r.Source, For: forWhom}

	if ledger != nil {
		if err := ledger.Add(e); err != nil {
			return e, err
		}
	}

	return e, nil
}

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}]+`)

/*
 * GoalFolder is the folder a goal's work happens in: <brain>/goals/<id>-<name>,
 * with project settings that keep the work inside it. Inside the brain so it
 * travels with it; one per goal so two goals never write over each other.
 */
func GoalFolder(root string, id int64, name string) (string, error) {
	slug := strings.Trim(unsafeName.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if r := []rune(slug); len(r) > 40 {
		slug = strings.Trim(string(r[:40]), "-")
	}

	if slug == "" {
		slug = "goal"
	}

	dir := filepath.Join(root, "goals", fmt.Sprintf("%d-%s", id, slug))

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	if _, err := workspace.Load(dir); err == nil {
		return dir, nil
	}

	cfg := workspace.Config{
		Version: 1, Name: name, Kind: "other",
		Roots: []string{"."}, Outputs: []string{"."},
		Created: time.Now().UTC().Format(time.RFC3339),
	}

	if err := workspace.Save(dir, cfg); err != nil {
		return "", err
	}

	return dir, nil
}
