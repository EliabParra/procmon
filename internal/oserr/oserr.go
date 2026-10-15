// Package oserr gives the read door (procfs) and the act door (control) one
// shared vocabulary for the two failures a process monitor must expect:
// the process exited, or we are not allowed to touch it.
package oserr

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"
)

var (
	// ErrProcessGone means the process (or thread) no longer exists.
	ErrProcessGone = errors.New("process no longer exists")
	// ErrPermission means the kernel refused the operation for this user.
	ErrPermission = errors.New("permission denied")
)

// Classify maps an OS error to ErrProcessGone or ErrPermission when it matches.
// The original error stays in the chain, so errors.Is works for both the
// sentinel and the raw cause (for example syscall.ESRCH). A nil error stays
// nil and anything else passes through unchanged.
func Classify(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrProcessGone), errors.Is(err, ErrPermission):
		return err // already classified
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ESRCH):
		// /proc/<pid> disappears -> ENOENT; kill(2) on a dead pid -> ESRCH.
		return fmt.Errorf("%w: %w", ErrProcessGone, err)
	case errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.EPERM), errors.Is(err, syscall.EACCES):
		return fmt.Errorf("%w: %w", ErrPermission, err)
	default:
		return err
	}
}
