package brain

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/mcp"
	"pn-scripts-assistant/internal/brain/mcpimport"
	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * The MCP servers the user already set up in other programs (package
 * mcpimport): Claude Desktop, Claude Code, Cursor, VS Code.
 *
 * Their settings files are read when the brain starts and watched like the
 * program sources (the same debounced file events, polled when a folder is
 * not there yet). What is found is listed on the Organisation page; nothing
 * is used until its owner imports it there and approves it.
 *
 * An imported server follows its entry: removed there, it is stopped and
 * its tools are gone in the same look (kept, dormant); back unchanged, it
 * is usable again as it was; changed, it is asked about again. A file that
 * cannot be read changes nothing.
 */

// mcpSource is the one source the watch fires for: every file is read
// again, which is a few milliseconds.
const mcpSource programs.Kind = "mcp"

// The state of one settings file at the last look, as a code for the page.
const (
	FileRead   = "read"       // read, with or without servers
	FileNone   = "none"       // not there: the program is not set up here
	FileBroken = "broken"     // there, not readable JSON
	FileError  = "unreadable" // there, could not be read
	FileBig    = "too-big"    // larger than mcpimport.MostBytes
)

// MCPFileStates is every code an MCPFile's State can be.
func MCPFileStates() []string { return []string{FileBig, FileBroken, FileError, FileNone, FileRead} }

// MCPFile is one settings file and what the last look found.
type MCPFile struct {
	Client string `json:"client"`
	File   string `json:"file"`
	State  string `json:"state"`
	Count  int    `json:"count"`
}

// MCPFound is one server found, as the page shows it: never its secrets'
// values. Imported names the integration it became, if it was imported.
type MCPFound struct {
	mcpimport.Found

	Key      string `json:"key"`
	Title    string `json:"client_title"`
	Imported string `json:"imported,omitempty"`
	Where    string `json:"where,omitempty"`
	Leaves   bool   `json:"leaves_the_machine"`
}

type mcpKeeping struct {
	look sync.Mutex

	mu       sync.Mutex
	watching *programs.Watching
	cancel   context.CancelFunc
	found    []mcpimport.Found
	files    []MCPFile
	looked   bool

	// env and opts replace this computer and the timings in tests.
	env  *mcpimport.Env
	opts programs.Options
}

func (b *Brain) mcpEnv() mcpimport.Env {
	if b.mcpImports.env != nil {
		return *b.mcpImports.env
	}

	home, _ := os.UserHomeDir()

	return mcpimport.Env{
		GOOS:            runtime.GOOS,
		Home:            home,
		ConfigHome:      os.Getenv("XDG_CONFIG_HOME"),
		AppData:         os.Getenv("APPDATA"),
		ClaudeConfigDir: os.Getenv("CLAUDE_CONFIG_DIR"),
	}
}

/*
 * KeepMCPImports reads the other programs' MCP settings, then resumes the
 * integrations that were running (so one whose entry went while the
 * assistant was closed is never started), then watches the files; once.
 * StopMCPImports ends it.
 */
func (b *Brain) KeepMCPImports(ctx context.Context) {
	b.mcpImports.mu.Lock()
	defer b.mcpImports.mu.Unlock()

	if b.mcpImports.cancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	b.mcpImports.cancel = cancel

	opts := b.mcpImports.opts
	if opts.Debounce <= 0 {
		opts = programs.DefaultOptions()
	}

	var targets []programs.Target
	for _, p := range mcpimport.Places(b.mcpEnv()) {
		targets = append(targets, programs.Target{Dir: filepath.Dir(p.File), Only: filepath.Base(p.File)})
	}

	go func() {
		// The watch first: a change made during the first look is looked at again.
		w := programs.Watch(ctx, map[programs.Kind][]programs.Target{mcpSource: targets}, opts, func(programs.Kind) {
			b.lookAtMCPSettings(ctx)
		})

		b.mcpImports.mu.Lock()

		if ctx.Err() != nil {
			b.mcpImports.mu.Unlock()
			w.Stop()

			return
		}

		b.mcpImports.watching = w
		b.mcpImports.mu.Unlock()

		b.lookAtMCPSettings(ctx)

		if b.Integrations != nil && ctx.Err() == nil {
			b.Integrations.Resume(ctx)
		}
	}()
}

// StopMCPImports ends the watch; nothing of it runs afterwards.
func (b *Brain) StopMCPImports() {
	b.mcpImports.mu.Lock()
	cancel, w := b.mcpImports.cancel, b.mcpImports.watching
	b.mcpImports.watching = nil
	b.mcpImports.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if w != nil {
		w.Stop()
	}
}

// readMCPSettings reads every settings file: what was found, each file's
// state, and which files were read to the end.
func (b *Brain) readMCPSettings() ([]mcpimport.Found, []MCPFile, map[string]bool) {
	env := b.mcpEnv()

	var (
		found []mcpimport.Found
		files []MCPFile
	)

	read := map[string]bool{}

	for _, p := range mcpimport.Places(env) {
		f := MCPFile{Client: string(p.Client), File: p.File}

		st, err := os.Stat(p.File)

		switch {
		case errors.Is(err, fs.ErrNotExist):
			f.State, read[p.File] = FileNone, true
		case err != nil || st.IsDir():
			f.State = FileError
		case st.Size() > mcpimport.MostBytes:
			f.State = FileBig
		default:
			raw, err := os.ReadFile(p.File)
			if err != nil {
				f.State = FileError

				break
			}

			list, err := mcpimport.Read(p, raw, env.Home)
			if err != nil {
				f.State = FileBroken

				break
			}

			f.State, f.Count, read[p.File] = FileRead, len(list), true
			found = append(found, list...)
		}

		files = append(files, f)
	}

	return found, files, read
}

/*
 * lookAtMCPSettings reads the settings and follows every imported entry.
 * Only counts go to the log: which servers somebody uses is theirs.
 */
func (b *Brain) lookAtMCPSettings(ctx context.Context) {
	b.mcpImports.look.Lock()
	defer b.mcpImports.look.Unlock()

	found, files, read := b.readMCPSettings()

	b.mcpImports.mu.Lock()
	b.mcpImports.found, b.mcpImports.files, b.mcpImports.looked = found, files, true
	b.mcpImports.mu.Unlock()

	g := b.Integrations
	if g == nil {
		return
	}

	steps := mcpimport.Follow(g.Book.Servers(), found, read, g.SecretsOf)

	by := map[mcpimport.StepKind]int{}

	for _, s := range steps {
		if ctx.Err() != nil {
			return
		}

		var err error

		switch s.Kind {
		case mcpimport.StepGone:
			err = g.EntryGone(s.ID, time.Now())
		case mcpimport.StepBack:
			err = g.EntryBack(ctx, s.ID)
		case mcpimport.StepChanged:
			err = g.EntryChanged(s.Next, s.Secrets)
		case mcpimport.StepSecrets:
			err = g.EntrySecrets(s.ID, s.Secrets)
		}

		if err != nil {
			b.Log.Warn("could not follow an imported integration", "step", s.Kind, "err", err)
		}

		by[s.Kind]++
	}

	if len(steps) > 0 {
		b.Log.Info("imported integrations followed", "gone", by[mcpimport.StepGone], "back", by[mcpimport.StepBack],
			"changed", by[mcpimport.StepChanged], "secrets", by[mcpimport.StepSecrets])
	}
}

/*
 * MCPImports is what the last look found in the other programs' settings,
 * each file's state, and how the files are kept current (nil before the
 * watch started). Before the first look it reads them now.
 */
func (b *Brain) MCPImports() ([]MCPFound, []MCPFile, []programs.SourceState) {
	b.mcpImports.mu.Lock()
	found, files, looked, w := b.mcpImports.found, b.mcpImports.files, b.mcpImports.looked, b.mcpImports.watching
	b.mcpImports.mu.Unlock()

	if !looked {
		found, files, _ = b.readMCPSettings()
	}

	imported := map[string]string{}

	if b.Integrations != nil {
		for _, s := range b.Integrations.Book.Servers() {
			if s.From != nil {
				imported[mcpimport.OriginKey(s.From)] = s.ID
			}
		}
	}

	out := make([]MCPFound, 0, len(found))

	for _, f := range found {
		v := MCPFound{Found: f, Key: f.Key(), Title: f.Client.Title(), Imported: imported[f.Key()]}

		if f.Problem == "" {
			v.Where, v.Leaves = f.Server.Where(), f.Server.Remote()
		}

		out = append(out, v)
	}

	var state []programs.SourceState
	if w != nil {
		state = w.State()
	}

	return out, files, state
}

/*
 * ImportMCP makes one found server an integration, not approved: its
 * owner's request at the desk, never a model's. The settings are read
 * again first, so what is imported is what is there now.
 */
func (b *Brain) ImportMCP(key string) (mcp.Server, error) {
	g := b.Integrations
	if g == nil {
		return mcp.Server{}, errors.New("this build has no integrations")
	}

	b.mcpImports.look.Lock()
	defer b.mcpImports.look.Unlock()

	found, _, _ := b.readMCPSettings()

	for _, f := range found {
		if f.Key() != key {
			continue
		}

		for _, s := range g.Book.Servers() {
			if s.From != nil && mcpimport.OriginKey(s.From) == key {
				return mcp.Server{}, fmt.Errorf("%s is already imported, as %s", f.Name, s.ID)
			}
		}

		s, err := mcpimport.Imported(f, mcpimport.FreeID(f.Name, g.Known))
		if err != nil {
			return mcp.Server{}, err
		}

		if err := g.Import(s, f.Secrets); err != nil {
			return mcp.Server{}, err
		}

		return s, nil
	}

	return mcp.Server{}, errors.New("that server is no longer in the settings it was found in")
}

// foundNotImported is one line per server found in another program's
// settings and not imported: its name, the program, and whether it can be.
func (b *Brain) foundNotImported() []string {
	found, _, _ := b.MCPImports()

	var out []string

	for _, f := range found {
		if f.Imported != "" {
			continue
		}

		line := f.Name + ", in " + f.Title + "'s settings"
		if f.Scope != "" {
			line += " for the folder " + f.Scope
		}

		if f.Problem != "" {
			line += ": cannot be used as written there (" + f.Problem + ")"
		}

		out = append(out, line)
	}

	return out
}
