package brain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"pn-scripts-assistant/internal/brain/occupations"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/preflight"
)

/*
 * Reading a job classification into this brain.
 *
 * Two ways in, one way through. Somebody points the Organisation view at a
 * folder or a zip; or setup has downloaded a classification into this
 * machine's folder for them, and the brain reads it in the next time it opens
 * or the next time Organisation is looked at. Either way it is a background
 * job with a row that says how far it got, and either way it can be stopped
 * and started again.
 */

// importing is which job is doing which import, so Stop reaches it.
var importing sync.Map

// Import starts reading a classification in, and says which run it is.
func (b *Brain) Import(source, from string) (int64, error) {
	run := map[string]func(context.Context, *store.DB, *occupations.Source, func(occupations.Progress)) (occupations.Progress, error){
		occupations.ESCO: occupations.ImportESCO,
		occupations.ONET: occupations.ImportONET,
	}[strings.ToLower(strings.TrimSpace(source))]

	if run == nil {
		return 0, fmt.Errorf("that is esco or onet")
	}

	// Opened here as well as in the work, so a wrong path is said at once
	// rather than as a failed import a minute later.
	src, err := occupations.OpenSource(from)
	if err != nil {
		return 0, err
	}

	id, err := b.DB.StartImport(source, from)
	if err != nil {
		src.Close()

		return 0, err
	}

	job, err := b.Jobs.StartSilent("importing "+source, func(ctx context.Context) (string, error) {
		defer src.Close()
		defer importing.Delete(id)

		p, err := run(ctx, b.DB, src, func(p occupations.Progress) {
			b.DB.ImportProgress(id, p.Stage, p.Done, p.Of, p.So, p.Notes)
		})

		b.DB.ImportProgress(id, p.Stage, p.Done, p.Of, p.So, p.Notes)

		switch {
		case ctx.Err() != nil:
			b.DB.FinishImport(id, store.ImportStopped, "you stopped it")
		case err != nil:
			b.DB.FinishImport(id, store.ImportFailed, err.Error())
		default:
			b.DB.FinishImport(id, store.ImportDone, "")
			b.Log.Info("read in a job classification", "source", source,
				"jobs", p.So.Jobs, "capabilities", p.So.Capabilities, "links", p.So.Links)
		}

		return "", err
	})
	if err != nil {
		src.Close()
		b.DB.FinishImport(id, store.ImportFailed, err.Error())

		return 0, err
	}

	importing.Store(id, job.ID)

	return id, nil
}

// StopImport stops a run that is going.
func (b *Brain) StopImport(id int64) error {
	job, running := importing.Load(id)
	if !running {
		return fmt.Errorf("that import is not running")
	}

	return b.Jobs.Stop(job.(int64))
}

/*
 * ImportDownloaded reads in whatever setup downloaded that this brain has not
 * read yet.
 *
 * "Not read yet" is: no run of that source from that folder finished after the
 * newest file in it was written. So a brain reads a download once, reads it
 * again when setup fetches a newer release, and a second brain on the same
 * machine reads the same files into its own catalogue — which is right,
 * because a brain carried here on a drive has not seen them.
 *
 * Not when a run from that folder is already going, and not again after one
 * failed until the files change: a release that will not import should say so
 * once, in the view, not every time the view is opened.
 */
func (b *Brain) ImportDownloaded() []int64 {
	started := []int64{}

	runs, err := b.DB.Imports(50)
	if err != nil {
		return started
	}

	for _, source := range []string{occupations.ESCO, occupations.ONET} {
		folder := preflight.CatalogueFolder(source)

		zips, _ := filepath.Glob(filepath.Join(folder, "*.zip"))
		if len(zips) == 0 {
			continue
		}

		newest := int64(0)

		for _, path := range zips {
			if info, err := os.Stat(path); err == nil && info.ModTime().Unix() > newest {
				newest = info.ModTime().Unix()
			}
		}

		seen := false

		for _, run := range runs {
			if run.Source != source || run.From != folder {
				continue
			}

			if run.State == store.ImportRunning || run.Started.Unix() >= newest {
				seen = true

				break
			}
		}

		if seen {
			continue
		}

		if id, err := b.Import(source, folder); err == nil {
			started = append(started, id)
		} else {
			b.Log.Warn("could not read in a downloaded classification", "source", source, "error", err)
		}
	}

	return started
}
