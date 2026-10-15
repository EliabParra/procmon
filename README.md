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

