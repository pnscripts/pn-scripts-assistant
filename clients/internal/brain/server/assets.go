package server

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The interface is compiled into the binary.
//
// This is what makes the finished program one file. Serving these from disk
// would mean the download is a folder, that the folder can be half-updated, and
// that a user can break the interface by moving a file. Embedded, the interface
// and the code that serves it are always the same version.
//
//go:embed assets
var assets embed.FS

// indexTemplate is the only page. It is a template rather than a static file
// solely so the brain's configured name reaches the title and the header
// without a round trip.
var indexTemplate = template.Must(template.ParseFS(assets, "assets/index.html"))

// assetHandler serves the interface.
//
// Every response carries a validator and no-cache, and the reason is a bug that
// wasted a lot of time. The index said no-cache; the stylesheet and the scripts
// said nothing at all. A browser given no instruction caches heuristically and
// indefinitely, so the window kept running a version of console.js from several
// builds earlier — the server was serving the new file and nobody was reading
// it. Interface changes appeared not to work, repeatedly, for reasons that had
// nothing to do with the changes.
//
// no-cache does not mean "do not store". It means revalidate before use, which
// with an ETag costs one 304 and guarantees the window is never running code
// older than the binary serving it.
func assetHandler(logger *slog.Logger) http.Handler {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		// Impossible unless the embed directive above is wrong, which is a
		// build-time mistake rather than a runtime condition.
		panic(err)
	}

	files := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			// Handled by the server so the name can be injected; see
			// Server.renderIndex.
			http.NotFound(w, r)

			return
		}

		if tag := assetETag(sub, r.URL.Path); tag != "" {
			w.Header().Set("ETag", tag)
		}

		w.Header().Set("Cache-Control", "no-cache")

		files.ServeHTTP(w, r)
	})
}

// assetETag identifies an asset by its contents.
//
// The files are compiled into the binary and cannot change while it runs, so
// the hash is computed once per path and kept.
func assetETag(files fs.FS, path string) string {
	name := strings.TrimPrefix(path, "/")

	etagOnce.Do(func() { etags = map[string]string{} })

	etagMu.RLock()
	tag, known := etags[name]
	etagMu.RUnlock()

	if known {
		return tag
	}

	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return ""
	}

	sum := sha256.Sum256(raw)
	tag = `"` + hex.EncodeToString(sum[:8]) + `"`

	etagMu.Lock()
	etags[name] = tag
	etagMu.Unlock()

	return tag
}

var (
	etags    map[string]string
	etagOnce sync.Once
	etagMu   sync.RWMutex
)

// renderIndex writes the page with the brain's name filled in.
func (s *Server) renderIndex(w http.ResponseWriter, r *http.Request) {
	name := s.brain.Cfg.Name
	if name == "" {
		name = "PN Brain"
	}

	var buf bytes.Buffer

	if err := indexTemplate.Execute(&buf, map[string]any{"Name": name}); err != nil {
		s.log.Error("rendering the interface failed", "error", err)
		http.Error(w, "The interface could not be rendered.", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The page is built into the binary and changes only when the program does,
	// but a stale cached copy after an update would look like a bug, so it is
	// revalidated rather than cached.
	w.Header().Set("Cache-Control", "no-cache")

	http.ServeContent(w, r, "index.html", buildTime(), bytes.NewReader(buf.Bytes()))
}

// buildTime is a stable timestamp for cache validation within one run.
var startedAt = time.Now()

func buildTime() time.Time { return startedAt }
