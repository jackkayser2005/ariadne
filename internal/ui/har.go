package ui

import (
	"github.com/jackkayser2005/ariadne/internal/browser"
	"net/http"
)

func (h handler) handleHAR(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	if h.harPath == "" || h.harOrigin == "" {
		http.NotFound(w, r)
		return
	}
	readReport := browser.ReadHARReport
	download := r.URL.Path == "/capture-report"
	if download {
		readReport = browser.ExportHARReport
	}
	report, err := readReport(h.harPath, h.harOrigin, h.harRulesPath)
	if err != nil {
		http.Error(w, "capture review unavailable", http.StatusUnprocessableEntity)
		return
	}
	if download {
		w.Header().Set("Content-Disposition", `attachment; filename="ariadne-capture.html"`)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(report)
}
