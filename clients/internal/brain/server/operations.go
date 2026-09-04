package server

import (
	"net/http"
	"time"

	"pn-brain/internal/brain/machine"
	"pn-brain/internal/brain/progress"
	"pn-brain/internal/brain/storage"
)

/*
 * The operations view: what the machine is doing, in detail.
 *
 * The dials on the command centre answer "is it working". This answers the
 * questions somebody has while watching it work — which core is pegged, how
 * much of the card's memory the model took, what is running, how long it has
 * been up. It is the same information a person would get from four terminals
 * running nvidia-smi, htop, nvtop and a service watch, which is exactly how
 * this machine's owner watches his other one.
 *
 * Read from the watcher rather than measured here. Every rate in it — processor
 * time, network, disk — is the difference between two readings, so a reading
 * taken out of turn eats the comparison the next scheduled one needed and the
 * whole panel reports a machine busier or quieter than it is, every other
 * second.
 */
func (s *Server) handleOperations(w http.ResponseWriter, r *http.Request) {
	now, ok := machine.Latest()

	if !ok {
		ok200(w, map[string]any{
			"available": false,
			"why": "This machine does not publish the readings the operations view is " +
				"made of. They come from the Linux kernel's /proc, which no other " +
				"system provides in the same form.",
		})

		return
	}

	/*
	 * And the disks, which are not something the kernel's /proc describes.
	 *
	 * Part of the same question — what this machine has — and the only part of
	 * it that already had a reader, since the brain has to know where it can
	 * live.
	 */
	disks, _ := storage.Drives(s.brain.Root)

	ok200(w, map[string]any{
		"available": now.Available,
		"machine":   now,
		"disks":     disks,
		"history":   machine.History(),
		// How far back the graphs go and how often the readings are taken, so
		// the axis is labelled from the truth rather than from a guess in the
		// page.
		"seconds_each": int(machine.HowOften / time.Second),
		"seconds_back": int(machine.HowOften/time.Second) * machine.HowMany,

		// And what the brain itself is doing, which is the reason anybody is
		// watching this machine at all. The analogue of the service watch in
		// the corner of an operations screen.
		"brain": map[string]any{
			"model":      workModel(s.brain),
			"models":     modelRoles(s.brain),
			"providers":  s.brain.Router.Availabilities(r.Context()),
			"step":       progress.Now(),
			"recent":     progress.Recent(),
			"storage":    s.brain.Storage(),
			"root":       s.brain.Root,
			"privacy":    s.brain.Mode.Describe(),
			"capability": s.brain.Capabilities(),
		},
	})
}

// ok200 is ok, named apart so this file reads without a variable called ok
// shadowing it in every handler that also asks a question.
func ok200(w http.ResponseWriter, body any) { ok(w, body) }
