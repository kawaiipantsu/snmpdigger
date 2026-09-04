package tui

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kawaiipantsu/snmpdigger/internal/config"
)

// exportDir is <config dir>/snmpdigger/exports, created on demand.
func exportDir() (string, error) {
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(d, "exports")
	if err := os.MkdirAll(p, 0o700); err != nil {
		return "", err
	}
	return p, nil
}

// exportCSV writes header + rows to <exports>/<kind>-<timestamp>.csv and returns
// the path.
func exportCSV(kind string, header []string, rows [][]string) (string, error) {
	dir, err := exportDir()
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s.csv", kind, time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, name)

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if len(header) > 0 {
		if err := w.Write(header); err != nil {
			return "", err
		}
	}
	if err := w.WriteAll(rows); err != nil {
		return "", err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return path, nil
}

// oneLineText collapses whitespace/newlines for single-line output.
func oneLineText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.Join(strings.Fields(s), " ")
}
