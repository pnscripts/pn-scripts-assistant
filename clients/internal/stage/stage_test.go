package stage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

/*
 * Both files arrive as scripts a module may import.
 *
 * A browser refuses to run a module served under any other type, and refuses
 * silently as far as the page is concerned: the stage never starts and every
 * panel stays flat, which looks exactly like a machine without WebGL.
 */
func TestTheStageIsServedAsScripts(t *testing.T) {
	handler := Handler()

	for _, name := range []string{"stage.js", "three.module.js"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, Prefix+name, nil))

		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", name, w.Code)
		}

		if kind := w.Header().Get("Content-Type"); !strings.Contains(kind, "javascript") {
			t.Errorf("%s came as %q, which a module import refuses", name, kind)
		}

		if w.Header().Get("ETag") == "" || w.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s is not revalidated, so an old copy could outlive an update", name)
		}
	}
}

// Nothing else is reachable through it: not the Go source beside the scripts,
// and not a path that climbs out.
func TestTheStageServesNothingElse(t *testing.T) {
	handler := Handler()

	for _, path := range []string{Prefix + "stage.go", Prefix, Prefix + "../stage.js", Prefix + "x/stage.js"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))

		if w.Code == http.StatusOK {
			t.Errorf("%s was served", path)
		}
	}
}

// The scene imports three.js from beside itself, which is the same address the
// program's import map gives the core — so the page loads one copy, not two.
func TestTheStageImportsTheThreeBesideIt(t *testing.T) {
	raw, err := files.ReadFile("stage.js")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(raw), "from './three.module.js'") {
		t.Error("stage.js no longer imports ./three.module.js")
	}
}

// The revision written down is the one compiled in.
func TestTheRevisionIsTheOneCompiledIn(t *testing.T) {
	if !strings.Contains(string(Three()[:400]), `const t="`+ThreeRevision+`"`) {
		t.Errorf("three.js is not r%s", ThreeRevision)
	}
}
