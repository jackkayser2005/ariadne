package ui

import (
	"github.com/jackkayser2005/ariadne/internal/browser"
	"net/http"
)

func (h handler) handleHARComparison(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	if h.harPath == "" || h.harSecondPath == "" || h.harOrigin == "" || h.harRulesPath == "" {
		http.NotFound(w, r)
		return
	}
	readReport := browser.ReadHARComparisonReport
	download := r.URL.Path == "/capture-comparison-report"
	if download {
		readReport = browser.ExportHARComparisonReport
	}
	report, err := readReport(h.harPath, h.harSecondPath, h.harOrigin, h.harRulesPath)
	if err != nil {
		http.Error(w, "capture comparison unavailable", http.StatusUnprocessableEntity)
		return
	}
	if download {
		w.Header().Set("Content-Disposition", `attachment; filename="ariadne-comparison.html"`)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(report)
}
