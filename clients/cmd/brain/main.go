// Command brain is PN Brain itself.
//
// One binary, one file on disk for the data, and nothing else running. There is
// no database server to install, no container runtime, no queue daemon and no
// PHP. That is the point of it: the previous shape needed Docker with Postgres,
// pgvector and Redis beside it, which meant the assistant could not start on a
// machine that did not already have a container stack configured.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"context"
	"log/slog"
	"os/signal"
	"strings"
	"syscall"

	"pn-brain/internal/brain/brain"
	"pn-brain/internal/brain/config"
	"pn-brain/internal/brain/paths"
	"pn-brain/internal/brain/server"
	"pn-brain/internal/brain/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error

	switch os.Args[1] {
	case "serve":
		err = runServe(os.Args[2:])
	case "rewrite-paths":
		err = runRewritePaths(os.Args[2:])
	case "promote":
		err = runPromote(os.Args[2:])
	case "import":
		err = runImport(os.Args[2:])
	case "status":
		err = runStatus(os.Args[2:])
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

func usage() {
	fmt.Fprint(os.Stderr, `PN Brain

  brain serve               run the brain and serve its interface
  brain status              where the data lives and what is in it
  brain promote             turn validated lessons into durable knowledge
  brain import <dir>        load a Postgres export into a fresh database
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

func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	fs.Parse(args)

	if fs.NArg() != 1 {
		return errors.New("give the directory holding the exported .json files")
	}

	db, root, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	fmt.Printf("  data root : %s\n", root.Path)
	fmt.Printf("  database  : %s\n\n", root.DatabasePath())

	rep, err := db.ImportPostgresExport(fs.Arg(0))
	if err != nil {
		return err
	}

	fmt.Printf("  conversations   %d\n", rep.Conversations)
	fmt.Printf("  messages        %d\n", rep.Messages)
	fmt.Printf("  lessons         %d\n", rep.Lessons)
	fmt.Printf("  facts           %d\n", rep.Facts)
	fmt.Printf("  tool calls      %d\n", rep.Invocations)

	for _, s := range rep.Skipped {
		fmt.Printf("  ! %s\n", s)
	}

	return nil
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

	// The strongest link is a cheap equivalence check against the Postgres
	// version this replaced: same vectors and a correct cosine must reproduce
	// the same pair and the same score.
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

	b := brain.New(db, cfg, root.Path, root.DatabasePath(), logger)
	srv := server.New(b, logger)

	b.Start(ctx)
	defer b.Stop()

	ln, err := server.Listen(cfg.Addr)
	if err != nil {
		return err
	}

	facts, _ := db.CountFacts()

	fmt.Printf("\n  %s\n", cfg.Name)
	fmt.Printf("  %d facts  ·  privacy: %s  ·  %s\n", facts, b.Mode, cfg.OllamaModel)
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
