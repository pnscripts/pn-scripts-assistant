// Command brain is PN Brain itself.
//
// One binary, one file on disk for the data, and nothing else running. There is
// no database server to install, no container runtime and no queue daemon. That
// is the point of it: an assistant that cannot start until somebody has stood a
// stack of services up first is not one anybody keeps.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"context"
	"log/slog"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"pn-brain/internal/away"
	"pn-brain/internal/brain/brain"
	"pn-brain/internal/brain/config"
	"pn-brain/internal/brain/copies"
	"pn-brain/internal/brain/desktop"
	"pn-brain/internal/brain/learning"
	"pn-brain/internal/brain/models"
	"pn-brain/internal/brain/paths"
	"pn-brain/internal/brain/places"
	"pn-brain/internal/brain/progress"
	"pn-brain/internal/brain/server"
	"pn-brain/internal/brain/speech"
	"pn-brain/internal/brain/storage"
	"pn-brain/internal/brain/store"
	"pn-brain/internal/brain/window"
	"pn-brain/internal/preflight"
	"pn-brain/internal/setup"
)

func main() {
	// No arguments means "run the app". Double-clicking an icon passes none,
	// and that is the path most people will take.
	if len(os.Args) < 2 {
		if err := runApp(nil); err != nil {
			fmt.Fprintf(os.Stderr, "\n  %v\n\n", err)
			os.Exit(1)
		}

		return
	}

	var err error

	switch os.Args[1] {
	case "app":
		err = runApp(os.Args[2:])
	case "serve":
		err = runServe(os.Args[2:])
	case "rewrite-paths":
		err = runRewritePaths(os.Args[2:])
	case "ingest":
		err = runIngest(os.Args[2:])
	case "mic-test":
		err = runMicTest(os.Args[2:])
	case "drives":
		err = runDrives(os.Args[2:])
	case "move":
		err = runMove(os.Args[2:])
	case "tidy":
		err = runTidy(os.Args[2:])
	case "start-over":
		err = runStartOver(os.Args[2:])
	case "promote":
		err = runPromote(os.Args[2:])
	case "copies":
		err = runCopies(os.Args[2:])
	case "places":
		err = runPlaces(os.Args[2:])
	case "status":
		err = runStatus(os.Args[2:])
	case "menu":
		err = runMenu(os.Args[2:])
	case "setup":
		err = runSetup(os.Args[2:])
	case "start-again":
		err = runStartAgain(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  %v\n\n", err)
		os.Exit(1)
	}
}

/*
 * runMenu puts the brain in the applications menu.
 *
 * The same thing the System tab does, for somebody who would rather type it —
 * and for the very first launch, where a person who has just downloaded one
 * file has nothing to click yet.
 */
func runMenu(args []string) error {
	fs := flag.NewFlagSet("menu", flag.ExitOnError)
	remove := fs.Bool("remove", false, "take the entry out of the menu again")
	fs.Parse(args)

	if *remove {
		if err := desktop.Remove(); err != nil {
			return err
		}

		fmt.Print("\n  Taken out of the applications menu.\n\n")

		return nil
	}

	cfg := config.Config{Name: "PN Brain"}

	if root, err := paths.Find(); err == nil {
		if loaded, err := config.Load(root.Path); err == nil {
			cfg = loaded
		}
	}

	entry, err := desktop.Install(cfg.Name)
	if err != nil {
		return err
	}

	fmt.Printf("\n  %s is in the applications menu.\n  %s\n\n", cfg.Name, entry)

	return nil
}

func usage() {
	fmt.Fprint(os.Stderr, `PN Brain

  brain                     run the app: serve, and open the window
  brain serve               serve only, without a window
  brain status              where the data lives and what is in it
  brain ingest <dir>...     learn about the projects and documents in a folder
  brain ingest --browser    learn which websites you use (domains only, never URLs)
  brain promote             turn validated lessons into durable knowledge
  brain mic-test            listen once and report what the microphone heard
  brain drives              where the brain could live, and how much room is left
  brain copies              where copies of the brain are kept  (--now to copy)
  brain places              the drives and folders it learns from  (--read to read some)
  brain move <dir>          move the brain to another drive, verifying every byte
  brain tidy                clear self-descriptions out of the review queue
  brain start-over          empty the review queue and let those files be read again
  brain start-over --facts  forget memories of files in folders it no longer reads
  brain start-over --all    forget everything learned and read every folder again
  brain setup               choose the drive, the model and the keys again
  brain start-again         stop waiting for a drive that is gone for good
  brain menu                put the brain in the applications menu
  brain menu --remove       take it out again
  brain rewrite-paths       repair stored paths after a move, then re-embed

`)
}

// openDB resolves the data root and opens the database inside it.
func openDB() (*store.DB, paths.Root, error) {
	root, err := paths.FindOrCreate()
	if err != nil {
		return nil, paths.Root{}, fmt.Errorf("locating the data root: %w", err)
	}

	db, err := store.Open(root.DatabasePath())
	if err != nil {
		return nil, root, err
	}

	return db, root, nil
}

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	fs.Parse(args)

	db, root, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	fmt.Printf("\n  data root : %s\n", root.Path)
	fmt.Printf("  database  : %s\n", root.DatabasePath())

	if info, err := os.Stat(root.DatabasePath()); err == nil {
		fmt.Printf("  size      : %.1f MB\n", float64(info.Size())/(1<<20))
	}

	total, err := db.CountFacts()
	if err != nil {
		return err
	}

	byCat, err := db.FactsByCategory()
	if err != nil {
		return err
	}

	fmt.Printf("\n  facts     : %d\n", total)

	for k, n := range byCat {
		fmt.Printf("    %-14s %d\n", k, n)
	}

	m, err := db.BuildMap()
	if err != nil {
		return err
	}

	fmt.Printf("\n  map       : %d nodes, %d links\n", len(m.Nodes), len(m.Links))

	// The strongest link is a cheap check that the similarity maths is doing
	// anything at all: a database with facts in it and a correct cosine must
	// produce a strongest pair, and a score that is neither 0 nor 1.
	var best store.MapLink
	for _, l := range m.Links {
		if l.Strength > best.Strength {
			best = l
		}
	}

	if best.Strength > 0 {
		fmt.Printf("  strongest : %d <-> %d at %.3f\n", best.Source, best.Target, best.Strength)
	}

	fmt.Println()

	return nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "", "address to listen on (loopback only)")
	fs.Parse(args)

	db, root, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	cfg, err := config.Load(root.Path)
	if err != nil {
		return err
	}

	if *addr != "" {
		cfg.Addr = *addr
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Ctrl-C and a service stop both arrive as signals; either should close the
	// listener and let in-flight replies and learning finish rather than
	// cutting them off.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	/*
	 * Close off anything the last run was in the middle of.
	 *
	 * A reply is written when a turn finishes, and a turn that was killed
	 * never did — leaving the question in the transcript with nothing after
	 * it, which reads as having been asked and ignored. Nothing running could
	 * have fixed it, because nothing was running; this is the first moment
	 * anything can.
	 */
	cutShort, err := db.FinishAbandonedTurns()
	if err != nil {
		logger.Warn("could not close off unfinished conversations", "error", err)
	} else if cutShort > 0 {
		logger.Info("closed conversations that were cut short", "count", cutShort)
	}

	b := brain.New(db, cfg, root.Path, root.DatabasePath(), logger)
	srv := server.New(b, logger)

	// The same two things the windowed start records: what the last run left
	// unfinished, and whether the drive has been carried here from another
	// machine, where it sat at a different path.
	b.NoteCutShort(cutShort)
	b.NoteJourney(root.MovedFrom)

	b.Start(ctx)
	defer b.Stop()

	/*
	 * And the icon in the menu, if it has come to point at nothing.
	 *
	 * The program can be moved — a folder renamed, a drive remounted somewhere
	 * else, a new build put in a different place — and the entry written when
	 * it was installed still names where it used to be. It stays in the menu
	 * looking exactly as it should, and does nothing when pressed.
	 */
	if fixed, err := desktop.RepairIfStale(cfg.Name); err != nil {
		logger.Warn("the menu entry points somewhere else and could not be rewritten", "error", err)
	} else if fixed {
		logger.Info("the menu entry pointed at an older location and was rewritten")
	}

	ln, err := server.Listen(cfg.Addr)
	if err != nil {
		return err
	}

	facts, _ := db.CountFacts()

	fmt.Printf("\n  %s\n", cfg.Name)
	printBanner(facts, b)
	fmt.Printf("  http://%s\n\n", ln.Addr())

	return srv.Serve(ctx, ln)
}

// runRewritePaths repairs memories that record a path which no longer exists.
//
// Every fact this brain learned was recorded while it ran inside a container,
// where the owner's disk appeared under /mnt/scan. Outside the container those
// paths point nowhere, so the knowledge was accurate and unusable at once.
//
// Rewriting the text is only half the job: the vector beside it was computed
// from the old wording, and leaving the two out of step degrades recall in a
// way nothing would ever surface. So anything changed here is embedded again.
func runRewritePaths(args []string) error {
	fs := flag.NewFlagSet("rewrite-paths", flag.ExitOnError)
	dry := fs.Bool("dry-run", false, "show what would change without writing")
	var rules multiFlag
	fs.Var(&rules, "map", "a substitution, from=to (repeatable)")
	fs.Parse(args)

	if len(rules) == 0 {
		return errors.New("give at least one -map from=to")
	}

	var parsed []store.PathRewrite

	for _, r := range rules {
		from, to, found := strings.Cut(r, "=")
		if !found || from == "" {
			return fmt.Errorf("%q is not from=to", r)
		}

		parsed = append(parsed, store.PathRewrite{From: from, To: to})
	}

	db, root, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if *dry {
		return previewRewrite(db, parsed)
	}

	changed, err := db.RewritePaths(parsed)
	if err != nil {
		return err
	}

	fmt.Printf("  rewrote %d rows\n", len(changed))

	if len(changed) == 0 {
		return nil
	}

	cfg, err := config.Load(root.Path)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	b := brain.New(db, cfg, root.Path, root.DatabasePath(), logger)

	embedder, err := b.Router.Embedder()
	if err != nil {
		return fmt.Errorf("rows were rewritten but cannot be re-embedded: %w", err)
	}

	ctx := context.Background()
	done, failed := 0, 0

	for _, r := range changed {
		vec, err := embedder.Embed(ctx, r.New)
		if err != nil {
			failed++
			fmt.Printf("  ! %s %d could not be re-embedded: %v\n", r.Table, r.ID, err)

			continue
		}

		if err := db.SetEmbedding(r.Table, r.ID, vec); err != nil {
			failed++
			fmt.Printf("  ! %s %d: %v\n", r.Table, r.ID, err)

			continue
		}

		done++

		if done%20 == 0 {
			fmt.Printf("  re-embedded %d/%d\n", done, len(changed))
		}
	}

	fmt.Printf("  re-embedded %d of %d", done, len(changed))

	if failed > 0 {
		fmt.Printf(" (%d failed — their text is new but their vector is stale)", failed)
	}

	fmt.Println()

	return nil
}

func previewRewrite(db *store.DB, rules []store.PathRewrite) error {
	facts, err := db.RecentFacts(500)
	if err != nil {
		return err
	}

	n := 0

	for _, f := range facts {
		after := f.Content

		for _, r := range rules {
			after = strings.ReplaceAll(after, r.From, r.To)
		}

		if after == f.Content {
			continue
		}

		n++

		if n <= 3 {
			fmt.Printf("  %d:\n    - %s\n    + %s\n", f.ID, trim(f.Content, 110), trim(after, 110))
		}
	}

	fmt.Printf("\n  %d of %d facts would change (nothing written)\n", n, len(facts))

	return nil
}

func trim(s string, n int) string {
	r := []rune(s)

	if len(r) <= n {
		return s
	}

	return string(r[:n]) + "…"
}

// multiFlag collects a repeatable flag.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ", ") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)

	return nil
}

func runPromote(args []string) error {
	fs := flag.NewFlagSet("promote", flag.ExitOnError)
	limit := fs.Int("limit", 100, "how many lessons to consider")
	fs.Parse(args)

	db, root, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	cfg, err := config.Load(root.Path)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	b := brain.New(db, cfg, root.Path, root.DatabasePath(), logger)

	counts, err := db.CountLessonsByStatus()
	if err != nil {
		return err
	}

	fmt.Printf("\n  lessons: %v\n\n", counts)

	promoted, duplicates, err := b.Learner.PromoteValidated(context.Background(), *limit)
	if err != nil {
		return err
	}

	fmt.Printf("  promoted   %d\n", promoted)
	fmt.Printf("  duplicates %d  (already known, rejected)\n\n", duplicates)

	return nil
}

// runApp is the whole program for someone who just wants their assistant: the
// brain starts, and a window opens onto it.
//
// Both live in this one process, so there is no launcher script, no backend to
// wait for and no window polling for something that may never answer — and none
// of the ways those could half-start.
func runApp(args []string) error {
	fs := flag.NewFlagSet("app", flag.ExitOnError)
	addr := fs.String("addr", "", "address to listen on (loopback only)")
	skipSetup := fs.Bool("skip-setup", false, "start even if the machine is missing something")
	fs.Parse(args)

	db, root, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	cfg, err := config.Load(root.Path)
	if err != nil {
		return err
	}

	if *addr != "" {
		cfg.Addr = *addr
	}

	/*
	 * One at a time.
	 *
	 * Two copies on one data root are two processes writing the same database,
	 * two learning workers, and two microphone loops both recording the room
	 * and both answering it. Somebody clicking the icon a second time wants the
	 * window they already have, so that is what they get.
	 */
	lock, err := server.Claim(root.Path)
	if err != nil {
		if !errors.Is(err, server.ErrAlreadyRunning) {
			return err
		}

		if server.Raise(cfg.Addr) {
			fmt.Fprintf(os.Stderr, "\n  %s is already running — brought it to the front.\n\n",
				cfg.Name)

			return nil
		}

		/*
		 * Holding the root and not answering: almost always a copy on its way
		 * out, which still holds it while the learning worker stops, the
		 * database closes and the microphone is let go. Waiting a few seconds
		 * costs nothing and covers a restart, or somebody closing the window
		 * and pressing the icon again straight away.
		 */
		if lock, err = server.ClaimWaiting(root.Path, 8*time.Second); err != nil {
			return fmt.Errorf(
				"%s is already running on this data root but is not answering on %s. "+
					"Close it and try again", cfg.Name, cfg.Addr)
		}
	}

	if lock != nil {
		defer lock.Release()
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	/*
	 * A signal has to close the window, not just cancel everything behind it.
	 *
	 * The main loop owns the process: gtk_main() does not return because a
	 * context was cancelled, so without this the program stops serving and goes
	 * on existing. It then still holds the lock on its data root, which stops
	 * the next copy from starting — so "close it and open it again" becomes
	 * impossible, which is the one thing it must never be. Seen once at eleven
	 * minutes after a signal, asleep in a graphics wait.
	 */
	go func() {
		<-ctx.Done()
		window.Close()
	}()

	// A machine with no model cannot answer anything, and the brain cannot
	// serve its own setup page while it is the thing that is missing. So setup
	// runs first, in its own window, and only hands over once the machine can
	// actually run an assistant.
	/*
	 * Setup runs when something is missing, and on a brand new brain.
	 *
	 * It used to run only when something essential was absent, so somebody who
	 * already had Ollama and a model — or who had just reset a working machine
	 * — was dropped straight at "what shall I call you" with every other
	 * decision already made for them. Where the brain is kept, which model it
	 * thinks with, what it asks permission for: all chosen silently, by this
	 * program, on their behalf.
	 *
	 * Those are the decisions setup exists to put in front of a person, and
	 * the first run is the one moment they are all still open. A machine that
	 * happens to have the pieces already is not a machine whose owner has
	 * agreed to anything.
	 */
	/*
	 * Anything an interrupted install left half-written, cleared first.
	 *
	 * Closing the window during a download is the ordinary way these end —
	 * somebody changes their mind, or the machine is needed for something else
	 * — and it used to leave a truncated file wearing the real name, which
	 * every check in this program then agreed was installed. Cleaned up before
	 * the requirements are read, so what setup is told is the truth.
	 */
	if removed, freed := preflight.ClearHalfFinished(); removed > 0 {
		logger.Info("cleared downloads that were interrupted",
			"files", removed, "megabytes", freed/(1<<20))
	}

	if !*skipSetup && (missingEssentials() || !cfg.SetupDone) {
		finished, err := runFirstRunSetup(config.Path(root.Path), cfg.Name)
		if err != nil {
			return err
		}

		// Settings may have changed during setup — an API key, a model.
		if cfg, err = config.Load(root.Path); err != nil {
			return err
		}

		/*
		 * Written down only when somebody actually reached the end.
		 *
		 * Marked on the way past, at first, which meant closing the window
		 * counted as finishing: shut it to go and read something, and setup
		 * never offered itself again — the drive, the model and the privacy
		 * rules all silently settled by their defaults. Closing a wizard is
		 * how people leave a wizard they have not finished, so it is the one
		 * case that must not be read as agreement.
		 *
		 * Once it is set, setup runs when it is asked for — the button under
		 * System, or "brain setup" — and on its own only when something
		 * essential has gone missing, which is a repair rather than a setup
		 * and is worth interrupting somebody for.
		 */
		if finished && !cfg.SetupDone {
			cfg.SetupDone = true

			if err := cfg.Save(root.Path); err != nil {
				logger.Warn("could not record that setup is finished", "error", err)
			}
		}
	}

	/*
	 * Close off anything the last run was in the middle of.
	 *
	 * A reply is written when a turn finishes, and a turn that was killed
	 * never did — leaving the question in the transcript with nothing after
	 * it, which reads as having been asked and ignored. Nothing running could
	 * have fixed it, because nothing was running; this is the first moment
	 * anything can.
	 */
	cutShort, err := db.FinishAbandonedTurns()
	if err != nil {
		logger.Warn("could not close off unfinished conversations", "error", err)
	} else if cutShort > 0 {
		logger.Info("closed conversations that were cut short", "count", cutShort)
	}

	b := brain.New(db, cfg, root.Path, root.DatabasePath(), logger)

	// So the greeting can say the last run stopped in the middle of something,
	// which is the first thing worth knowing on opening it again.
	b.NoteCutShort(cutShort)

	// And whether the drive has been carried here from another machine, where
	// it sat at a different path — which quietly breaks every memory that
	// names a file.
	b.NoteJourney(root.MovedFrom)

	/*
	 * Finish anything the last run validated but never promoted.
	 *
	 * A lesson goes validated, then promoted, and a scan killed between the
	 * two leaves it stranded: checked against reality, waiting for a step that
	 * nothing was ever going to run again. Four were found sitting from
	 * yesterday's interrupted scans, and nothing but a command nobody knows
	 * about would have moved them.
	 *
	 * In the background and after the window is up, because promoting means
	 * embedding and embedding is the slow thing on this machine — a start that
	 * waits for it is a start that looks broken.
	 */
	go func() {
		if b.Learner == nil {
			return
		}

		promoted, duplicates, err := b.Learner.PromoteValidated(ctx, 200)
		if err != nil {
			logger.Warn("could not finish promoting what the last run validated", "error", err)

			return
		}

		if promoted > 0 || duplicates > 0 {
			logger.Info("finished what the last run left validated",
				"promoted", promoted, "already known", duplicates)
		}
	}()

	b.Start(ctx)
	defer b.Stop()

	ln, err := server.Listen(cfg.Addr)
	if err != nil {
		return err
	}

	srv := server.New(b, logger)

	// So a second copy can ask this one to show itself rather than opening
	// another window onto the same brain.
	srv.OnPresent(window.Present)

	// Make sure there is something to run on, at every start rather than only
	// the first. On a machine that has never had this before there is nothing
	// installed at all; on one that has, a model may have been deleted since —
	// and a brain whose embedding model has gone remembers nothing while
	// carrying on as though it does.
	//
	// In the background, so the window opens straight away and the download
	// reports itself on the progress line rather than behind a blank screen.
	go func() {
		client := models.New(cfg.OllamaURL)

		chosen, err := models.Ensure(ctx, client, cfg.OllamaModel, cfg.EmbedModel, func(note string) {
			progress.Set("model", note)
		})

		progress.Done()

		if err != nil {
			logger.Warn("could not make sure of the models", "error", err)

			return
		}

		if chosen != cfg.OllamaModel {
			logger.Info("using the model that is installed", "model", chosen)
			b.UseModel(chosen)
		}
	}()

	serveErr := make(chan error, 1)

	go func() { serveErr <- srv.Serve(ctx, ln) }()

	url := "http://" + ln.Addr().String()

	facts, _ := db.CountFacts()
	fmt.Printf("\n  %s\n", cfg.Name)
	printBanner(facts, b)
	fmt.Printf("  %s\n\n", url)

	if !window.Available() {
		// Without a window the brain is a reduced program, not a broken one:
		// it is serving, and any browser can reach it. Say so and keep running
		// rather than exiting on a missing build dependency.
		fmt.Fprintf(os.Stderr, "  %v\n\n", window.Open(url, cfg.Name, 1280, 860))

		return <-serveErr
	}

	// The window owns the main thread from here; closing it ends the program,
	// which is what closing an application's window should do.
	if err := window.Open(url, cfg.Name, 1280, 860); err != nil {
		return err
	}

	stop()

	return nil
}

// runIngest is how the brain learns about a machine.
//
// Read-only: it looks at somebody's entire working life, and the one guarantee
// worth making about that is that looking changes nothing.
func runIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	docs := fs.Bool("documents", false, "scan for documents instead of projects")
	browser := fs.Bool("browser", false, "scan browser history for the sites you use")
	dry := fs.Bool("dry-run", false, "list what would be learned, without storing it")
	fs.Parse(args)

	if fs.NArg() == 0 && !*browser {
		return errors.New("give at least one directory to scan")
	}

	db, root, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	cfg, err := config.Load(root.Path)
	if err != nil {
		return err
	}

	var observations learning.Observations

	if *browser {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		sites, err := learning.ScanBrowsers(home)
		if err != nil {
			return err
		}

		fmt.Printf("  browser history — %d sites you use regularly\n", len(sites))
		fmt.Printf("  (domains and visit counts only; no URLs are read or stored)\n")

		observations = append(observations, learning.FromSites(sites, cfg.Owner)...)
	}

	for _, dir := range fs.Args() {
		if *docs {
			found, err := learning.ScanDocuments(dir)
			if err != nil {
				return err
			}

			fmt.Printf("  %s — %d documents\n", dir, len(found))
			observations = append(observations, learning.FromDocuments(found, cfg.Owner)...)

			continue
		}

		found, err := learning.ScanProjects(dir)
		if err != nil {
			return err
		}

		fmt.Printf("  %s — %d projects\n", dir, len(found))
		observations = append(observations, learning.FromProjects(found, cfg.Owner)...)
	}

	if *dry {
		fmt.Printf("\n  %d observations (nothing stored)\n\n", len(observations))

		for i, o := range observations {
			if i >= 10 {
				fmt.Printf("  … and %d more\n", len(observations)-10)

				break
			}

			fmt.Printf("  - %s\n", firstLine(o.Content))
		}

		fmt.Println()

		return nil
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	b := brain.New(db, cfg, root.Path, root.DatabasePath(), logger)

	fmt.Printf("\n  embedding %d observations…\n", len(observations))

	rep, err := b.Learner.Ingest(context.Background(), observations, func(r learning.IngestReport) {
		if r.Promoted%25 == 0 {
			fmt.Printf("    %d learned, %d already known\n", r.Promoted, r.Duplicates)
		}
	})
	if err != nil {
		return err
	}

	fmt.Printf("\n  seen       %d\n", rep.Seen)
	fmt.Printf("  learned    %d\n", rep.Promoted)
	fmt.Printf("  known      %d  (duplicates, not stored twice)\n", rep.Duplicates)

	if rep.Waiting > 0 {
		fmt.Printf("  waiting    %d  (needs your judgement in the app)\n", rep.Waiting)
	}

	fmt.Printf("  vanished   %d  (path no longer exists)\n", rep.Rejected)

	if rep.Failed > 0 {
		fmt.Printf("  failed     %d\n", rep.Failed)
	}

	fmt.Println()

	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}

	return trim(s, 110)
}

// runTidy clears self-description out of the review queue.
//
// Extraction now refuses to record the assistant describing itself, but lessons
// captured before that guard existed are still sitting there — six of the
// twelve in this brain. Rejecting them by hand is busywork a person should not
// have to do, and leaving them buries the ones that genuinely need judgement.
//
// It shows what it would do before doing it, because this deletes nothing a
// person asked for and everything it touches was somebody's data.
func runTidy(args []string) error {
	fs := flag.NewFlagSet("tidy", flag.ExitOnError)
	apply := fs.Bool("apply", false, "actually reject them (otherwise only lists)")
	fs.Parse(args)

	db, root, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	cfg, err := config.Load(root.Path)
	if err != nil {
		return err
	}

	lessons, err := db.LessonsByStatus(learning.StatusProposed, 500)
	if err != nil {
		return err
	}

	var doomed, kept []store.Lesson
	reason := map[int64]string{}

	for _, l := range lessons {
		switch {
		case learning.IsAboutTheAssistant(l.Content, cfg.Name, cfg.Owner):
			reason[l.ID] = "describes the assistant"
			doomed = append(doomed, l)
		case learning.IsNotAFact(l.Content):
			// A bare path or filename: the model handing back the answer it
			// just gave rather than learning anything.
			reason[l.ID] = "not a fact, just a fragment"
			doomed = append(doomed, l)
		default:
			kept = append(kept, l)
		}
	}

	fmt.Printf("\n  %d waiting for review\n\n", len(lessons))

	if len(doomed) > 0 {
		fmt.Println("  not worth remembering:")

		for _, l := range doomed {
			fmt.Printf("    %4d  %-70s  (%s)\n",
				l.ID, trim(oneLine(l.Content), 70), reason[l.ID])
		}

		fmt.Println()
	}

	if len(kept) > 0 {
		fmt.Println("  genuinely needs your judgement:")

		for _, l := range kept {
			fmt.Printf("    %4d  %s\n", l.ID, trim(oneLine(l.Content), 96))
		}

		fmt.Println()
	}

	if !*apply {
		if len(doomed) > 0 {
			fmt.Printf("  nothing changed. Run with --apply to reject those %d.\n\n", len(doomed))
		}

		return nil
	}

	for _, l := range doomed {
		if err := db.SetLessonStatus(l.ID, learning.StatusRejected); err != nil {
			return err
		}
	}

	fmt.Printf("  rejected %d; %d left for you.\n\n", len(doomed), len(kept))

	return nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// missingEssentials reports whether anything the brain cannot work without is
// absent.
//
// Optional requirements — the WebKit headers, for instance — do not count. A
// brain without a native window still answers questions, and stopping to demand
// a build dependency from somebody who only wants to ask it something would be
// the program serving itself.
func missingEssentials() bool {
	return preflight.BlockingCount(preflight.Check()) > 0
}

// runFirstRunSetup shows the setup page and waits for it to finish.
//
// It carries its own tiny web server because of an ordering problem: the brain
// cannot serve a page explaining that Ollama is missing while Ollama being
// missing is what stops the brain from starting.
func runFirstRunSetup(settingsPath, name string) (bool, error) {
	srv, err := setup.New(settingsPath)
	if err != nil {
		return false, fmt.Errorf("could not start setup: %w", err)
	}

	done := make(chan struct{})
	go srv.Serve(func() { close(done) })

	/*
	 * Whatever drive was chosen is created before anything else runs.
	 *
	 * Setup only records the choice — nothing is written until it finishes —
	 * so this is where it takes effect. Doing it here rather than inside the
	 * page means changing your mind twice leaves nothing behind on the drives
	 * you changed your mind about.
	 */
	defer func() {
		chosen := srv.ChosenRoot()
		if chosen == "" {
			return
		}

		// Choose, not Create: the folder existing somewhere is not the same
		// as the program knowing that is the one to open.
		if _, err := paths.Choose(chosen); err != nil {
			fmt.Fprintf(os.Stderr, "\n  could not use %s: %v\n\n", chosen, err)
		}
	}()

	/*
	 * Why setup is on the screen, which is not always the same reason.
	 *
	 * Opened deliberately to change a drive, it announced that the machine was
	 * missing something PN Brain needs — which is alarming, wrong, and sends
	 * somebody looking for a fault that is not there.
	 */
	if blocking := preflight.Blocking(preflight.Check()); len(blocking) > 0 {
		names := make([]string, 0, len(blocking))
		for _, req := range blocking {
			names = append(names, req.Requirement.Name)
		}

		fmt.Printf("\n  Still needed: %s\n", strings.Join(names, ", "))
	} else {
		fmt.Printf("\n  Everything needed is already here.\n")
	}

	fmt.Printf("  Setup: %s\n\n", srv.URL())

	if !window.Available() {
		/*
		 * No native window, so the system's browser is asked instead.
		 *
		 * This is the macOS and Windows path, where printing an address and
		 * waiting means setup only happens for somebody who was watching a
		 * terminal. The address is still printed, because opening a browser
		 * can fail and a visible URL is the fallback that always works.
		 */
		if !window.OpenInBrowser(srv.URL()) {
			fmt.Fprintf(os.Stderr, "  Open this in a browser to continue:\n    %s\n\n",
				srv.URL())

			/*
			 * And said somewhere it can be seen.
			 *
			 * The Windows build is a GUI program with no console attached, so
			 * the line above goes nowhere. Failing to open the browser there
			 * would otherwise be a program that starts, shows nothing, and
			 * exits — indistinguishable from one that does not work.
			 */
			window.Alert("PN Brain — Setup",
				"Setup is ready but the browser could not be opened.\n\n"+
					"Open this address yourself to continue:\n"+srv.URL())
		}

		<-done

		return srv.Finished(), stillMissing(srv)
	}

	/*
	 * Pressing Continue has to close the window, or nothing happens at all.
	 *
	 * window.Open sits in the GTK main loop until the window goes away, and
	 * the /done handler closed a channel that only the browser path was
	 * listening to. So the button set a flag, the window stayed exactly where
	 * it was, and setup looked frozen at the moment somebody had finished it —
	 * the last screen of the first thing they ever did with this program.
	 */
	go func() {
		<-done
		window.Close()
	}()

	if err := window.Open(srv.URL(), name+" — Setup", 900, 700); err != nil {
		return false, err
	}

	return srv.Finished(), stillMissing(srv)
}

/*
 * stillMissing stops a half-set-up brain from starting as though it were ready.
 *
 * Closing the setup window is not the same as finishing setup, and treating it
 * as the same started a brain with nothing to think with — which presents as a
 * window that opens, accepts what you say, and answers nothing. The missing
 * piece is named here instead, along with the fact that nothing was changed,
 * because "it does not work" is the least useful thing a program can tell you
 * about itself when it knows exactly what is wrong.
 *
 * Only blocking requirements count. Setup closed on a machine that is merely
 * missing something optional is a machine that is ready.
 */
func stillMissing(srv *setup.Server) error {
	if srv.Finished() {
		return nil
	}

	missing := preflight.Blocking(preflight.Check())
	if len(missing) == 0 {
		return nil
	}

	names := make([]string, 0, len(missing))
	for _, req := range missing {
		names = append(names, req.Requirement.Name)
	}

	return fmt.Errorf("setup was closed before it finished, so nothing was changed.\n"+
		"  Still needed: %s\n"+
		"  Run it again with: brain setup",
		strings.Join(names, ", "))
}

/*
 * runSetup opens setup on a machine that does not need it.
 *
 * The first-run path only appears when something is missing, which left no way
 * back in afterwards — so the drive, the model and the API key were all
 * decisions you could make exactly once, and only on a machine that happened
 * to be broken at the time.
 */
func runSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	fs.Parse(args)

	root, err := paths.FindOrCreate()
	if err != nil {
		return err
	}

	cfg, err := config.Load(root.Path)
	if err != nil {
		return err
	}

	finished, err := runFirstRunSetup(config.Path(root.Path), cfg.Name)
	if err != nil {
		return err
	}

	/*
	 * And then actually continue to PN Brain, which is what the button says.
	 *
	 * This path is reached by "brain setup" and by the button in the running
	 * program's settings. Pressing Continue closed setup and stopped, so the
	 * button promised the assistant and delivered an empty screen.
	 *
	 * Started unconditionally when somebody pressed it: if a brain is already
	 * running — which it is when this was opened from its own settings — the
	 * one-at-a-time guard brings that window to the front instead of starting
	 * a second one, which is the right answer to "continue to PN Brain" in
	 * both cases.
	 */
	if !finished {
		return nil
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}

	brain := exec.Command(self)
	if err := brain.Start(); err != nil {
		return fmt.Errorf("could not start PN Brain: %w", err)
	}

	// Released rather than waited on: the assistant outlives the setup process
	// that opened it, and holding this one alive would leave a stray parent
	// around for the whole session.
	go func() { _ = brain.Wait() }()

	return nil
}

/*
 * brainAway explains a brain that is somewhere else, and offers the two ways
 * out — without taking either on somebody's behalf.
 *
 * In the window when there is one, because "everything works from inside the
 * program" matters most in exactly this case: the person reading this has just
 * been told their assistant cannot find what it knows, and answering that with
 * a line of terminal output they may never see is the worst available moment
 * to fall back on a terminal.
 */
func brainAway(elsewhere *paths.AwayError) error {
	fmt.Fprintf(os.Stderr, "\n  PN Brain keeps everything it knows at:\n    %s\n\n", elsewhere.Path)
	fmt.Fprint(os.Stderr, "  That place is not available right now. If it is on a drive,\n")
	fmt.Fprint(os.Stderr, "  plug the drive in and start PN Brain again — nothing is lost.\n\n")
	fmt.Fprint(os.Stderr, "  If it is gone for good and you want to begin again:\n")
	fmt.Fprint(os.Stderr, "    brain start-again\n\n")

	if !window.Available() {
		return errNothingMore
	}

	srv, err := away.New(elsewhere.Path)
	if err != nil {
		return errNothingMore
	}

	done := make(chan struct{})
	go srv.Serve(func() { close(done) })

	_ = window.Open(srv.URL(), "PN Brain", 640, 460)

	if srv.StartAgain() {
		paths.Forget()

		fmt.Fprint(os.Stderr, "  Starting again. Run PN Brain once more.\n\n")
	}

	return errNothingMore
}

// errNothingMore ends the program without printing a second explanation on top
// of the one that was just given.
var errNothingMore = errors.New("")

func runStartAgain(args []string) error {
	fs := flag.NewFlagSet("start-again", flag.ExitOnError)
	fs.Parse(args)

	away, ok := paths.LastKnown()
	if !ok {
		fmt.Print("\n  Nothing to forget — PN Brain is not waiting on anywhere.\n\n")

		return nil
	}

	paths.Forget()

	fmt.Printf("\n  PN Brain will no longer wait for %s.\n", away)
	fmt.Print("  The next start makes a new, empty brain. Anything at that\n")
	fmt.Print("  place is untouched, and plugging it back in still works —\n")
	fmt.Print("  it is found again by being there, not by being remembered.\n\n")

	return nil
}

/*
 * runCopies says where the copies are, and makes them on request.
 *
 * The panel in the app does this too, and that is the one people use. This is
 * for the moment somebody is at a terminal with the drive in their hand and
 * wants an answer without opening a window — and for scripting a copy before
 * unplugging.
 */
/*
 * runPlaces lists the drives and folders the brain looks after.
 *
 * The panel in the app does this too, and that is the one people use. This is
 * for the moment somebody is at a terminal with a drive in their hand, and for
 * scripting a read before unplugging it.
 */
func runPlaces(args []string) error {
	fs := flag.NewFlagSet("places", flag.ExitOnError)
	read := fs.String("read", "", "read some more from this place now, by name or path")
	fs.Parse(args)

	db, root, err := openDB()
	if err != nil {
		var elsewhere *paths.AwayError
		if errors.As(err, &elsewhere) {
			return brainAway(elsewhere)
		}

		return err
	}
	defer db.Close()

	cfg, err := config.Load(root.Path)
	if err != nil {
		return err
	}

	list, err := places.Status(root.Path)
	if err != nil {
		return err
	}

	if len(list) == 0 {
		fmt.Print("\n  Nowhere yet. It only knows what it has been shown.\n\n" +
			"  Add a drive or folder in the app, under Storage.\n\n")

		return nil
	}

	if want := strings.TrimSpace(*read); want != "" {
		if err := readSomeNow(db, cfg.Owner, root.Path, list, want); err != nil {
			return err
		}

		list, _ = places.Status(root.Path)
	}

	fmt.Println()

	for _, p := range list {
		fmt.Printf("  %s  —  %s\n", p.Name, places.Short(p.Path))

		switch {
		case !p.Reachable:
			fmt.Printf("      %s, %d learned from it so far", p.Trouble, p.Learned)

		case p.Never() && p.Learned == 0:
			fmt.Print("      attached, nothing read yet")

		default:
			fmt.Printf("      attached, %d learned", p.Learned)
		}

		if p.Waiting > 0 {
			fmt.Printf(", %d still to read (about %d minutes)",
				p.Waiting, p.Waiting*places.SecondsEach/60)
		} else if p.Reachable && !p.Never() {
			fmt.Print(", all of it read")
		}

		fmt.Print("\n\n")
	}

	return nil
}

// readSomeNow takes one bite out of a named place, the same size the
// background pass takes — a whole drive is hours and belongs to the background.
func readSomeNow(db *store.DB, owner, root string, list []places.Place, want string) error {
	for _, p := range list {
		if !strings.EqualFold(p.Name, want) && p.Path != filepath.Clean(want) {
			continue
		}

		if !p.Reachable {
			return fmt.Errorf("%s is %s", p.Name, p.Trouble)
		}

		// The same brain the app builds, for the same learner. Quietly: this
		// is a command whose output is the report, not a log.
		quiet := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
		cfg, err := config.Load(root)
		if err != nil {
			return err
		}

		b := brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"), quiet)

		fmt.Printf("\n  reading %s…\n", p.Name)

		pass, err := places.Look(context.Background(), b.Learner, db, owner, p)

		places.Note(root, pass.Place)

		if err != nil {
			return err
		}

		fmt.Printf("\n  read %d from %s: %d new, %d already known\n",
			pass.Took, p.Name, pass.Learned, pass.Known)

		return nil
	}

	return fmt.Errorf("%q is not one of the places", want)
}

func runCopies(args []string) error {
	fs := flag.NewFlagSet("copies", flag.ExitOnError)
	now := fs.Bool("now", false, "refresh every copy that can be reached, then report")
	fs.Parse(args)

	db, root, err := openDB()
	if err != nil {
		var elsewhere *paths.AwayError
		if errors.As(err, &elsewhere) {
			return brainAway(elsewhere)
		}

		return err
	}
	defer db.Close()

	fmt.Printf("\n  the brain is at %s\n\n", root.Path)

	var held []copies.Copy

	if *now {
		held = copies.WriteAll(context.Background(), db, root.Path)
	} else if held, err = copies.Status(root.Path); err != nil {
		return err
	}

	if len(held) == 0 {
		fmt.Print("  No copies. Everything it knows is in one place.\n\n" +
			"  Add one in the app, under Storage — or on any drive or folder you like.\n\n")

		return nil
	}

	for _, c := range held {
		fmt.Printf("  %s\n", c.Path)

		switch {
		case c.Never():
			fmt.Print("      nothing copied here yet")

		default:
			fmt.Printf("      %d things, %s, copied %s",
				c.Facts, roughSize(c.Bytes), c.At.Local().Format("2 Jan 15:04"))
		}

		if c.Trouble != "" {
			fmt.Printf("  —  %s", c.Trouble)
		}

		fmt.Print("\n\n")
	}

	return nil
}

// roughSize is a byte count in the unit somebody would say it in. A new brain
// is tens of kilobytes, and printing that as "0MB" reads as a failed copy.
func roughSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(bytes)/(1<<30))

	case bytes >= 1<<20:
		return fmt.Sprintf("%dMB", bytes>>20)
	}

	return fmt.Sprintf("%dKB", bytes>>10)
}

func runDrives(args []string) error {
	fs := flag.NewFlagSet("drives", flag.ExitOnError)
	fs.Parse(args)

	root, err := paths.FindOrCreate()
	if err != nil {
		return err
	}

	drives, err := storage.Drives(root.Path)
	if err != nil {
		return err
	}

	report := storage.Check(root.Path, root.DatabasePath())

	fmt.Printf("\n  the brain is on %s — %.0fGB free of %.0fGB (%s)\n\n",
		root.Path,
		float64(report.FreeBytes)/(1<<30), float64(report.TotalBytes)/(1<<30),
		report.Level)

	if report.Advice != "" {
		fmt.Printf("  %s\n\n", report.Advice)
	}

	fmt.Printf("  %-42s %8s %8s  %s\n", "MOUNTED AT", "SIZE", "FREE", "")

	for _, d := range drives {
		marks := []string{}

		if d.Current {
			marks = append(marks, "in use")
		}

		if d.Removable {
			marks = append(marks, "removable")
		}

		if !d.Writable {
			marks = append(marks, "not writable")
		}

		fmt.Printf("  %-42s %7.0fG %7.0fG  %s\n",
			trim(d.MountPoint, 42),
			float64(d.TotalBytes)/(1<<30),
			float64(d.FreeBytes)/(1<<30),
			strings.Join(marks, ", "))
	}

	fmt.Printf("\n  Move it with:  brain move <folder on another drive>\n\n")

	return nil
}

// runMove relocates the brain. It refuses to run while the brain is serving,
// because copying a SQLite file that something else is writing to produces a
// file that looks fine and is not.
func runMove(args []string) error {
	fs := flag.NewFlagSet("move", flag.ExitOnError)
	keep := fs.Bool("keep-source", false, "leave the original in place as a backup")
	fs.Parse(args)

	if fs.NArg() != 1 {
		return errors.New("give the folder to move the brain to")
	}

	root, err := paths.Find()
	if err != nil {
		return fmt.Errorf("no brain found to move: %w", err)
	}

	destination := fs.Arg(0)

	fmt.Printf("\n  from  %s\n  to    %s\n\n", root.Path, destination)

	last := 0

	rep, err := storage.Move(root.Path, destination, *keep, func(_ string, done, total int) {
		// Percentage rather than filenames: a brain is thousands of small files
		// and scrolling all of them tells nobody anything.
		if pct := done * 100 / total; pct >= last+10 {
			last = pct
			fmt.Printf("  %d%%  (%d of %d files)\n", pct, done, total)
		}
	})
	if err != nil {
		return err
	}

	fmt.Printf("\n  moved %d files, %.1fGB, all %d verified\n",
		rep.Files, float64(rep.Bytes)/(1<<30), rep.Verified)

	if rep.SourceKept {
		fmt.Printf("  the original is still at %s\n", rep.From)
	}

	fmt.Printf("\n  The brain will find itself there on the next start.\n\n")

	return nil
}

// runMicTest listens for one turn and reports the numbers behind it.
//
// "It cannot hear me" is unanswerable without them: the room's noise floor,
// what therefore counted as speech, and how loud the loudest moment actually
// was. Three numbers separate a muted microphone from a quiet voice from a
// recogniser that heard fine and understood nothing.
func runMicTest(args []string) error {
	fs := flag.NewFlagSet("mic-test", flag.ExitOnError)
	device := fs.String("device", "", "input to listen through (default: the app's choice)")
	fs.Parse(args)

	ctx := context.Background()

	mics, err := speech.Microphones(ctx)
	if err != nil {
		return err
	}

	target := *device

	if target == "" {
		// Prefer something plainly named a microphone over the system default,
		// which is often a jack with nothing in it.
		for _, m := range mics {
			if strings.Contains(strings.ToLower(m.Name), "mic") {
				target = m.ID

				break
			}
		}
	}

	fmt.Println("\n  inputs:")

	for _, m := range mics {
		mark := "  "
		if m.ID == target {
			mark = "->"
		}

		fmt.Printf("   %s %s%s\n", mark, m.Name, map[bool]string{true: "  (system default)"}[m.Default])
	}

	f, err := os.CreateTemp("", "pn-brain-mictest-*.wav")
	if err != nil {
		return err
	}

	path := f.Name()
	f.Close()

	defer os.Remove(path)

	fmt.Printf("\n  Speak now — it stops when you stop.\n\n")

	turn, err := speech.RecordTurn(ctx, target, path)
	if err != nil {
		return err
	}

	level, _ := speech.MeasureWAV(path)

	fmt.Printf("  room noise floor   %d\n", turn.NoiseFloor)
	fmt.Printf("  speech needed      %d\n", turn.Threshold)
	fmt.Printf("  loudest you got    %d\n", turn.PeakRMS)
	fmt.Printf("  peak of full scale %d%%\n\n", int(level.Ratio*100))

	if !turn.HeardSpeech {
		fmt.Println("  Nothing rose above the room. Either the input is not the one you")
		fmt.Println("  are speaking into, or its gain is too low.")

		return nil
	}

	text, err := speech.TranscribeFast(ctx, path)
	if err != nil {
		return err
	}

	if text == "" {
		fmt.Println("  Heard you, but made out no words. Try speaking more clearly.")

		return nil
	}

	fmt.Printf("  Heard: %q\n\n", text)

	return nil
}

// printBanner names the model that will answer, not the one configured.
//
// They differ whenever nobody has chosen one, which is the ordinary case. The
// banner said qwen2.5-coder while llama3.2 wrote every reply.
func printBanner(facts int, b *brain.Brain) {
	work, _, _ := b.Roles()

	fmt.Printf("  %d facts  ·  privacy: %s  ·  %s\n", facts, b.Mode, work)
}

/*
 * runStartOver empties the review queue and lets those files be read again.
 *
 * The queue is meant to be a handful of judgements. Petar's held 9,199, and
 * grouped by the folder they came from the top ten were all somebody else's —
 * a game engine's package cache, a vendored PHP library, two client sites'
 * upload folders. The rules that let those in have been tightened; this is the
 * other half, because tightening them changes nothing about what is already
 * there.
 *
 * Deleting rather than rejecting, and that is the whole point of the command.
 * A rejected lesson still records that its file was seen, so a queue emptied
 * by rejection is a queue that never refills — including for the documents
 * that should be read again under the new rules. See DeleteLessonsByStatus.
 *
 * Everything is written out first. The rows are somebody's data even when they
 * are nine thousand lines of a Unity manual, and a command that deletes
 * without leaving a copy is one nobody should run twice.
 */
func runStartOver(args []string) error {
	fs := flag.NewFlagSet("start-over", flag.ExitOnError)
	apply := fs.Bool("apply", false, "actually do it (otherwise only reports)")
	facts := fs.Bool("facts", false,
		"also forget what was already remembered from folders now skipped")
	all := fs.Bool("all", false,
		"forget everything learned and read every folder again from nothing")
	fs.Parse(args)

	db, root, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if *all {
		return learnFromNothing(db, root, *apply)
	}

	if *facts {
		return forgetSkippedFolders(db, root, *apply)
	}

	waiting, err := db.CountPendingLessons()
	if err != nil {
		return err
	}

	if waiting == 0 {
		fmt.Println("Nothing is waiting for review.")

		return nil
	}

	// By folder, because that is the reading that makes the number mean
	// something: 9,199 is a shrug, and 3,589 of them from one package cache is
	// a diagnosis.
	byFolder := map[string]int{}

	var after int64

	for {
		batch, err := db.LessonsAfter(learning.StatusProposed, after, 1000)
		if err != nil {
			return err
		}

		if len(batch) == 0 {
			break
		}

		for _, l := range batch {
			after = l.ID
			byFolder[folderOf(l.Source)]++
		}
	}

	type row struct {
		dir string
		n   int
	}

	rows := make([]row, 0, len(byFolder))

	for dir, n := range byFolder {
		rows = append(rows, row{dir, n})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].n > rows[j].n })

	fmt.Printf("%d waiting, from %d folders. The largest:\n\n", waiting, len(rows))

	for i, r := range rows {
		if i >= 10 {
			fmt.Printf("  and %d more folders\n", len(rows)-i)

			break
		}

		fmt.Printf("  %6d  %s\n", r.n, r.dir)
	}

	if !*apply {
		fmt.Println("\nNothing has been changed. Run with --apply to empty the queue.")
		fmt.Println("Everything removed is written to a file first, and those files")
		fmt.Println("become readable again on the next pass, under the current rules.")

		return nil
	}

	kept, err := backupLessons(db, root)
	if err != nil {
		return fmt.Errorf("writing the backup, so nothing was deleted: %w", err)
	}

	fmt.Printf("\nWritten to %s\n", kept)

	gone, err := db.DeleteLessonsByStatus(learning.StatusProposed)
	if err != nil {
		return err
	}

	fmt.Printf("Removed %d. Those documents will be read again on the next pass.\n", gone)

	return nil
}

// folderOf is the directory a lesson's source refers to, for grouping.
func folderOf(source string) string {
	if source == "" {
		return "(no source)"
	}

	if i := strings.Index(source, ":"); i >= 0 {
		source = source[i+1:]
	}

	return filepath.Dir(source)
}

// backupLessons writes every proposed lesson to a file beside the database.
func backupLessons(db *store.DB, root paths.Root) (string, error) {
	name := filepath.Join(root.Path,
		"discarded-"+time.Now().Format("2006-01-02-150405")+".json")

	f, err := os.Create(name)
	if err != nil {
		return "", err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")

	var after int64

	for {
		batch, err := db.LessonsAfter(learning.StatusProposed, after, 1000)
		if err != nil {
			return "", err
		}

		if len(batch) == 0 {
			break
		}

		for _, l := range batch {
			after = l.ID

			if err := enc.Encode(l); err != nil {
				return "", err
			}
		}
	}

	return name, f.Sync()
}

/*
 * forgetSkippedFolders removes memories of files the walk would now refuse.
 *
 * Tightening the rules stops new rubbish arriving and does nothing about what
 * is already remembered. Petar's brain held 1,220 facts, and 746 of them were
 * listings of files inside a Unity package cache, a vendored copy of
 * HTMLPurifier, and two client sites' upload folders — "Petar has a text
 * document called Attr.AllowedFrameTargets.txt at …". Three in five of
 * everything it knew, and every search had to wade through it.
 *
 * Judged by asking the scanner, not by a list of bad words written out again
 * here: a fact is removed when the folder its file sits in is one the walk
 * would not now enter. So this stays right as the skip list grows, and it can
 * never disagree with what the scanner is actually doing.
 *
 * Facts with no file in them are never touched. A sentence about somebody's
 * preferences is not about a folder and has nothing to do with any of this.
 */
func forgetSkippedFolders(db *store.DB, root paths.Root, apply bool) error {
	all, err := db.AllFacts()
	if err != nil {
		return err
	}

	var doomed []store.Fact

	for _, f := range all {
		path := learning.PathIn(f.Content)

		if path == "" || !learning.UnderASkippedFolder(path) {
			continue
		}

		doomed = append(doomed, f)
	}

	if len(doomed) == 0 {
		fmt.Printf("All %d memories are from folders it would still read.\n", len(all))

		return nil
	}

	fmt.Printf("%d of %d memories name a file in a folder that is now skipped.\n\n",
		len(doomed), len(all))

	for i, f := range doomed {
		if i >= 5 {
			fmt.Printf("  and %d more\n", len(doomed)-i)

			break
		}

		fmt.Printf("  %s\n", truncate(f.Content, 110))
	}

	if !apply {
		fmt.Println("\nNothing has been changed. Add --apply to forget them.")

		return nil
	}

	kept, err := backupFacts(doomed, root)
	if err != nil {
		return fmt.Errorf("writing the backup, so nothing was deleted: %w", err)
	}

	fmt.Printf("\nWritten to %s\n", kept)

	var gone int

	for _, f := range doomed {
		if err := db.DeleteFact(f.ID); err != nil {
			return fmt.Errorf("after forgetting %d of them: %w", gone, err)
		}

		gone++
	}

	fmt.Printf("Forgotten %d. %d memories left.\n", gone, len(all)-gone)

	return nil
}

// backupFacts writes what is about to be forgotten to a file beside the
// database, because it was somebody's data even when it was a build directory.
func backupFacts(facts []store.Fact, root paths.Root) (string, error) {
	name := filepath.Join(root.Path,
		"forgotten-"+time.Now().Format("2006-01-02-150405")+".json")

	f, err := os.Create(name)
	if err != nil {
		return "", err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")

	for _, fact := range facts {
		if err := enc.Encode(fact); err != nil {
			return "", err
		}
	}

	return name, f.Sync()
}

// truncate shortens a line for a listing, by runes so it cannot split a letter.
func truncate(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))

	if len(r) <= n {
		return string(r)
	}

	return string(r[:n]) + "…"
}

/*
 * learnFromNothing empties the knowledge and starts the reading again.
 *
 * The strongest of the three, and the only one that throws away things that
 * were right. It exists because the rules for what is worth reading changed
 * enough that what is stored was produced by rules nobody would choose now:
 * two in three of Petar's memories were listings of a package cache, and the
 * documents that should have been read instead are all marked as read.
 * Tidying that leaves a memory that is half one set of rules and half another.
 *
 * Three things go, and all three have to. The facts, which are what it knows.
 * The lessons, which are the record of every file it has been shown — keeping
 * those would give a brain that knows nothing and believes it has read
 * everything. And the counters beside each place, or the panel spends the next
 * week reporting a hundred things learned from a memory holding none of them.
 *
 * The conversations stay, and so do the places themselves. Somebody asking to
 * read their documents again has not asked to lose what they have said or to
 * put four folders back by hand.
 */
func learnFromNothing(db *store.DB, root paths.Root, apply bool) error {
	facts, err := db.CountFacts()
	if err != nil {
		return err
	}

	byKind, err := db.FactsByCategory()
	if err != nil {
		return err
	}

	waiting, err := db.CountPendingLessons()
	if err != nil {
		return err
	}

	fmt.Printf("This forgets everything learned and reads every folder again.\n\n")
	fmt.Printf("  %d memories", facts)

	if len(byKind) > 0 {
		kinds := make([]string, 0, len(byKind))

		for k, n := range byKind {
			kinds = append(kinds, fmt.Sprintf("%d %s", n, k))
		}

		sort.Strings(kinds)

		fmt.Printf("  (%s)", strings.Join(kinds, ", "))
	}

	fmt.Printf("\n  %d waiting for review\n", waiting)
	fmt.Printf("  every record of which files have been read\n")
	fmt.Printf("  the learned and waiting counts beside each place\n\n")
	fmt.Printf("Your conversations are not touched, and the places stay on the list.\n")

	if !apply {
		fmt.Println("\nNothing has been changed. Add --apply to do it.")

		return nil
	}

	kept, err := backupEverything(db, root)
	if err != nil {
		return fmt.Errorf("writing the backup, so nothing was deleted: %w", err)
	}

	fmt.Printf("\nWritten to %s\n", kept)

	goneFacts, goneLessons, err := db.ForgetEverythingLearned()
	if err != nil {
		return err
	}

	if err := places.StartReadingAgain(root.Path); err != nil {
		return fmt.Errorf("after emptying the memory, resetting the places: %w", err)
	}

	fmt.Printf("Forgotten %d memories and %d records of files read.\n", goneFacts, goneLessons)
	fmt.Println("It will start reading again a few minutes after the next launch.")

	return nil
}

// backupEverything writes the facts and the lessons out before they go.
func backupEverything(db *store.DB, root paths.Root) (string, error) {
	name := filepath.Join(root.Path,
		"everything-"+time.Now().Format("2006-01-02-150405")+".json")

	f, err := os.Create(name)
	if err != nil {
		return "", err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")

	facts, err := db.AllFacts()
	if err != nil {
		return "", err
	}

	for _, fact := range facts {
		if err := enc.Encode(fact); err != nil {
			return "", err
		}
	}

	// Every status, not only the proposed ones: this is the record of what has
	// been read, and it is about to stop existing.
	for _, status := range []string{"proposed", "validated", "promoted", "rejected"} {
		var after int64

		for {
			batch, err := db.LessonsAfter(status, after, 1000)
			if err != nil {
				return "", err
			}

			if len(batch) == 0 {
				break
			}

			for _, l := range batch {
				after = l.ID

				if err := enc.Encode(l); err != nil {
					return "", err
				}
			}
		}
	}

	return name, f.Sync()
}
