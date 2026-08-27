// Package visuals runs the hardware-accelerated part of the interface.
//
// The core is drawn by a small Godot program in a window of its own, which is
// then made a child of the brain's window and moved to sit exactly over the
// panel the page has reserved for it. The page cannot host it directly: a web
// page has no element that owns a piece of the graphics card, so the surface
// has to be a real window placed on top.
//
// It is entirely optional. Where the runtime is missing, where the platform is
// not X11, or where anything at all goes wrong, the page draws the same things
// itself in two dimensions — which is what it did before this existed and is
// why that code has not been deleted. Nothing here holds state worth keeping,
// so it can die and be restarted without consequence.
package visuals

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"pn-brain/internal/brain/window"
)

// Rect is where on the page a surface should sit, in device pixels.
type Rect struct {
	X       int  `json:"x"`
	Y       int  `json:"y"`
	Width   int  `json:"width"`
	Height  int  `json:"height"`
	Visible bool `json:"visible"`
}

// Surface is one running visual.
type Surface struct {
	log *slog.Logger

	mu      sync.Mutex
	cmd     *exec.Cmd
	xid     uint64
	placed  Rect
	running bool
}

// Start launches the visual, if there is one to launch.
//
// Returns a Surface either way. A surface that never started answers Running as
// false and ignores everything else, so callers do not have to check.
func Start(ctx context.Context, log *slog.Logger) *Surface {
	s := &Surface{log: log}

	binary, args := find()
	if binary == "" {
		log.Info("no accelerated visual available; the page will draw it instead")

		return s
	}

	cmd := exec.CommandContext(ctx, binary, args...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Warn("could not start the visual", "error", err, "binary", binary)

		return s
	}

	s.cmd = cmd
	s.running = true

	return s
}

// Announce is called when the visual reports which window it has.
//
// The visual tells the brain over its own API rather than being discovered.
// Printing it was tried first and worked only in development: a release build
// does not write to stdout the way the editor does.
func (s *Surface) Announce(id uint64) {
	if id == 0 {
		return
	}

	go s.attach(id)
}

// attach makes the visual's window a child of the brain's.
//
// The brain's window may not be realised yet — the visual starts at the same
// time as the interface — so this retries for a short while rather than giving
// up on the first attempt.
func (s *Surface) attach(id uint64) {
	for attempt := 0; attempt < 40; attempt++ {
		s.mu.Lock()
		at := s.placed
		s.mu.Unlock()

		if at.Width == 0 {
			// Somewhere off-screen until the page says where the panel is, so
			// there is never a moment where it appears in the wrong place.
			at = Rect{X: -4000, Y: 0, Width: 640, Height: 360}
		}

		if window.Embed(id, at.X, at.Y, at.Width, at.Height) {
			s.mu.Lock()
			s.xid = id
			s.mu.Unlock()

			s.log.Info("accelerated visual attached", "window", id)

			return
		}

		time.Sleep(150 * time.Millisecond)
	}

	s.log.Warn("the visual started but could not be attached to the window")
}

// Place moves the surface to where the page says its panel is.
func (s *Surface) Place(at Rect) {
	s.mu.Lock()
	s.placed = at
	id := s.xid
	s.mu.Unlock()

	if id == 0 {
		return
	}

	if !at.Visible || at.Width <= 0 || at.Height <= 0 {
		window.Show(id, false)

		return
	}

	window.Place(id, at.X, at.Y, at.Width, at.Height)
	window.Show(id, true)
}

// Running reports whether the page should leave the drawing to the surface.
func (s *Surface) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.running && s.xid != 0
}

// Stop shuts the visual down.
func (s *Surface) Stop() {
	s.mu.Lock()
	cmd := s.cmd
	s.cmd = nil
	s.running = false
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}

	cmd.Process.Kill()
	cmd.Wait()
}

// find locates something that can draw the core.
//
// Two arrangements, in order. A packaged build ships an exported binary beside
// the brain, which is the one users get. A working copy has the project and a
// downloaded engine, which is the one this was developed against — worth
// supporting because otherwise every change to the visual would need a full
// export before it could be seen.
func find() (string, []string) {
	exe, err := os.Executable()
	if err == nil {
		beside := filepath.Join(filepath.Dir(exe), "pn-brain-visuals")

		if isExecutable(beside) {
			return beside, nil
		}
	}

	root := developmentRoot()
	if root == "" {
		return "", nil
	}

	project := filepath.Join(root, "godot")
	if _, err := os.Stat(filepath.Join(project, "project.godot")); err != nil {
		return "", nil
	}

	matches, _ := filepath.Glob(filepath.Join(root, ".tools", "Godot_v*"))

	for _, candidate := range matches {
		if isExecutable(candidate) {
			return candidate, []string{"--path", project}
		}
	}

	return "", nil
}

// developmentRoot walks up from the executable looking for the project.
func developmentRoot() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}

	at := filepath.Dir(exe)

	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(at, "godot", "project.godot")); err == nil {
			return at
		}

		parent := filepath.Dir(at)
		if parent == at {
			break
		}

		at = parent
	}

	return ""
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)

	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}
