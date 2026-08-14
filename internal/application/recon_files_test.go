package application

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "recon_all_all_2026-08-14.csv"), []byte("a,b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewReconcileService(nil, nil, nil, nil).WithOutput(dir)
	list, err := s.ListFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "recon_all_all_2026-08-14.csv" {
		t.Fatalf("got %+v", list)
	}
}

func TestListFilesEmptyDir(t *testing.T) {
	s := NewReconcileService(nil, nil, nil, nil)
	list, err := s.ListFiles()
	if err != nil || len(list) != 0 {
		t.Fatalf("empty outDir want [], got %+v err=%v", list, err)
	}
}
