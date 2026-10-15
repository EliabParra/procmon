package procfs

import (
	"errors"
	"testing"

	"github.com/EliabParra/procmon/internal/oserr"
)

func TestCPUTimesTotal(t *testing.T) {
	tests := []struct {
		name string
		in   CPUTimes
		want uint64
	}{
		{name: "zero value", in: CPUTimes{}, want: 0},
		{name: "every column is summed once",
			in: CPUTimes{User: 1, Nice: 2, System: 3, Idle: 4, IOWait: 5, IRQ: 6, SoftIRQ: 7, Steal: 8}, want: 36},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Total(); got != tt.want {
				t.Fatalf("Total() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSystemStat(t *testing.T) {
	t.Run("aggregate and per-core lines, other lines ignored", func(t *testing.T) {
		got, err := fixtureFS().SystemStat()
		if err != nil {
			t.Fatal(err)
		}
		wantCPU := CPUTimes{User: 1000, Nice: 20, System: 300, Idle: 4000, IOWait: 50, IRQ: 6, SoftIRQ: 7, Steal: 8}
		if got.CPU != wantCPU {
			t.Errorf("CPU = %+v, want %+v", got.CPU, wantCPU)
		}
		// The fixture line also carries guest=9 and guest_nice=10: they must not leak into Total.
		if got.CPU.Total() != 5391 {
			t.Errorf("CPU.Total() = %d, want 5391", got.CPU.Total())
		}
		wantCores := []CPUTimes{
			{User: 500, Nice: 10, System: 150, Idle: 2000, IOWait: 25, IRQ: 3, SoftIRQ: 4, Steal: 4},
			{User: 500, Nice: 10, System: 150, Idle: 2000, IOWait: 25, IRQ: 3, SoftIRQ: 3, Steal: 4},
		}
		if len(got.Cores) != len(wantCores) {
			t.Fatalf("got %d cores, want %d", len(got.Cores), len(wantCores))
		}
		for i, want := range wantCores {
			if got.Cores[i] != want {
				t.Errorf("Cores[%d] = %+v, want %+v", i, got.Cores[i], want)
			}
		}
	})

	t.Run("old kernel with only four columns", func(t *testing.T) {
		got, err := tempFS(t, map[string]string{"stat": "cpu  1 2 3 4\ncpu0 1 2 3 4\n"}).SystemStat()
		if err != nil {
			t.Fatal(err)
		}
		want := CPUTimes{User: 1, Nice: 2, System: 3, Idle: 4}
		if got.CPU != want || len(got.Cores) != 1 || got.Cores[0] != want {
			t.Fatalf("got %+v, want aggregate and one core equal to %+v", got, want)
		}
	})

	t.Run("no per-core lines", func(t *testing.T) {
		got, err := tempFS(t, map[string]string{"stat": "cpu  1 2 3 4 5 6 7 8\nintr 1\n"}).SystemStat()
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Cores) != 0 {
			t.Fatalf("Cores = %+v, want none", got.Cores)
		}
	})

	errCases := []struct {
		name string
		stat string
	}{
		{name: "no cpu line", stat: "intr 1\nctxt 2\n"},
		{name: "cpu line too short", stat: "cpu  1 2 3\n"},
		{name: "non numeric column", stat: "cpu  1 x 3 4\n"},
		{name: "empty file", stat: ""},
	}
	for _, tt := range errCases {
		t.Run("error: "+tt.name, func(t *testing.T) {
			if _, err := tempFS(t, map[string]string{"stat": tt.stat}).SystemStat(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}

	t.Run("error: missing file is not reported as a gone process", func(t *testing.T) {
		_, err := tempFS(t, nil).SystemStat()
		if err == nil {
			t.Fatal("expected an error")
		}
		if errors.Is(err, oserr.ErrProcessGone) {
			t.Fatalf("a missing /proc/stat is not a vanished process: %v", err)
		}
	})
}
