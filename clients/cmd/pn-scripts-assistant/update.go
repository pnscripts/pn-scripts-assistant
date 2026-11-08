package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/latest"
	"pn-scripts-assistant/internal/brain/release"
)

/*
 * runUpdate is `pn-scripts-assistant update`: the Updates page's button, for
 * a terminal or a machine nobody sits at.
 *
 * It asks GitHub once, says what is installed, what is newest and how this
 * copy is updated, and — only for the Windows installer, the one channel that
 * downloads — fetches the new setup, checks it against SHA256SUMS and starts
 * it quietly. It returns straight away: the installer cannot replace this
 * program while it is still running.
 */
func runUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	check := fs.Bool("check", false, "only say whether a newer version is out")
	fs.Parse(args)

	have := release.Version
	channel := latest.How(have)

	fmt.Printf("%s %s, installed as: %s\n", config.Product, release.Is(), channel)

	if channel == latest.Source {
		return errors.New("this copy was built by hand: update it from its checkout (git pull, then build)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	rel, err := latest.Newest(ctx, nil)
	if err != nil {
		return err
	}

	if !latest.Newer(have, rel.Version) {
		fmt.Printf("Up to date: the newest version is %s.\n", rel.Version)

		return nil
	}

	fmt.Printf("A newer version is out: %s (%s)\n", rel.Version, rel.Page)

	if *check {
		return nil
	}

	if channel == latest.Installer {
		dir := filepath.Join(os.TempDir(), "pn-scripts-assistant-update", rel.Version)

		if err := latest.InstallQuietly(ctx, rel, dir); err != nil {
			return err
		}

		fmt.Println("Downloaded, checked against SHA256SUMS, and the installer is running.")

		return nil
	}

	out, err := latest.Apply(ctx, channel, rel, "")
	if err != nil {
		return err
	}

	fmt.Println(out.Say)

	if out.Command != "" {
		fmt.Println("  " + out.Command)
	}

	if out.Open != "" {
		fmt.Println("  " + out.Open)
	}

	return nil
}
