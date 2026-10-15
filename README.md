# procmon

A full-screen process monitor for the terminal (in the spirit of `top` and `btop`), written in Go for the
Operating Systems course (URU, 2026C, Project 1). It lists every process with its CPU, memory, state and
priority, summarises the machine, shows a parent/child tree, the threads and the CPU history of one
process, and can terminate, kill, suspend, resume and renice processes.

Constraints from the project statement, and how the code honours them:

| Constraint | How |
|---|---|
| Process data comes from `/proc` | `internal/procfs` parses `/proc/[pid]/{stat,status,cmdline}`, `/proc/[pid]/task/*/stat` and `/proc/{stat,meminfo,loadavg,uptime,self/auxv}`. One extra read: `/etc/passwd`, only to turn a UID into a user name (section 5). |
| No library that lists processes or computes CPU % | `go.mod` requires only UI libraries: Bubble Tea, Lip Gloss and `x/ansi` (text width and truncation). No gopsutil, no procfs package. |
| No external programs | No `os/exec` in non-test code. Signals use `kill(2)`, priority uses `getpriority(2)` and `setpriority(2)`, all through package `syscall`. |
| %CPU is computed by the program | `internal/monitor/cpu.go`, from two samples of the kernel counters (section 6). |

The guard for the third row prints nothing:

```sh
rg -n 'os/exec' --glob '*.go' --glob '!*_test.go' .
```

The only use of `os/exec` in the repository is a test in `internal/control` that starts a `sleep` child to
have a harmless process to signal.

## Build and run

```sh
go build -o procmon ./cmd/procmon
./procmon                    # refresh every 1 s
./procmon -interval 500ms    # any value from 200ms to 60s
./procmon -plain             # square pill ends, for fonts without Nerd Font glyphs
./procmon -cpu-mode core     # start with %CPU per core, like top (the I key switches it while running)
```

- Go 1.27.1 (the `go` line of `go.mod`). Linux only: it reads `/proc`.
- A normal user can see and read most processes. Acting on processes of other users needs root
  (`CAP_KILL` for signals, `CAP_SYS_NICE` to lower a nice value).
- `q` or Ctrl+C leaves the alternate screen and restores the terminal. Bad flags exit with status 2.

## Keymap

Hot keys need no Enter; the ones that send a signal ask `y`/`n` first. The main ones are also listed in the
help line at the bottom of the screen (it shows as many as fit).

| Key | Where | Action |
|---|---|---|
| `q` | table, detail | Quit |
| `Ctrl+C` | everywhere, also while typing or confirming | Quit |
| `Up`/`k`, `Down`/`j` | table | Move the selection |
| `PgUp`, `PgDn` | table | Move one page |
| `Home`, `End` | table | First row (the selection follows the top row again), last row |
| `Enter` | table | Open the detail view of the selected process |
| `c`, `m`, `p` | table | Sort by CPU (default), by memory, by PID |
| `t` | table | Switch between the flat table and the process tree |
| `/` | table | Filter by name: command line or process name, case-insensitive; an empty filter clears it |
| `Esc`, `Enter` | detail | Back to the table |
| `Up`/`k`, `Down`/`j`, `PgUp`, `PgDn`, `Home`, `End` | detail | Scroll the thread list |
| `x` | table, detail | Terminate (SIGTERM), asks `y`/`n` first |
| `X` | table, detail | Force kill (SIGKILL), asks its own question |
| `s`, `r` | table, detail | Suspend (SIGSTOP), resume (SIGCONT), each asks `y`/`n` first |
| `n` | table, detail | Change nice: shows the current value, asks for a whole number from -20 to 19 |
| `I` (Shift+i) | table, detail | Switch %CPU between the share of the whole machine (default) and per core, like `top`'s Shift+I; the header pill `cpu machine` / `cpu per-core` shows which one is active |
| `+`, `-` | table, detail | Longer, shorter refresh interval (steps: 0.5, 1, 2, 3, 5, 10 s) |
| `i` | table, detail | Type a refresh interval in seconds (0.2 to 60) |

While a line is being typed (filter, interval, nice): `Enter` applies, `Esc` cancels, `Backspace` deletes,
`Ctrl+U` clears the line, pasted text is inserted, and the hot keys are off. While a confirmation is open,
`y` confirms, `n` or `Esc` cancels, and every other key is ignored. Actions apply to the selected row, or to the
open process in the detail view.

The screen has three parts: a header box (task counts, load, uptime, memory, swap; the refresh interval, sort
key, %CPU mode, view and filter appear as pills in its border), the process table or detail box, and one status/help line.
It uses rounded boxes from 40x12 up and a compact layout below that. Rows in state Z (red), D (amber) and
T/t (violet) are highlighted, and the header counts use the same colours. The last row always belongs to that
status/help line, even in a terminal one or two rows tall, so a question or message is never hidden.

