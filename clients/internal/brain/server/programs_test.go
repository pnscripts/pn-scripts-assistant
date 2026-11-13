package server

import (
	"net/http"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/programs"
)

// The inventory is read without a model or the network, present and
// removed programs alike, and narrowed by kind.
func TestTheProgramInventoryIsServed(t *testing.T) {
	ts, db, _ := newServer(t)

	now := time.Now().UTC()

	save, _ := programs.Reconcile(programs.Deb, nil, []programs.Program{
		{Kind: programs.Deb, ID: "gimp", Name: "gimp", Version: "2.10"},
		{Kind: programs.Deb, ID: "krita", Name: "krita", Version: "5.2"},
	}, now)
	if err := db.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	known, _ := db.Programs(programs.Deb)
	save, _ = programs.Reconcile(programs.Deb, known, []programs.Program{{Kind: programs.Deb, ID: "gimp", Name: "gimp", Version: "2.10"}}, now)
	db.SavePrograms(save)

	apps, _ := programs.Reconcile(programs.App, nil, []programs.Program{{Kind: programs.App, ID: "gimp.desktop", Name: "GIMP", Exec: "gimp"}}, now)
	db.SavePrograms(apps)

	if err := db.SaveLook(programs.Snap, nil, now); err != nil {
		t.Fatal(err)
	}

	var all struct {
		Programs   []programs.Record           `json:"programs"`
		Sources    []programs.SourceState      `json:"sources"`
		FirstLooks map[programs.Kind]time.Time `json:"first_looks"`
	}

	getJSON(t, ts.URL+"/api/programs", &all)

	if len(all.Programs) != 3 || all.Sources == nil {
		t.Fatalf("%+v", all)
	}

	for _, r := range all.Programs {
		if r.ID == "krita" && (r.Present || r.RemovedAt == nil || r.LastChange != programs.Removed) {
			t.Errorf("removed krita %+v", r)
		}
	}

	// The page says "installed since" from each source's first look.
	if len(all.FirstLooks) != 1 || !all.FirstLooks[programs.Snap].Equal(now) {
		t.Errorf("first looks %v", all.FirstLooks)
	}

	var debs struct {
		Programs []programs.Record `json:"programs"`
	}

	getJSON(t, ts.URL+"/api/programs?kind=app", &debs)

	if len(debs.Programs) != 1 || debs.Programs[0].Name != "GIMP" {
		t.Errorf("by kind %+v", debs)
	}

	resp, err := http.Get(ts.URL + "/api/programs?kind=rpm")
	if err != nil {
		t.Fatal(err)
	}

	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an unknown kind: %d", resp.StatusCode)
	}
}
