// Command procmon is a full-screen process monitor for Linux: it reads
// /proc, shows the processes and acts on them.
package main

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/EliabParra/procmon/internal/control"
	"github.com/EliabParra/procmon/internal/monitor"
	"github.com/EliabParra/procmon/internal/procfs"
	"github.com/EliabParra/procmon/internal/tui"
)

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
	model := tui.New(monitor.New(procfs.New()), controller{})

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
