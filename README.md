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

## How %CPU is computed

The course statement gives the formula: divide the difference of the process by the difference of the system.
That is what the program computes and what it shows by default:

```
%CPU = (delta of the process's utime+stime) / (delta of the total jiffies of /proc/stat) x 100
```

The result is the share of the **whole machine**, from 0 to 100. The `cpu` line of `/proc/stat` adds up all
cores, so on 4 cores a process that keeps one core flat out gives 25. Example: 4 cores, 1 s, 100 ticks/s: the
system grows by 400 ticks, a process that grew by 100 ticks shows 100 / 400 x 100 = 25%.

- **Two samples.** `Refresh` remembers, for every PID, its `utime+stime` and the system total of the previous
  refresh. The total is the sum of the eight columns of the `cpu` line (user, nice, system, idle, iowait, irq,
  softirq, steal; guest is ignored because the kernel already counts it inside user).
- **One canonical value.** `monitor.CPUPercent` returns the formula above and nothing else; the history ring
  buffer, the sort and every comparison use that value. The display mode below only changes how the screen
  draws it, so switching it never reorders the table or rewrites the history.
- **Per-core mode (`I`).** Press `I` (Shift+i) to see what `top` shows by default, its Irix mode: the same
  figure multiplied by the number of cores (`monitor.PerCore`), so 100 means one core flat out and a
  multithreaded process can go beyond it (up to 100 x cores). The same example reads 100 / 400 x 100 x 4 =
  100%. `top` has the same switch under Shift+I: Irix mode on is per-core, off (called Solaris mode) is the
  share of the whole machine. Pressing `I` again goes back to the formula of the course. The header pill says
  `cpu machine` or `cpu per-core`, a one-line notice says what 100 means, and `-cpu-mode core` starts in
  per-core mode. The mode applies to the table, the tree, the CPU line of the detail view and its history
  chart; on a machine with 1 core the two modes are the same.
- **History chart scale.** The detail view draws the last 60 samples as a one-line bar chart. Its top is the
  largest sample rounded up to a step of 1, 2, 5, 10, 20, 50, 100, 200, 500... (1, 2 or 5 times a power of
  ten), with a floor of 1%: a light process fills the chart, an idle one stays on the lowest block instead of
  amplifying counter noise. The label `scale 0-10%` next to min, avg and max says what a full-height block
  stands for, and those three figures are always the real ones. The top never goes above the whole machine in
  the active mode (100, or 100 x cores in per-core mode): a sample past that is a skew between the two `/proc`
  reads and is drawn clipped. Example: the samples 1, 6 and 3 give `scale 0-10%`, and the 6 is the tallest
  column.
- **First sample is 0.** With no previous sample there is no delta. The same applies to a process that first
  appears at a later refresh. The screen schedules the second refresh no later than 0.5 s after start, so the
  column does not sit at zero for a whole interval.
- **A recycled PID is not the same process.** Samples are matched by PID and start time (`stat` field 22). The
  same PID with another start time is a new process: its delta starts from zero and its history is reset.
- **Counters going backwards give 0**, not a huge unsigned number (saturating subtraction).
- **A failed refresh keeps the previous baseline**, so the next successful one still has something to subtract.
- **Not clamped.** The two files are read a moment apart, so a busy process can read a fraction of a point
  above 100 in the default mode; the code shows what the formula gives.
- **Resolution** is one clock tick of the process. In per-core mode it moves %CPU by 100 / (CLK_TCK x interval
  in seconds) points: 1 point at 1 s, 5 points at the 0.2 s minimum. In the default mode divide that by the
  number of cores (0.25 points at 1 s on 4 cores).

## Acting on processes

All actions go through `internal/control`, which calls `syscall.Kill`, `syscall.Getpriority` and
`syscall.Setpriority`. Numbers below are for Linux on x86-64 and arm64.

| Key | Function | Signal | Effect | Can the process prevent it? |
|---|---|---|---|---|
| `x` | `Terminate` | SIGTERM (15) | Asks the process to exit; it can clean up first | Yes: it can handle or ignore it |
| `X` | `Kill` | SIGKILL (9) | The kernel ends the process; nothing runs in it | No |
| `s` | `Suspend` | SIGSTOP (19) | Stops the process; it shows state T | No: cannot be caught, blocked or ignored |
| `r` | `Resume` | SIGCONT (18) | Continues a stopped process | A handler may run, but the process continues anyway |

**Priority.** `n` changes the nice value, from -20 (highest priority) to 19 (lowest). The owner can raise the
value; lowering it needs root or `CAP_SYS_NICE` (the kernel also allows it within the target's `RLIMIT_NICE`).
On Linux the nice value belongs to a single thread, so `getpriority`/`setpriority` with a PID act on the main
thread, the one `/proc/[pid]/stat` reports.

**The `20 - nice` detail.** The raw `getpriority` system call returns `20 - nice` (1 to 40), so that -1 never
looks like an error. glibc converts it back; Go's `syscall` package does not, so `control.Priority` returns
`20 - raw`. Nice 10 is the fixed point of that formula, which is why the round-trip test also uses 5 and 15.

**Safety rails**

1. `control` rejects `pid <= 0` before any system call: `kill(0, sig)` signals the caller's whole process group
   and `kill(-1, sig)` every process the caller may signal.
2. The screen refuses to act on procmon's own PID (`WithSelfPID`, default `os.Getpid()`). It checks when the
   key is pressed and again right before the signal or `setpriority`.
3. The target (PID, start time, name) is captured when the action starts, so a refresh under an open question
   cannot redirect the answer. Right before every signal or `setpriority` the screen calls `Monitor.Verify`,
   which re-reads `/proc/[pid]/stat` and compares the start time. If the process is gone or the PID now
   belongs to another process, nothing is sent and the message says "process N no longer exists".
4. Every signal key asks first, naming the action, the PID and the command. Force kill has its own wording:
   it is drawn in the alarm colour and says "the process cannot clean up". While a question is open, only `y`,
   `n` and `Esc` answer it. On a narrow line the explanation is cut first, then the end of the name; the PID
   and the answer stay (force kill's "Press y to confirm, n to cancel" becomes `y/n` below 60 columns).
5. Text from `/proc` is sanitised before it is drawn: control characters (an ESC would start a terminal
   sequence) and invalid UTF-8 become `?`.

**What `Verify` does not close.** It narrows the stale-row window from "up to one refresh interval" to the few
microseconds between its read of `stat` and the `kill(2)` call. It cannot close it: the PID could still be
recycled in between. Only a pidfd (`pidfd_open(2)` plus `pidfd_send_signal(2)`), which refers to one specific
process, would.

**Messages.** Every action reports what happened, naming the action, the PID and the command, for example
`SIGSTOP sent to 4321 (firefox) — stopped` or `nice of 4321 (firefox) set from 0 to 5`. Failures never crash the
program: `process 4321 no longer exists`, `permission denied: only root or the owner of 4321 (firefox) can send
SIGSTOP`, `permission denied: lowering nice needs root (CAP_SYS_NICE); only the owner can raise it`, `invalid
nice "abc": not a whole number`.

