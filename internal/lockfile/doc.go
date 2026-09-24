// Package lockfile provides a cross-process exclusive file lock used to
// serialise access to shared on-disk state — the task-ID counter
// (internal/core/taskid.go) and backlog.yaml (internal/storage/backlog.go).
//
// Lock blocks until the lock is available and returns a release function. The
// platform-specific implementations live in lock_unix.go (BSD flock, advisory)
// and lock_windows.go (LockFileEx, mandatory). Callers should serialise
// same-process access with an in-process mutex before taking the file lock, so
// a single process never contends with itself on the OS lock.
package lockfile
