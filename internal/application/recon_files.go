package application

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/davveo/ledger-hub/internal/domain"
)

type ReconcileFile struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

func (s *ReconcileService) ListFiles() ([]ReconcileFile, error) {
	if s == nil || s.outDir == "" {
		return []ReconcileFile{}, nil
	}
	entries, err := os.ReadDir(s.outDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ReconcileFile{}, nil
		}
		return nil, err
	}
	out := make([]ReconcileFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, ReconcileFile{
			Name:    e.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func (s *ReconcileService) writeReconcileFiles(date, sys, asset string, files map[string]string) (map[string]string, error) {
	if s == nil || s.outDir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(s.outDir, 0o755); err != nil {
		return nil, err
	}
	names := map[string]string{
		"recon":           "recon_" + sys + "_" + asset + "_" + date + ".csv",
		"diff":            "diff_" + sys + "_" + asset + "_" + date + ".csv",
		"balance_tie_out": "balance_tie_out_" + sys + "_" + asset + "_" + date + ".csv",
		"fx_journal":      "fx_journal_" + sys + "_" + asset + "_" + date + ".csv",
	}
	paths := map[string]string{}
	for key, name := range names {
		body, ok := files[key]
		if !ok {
			continue
		}
		p := filepath.Join(s.outDir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return paths, err
		}
		paths[key] = p
	}
	return paths, nil
}

func (s *ReconcileService) FilePath(name string) (string, error) {
	if s == nil || s.outDir == "" {
		return "", domain.ErrNotFound
	}
	base := filepath.Base(name)
	if base != name || strings.Contains(base, "..") || base == "." || base == "/" {
		return "", domain.ErrInvalidParam
	}
	p := filepath.Join(s.outDir, base)
	if _, err := os.Stat(p); err != nil {
		return "", domain.ErrNotFound
	}
	return p, nil
}
