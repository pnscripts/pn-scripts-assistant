package server

import (
	"net/http"
	"path/filepath"
	"regexp"
)

// reportName is the only shape of name a report is served under: no
// folders, no dots but the extension, so nothing else in the brain can be
// asked for through it.
var reportName = regexp.MustCompile(`^originality-audit-[0-9]{8}-[0-9]{6}(-[0-9]+)?\.(md|csv)$`)

// reportFile is where a requested report is, or "" when the name is not one.
func reportFile(root, name string) string {
	if !reportName.MatchString(name) {
		return ""
	}

	return filepath.Join(root, "reports", "originality", name)
}

// handleReport hands an originality report over as a file to keep.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	path := reportFile(s.brain.Root, name)
	if path == "" {
		fail(w, http.StatusNotFound, "there is no such report")

		return
	}

	kind := "text/markdown; charset=utf-8"
	if filepath.Ext(name) == ".csv" {
		kind = "text/csv; charset=utf-8"
	}

	w.Header().Set("Content-Type", kind)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, path)
}
