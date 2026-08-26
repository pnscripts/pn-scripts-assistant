package server

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
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

		files.ServeHTTP(w, r)
	})
}

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
