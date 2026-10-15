// Package procfs is the read door of the monitor: it parses the files the
// Linux kernel exposes under /proc into plain structs. It does no calculation
// (no percentages, no deltas) and never runs external programs.
package procfs

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/EliabParra/procmon/internal/oserr"
)

// defaultRoot is where the kernel mounts the process information pseudo-filesystem.
const defaultRoot = "/proc"

// FS reads process and system state below Root. Production code uses New();
// tests point Root at a fixture directory laid out like /proc.
type FS struct {
	Root string
}

// New returns an FS reading the real /proc.
func New() FS { return NewFS(defaultRoot) }

// NewFS returns an FS reading a /proc-shaped tree rooted at root.
func NewFS(root string) FS { return FS{Root: root} }

// PIDs lists the numeric directories of Root (one per live process), sorted.
func (f FS) PIDs() ([]int, error) {
	entries, err := os.ReadDir(f.Root)
	if err != nil {
		return nil, err
	}
	pids := make([]int, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if pid, ok := parsePID(e.Name()); ok {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)
	return pids, nil
}

// parsePID accepts only plain decimal digits, so "self", "sys", "+5" and
// "1234abc" are rejected.
func parsePID(name string) (int, bool) {
	if !isDigits(name) {
		return 0, false
	}
	pid, err := strconv.Atoi(name)
	return pid, err == nil && pid > 0
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// readPidFile reads Root/<pid>/<name...>. Errors go through oserr.Classify so
// callers can tell "the process exited" and "not allowed" from other failures.
func (f FS) readPidFile(pid int, name ...string) ([]byte, error) {
	parts := append([]string{f.Root, strconv.Itoa(pid)}, name...)
	data, err := os.ReadFile(filepath.Join(parts...))
	return data, oserr.Classify(err)
}
