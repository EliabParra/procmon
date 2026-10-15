package procfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/EliabParra/procmon/internal/oserr"
)

// fixtureFS reads the /proc-shaped tree under testdata/proc.
func fixtureFS() FS { return NewFS(filepath.Join("testdata", "proc")) }

// tempFS builds a throw-away /proc tree from relative path -> content.
func tempFS(t *testing.T, files map[string]string) FS {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return NewFS(root)
}

// statLine renders a valid /proc/[pid]/stat line (utime 7, stime 3, nice 0, 1 thread).
func statLine(pid int, comm string, state byte) string {
	return fmt.Sprintf("%d (%s) %c 1 %d %d 0 -1 4194304 100 0 0 0 7 3 0 0 20 0 1 0 5000 4096000 120 0 0 0\n",
		pid, comm, state, pid, pid)
}

func TestPIDs(t *testing.T) {
	t.Run("numeric directories only, sorted numerically", func(t *testing.T) {
		got, err := fixtureFS().PIDs()
		if err != nil {
			t.Fatal(err)
		}
		// cpuinfo, self, sys and 1234abc are not pids; 2 sorts before 1234 numerically.
		want := []int{2, 1234, 4321}
		if !slices.Equal(got, want) {
			t.Fatalf("PIDs() = %v, want %v", got, want)
		}
	})

	t.Run("missing root is an error", func(t *testing.T) {
		if _, err := NewFS(filepath.Join(t.TempDir(), "nope")).PIDs(); err == nil {
			t.Fatal("expected an error for a missing root")
		}
	})
}

func TestStat(t *testing.T) {
	tests := []struct {
		name string
		pid  int
		want ProcStat
	}{
		{
			name: "regular multi-threaded process",
			pid:  1234,
			want: ProcStat{PID: 1234, Comm: "firefox", State: 'S', PPID: 1, UTime: 4200, STime: 300,
				Priority: 20, Nice: 0, NumThreads: 3, StartTime: 123456},
		},
		{
			name: "comm containing spaces and parentheses",
			pid:  4321,
			want: ProcStat{PID: 4321, Comm: "my (weird) app", State: 'R', PPID: 1234, UTime: 12, STime: 34,
				Priority: 20, Nice: 10, NumThreads: 1, StartTime: 999},
		},
		{
			name: "kernel thread",
			pid:  2,
			want: ProcStat{PID: 2, Comm: "kthreadd", State: 'S', PPID: 0, UTime: 0, STime: 5,
				Priority: 20, Nice: 0, NumThreads: 1, StartTime: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fixtureFS().Stat(tt.pid)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("Stat(%d)\n got  %+v\n want %+v", tt.pid, got, tt.want)
			}
		})
	}
}

func TestStatErrors(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantGone bool
	}{
		{name: "process is gone", files: map[string]string{}, wantGone: true},
		{name: "no parentheses", files: map[string]string{"7/stat": "7 comm S 1 2 3\n"}},
		{name: "closing parenthesis before the opening one", files: map[string]string{"7/stat": "7 )comm( S 1\n"}},
		{name: "too few fields", files: map[string]string{"7/stat": "7 (comm) S 1 7 7\n"}},
		{name: "non numeric utime", files: map[string]string{
			"7/stat": "7 (comm) S 1 7 7 0 -1 4194304 100 0 0 0 abc 3 0 0 20 0 1 0 5000 0 0\n"}},
		{name: "multi-character state", files: map[string]string{
			"7/stat": "7 (comm) SS 1 7 7 0 -1 4194304 100 0 0 0 7 3 0 0 20 0 1 0 5000 0 0\n"}},
		{name: "empty file", files: map[string]string{"7/stat": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tempFS(t, tt.files).Stat(7)
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := errors.Is(err, oserr.ErrProcessGone); got != tt.wantGone {
				t.Fatalf("errors.Is(ErrProcessGone) = %v, want %v (err=%v)", got, tt.wantGone, err)
			}
		})
	}
}

func TestStatPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	fs := tempFS(t, map[string]string{"7/stat": statLine(7, "x", 'S')})
	if err := os.Chmod(filepath.Join(fs.Root, "7", "stat"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(7); !errors.Is(err, oserr.ErrPermission) {
		t.Fatalf("Stat on an unreadable file = %v, want ErrPermission", err)
	}
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name string
		pid  int
		want ProcStatus
	}{
		// Uid line is "1000 1001 1002 1003": the first one is the real UID.
		{name: "real uid and rss", pid: 1234, want: ProcStatus{UID: 1000, VmRSSKB: 412344}},
		{name: "kernel thread has no VmRSS", pid: 2, want: ProcStatus{UID: 0, VmRSSKB: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fixtureFS().Status(tt.pid)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("Status(%d) = %+v, want %+v", tt.pid, got, tt.want)
			}
		})
	}
}

func TestStatusErrors(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantGone bool
	}{
		{name: "process is gone", files: map[string]string{}, wantGone: true},
		{name: "missing Uid line", files: map[string]string{"7/status": "Name:\tx\nVmRSS:\t10 kB\n"}},
		{name: "non numeric Uid", files: map[string]string{"7/status": "Uid:\tabc\t0\t0\t0\n"}},
		{name: "non numeric VmRSS", files: map[string]string{"7/status": "Uid:\t0\t0\t0\t0\nVmRSS:\tabc kB\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tempFS(t, tt.files).Status(7)
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := errors.Is(err, oserr.ErrProcessGone); got != tt.wantGone {
				t.Fatalf("errors.Is(ErrProcessGone) = %v, want %v (err=%v)", got, tt.wantGone, err)
			}
		})
	}
}

func TestCmdline(t *testing.T) {
	tests := []struct {
		name string
		fs   FS
		pid  int
		want string
	}{
		{name: "NUL separated args are joined by spaces", fs: fixtureFS(), pid: 1234,
			want: "/usr/lib/firefox/firefox --new-window https://example.com"},
		{name: "kernel thread has an empty cmdline", fs: fixtureFS(), pid: 2, want: ""},
		{name: "no trailing NUL", pid: 7, want: "a b",
			fs: tempFS(t, map[string]string{"7/cmdline": "a\x00b"})},
		{name: "several trailing NULs are trimmed", pid: 7, want: "a b",
			fs: tempFS(t, map[string]string{"7/cmdline": "a\x00b\x00\x00\x00"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fs.Cmdline(tt.pid)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("Cmdline(%d) = %q, want %q", tt.pid, got, tt.want)
			}
		})
	}

	t.Run("process is gone", func(t *testing.T) {
		if _, err := fixtureFS().Cmdline(99999); !errors.Is(err, oserr.ErrProcessGone) {
			t.Fatalf("err = %v, want ErrProcessGone", err)
		}
	})
}
