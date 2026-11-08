package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/machinecare"
)

// The look answers without a model or the network, and a process that is
// not on its list cannot be stopped through it.
func TestTheMachineCareCard(t *testing.T) {
	ts, _, _ := newServer(t)

	var got struct {
		Restart   machinecare.Readiness  `json:"restart"`
		Leftovers []machinecare.Leftover `json:"leftovers"`
		LastStop  machinecare.LastStop   `json:"last_stop"`
	}

	getJSON(t, ts.URL+"/api/machine/care", &got)

	if got.Leftovers == nil || got.Restart.Items == nil {
		t.Fatalf("%+v", got)
	}

	for _, pid := range []int{0, 1, os.Getpid()} {
		if code := sendJSON(t, ts.URL+"/api/machine/care/stop", map[string]any{"pid": pid, "command": "x"}, nil); code == http.StatusOK {
			t.Errorf("pid %d was stopped", pid)
		}
	}
}

// Every item, kind and reason the card shows is said in both languages.
func TestTheMachineCareCardIsSaidInBothLanguages(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("assets", "js", "i18n.js"))
	if err != nil {
		t.Fatal(err)
	}

	keys := []string{"mcare.why.ours", "mcare.why.orphan", "mcare.unclean", "mcare.ready", "mcare.notReady", "mcare.unknown"}

	for _, k := range []string{"kernel_waiting", "dkms_missing", "reboot_wanted", "installing", "busy", "uncommitted", "unpushed", "repo_unchecked"} {
		keys = append(keys, "mcare.r."+k)
	}

	for _, k := range machinecare.Kinds() {
		keys = append(keys, "mcare.k."+strings.ReplaceAll(k, " ", "_"))
	}

	for _, key := range keys {
		if !strings.Contains(string(js), "'"+key+"': [") {
			t.Errorf("%s is not in i18n.js", key)
		}
	}
}

/*
 * No string is defined twice: in one object literal the later one wins
 * silently, and the page whose words were defined first shows somebody
 * else's. Found when the machine-care card's "care.title" replaced the
 * customer-care page's.
 */
func TestNoStringIsDefinedTwice(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("assets", "js", "i18n.js"))
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}

	for _, line := range strings.Split(string(js), "\n") {
		trimmed := strings.TrimSpace(line)

		if !strings.HasPrefix(line, "        ") || strings.HasPrefix(line, "         ") {
			continue
		}

		i := strings.Index(trimmed, ": [")
		if i <= 0 {
			continue
		}

		key := strings.Trim(trimmed[:i], "'")
		if seen[key] {
			t.Errorf("%s is defined twice", key)
		}

		seen[key] = true
	}

	if len(seen) < 500 {
		t.Fatalf("only %d strings read: the check is not reading the file", len(seen))
	}
}
