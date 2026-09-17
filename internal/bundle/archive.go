package bundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ArchiveEntry is the safe summary of one verified child directory.
type ArchiveEntry struct {
	Directory    string `json:"directory"`
	ManifestName string `json:"manifest_name"`
	Differences  int    `json:"differences"`
	Unknowns     int    `json:"unknowns"`
}

// Index verifies each immediate child directory under archiveRoot.
func Index(archiveRoot string) ([]ArchiveEntry, error) {
	if strings.TrimSpace(archiveRoot) == "" {
		return nil, errors.New("archive root is required")
	}
	rootInfo, err := lstatNoSymlinkPath(archiveRoot)
	if err != nil {
		return nil, fmt.Errorf("archive root: %w", err)
	}
	if !rootInfo.IsDir() {
		return nil, errors.New("archive root is not a directory")
	}

	entries, err := os.ReadDir(archiveRoot)
	if err != nil {
		return nil, fmt.Errorf("read archive root: %w", err)
	}
	indexed := make([]ArchiveEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !validArchiveEntryName(name) {
			return nil, errors.New("archive entry name is invalid")
		}
		entryPath := filepath.Join(archiveRoot, name)
		entryInfo, err := lstatNoSymlinkPath(entryPath)
		if err != nil {
			return nil, fmt.Errorf("archive entry %q: %w", name, err)
		}
		if !entryInfo.IsDir() {
			return nil, fmt.Errorf("archive entry %q is not a directory", name)
		}

		summary, err := Verify(entryPath)
		if err != nil {
			return nil, fmt.Errorf("archive entry %q: %w", name, err)
		}
		indexed = append(indexed, ArchiveEntry{
			Directory:    name,
			ManifestName: summary.ManifestName,
			Differences:  summary.Differences,
			Unknowns:     summary.Unknowns,
		})
	}
	return indexed, nil
}

func validArchiveEntryName(name string) bool {
	return name != "" &&
		name != "." &&
		name != ".." &&
		len(name) <= 255 &&
		utf8.ValidString(name) &&
		!strings.ContainsAny(name, `/\\`) &&
		!strings.ContainsFunc(name, unicode.IsControl)
}
