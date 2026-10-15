package control

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/EliabParra/procmon/internal/procfs"
)

// ---- seams: fake the three system calls so nothing real is touched ----

type kernelCalls struct {
	kills [][2]int // {pid, signal}
	err   error    // error every fake call returns
}

// fakeKernel replaces sysKill/sysGetpriority/sysSetpriority for one test.
func fakeKernel(t *testing.T) *kernelCalls {
	t.Helper()
	k := &kernelCalls{}
	origKill := sysKill
	t.Cleanup(func() { sysKill = origKill })

	sysKill = func(pid int, sig syscall.Signal) error {
		k.kills = append(k.kills, [2]int{pid, int(sig)})
		return k.err
	}
	return k
}

func (k *kernelCalls) touched() bool {
	return len(k.kills) > 0
}

// ---- real child process ----

type child struct {
	pid    int
	exited chan struct{}
	err    error // result of Wait, valid once exited is closed
}

// spawnSleep starts `sleep 30` in its own process group and guarantees it is
// killed and reaped when the test ends.
func spawnSleep(t *testing.T) *child {
	t.Helper()
	path, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	cmd := exec.Command(path, "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting sleep: %v", err)
	}
	c := &child{pid: cmd.Process.Pid, exited: make(chan struct{})}
	go func() {
		c.err = cmd.Wait()
		close(c.exited)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill() // already-finished is fine
		select {
		case <-c.exited:
		case <-time.After(5 * time.Second):
			t.Errorf("child %d was not reaped", c.pid)
		}
	})
	return c
}

// wait blocks until the child has exited and been reaped, returning Wait's error.
func (c *child) wait(t *testing.T) error {
	t.Helper()
	select {
	case <-c.exited:
		return c.err
	case <-time.After(5 * time.Second):
		t.Fatalf("child %d did not exit in time", c.pid)
		return nil
	}
}

// diedBySignal reports whether Wait's error says the child was killed by sig.
func diedBySignal(err error, sig syscall.Signal) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && ws.Signaled() && ws.Signal() == sig
}

// waitState polls /proc until the child's state satisfies ok (signals are delivered asynchronously).
func waitState(t *testing.T, pid int, ok func(state byte) bool) byte {
	t.Helper()
	fs := procfs.New()
	deadline := time.Now().Add(3 * time.Second)
	for {
		st, err := fs.Stat(pid)
		if err == nil && ok(st.State) {
			return st.State
		}
		if time.Now().After(deadline) {
			t.Fatalf("pid %d never reached the expected state (last=%+v err=%v)", pid, st, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ---- safety and mapping (no real signal is sent) ----

func TestInvalidPIDNeverReachesTheKernel(t *testing.T) {
	// kill(0) signals the whole process group and kill(-1) every process we may
	// signal; setpriority(0) would act on the caller. None may ever be issued.
	actions := map[string]func(pid int) error{
		"Terminate": Terminate,
	}
	for name, act := range actions {
		for _, pid := range []int{0, -1, -4242} {
			t.Run(fmt.Sprintf("%s/pid=%d", name, pid), func(t *testing.T) {
				k := fakeKernel(t)
				if err := act(pid); !errors.Is(err, ErrInvalidPID) {
					t.Fatalf("%s(%d) error = %v, want ErrInvalidPID", name, pid, err)
				}
				if k.touched() {
					t.Fatalf("%s(%d) reached the kernel: %+v", name, pid, k)
				}
			})
		}
	}
}

func TestSignalsUseTheRightSignal(t *testing.T) {
	tests := []struct {
		name string
		fn   func(int) error
		want syscall.Signal
	}{
		{"Terminate", Terminate, syscall.SIGTERM},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := fakeKernel(t)
			if err := tt.fn(4321); err != nil {
				t.Fatal(err)
			}
			if len(k.kills) != 1 || k.kills[0] != [2]int{4321, int(tt.want)} {
				t.Fatalf("kill calls = %v, want one call {4321, %d}", k.kills, tt.want)
			}
		})
	}
}

// ---- real syscalls against a child process spawned by the test ----

func TestTerminateKillsTheChildWithSIGTERM(t *testing.T) {
	c := spawnSleep(t)
	if err := Terminate(c.pid); err != nil {
		t.Fatal(err)
	}
	if err := c.wait(t); !diedBySignal(err, syscall.SIGTERM) {
		t.Fatalf("Wait = %v, want death by SIGTERM", err)
	}
}
