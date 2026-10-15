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

## Architecture

Four packages and one shared error package, wired in `cmd/procmon/main.go`:

| Package | Role | Main API |
|---|---|---|
| `internal/procfs` | Read door. Parses `/proc` files into plain structs. No percentages or deltas. The `/proc` root is injectable. | `FS` with `PIDs`, `Stat`, `Status`, `Cmdline`, `Threads`, `SystemStat`, `MemInfo`, `LoadAvg`, `Uptime`, `ClockTicks` |
| `internal/control` | Act door. Signals and priority through system calls, with validation. | `Terminate`, `Kill`, `Suspend`, `Resume`, `Priority`, `SetPriority` |
| `internal/monitor` | Logic. Snapshots, %CPU from deltas, summary, sort, filter, tree, CPU history, UID to name. | `Monitor` with `Refresh`, `History`, `Threads`, `Verify`; `Sort`, `Filter`, `BuildTree`, `WithAncestors` |
| `internal/tui` | Screen. Bubble Tea model, layout, keys, views. Never reads `/proc` and never sends signals itself. | `New`, `Model`, ports `Monitor` and `Control` |
| `internal/oserr` | Shared vocabulary for the two failures to expect: the process is gone, or permission is denied. | `ErrProcessGone`, `ErrPermission`, `Classify` |

Dependency direction. A solid arrow means "imports"; a dotted one means "only through an interface".
`cmd/procmon` imports the four packages to wire them; `internal/oserr` is imported by procfs, control, monitor
and tui.

```
cmd/procmon     wiring only: imports tui, monitor, procfs and control

internal/tui ------> internal/monitor ------> internal/procfs ------> /proc
     :
     :  Control port (an interface; the adapter lives in cmd/procmon)
     v
internal/control ------> kill(2), getpriority(2), setpriority(2)
```

`internal/tui` imports neither `procfs` nor `control`: it only knows the two interfaces below, and `main.go`
plugs the real implementations in.

- **`monitor.Source`** (`internal/monitor/source.go`) is everything the monitor reads from the system:
  `PIDs`, `Stat`, `Status`, `Cmdline`, `Threads`, `SystemStat`, `MemInfo`, `LoadAvg`, `Uptime`, `ClockTicks`.
  `procfs.FS` is the adapter; `var _ Source = procfs.FS{}` makes the build fail if it ever stops fitting.
  Tests use a fake source whose counters change between two refreshes.
- **`tui.Monitor`** (`Refresh`, `History`, `Threads`, `Verify`) is satisfied by `*monitor.Monitor`.
  **`tui.Control`** (`Terminate`, `Kill`, `Suspend`, `Resume`, `Priority`, `SetPriority`) is satisfied by a small
  `controller` adapter in `main.go` that forwards to the functions of `control`.
  Tests plug in fakes that record calls, so no test of the screen sends a real signal.

The screen follows the Elm architecture of Bubble Tea: one model, one `Update`, one `View`.

```
Init           -> Cmd: refresh (in the background) + Cmd: tick after min(interval, 0.5 s)
tickMsg        -> Update -> Cmd: refresh + Cmd: next tick   (a tick from an old interval is dropped)
refreshedMsg   -> Update -> store the snapshot, rebuild the rows: filter -> sort -> tree
KeyPressMsg    -> Update -> change mode, sort, filter or selection, or call Control
WindowSizeMsg  -> Update -> new size, layout recomputed
every message  -> View   -> a frame of exactly `height` lines, none wider than the terminal
```

`Update` never blocks: `Refresh` runs inside a `tea.Cmd`, and only one refresh can be in flight. A failed
refresh keeps the old snapshot on screen and shows "refresh failed: ..." in the status line. Bubble Tea's
renderer draws from a cell buffer and writes only what changed between frames, which is what avoids flicker.

## Where each datum comes from

`stat` field numbers follow `proc(5)`. `comm` (field 2) may contain spaces and `)`, so the parser splits the
line at the last `)` (one fixture is a process called `my (weird) app`).

**Process table**

| Column | Source |
|---|---|
| PID | Numeric directory names under `/proc` (`self`, `sys`... are ignored) |
| PPID | `/proc/[pid]/stat` field 4 |
| USER | `/proc/[pid]/status` line `Uid:` (first value, the real UID), then name from `/etc/passwd`. Shows the number if the UID is not listed, `?` if `status` cannot be read |
| S | `stat` field 3 (state letter) |
| %CPU | Per process: `stat` fields 14 (`utime`) + 15 (`stime`). System: the `cpu` line of `/proc/stat`. See the next section |
| %MEM | `VmRSS` of `status` divided by `MemTotal` of `/proc/meminfo`, times 100 |
| RES | `status` line `VmRSS:` (kB). Kernel threads have none and show `0K` |
| THR | `stat` field 20 (`num_threads`) |
| PRI | `stat` field 18 (`priority`, 20 + nice for normal tasks). Values below -99 (real-time) show as `rt` |
| NI | `stat` field 19 (`nice`) |
| TIME | (`utime` + `stime`) / CLK_TCK. CLK_TCK is read from `/proc/self/auxv` (entry `AT_CLKTCK`), 100 if unavailable |
| COMMAND | `/proc/[pid]/cmdline` with NUL separators turned into spaces; when empty (kernel threads, zombies) `[comm]` |

**Header**

| Item | Source |
|---|---|
| Tasks | Number of processes read in this snapshot |
| running, sleeping, disk-sleep, stopped, zombie | Count of state letters R, S plus I, D, T plus t, Z (idle kernel threads, state I, count as sleeping as in `top`) |
| Load | `/proc/loadavg` fields 1 to 3 |
| up | `/proc/uptime` field 1 |
| Mem used / total, avail | `/proc/meminfo` `MemTotal` and `MemAvailable`; used = `MemTotal` - `MemAvailable` |
| Swap | `SwapTotal`, and used = `SwapTotal` - `SwapFree` |
| Cores (factor of the per-core %CPU) | Number of `cpuN` lines of `/proc/stat` |

**Detail view**

| Item | Source |
|---|---|
| Threads: TID, state, TIME, name | `/proc/[pid]/task/[tid]/stat` (fields 1, 3, 14 + 15, 2) |
| CPU history | The last 60 %CPU samples of the process, one per refresh, kept in memory by `monitor` in the unit of the course formula and drawn in the active %CPU mode, on an autoscaled chart (see the next section) |

