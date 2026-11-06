package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Record retains ownership even after Git has forgotten a deleted worktree.
type Record struct {
	Root         string `json:"root"`
	RepositoryID string `json:"repository_id"`
}

func (r Record) ID() string { return (base{root: r.Root}).ID() }

// RecordPath is launcher-owned metadata, not a path inside the workspace.
func RecordPath(stateDir, id string) string {
	return filepath.Join(stateDir, "opencode-sandbox", "workspaces", id+".json")
}

// Track atomically publishes one independent record per canonical workspace.
func Track(stateDir string, w Workspace) error {
	path := RecordPath(stateDir, w.ID())
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create workspace registry: %w", err)
	}
	data, err := json.Marshal(Record{Root: w.Root(), RepositoryID: w.RepositoryID()})
	if err != nil {
		return fmt.Errorf("encode workspace ownership: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".workspace-*")
	if err != nil {
		return fmt.Errorf("create workspace ownership: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }() //nolint:errcheck // Temporary file may already have been renamed.
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("write workspace ownership: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close workspace ownership: %w", closeErr)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("publish workspace ownership: %w", err)
	}
	return nil
}

// Records reads only validated records; no workspace needs to still exist.
func Records(stateDir string) ([]Record, error) {
	dir := filepath.Dir(RecordPath(stateDir, ""))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workspace registry: %w", err)
	}
	var records []Record
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read workspace ownership: %w", err)
		}
		var record Record
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("decode workspace ownership: %w", err)
		}
		if !filepath.IsAbs(record.Root) || filepath.Clean(record.Root) != record.Root || record.RepositoryID == "" || entry.Name() != record.ID()+".json" {
			return nil, fmt.Errorf("invalid workspace ownership record %q", entry.Name())
		}
		records = append(records, record)
	}
	return records, nil
}
