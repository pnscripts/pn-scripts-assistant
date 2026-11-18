package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/mcpserve"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/programs"
)

/*
 * The tools other programs are offered ("pn-scripts-assistant mcp"), held
 * to the real routes.
 *
 * mcpserve asks this server over HTTP and has its own tests against a
 * stand-in. A stand-in agrees with whoever wrote it: the terminal client
 * asked a route this server never had for months and nothing failed. So
 * here the same tools ask the real handler.
 */

func asked(t *testing.T, address, tool, arguments string) (string, error) {
	t.Helper()

	for _, each := range mcpserve.New(address, "test").Tools {
		if each.Name == tool {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			return each.Run(ctx, json.RawMessage(arguments), mcpserve.Caller{Name: "a test"})
		}
	}

	t.Fatalf("no tool %s", tool)

	return "", nil
}

func TestOtherProgramsAreToldWhichProgramsAreHere(t *testing.T) {
	ts, db, b := newServer(t)

	now := time.Now().UTC()

	save, _ := programs.Reconcile(programs.Deb, nil, []programs.Program{
		{Kind: programs.Deb, ID: "gimp", Name: "gimp", Version: "2.10"},
		{Kind: programs.Deb, ID: "krita", Name: "krita", Version: "5.2"},
	}, now)
	if err := db.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	// krita is removed.
	known, _ := db.Programs(programs.Deb)

	save, _ = programs.Reconcile(programs.Deb, known, []programs.Program{{Kind: programs.Deb, ID: "gimp", Name: "gimp", Version: "2.10"}}, now)
	if err := db.SavePrograms(save); err != nil {
		t.Fatal(err)
	}

	apps, _ := programs.Reconcile(programs.App, nil, []programs.Program{{Kind: programs.App, ID: "gimp.desktop", Name: "GIMP", Exec: "gimp"}}, now)
	if err := db.SavePrograms(apps); err != nil {
		t.Fatal(err)
	}

	// Its user allowed other programs to ask which programs are installed
	// (off until then: server/outside_test.go).
	if err := b.AllowOutside(otherprograms.Programs, true); err != nil {
		t.Fatal(err)
	}

	text, err := asked(t, ts.URL, "list_programs", `{}`)
	if err != nil {
		t.Fatal(err)
	}

	want := "Programs on this computer: 2 installed, 1 removed and remembered. Showing 2 installed.\n" +
		"- GIMP (app, gimp.desktop)\n" +
		"- gimp 2.10 (deb)"

	if text != want {
		t.Errorf("got:\n%s\nwant:\n%s", text, want)
	}

	text, err = asked(t, ts.URL, "list_programs", `{"search":"krita","include_removed":true,"kind":"deb"}`)
	if err != nil {
		t.Fatal(err)
	}

	if want := "- krita 5.2 (deb), removed " + now.Format("2006-01-02"); !strings.HasSuffix(text, want) {
		t.Errorf("the removed one:\n%s\nwant it to end:\n%s", text, want)
	}

	// The server's own refusal of a kind it does not know, in its words.
	if _, err := asked(t, ts.URL, "list_programs", `{"kind":"rpm"}`); err == nil || !strings.Contains(err.Error(), "kind is app, deb") {
		t.Errorf("an unknown kind: %v", err)
	}

	text, err = asked(t, ts.URL, "assistant_status", `{}`)
	if err != nil || !strings.HasPrefix(text, "PN Scripts Assistant ") || !strings.Contains(text, "is running on this computer") ||
		!strings.HasSuffix(text, "Its terms of use are accepted, so it answers.") {
		t.Errorf("status: %q %v", text, err)
	}
}

// Until the terms are accepted nothing is told but that: the door the
// program keeps for everyone is kept for other programs too.
func TestOtherProgramsAreToldNothingBeforeTheTerms(t *testing.T) {
	ts, _ := notAgreed(t)

	text, err := asked(t, ts.URL, "list_programs", `{}`)
	if err == nil || text != "" || !strings.Contains(err.Error(), "terms of use have not been accepted") {
		t.Errorf("the list before the terms: %q %v", text, err)
	}

	text, err = asked(t, ts.URL, "assistant_status", `{}`)
	if err != nil || !strings.Contains(text, "is running on this computer") || !strings.Contains(text, "have not been accepted yet") {
		t.Errorf("status before the terms: %q %v", text, err)
	}
}

func TestOtherProgramsLookWhereTheProgramServes(t *testing.T) {
	if want := "http://" + config.Default().Addr; mcpserve.DefaultAddress != want {
		t.Errorf("other programs look at %s, the program serves on %s", mcpserve.DefaultAddress, want)
	}

	if _, err := mcpserve.Loopback(mcpserve.DefaultAddress); err != nil {
		t.Error(err)
	}
}
