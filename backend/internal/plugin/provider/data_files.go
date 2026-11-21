package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TokenFlux/TokenRouter/plugin-sdk/go/manifest"
)

// data_files.go implements plugin.DataStorage with one host directory per
// plugin beneath a shared root. Directories are created on first use and
// retained until an explicit removal, keeping plugin business files stable
// across package upgrades, process restarts, and ordinary uninstallation.

// DataFiles owns the root directory holding every plugin's persistent files.
type DataFiles struct {
	root string // absolute root containing one subdirectory per plugin ID
}

// NewDataFiles prepares durable storage rooted at directory.
// The directory is created with owner-only permissions when missing, and the
// returned storage serves absolute per-plugin paths beneath it.
func NewDataFiles(directory string) (*DataFiles, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, err
	}
	return &DataFiles{root: absolute}, nil
}

// Directory creates the persistent directory for one validated plugin ID and
// returns its absolute path. Existing contents are preserved untouched, and a
// pre-existing link or non-directory at the owner path is rejected so plugin
// files cannot land outside the storage root.
func (d *DataFiles) Directory(ctx context.Context, id string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path, err := d.ownerPath(id)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("plugin data path for %s is not a directory", id)
	}
	return path, nil
}

// Remove deletes one plugin's persistent directory, including every file the
// plugin stored there. Missing directories succeed so a repeated cleanup
// after an interrupted removal stays harmless.
func (d *DataFiles) Remove(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := d.ownerPath(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}

// ownerPath resolves one plugin ID to its directory beneath the storage root.
// IDs failing manifest validation are rejected before any path is built, so
// separators or parent references can never escape the root.
func (d *DataFiles) ownerPath(id string) (string, error) {
	if !manifest.ValidID(id) {
		return "", fmt.Errorf("invalid plugin ID %q", id)
	}
	return filepath.Join(d.root, id), nil
}
