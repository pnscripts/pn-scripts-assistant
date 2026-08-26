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

	"pn-brain/internal/brain/paths"
	"pn-brain/internal/brain/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error

	switch os.Args[1] {
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

  brain status              where the data lives and what is in it
  brain import <dir>        load a Postgres export into a fresh database

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
