package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/internal/lockfile"
)

// acquireRegistryLock takes the cross-process exclusive lock guarding the YAML
// registry at registryPath. The lock lives on a dedicated sidecar file
// (registryPath + ".lock", the same convention backlog.go uses) rather than on
// the registry file itself, because a load-modify-save cycle opens the registry
// through separate handles — locking the data file would not span the whole
// cycle. It creates the parent directory and the lock file as needed and returns
// a release function the caller MUST defer.
//
// Callers must already hold their in-process mutex before calling this, so a
// single process never blocks on its own OS lock (mirroring backlog.go).
func acquireRegistryLock(registryPath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		return nil, fmt.Errorf("create registry directory: %w", err)
	}

	f, err := os.OpenFile(registryPath+".lock", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open registry lock file: %w", err)
	}

	unlock, err := lockfile.Lock(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquire registry file lock: %w", err)
	}

	return func() {
		unlock()
		_ = f.Close()
	}, nil
}
