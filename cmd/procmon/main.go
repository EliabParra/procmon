// Command procmon is a full-screen process monitor for Linux: it reads
// /proc, shows the processes and acts on them.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/EliabParra/procmon/internal/control"
	"github.com/EliabParra/procmon/internal/monitor"
	"github.com/EliabParra/procmon/internal/procfs"
	"github.com/EliabParra/procmon/internal/tui"
)

type config struct {
	interval time.Duration
	plain    bool
	perCore  bool // %CPU per core, like top, instead of the share of the machine
}

// parseArgs reads the command line; usage and errors go to out.
func parseArgs(args []string, out io.Writer) (config, error) {
	fs := flag.NewFlagSet("procmon", flag.ContinueOnError)
	fs.SetOutput(out)
	interval := fs.Duration("interval", tui.DefaultInterval, "refresh interval, e.g. 500ms or 2s")
	plain := fs.Bool("plain", false, "draw pills with square ends, for fonts without Nerd Font glyphs")
	cpuMode := fs.String("cpu-mode", "machine", "how %CPU is shown: machine (share of the whole machine, the course formula) or core (per core, like top; I switches it while running)")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() > 0 {
		return config{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *interval < tui.MinInterval || *interval > tui.MaxInterval {
		return config{}, fmt.Errorf("-interval must be between %v and %v, got %v", tui.MinInterval, tui.MaxInterval, *interval)
	}
	if *cpuMode != "machine" && *cpuMode != "core" {
		return config{}, fmt.Errorf("-cpu-mode must be machine or core, got %q", *cpuMode)
	}
	return config{interval: *interval, plain: *plain, perCore: *cpuMode == "core"}, nil
}

// controller adapts the functions of package control to the port the screen uses.
type controller struct{}

func (controller) Terminate(pid int) error         { return control.Terminate(pid) }
func (controller) Kill(pid int) error              { return control.Kill(pid) }
func (controller) Suspend(pid int) error           { return control.Suspend(pid) }
func (controller) Resume(pid int) error            { return control.Resume(pid) }
func (controller) Priority(pid int) (int, error)   { return control.Priority(pid) }
func (controller) SetPriority(pid, nice int) error { return control.SetPriority(pid, nice) }

var _ tui.Control = controller{}

func main() {
	cfg, err := parseArgs(os.Args[1:], os.Stderr)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return
	case err != nil:
		fmt.Fprintln(os.Stderr, "procmon:", err)
		os.Exit(2)
	}

	opts := []tui.Option{tui.WithInterval(cfg.interval)}
	if cfg.plain {
		opts = append(opts, tui.WithPlain())
	}
	if cfg.perCore {
		opts = append(opts, tui.WithPerCoreCPU())
	}
	model := tui.New(monitor.New(procfs.New()), controller{}, opts...)

	// Bubble Tea puts the terminal in the alternate screen and raw mode and
	// restores it whichever way Run returns: q, Ctrl+C, a signal or an error.
	if _, err := tea.NewProgram(model).Run(); err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "procmon:", err)
		os.Exit(1)
	}
}
