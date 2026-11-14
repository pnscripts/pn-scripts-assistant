package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/tools"
)

// importedFake is the fake server as if found in Claude Desktop's settings.
func importedFake(t *testing.T) Server {
	t.Helper()

	s, err := fakeCommand("modern")
	if err != nil {
		t.Fatal(err)
	}

	s.From = &Origin{Client: "claude-desktop", File: "/home/you/.config/Claude/claude_desktop_config.json", Name: "fake", Print: "p1"}

	return s
}

/*
 * The whole life of an imported server against a real process: imported
 * (not approved), approved and started; its entry removed in the other
 * program, so it stops and its tools are gone at once and it cannot be
 * started; back unchanged, so it runs again by itself; changed, so it stops
 * and is asked about again.
 */
func TestAnImportedServerFollowsItsEntry(t *testing.T) {
	g, _ := gateway(t)
	ctx := context.Background()

	s := importedFake(t)

	if err := g.Import(s, map[string]string{"FAKE_KEY": "k1"}); err != nil {
		t.Fatal(err)
	}

	if err := g.Import(s, nil); err == nil {
		t.Error("the same id was imported twice")
	}

	got, _ := g.Book.Get("fake")
	if got.Approved || got.Active || got.From == nil || got.From.Client != "claude-desktop" || !g.Book.HasSecret("fake", "FAKE_KEY") {
		t.Fatalf("imported as %+v", got)
	}

	if _, err := g.Activate(ctx, "fake"); err == nil {
		t.Fatal("an imported server nobody approved was started")
	}

	if _, err := g.Approve("fake", nil); err != nil {
		t.Fatal(err)
	}

	if _, err := g.Activate(ctx, "fake"); err != nil {
		t.Fatal(err)
	}

	offered, ok := g.Registry.Get("mcp_fake_echo")
	if !ok {
		t.Fatal("its tool was not offered")
	}

	// The server's own name for it counts as naming it (agent.Choosing).
	if named, ok := offered.(tools.Named); !ok || len(named.OwnNames()) == 0 || named.OwnNames()[0] != "echo" {
		t.Errorf("the tool does not carry the server's name for it: %v", offered)
	}

	// Gone from the settings.
	if err := g.EntryGone("fake", time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	if _, ok := g.Registry.Get("mcp_fake_echo"); ok {
		t.Error("a gone server's tool is still offered")
	}

	if len(g.Running()) != 0 {
		t.Error("a gone server still runs")
	}

	got, _ = g.Book.Get("fake")
	if !got.Gone() || !got.Approved || got.Active || !got.From.WasActive || got.From.GoneSince != "2026-11-14T08:00:00Z" {
		t.Errorf("gone as %+v %+v", got, got.From)
	}

	if _, err := g.Activate(ctx, "fake"); err == nil || !strings.Contains(err.Error(), "no longer in the settings") {
		t.Errorf("a gone server could be started: %v", err)
	}

	// Everything allowed does not start a gone one either.
	g.Unbounded = func() bool { return true }
	if _, err := g.Activate(ctx, "fake"); err == nil {
		t.Error("everything allowed started a gone server")
	}

	g.Unbounded = nil

	// Resume after a restart does not start it.
	g.Resume(ctx)

	if len(g.Running()) != 0 {
		t.Error("resume started a gone server")
	}

	// Back, unchanged: running again with the approval it had.
	if err := g.EntryBack(ctx, "fake"); err != nil {
		t.Fatal(err)
	}

	if _, ok := g.Registry.Get("mcp_fake_echo"); !ok {
		t.Error("a server back unchanged was not started again")
	}

	got, _ = g.Book.Get("fake")
	if got.Gone() || !got.Approved || !got.Active || got.From.WasActive {
		t.Errorf("back as %+v %+v", got, got.From)
	}

	// A new key: kept, nothing else changes.
	if err := g.EntrySecrets("fake", map[string]string{"FAKE_KEY": "k2"}); err != nil {
		t.Fatal(err)
	}

	if g.SecretsOf("fake")["FAKE_KEY"] != "k2" {
		t.Error("the new key was not kept")
	}

	if _, ok := g.Registry.Get("mcp_fake_echo"); !ok {
		t.Error("a new key stopped the server")
	}

	// Changed: stopped, asked again, grants and read-only marks kept, and
	// a secret it no longer has is removed.
	if _, err := g.MarkReadOnly("fake", []string{"echo"}); err != nil {
		t.Fatal(err)
	}

	next := importedFake(t)
	next.Command = append(next.Command, "-test.v=false")
	next.Secrets = nil
	next.From.Print = "p2"

	if err := g.EntryChanged(next, nil); err != nil {
		t.Fatal(err)
	}

	got, _ = g.Book.Get("fake")
	if got.Approved || got.Active || len(got.Command) != 3 || got.From.Print != "p2" || len(got.ReadOnly) != 1 {
		t.Errorf("changed as %+v", got)
	}

	if _, ok := g.Registry.Get("mcp_fake_echo"); ok {
		t.Error("a changed server still runs as approved before")
	}

	if g.Book.HasSecret("fake", "FAKE_KEY") {
		t.Error("a secret the entry no longer has was kept")
	}

	// Following is only for imported servers.
	own, _ := fakeCommand("modern")
	own.ID = "own"

	if err := g.AddOwn(own); err != nil {
		t.Fatal(err)
	}

	if g.EntryGone("own", time.Now()) == nil || g.EntryBack(ctx, "own") == nil || g.SecretsOf("own") != nil {
		t.Error("one added by hand was followed")
	}
}

// One added by hand cannot claim to have come from somewhere.
func TestAnOwnServerCannotPretendToBeImported(t *testing.T) {
	g, _ := gateway(t)

	s := importedFake(t)

	if err := g.AddOwn(s); err != nil {
		t.Fatal(err)
	}

	if got, _ := g.Book.Get("fake"); got.From != nil {
		t.Errorf("an own server kept an origin: %+v", got.From)
	}

	s.From = nil
	s.ID = "other"

	if err := g.Import(s, nil); err == nil {
		t.Error("a server with no origin was imported")
	}
}
