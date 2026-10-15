package procfs

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/EliabParra/procmon/internal/oserr"
)

// TestRealProc reads the live /proc of the machine running the tests: the
// test process itself is a safe, always-existing target.
func TestRealProc(t *testing.T) {
	if _, err := os.Stat(defaultRoot + "/self/stat"); err != nil {
		t.Skipf("no usable /proc: %v", err)
	}
	fs := New()
	self := os.Getpid()

	t.Run("PIDs contains own pid", func(t *testing.T) {
		pids, err := fs.PIDs()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(pids, self) {
			t.Fatalf("own pid %d not found among %d pids", self, len(pids))
		}
		if !slices.IsSorted(pids) {
			t.Fatal("pids are not sorted")
		}
	})

	t.Run("Stat of own pid", func(t *testing.T) {
		st, err := fs.Stat(self)
		if err != nil {
			t.Fatal(err)
		}
		if st.PID != self {
			t.Errorf("PID = %d, want %d", st.PID, self)
		}
		if st.Comm == "" {
			t.Error("empty Comm")
		}
		if st.PPID != os.Getppid() {
			t.Errorf("PPID = %d, want %d", st.PPID, os.Getppid())
		}
		if st.NumThreads < 1 {
			t.Errorf("NumThreads = %d, want >= 1", st.NumThreads)
		}
		if !strings.ContainsRune("RSDTtZXIW", rune(st.State)) {
			t.Errorf("unexpected State %q", st.State)
		}
	})

	t.Run("Status of own pid", func(t *testing.T) {
		st, err := fs.Status(self)
		if err != nil {
			t.Fatal(err)
		}
		if st.UID != os.Getuid() {
			t.Errorf("UID = %d, want %d", st.UID, os.Getuid())
		}
		if st.VmRSSKB == 0 {
			t.Error("a running Go test must have resident memory")
		}
	})

	t.Run("Cmdline of own pid", func(t *testing.T) {
		got, err := fs.Cmdline(self)
		if err != nil {
			t.Fatal(err)
		}
		if got == "" {
			t.Error("a test binary has a command line")
		}
	})

	// Robustness: every process of the machine (kernel threads, zombies, odd
	// comm names) must parse; the only tolerated failures are the two
	// expected ones, because processes exit and some files are restricted.
	t.Run("every live process parses or fails with a classified error", func(t *testing.T) {
		pids, err := fs.PIDs()
		if err != nil {
			t.Fatal(err)
		}
		expected := func(err error) bool {
			return err == nil || errors.Is(err, oserr.ErrProcessGone) || errors.Is(err, oserr.ErrPermission)
		}
		for _, pid := range pids {
			if _, err := fs.Stat(pid); !expected(err) {
				t.Errorf("Stat(%d): %v", pid, err)
			}
			if _, err := fs.Status(pid); !expected(err) {
				t.Errorf("Status(%d): %v", pid, err)
			}
			if _, err := fs.Cmdline(pid); !expected(err) {
				t.Errorf("Cmdline(%d): %v", pid, err)
			}
			if _, err := fs.Threads(pid); !expected(err) {
				t.Errorf("Threads(%d): %v", pid, err)
			}
		}
	})

	t.Run("system-wide files", func(t *testing.T) {
		ss, err := fs.SystemStat()
		if err != nil {
			t.Fatal(err)
		}
		if ss.CPU.Total() == 0 || len(ss.Cores) == 0 {
			t.Errorf("SystemStat looks empty: %+v", ss)
		}

		mi, err := fs.MemInfo()
		if err != nil {
			t.Fatal(err)
		}
		if mi.MemTotalKB == 0 || mi.MemAvailableKB > mi.MemTotalKB {
			t.Errorf("implausible MemInfo: %+v", mi)
		}

		la, err := fs.LoadAvg()
		if err != nil {
			t.Fatal(err)
		}
		if la.Total < 1 || la.Running < 1 {
			t.Errorf("implausible LoadAvg: %+v", la)
		}

		up, err := fs.Uptime()
		if err != nil {
			t.Fatal(err)
		}
		if up <= 0 {
			t.Errorf("Uptime = %v, want > 0", up)
		}

		if hz := fs.ClockTicks(); hz < 1 {
			t.Errorf("ClockTicks = %d, want >= 1", hz)
		}
	})

	t.Run("Threads of own pid include the main thread", func(t *testing.T) {
		ths, err := fs.Threads(self)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, th := range ths {
			found = found || th.PID == self
		}
		if !found {
			t.Fatalf("main thread %d missing from %d threads", self, len(ths))
		}
	})
}
