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
	kills    [][2]int // {pid, signal}
	getCalls []int    // pid
	setCalls [][3]int // {which, pid, nice}
	getRaw   int      // value the fake getpriority returns
	err      error    // error every fake call returns
}

// fakeKernel replaces sysKill/sysGetpriority/sysSetpriority for one test.
func fakeKernel(t *testing.T) *kernelCalls {
	t.Helper()
	k := &kernelCalls{}
	origKill, origGet, origSet := sysKill, sysGetpriority, sysSetpriority
	t.Cleanup(func() { sysKill, sysGetpriority, sysSetpriority = origKill, origGet, origSet })

	sysKill = func(pid int, sig syscall.Signal) error {
		k.kills = append(k.kills, [2]int{pid, int(sig)})
		return k.err
	}
	sysGetpriority = func(which, who int) (int, error) {
		k.getCalls = append(k.getCalls, who)
		return k.getRaw, k.err
	}
	sysSetpriority = func(which, who, nice int) error {
		k.setCalls = append(k.setCalls, [3]int{which, who, nice})
		return k.err
	}
	return k
}

func (k *kernelCalls) touched() bool {
	return len(k.kills) > 0 || len(k.getCalls) > 0 || len(k.setCalls) > 0
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
		"Terminate":   Terminate,
		"Kill":        Kill,
		"Suspend":     Suspend,
		"Resume":      Resume,
		"SetPriority": func(pid int) error { return SetPriority(pid, 5) },
		"Priority":    func(pid int) error { _, err := Priority(pid); return err },
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
		{"Kill", Kill, syscall.SIGKILL},
		{"Suspend", Suspend, syscall.SIGSTOP},
		{"Resume", Resume, syscall.SIGCONT},
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

func TestPriorityConvertsTheRawSyscallValue(t *testing.T) {
	// The Linux getpriority syscall returns 20 - nice (1..40) so that -1 stays
	// free as an error marker; glibc converts it back, Go's syscall does not.
	tests := []struct {
		raw  int
		want int
	}{
		{raw: 20, want: 0},
		{raw: 10, want: 10},
		{raw: 30, want: -10},
		{raw: 1, want: 19},
		{raw: 40, want: -20},
	}
	for _, tt := range tests {
		k := fakeKernel(t)
		k.getRaw = tt.raw
		got, err := Priority(77)
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("raw %d: Priority = %d, want %d", tt.raw, got, tt.want)
		}
		if len(k.getCalls) != 1 || k.getCalls[0] != 77 {
			t.Errorf("getpriority calls = %v, want [77]", k.getCalls)
		}
	}
}

func TestSetPriorityValidatesRange(t *testing.T) {
	tests := []struct {
		nice    int
		wantErr bool
	}{
		{nice: -21, wantErr: true},
		{nice: 20, wantErr: true},
		{nice: 1000, wantErr: true},
		{nice: -20},
		{nice: 0},
		{nice: 19},
	}
	for _, tt := range tests {
		k := fakeKernel(t)
		err := SetPriority(55, tt.nice)
		if tt.wantErr {
			if !errors.Is(err, ErrInvalidPriority) {
				t.Errorf("nice %d: err = %v, want ErrInvalidPriority", tt.nice, err)
			}
			if k.touched() {
				t.Errorf("nice %d: out-of-range value reached the kernel", tt.nice)
			}
			continue
		}
		if err != nil {
			t.Errorf("nice %d: unexpected error %v", tt.nice, err)
		}
		if len(k.setCalls) != 1 || k.setCalls[0] != [3]int{syscall.PRIO_PROCESS, 55, tt.nice} {
			t.Errorf("nice %d: setpriority calls = %v", tt.nice, k.setCalls)
		}
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

func TestKillKillsTheChildWithSIGKILL(t *testing.T) {
	c := spawnSleep(t)
	if err := Kill(c.pid); err != nil {
		t.Fatal(err)
	}
	if err := c.wait(t); !diedBySignal(err, syscall.SIGKILL) {
		t.Fatalf("Wait = %v, want death by SIGKILL", err)
	}
}

func TestSuspendAndResume(t *testing.T) {
	c := spawnSleep(t)
	waitState(t, c.pid, func(s byte) bool { return s != 'T' }) // starts running or sleeping

	if err := Suspend(c.pid); err != nil {
		t.Fatal(err)
	}
	waitState(t, c.pid, func(s byte) bool { return s == 'T' })

	if err := Resume(c.pid); err != nil {
		t.Fatal(err)
	}
	waitState(t, c.pid, func(s byte) bool { return s != 'T' })
}

func TestPriorityMatchesWhatProcReports(t *testing.T) {
	c := spawnSleep(t)
	got, err := Priority(c.pid)
	if err != nil {
		t.Fatal(err)
	}
	st, err := procfs.New().Stat(c.pid)
	if err != nil {
		t.Fatal(err)
	}
	if got != st.Nice {
		t.Fatalf("Priority = %d but /proc/%d/stat says nice %d", got, c.pid, st.Nice)
	}
}

func TestSetPriorityRoundTrip(t *testing.T) {
	c := spawnSleep(t)
	// /proc is the independent oracle for the starting point, not Priority itself.
	start, err := procfs.New().Stat(c.pid)
	if err != nil {
		t.Fatal(err)
	}
	before := start.Nice
	// Nice 10 is the fixed point of the kernel's 20 - nice encoding, so on its
	// own it cannot tell a missing conversion from a correct one: 5 and 15 do.
	// Targets only go up because an unprivileged user cannot lower nice.
	executed := 0
	for _, target := range []int{5, 10, 15} {
		if target < before {
			continue
		}
		executed++
		if err := SetPriority(c.pid, target); err != nil {
			t.Fatalf("SetPriority(%d): %v", target, err)
		}
		got, err := Priority(c.pid)
		if err != nil {
			t.Fatal(err)
		}
		if got != target {
			t.Errorf("Priority after SetPriority(%d) = %d", target, got)
		}
		st, err := procfs.New().Stat(c.pid)
		if err != nil {
			t.Fatal(err)
		}
		if st.Nice != target {
			t.Errorf("/proc reports nice %d after SetPriority(%d)", st.Nice, target)
		}
	}
	if executed == 0 {
		t.Skipf("the child already runs at nice %d, above every target; nothing was checked", before)
	}
}
