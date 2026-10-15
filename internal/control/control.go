// Package control is the act door of the monitor: it sends signals and
// changes priorities through system calls only. It never runs external
// programs such as kill or renice.
package control

import (
	"errors"
	"fmt"
	"syscall"
)

var (
	// ErrInvalidPID rejects pid <= 0 before any system call: kill(0, sig)
	// signals the caller's whole process group and kill(-1, sig) every
	// process the caller may signal.
	ErrInvalidPID = errors.New("invalid pid")
)

// The three system calls, behind variables so tests can check exactly what
// would reach the kernel (and simulate EPERM) without touching a real process.
var (
	sysKill = syscall.Kill
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
	return sysKill(pid, sig)
}

func checkPID(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("%w: %d", ErrInvalidPID, pid)
	}
	return nil
}
