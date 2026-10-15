// Package control is the act door of the monitor: it sends signals and
// changes priorities through system calls only. It never runs external
// programs such as kill or renice.
package control

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/EliabParra/procmon/internal/oserr"
)

var (
	// ErrInvalidPID rejects pid <= 0 before any system call: kill(0, sig)
	// signals the caller's whole process group and kill(-1, sig) every
	// process the caller may signal.
	ErrInvalidPID = errors.New("invalid pid")
	// ErrInvalidPriority rejects a nice value outside the kernel range.
	ErrInvalidPriority = errors.New("nice value must be between -20 and 19")
)

const (
	minNice = -20 // highest priority
	maxNice = 19  // lowest priority
)

// The three system calls, behind variables so tests can check exactly what
// would reach the kernel (and simulate EPERM) without touching a real process.
var (
	sysKill        = syscall.Kill
	sysGetpriority = syscall.Getpriority
	sysSetpriority = syscall.Setpriority
)

// Terminate asks the process to exit cleanly: kill(2) with SIGTERM.
// The process may handle or ignore it.
func Terminate(pid int) error { return signal(pid, syscall.SIGTERM) }

// Kill ends the process immediately: kill(2) with SIGKILL, which cannot be caught.
func Kill(pid int) error { return signal(pid, syscall.SIGKILL) }

// Suspend stops the process: kill(2) with SIGSTOP (state T in /proc).
func Suspend(pid int) error { return signal(pid, syscall.SIGSTOP) }

// Resume continues a stopped process: kill(2) with SIGCONT.
func Resume(pid int) error { return signal(pid, syscall.SIGCONT) }

func signal(pid int, sig syscall.Signal) error {
	if err := checkPID(pid); err != nil {
		return err
	}
	// ESRCH (no such process) becomes oserr.ErrProcessGone, EPERM becomes oserr.ErrPermission.
	return oserr.Classify(sysKill(pid, sig))
}

// Priority returns the nice value (-20..19) of pid through getpriority(2).
// On Linux the priority belongs to a single thread, so for a multi-threaded
// process this is the nice of its main thread, the one /proc/[pid]/stat shows.
func Priority(pid int) (int, error) {
	if err := checkPID(pid); err != nil {
		return 0, err
	}
	raw, err := sysGetpriority(syscall.PRIO_PROCESS, pid)
	if err != nil {
		return 0, oserr.Classify(err)
	}
	// The raw Linux syscall returns 20 - nice (1..40) so that -1 never looks
	// like an error. glibc undoes this; Go's syscall package does not.
	return 20 - raw, nil
}

// SetPriority changes the nice value of pid with setpriority(2). Raising the
// nice value (lower priority) is allowed for the owner; lowering it needs
// privilege and fails with oserr.ErrPermission. Like Priority, it applies to
// the thread whose id is pid, i.e. the main thread of a process.
func SetPriority(pid, nice int) error {
	if err := checkPID(pid); err != nil {
		return err
	}
	if nice < minNice || nice > maxNice {
		return fmt.Errorf("%w: got %d", ErrInvalidPriority, nice)
	}
	return oserr.Classify(sysSetpriority(syscall.PRIO_PROCESS, pid, nice))
}

func checkPID(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("%w: %d", ErrInvalidPID, pid)
	}
	return nil
}
