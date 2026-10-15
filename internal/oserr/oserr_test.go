package oserr

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
	"testing"
)

func TestClassify(t *testing.T) {
	other := errors.New("boom")

	tests := []struct {
		name     string
		in       error
		wantGone bool
		wantPerm bool
		wantOrig error // the original error must stay reachable via errors.Is
	}{
		{name: "nil stays nil", in: nil},
		{name: "fs.ErrNotExist is gone", in: fs.ErrNotExist, wantGone: true, wantOrig: fs.ErrNotExist},
		{name: "ESRCH is gone", in: syscall.ESRCH, wantGone: true, wantOrig: syscall.ESRCH},
		{name: "fs.ErrPermission is denied", in: fs.ErrPermission, wantPerm: true, wantOrig: fs.ErrPermission},
		{name: "EPERM is denied", in: syscall.EPERM, wantPerm: true, wantOrig: syscall.EPERM},
		{name: "EACCES is denied", in: syscall.EACCES, wantPerm: true, wantOrig: syscall.EACCES},
		{
			name:     "wrapped PathError ENOENT is gone",
			in:       &fs.PathError{Op: "open", Path: "/proc/1/stat", Err: syscall.ENOENT},
			wantGone: true,
			wantOrig: syscall.ENOENT,
		},
		{
			name:     "wrapped chain is still classified",
			in:       fmt.Errorf("reading: %w", &fs.PathError{Op: "open", Path: "x", Err: syscall.EACCES}),
			wantPerm: true,
			wantOrig: syscall.EACCES,
		},
		{name: "unrelated error passes through", in: other, wantOrig: other},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.in)

			if tt.in == nil {
				if got != nil {
					t.Fatalf("Classify(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("Classify(%v) = nil, want an error", tt.in)
			}
			if errors.Is(got, ErrProcessGone) != tt.wantGone {
				t.Errorf("errors.Is(ErrProcessGone) = %v, want %v (err=%v)", !tt.wantGone, tt.wantGone, got)
			}
			if errors.Is(got, ErrPermission) != tt.wantPerm {
				t.Errorf("errors.Is(ErrPermission) = %v, want %v (err=%v)", !tt.wantPerm, tt.wantPerm, got)
			}
			if !errors.Is(got, tt.wantOrig) {
				t.Errorf("original error %v no longer reachable from %v", tt.wantOrig, got)
			}
		})
	}
}

func TestClassifyRealMissingFile(t *testing.T) {
	_, err := os.ReadFile(t.TempDir() + "/does-not-exist")
	if !errors.Is(Classify(err), ErrProcessGone) {
		t.Fatalf("a real ENOENT from os.ReadFile should classify as ErrProcessGone, got %v", Classify(err))
	}
}
