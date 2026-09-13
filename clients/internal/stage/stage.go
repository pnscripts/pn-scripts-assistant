// Package stage is the three.js scene the interface stands in.
//
// Its own package because two pages stand in it: the program's, and setup's,
// which is served by a different server before the brain has started. Each
// carrying a copy would put three.js into the binary twice, at seven hundred
// kilobytes a copy, for the same picture.
//
// The files are compiled in for the reason the rest of the interface is: the
// program is one file, and a page and the code that serves it are never two
// different versions.
package stage

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"net/http"
	"strings"
)

//go:embed three.module.js stage.js
var files embed.FS

// Prefix is where both pages find the stage.
const Prefix = "/stage/"

// tags are the files' validators, worked out once: embedded files cannot
// change while the program runs.
var tags = func() map[string]string {
	out := map[string]string{}

	for _, name := range []string{"three.module.js", "stage.js"} {
		raw, err := files.ReadFile(name)
		if err != nil {
			// Impossible unless the embed directive above names a file that
			// is not there, which fails the build before it gets here.
			panic(err)
		}

		sum := sha256.Sum256(raw)
		out[name] = `"` + hex.EncodeToString(sum[:8]) + `"`
	}

	return out
}()

// Handler serves the stage under Prefix.
//
// Revalidated on every load rather than cached, for the reason the interface
// gives: a browser told nothing caches heuristically, and the window then runs
// a script from several builds ago while the server is serving the new one.
func Handler() http.Handler {
	server := http.FileServer(http.FS(files))

	return http.StripPrefix(strings.TrimSuffix(Prefix, "/"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		tag, known := tags[name]

		if !known {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("ETag", tag)
		w.Header().Set("Cache-Control", "no-cache")

		server.ServeHTTP(w, r)
	}))
}
