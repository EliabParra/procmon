package monitor

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/EliabParra/procmon/internal/oserr"
	"github.com/EliabParra/procmon/internal/procfs"
)

func TestThreads(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 10})
	src.threads[10] = []procfs.ProcStat{
		{PID: 10, Comm: "main", State: 'S', UTime: 100, STime: 50},
		{PID: 11, Comm: "worker", State: 'R', UTime: 30},
		{PID: 12, Comm: "io", State: 'D'},
	}
	got, err := newTestMonitor(t, src).Threads(10)
	if err != nil {
		t.Fatal(err)
	}
	want := []Thread{
		{TID: 10, Comm: "main", State: 'S', CPUTime: 1500 * time.Millisecond},
		{TID: 11, Comm: "worker", State: 'R', CPUTime: 300 * time.Millisecond},
		{TID: 12, Comm: "io", State: 'D', CPUTime: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("threads = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("thread %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestThreadsErrorsKeepTheirMeaning(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fakeSource)
		want  error
	}{
		{"process exited", func(f *fakeSource) { f.threadsErr[10] = oserr.ErrProcessGone }, oserr.ErrProcessGone},
		{"never existed", func(f *fakeSource) {}, oserr.ErrProcessGone},
		{"permission denied", func(f *fakeSource) { f.threadsErr[10] = oserr.ErrPermission }, oserr.ErrPermission},
		{"other failure", func(f *fakeSource) { f.threadsErr[10] = errBoom }, errBoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := fourCoreMachine()
			tt.setup(src)
			threads, err := newTestMonitor(t, src).Threads(10)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Threads(10) error = %v, want it to wrap %v", err, tt.want)
			}
			if threads != nil {
				t.Fatalf("threads = %+v alongside an error", threads)
			}
		})
	}
}

func TestThreadsOnTheRealProc(t *testing.T) {
	if _, err := os.Stat("/proc/self/task"); err != nil {
		t.Skip("no /proc on this system")
	}
	threads, err := New(procfs.New()).Threads(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, th := range threads {
		found = found || th.TID == os.Getpid() // the main thread has TID == PID
		if th.State == 0 || th.Comm == "" {
			t.Fatalf("incomplete thread %+v", th)
		}
	}
	if !found {
		t.Fatalf("main thread %d missing from %+v", os.Getpid(), threads)
	}
	if _, err := New(procfs.New()).Threads(1 << 30); !errors.Is(err, oserr.ErrProcessGone) {
		t.Fatalf("Threads of a pid that does not exist: error = %v, want ErrProcessGone", err)
	}
}
