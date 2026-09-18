package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"

	"pn-scripts-assistant/internal/brain/org"
	"pn-scripts-assistant/internal/brain/team"
)

func postBody(t *testing.T, url, body string) (int, map[string]any) {
	t.Helper()

	res, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)

	out := map[string]any{}
	json.Unmarshal(raw, &out)

	return res.StatusCode, out
}

/*
 * The Organisation view takes nobody on in one click any more. An audit made
 * a permanent agent there with every tool from a name and a sentence; now
 * every button that means "somebody new" makes a proposal, nothing is written
 * until it is confirmed, and what is written is narrowed.
 */
func TestTheOrganisationViewProposesRatherThanHires(t *testing.T) {
	ts, db, b := newServer(t)

	status, out := postBody(t, ts.URL+"/api/organisation/agents", `{"name":"pat","job":"software.game_developer"}`)
	if status != http.StatusOK || out["id"] == nil || out["kind"] != "hire" {
		t.Fatalf("taking somebody on did not make a proposal: %d %v", status, out)
	}

	team.Forget()

	if _, found := team.Find(team.Roster(b.Root), "pat"); found {
		t.Fatal("pat was hired before anybody confirmed it")
	}

	if status, out := postBody(t, ts.URL+"/api/organisation/agents", `{"name":"blank","for":"does whatever is asked"}`); status == http.StatusOK {
		t.Errorf("somebody with no job was taken on: %v", out)
	}

	id := strconv.FormatFloat(out["id"].(float64), 'f', 0, 64)

	if status, said := postBody(t, ts.URL+"/api/organisation/hire/"+id+"/confirm", `{"permanence":"permanent"}`); status != http.StatusOK {
		t.Fatalf("confirming failed: %d %v", status, said)
	}

	team.Forget()

	pat, found := team.Find(team.Roster(b.Root), "pat")
	if !found {
		t.Fatal("confirmed, and still nobody called pat")
	}

	fit := team.Settle(pat, org.Chart(b.Root), db, b.Agent.Registry)
	if only, _ := fit.Only(); only == nil {
		t.Errorf("the hire may use every tool: %+v", pat.Tools)
	}

	if status, out := postBody(t, ts.URL+"/api/organisation/agents", `{"name":"pat","tools":[]}`); status == http.StatusOK {
		t.Errorf("an emptied tool list — every tool — was saved: %v", out)
	}
}
