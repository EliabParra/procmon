package procfs

import (
	"path/filepath"
	"slices"
	"testing"
)

// fixtureFS reads the /proc-shaped tree under testdata/proc.
func fixtureFS() FS { return NewFS(filepath.Join("testdata", "proc")) }

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
