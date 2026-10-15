package monitor

import (
	"errors"
	"strings"
	"testing"

	"github.com/EliabParra/procmon/internal/oserr"
)

func TestVerify(t *testing.T) {
	src := fourCoreMachine().
		add(proc{pid: 100, comm: "alive", start: 5000}).
		add(proc{pid: 101, comm: "zombie", state: 'Z', start: 6000})
	src.statErr[102] = oserr.ErrPermission
	src.statErr[103] = errBoom
	m := newTestMonitor(t, src)

	tests := []struct {
		name  string
		pid   int
		start uint64
		check func(error) bool
		want  string
	}{
		{"the same process", 100, 5000, func(err error) bool { return err == nil }, "nil"},
		{"a zombie is still that process", 101, 6000, func(err error) bool { return err == nil }, "nil"},
		{"the process exited", 999, 5000, func(err error) bool { return errors.Is(err, oserr.ErrProcessGone) }, "ErrProcessGone"},
		{"the pid now belongs to another process", 100, 4999, func(err error) bool { return errors.Is(err, oserr.ErrProcessGone) }, "ErrProcessGone"},
		{"not allowed to look", 102, 1, func(err error) bool {
			return errors.Is(err, oserr.ErrPermission) && !errors.Is(err, oserr.ErrProcessGone)
		}, "ErrPermission only"},
		{"any other failure passes through", 103, 1, func(err error) bool { return errors.Is(err, errBoom) }, "errBoom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := m.Verify(tt.pid, tt.start); !tt.check(err) {
				t.Errorf("Verify(%d, %d) = %v, want %s", tt.pid, tt.start, err, tt.want)
			}
		})
	}
}

func TestVerifyExplainsAReusedPID(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 100, comm: "new-owner", start: 9000})
	err := newTestMonitor(t, src).Verify(100, 5000)
	if err == nil || !errors.Is(err, oserr.ErrProcessGone) {
		t.Fatalf("err = %v, want ErrProcessGone", err)
	}
	if got := err.Error(); !strings.Contains(got, "100") || !strings.Contains(got, "another process") {
		t.Errorf("the error must name the pid and say it was reused: %q", got)
	}
}

func TestVerifyDoesNotNeedARefreshAndChangesNothing(t *testing.T) {
	src := fourCoreMachine().add(proc{pid: 100, comm: "p", start: 5000})
	m := newTestMonitor(t, src)

	if err := m.Verify(100, 5000); err != nil { // before any Refresh
		t.Fatal(err)
	}
	if _, err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	before := len(m.History(100))
	if err := m.Verify(100, 5000); err != nil {
		t.Fatal(err)
	}
	if after := len(m.History(100)); after != before {
		t.Errorf("Verify added a sample to the history (%d -> %d)", before, after)
	}
}
