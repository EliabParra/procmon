# procmon

Un monitor de procesos a pantalla completa para la terminal (al estilo de `top` y `btop`), escrito en Go para la
asignatura de Sistemas Operativos (URU, 2026C, Proyecto 1). Lista todos los procesos con su CPU, memoria, estado y
prioridad, resume el estado de la máquina, muestra un árbol padre/hijo, los hilos y el historial de CPU de un
proceso, y permite terminar, matar, suspender, reanudar y cambiar la prioridad (renice) de los procesos.

Restricciones del enunciado del proyecto, y cómo las cumple el código:

| Restricción | Cómo |
|---|---|
| Los datos de los procesos salen de `/proc` | `internal/procfs` analiza `/proc/[pid]/{stat,status,cmdline}`, `/proc/[pid]/task/*/stat` y `/proc/{stat,meminfo,loadavg,uptime,self/auxv}`. Una lectura adicional: `/etc/passwd`, solo para convertir un UID en un nombre de usuario (sección 5). |
| Ninguna biblioteca que liste procesos o calcule el % de CPU | `go.mod` solo requiere bibliotecas de interfaz: Bubble Tea, Lip Gloss y `x/ansi` (ancho y truncado de texto). Ni gopsutil ni un paquete procfs. |
| Sin programas externos | No hay `os/exec` en el código que no es de prueba. Las señales usan `kill(2)`, la prioridad usa `getpriority(2)` y `setpriority(2)`, todo mediante el paquete `syscall`. |
| El %CPU lo calcula el programa | `internal/monitor/cpu.go`, a partir de dos muestras de los contadores del kernel (sección 6). |

La verificación de la tercera fila no imprime nada:

```sh
rg -n 'os/exec' --glob '*.go' --glob '!*_test.go' .
```

El único uso de `os/exec` en el repositorio es una prueba en `internal/control` que inicia un proceso hijo `sleep`
para tener un proceso inofensivo al que enviarle señales.

## Compilar y ejecutar

```sh
go build -o procmon ./cmd/procmon
./procmon                    # refresca cada 1 s
./procmon -interval 500ms    # cualquier valor de 200ms a 60s
./procmon -plain             # extremos de pill cuadrados, para fuentes sin glifos de Nerd Font
./procmon -cpu-mode core     # arranca con %CPU por núcleo, como top (la tecla I lo cambia en ejecución)
```

- Go 1.27.1 (la línea `go` de `go.mod`). Solo Linux: lee `/proc`.
- Un usuario normal puede ver y leer la mayoría de los procesos. Actuar sobre procesos de otros usuarios requiere
  root (`CAP_KILL` para las señales, `CAP_SYS_NICE` para bajar un valor de nice).
- `q` o Ctrl+C salen de la pantalla alternativa y restauran la terminal. Los flags incorrectos terminan con código
  de salida 2.

## Atajos de teclado

Las teclas de acceso rápido no necesitan Enter; las que envían una señal preguntan `y`/`n` antes. Las principales
también aparecen en la línea de ayuda al pie de la pantalla (muestra tantas como caben).

| Tecla | Dónde | Acción |
|---|---|---|
| `q` | tabla, detalle | Salir |
| `Ctrl+C` | en todas partes, también al escribir o confirmar | Salir |
| `Up`/`k`, `Down`/`j` | tabla | Mover la selección |
| `PgUp`, `PgDn` | tabla | Mover una página |
| `Home`, `End` | tabla | Primera fila (la selección vuelve a seguir la fila superior), última fila |
| `Enter` | tabla | Abrir la vista de detalle del proceso seleccionado |
| `c`, `m`, `p` | tabla | Ordenar por CPU (predeterminado), por memoria, por PID |
| `t` | tabla | Alternar entre la tabla plana y el árbol de procesos |
| `/` | tabla | Filtrar por nombre: línea de comandos o nombre del proceso, sin distinguir mayúsculas; un filtro vacío lo elimina |
| `Esc`, `Enter` | detalle | Volver a la tabla |
| `Up`/`k`, `Down`/`j`, `PgUp`, `PgDn`, `Home`, `End` | detalle | Desplazar la lista de hilos |
| `x` | tabla, detalle | Terminar (SIGTERM), pregunta `y`/`n` antes |
| `X` | tabla, detalle | Matar a la fuerza (SIGKILL), con su propia pregunta |
| `s`, `r` | tabla, detalle | Suspender (SIGSTOP), reanudar (SIGCONT), cada una pregunta `y`/`n` antes |
| `n` | tabla, detalle | Cambiar nice: muestra el valor actual, pide un número entero de -20 a 19 |
| `I` (Shift+i) | tabla, detalle | Alternar el %CPU entre la fracción de toda la máquina (predeterminado) y por núcleo, como el Shift+I de `top`; la pill del encabezado `cpu machine` / `cpu per-core` indica cuál está activo |
| `+`, `-` | tabla, detalle | Intervalo de refresco más largo, más corto (pasos: 0.5, 1, 2, 3, 5, 10 s) |
| `i` | tabla, detalle | Escribir un intervalo de refresco en segundos (0.2 a 60) |

Mientras se escribe una línea (filtro, intervalo, nice): `Enter` aplica, `Esc` cancela, `Backspace` borra, `Ctrl+U`
limpia la línea, el texto pegado se inserta y las teclas de acceso rápido quedan desactivadas. Mientras hay una
confirmación abierta, `y` confirma, `n` o `Esc` cancelan y cualquier otra tecla se ignora. Las acciones se aplican a
la fila seleccionada, o al proceso abierto en la vista de detalle.

La pantalla tiene tres partes: un recuadro de encabezado (cantidad de tareas, carga, uptime, memoria, swap; el
intervalo de refresco, el criterio de orden, el modo de %CPU, la vista y el filtro aparecen como pills en su borde),
el recuadro de la tabla de procesos o del detalle, y una línea de estado/ayuda.
Usa recuadros redondeados desde 40x12 y un diseño compacto por debajo de ese tamaño. Las filas en estado Z (rojo), D
(ámbar) y T/t (violeta) se resaltan, y los contadores del encabezado usan los mismos colores. La última fila siempre
pertenece a esa línea de estado/ayuda, incluso en una terminal de una o dos filas de alto, de modo que una pregunta
o un mensaje nunca queda oculto.

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

## Requirement checklist

| Req | What | Where |
|---|---|---|
| 1 | Table: PID, PPID, user, state, %CPU, %MEM, RSS, threads, priority, CPU time, command | `internal/procfs/process.go` (parse), `internal/monitor/monitor.go` (`readProcess`), `internal/tui/table.go` |
| 2 | Summary: total, running, stopped, zombies, load, memory | `internal/procfs/system.go`, `internal/monitor/monitor.go` (`summarize`), `internal/tui/header.go` |
| 3 | %CPU by delta, coherent with the system | `internal/monitor/cpu.go` (`CPUPercent`, `PerCore`), `internal/monitor/monitor.go` (`Refresh`), display mode in `internal/tui/cpumode.go` |
| 4 | Sort by CPU and by memory | `internal/monitor/sort.go`; keys `c`, `m`, `p` in `internal/tui/keys.go` |
| 5 | Process tree | `internal/monitor/tree.go` (`BuildTree`, `WithAncestors`); `t` in `internal/tui/keys.go`, `rebuild` in `internal/tui/model.go`, connectors in `internal/tui/table.go` |
| 6 | Thread view with per-thread state | `Threads` in `internal/procfs/process.go`, `internal/monitor/threads.go`, `internal/tui/detail.go` |
| 7 | CPU history per process | `internal/monitor/history.go`; sparkline in `internal/tui/chart.go`, shown by `internal/tui/detail.go` |
| 8 | Highlight zombies (Z) and I/O-blocked (D) | `internal/tui/style.go` (`state`), `styleRow` and `paint` in `internal/tui/table.go`, counts in `internal/tui/header.go` |
| 9 | Terminate, force kill (separate confirmation), suspend, resume | `internal/control/control.go`; `internal/tui/actions.go` |
| 10 | Show and change priority | `Priority` and `SetPriority` in `internal/control/control.go`; PRI and NI columns; `n` in `internal/tui/actions.go` |
| 11 | Full-screen TUI, real size, resize, no flicker, header/table/help line, terminal restored | `internal/tui/view.go` (alternate screen, layout, exact-size frames), `internal/tui/footer.go`, `cmd/procmon/main.go` |
| 12 | Refresh interval configurable from the UI | `internal/tui/interval.go`; keys `i`, `+`, `-`; flag `-interval` |
| 13 | Hot keys: quit, interval, sort, filter, tree, threads, act, priority (and `I`, the %CPU mode) | `internal/tui/keys.go`, help line in `internal/tui/footer.go` |
| 14 | Confirmation message naming process and action | `confirmLine` and the notices in `internal/tui/actions.go` |
| 15 | Clear message, no crash, when the process is gone or permission is denied | `internal/oserr/oserr.go`; `Classify` calls in `internal/procfs` and `internal/control`; `failure` in `internal/tui/actions.go`; `Refresh` skips processes that exit while being read |

## Testing

```sh
go test ./...                                   # all packages
go test -cover ./...                            # with statement coverage
go test -race ./internal/tui/ ./internal/monitor/
go vet ./... && gofmt -l .                      # gofmt prints nothing when clean
```

There are 271 test functions. Coverage observed on the last run of `go test -cover ./...`:

| Package | Statements covered |
|---|---|
| `internal/control` | 100.0% |
| `internal/monitor` | 100.0% |
| `internal/tui` | 99.9% |
| `internal/procfs` | 97.5% |
| `internal/oserr` | 83.3% |
| `cmd/procmon` | 39.5% |

What is not covered: in `procfs` and `oserr`, a few error returns (a system file that cannot be read, an error
that is already classified); in `cmd/procmon`, `main` and the thin `controller` adapter, which need a real
terminal. Those were exercised by hand in a terminal multiplexer instead.

- **TDD.** Each unit started red (a failing test, or a build failure for a type that did not exist yet), then
  green, then refactor. The progress log in `odd/tasks/process-monitor.md` lists the tests that failed first and
  the mutation checks (break the code on purpose, confirm a test fails).
- **Fixtures.** `internal/procfs/testdata/proc/` is a small tree laid out like `/proc` (19 files): a process
  with three threads, a kernel thread, a process whose name contains parentheses, a non-numeric directory that
  must be ignored, and the system files. `procfs.NewFS(root)` points the parsers at it, so the tests do not depend on the machine.
- **Fakes for the ports.** `internal/monitor/fake_test.go` is a `Source` whose counters change between two
  refreshes, which is how the %CPU tests work without sleeping. `internal/tui/fakes_test.go` has a fake
  `Monitor` and a fake `Control` that records calls. The screen is tested by sending `tea.KeyPressMsg` and
  `tea.WindowSizeMsg` to the model and reading the frame as text, checking that no line is wider than the terminal.
- **`control` tests.** The three system calls sit behind variables so a test can check exactly what would reach
  the kernel (and simulate `EPERM`). Tests with real processes signal a `sleep` child started by the test; the
  "process is gone" test uses a PID above `pid_max`, so no real signal can reach a recycled PID.
- **Real `/proc`.** `TestRealProc`, `TestRefreshOnTheRealProc` and `TestThreadsOnTheRealProc` read the live
  system and skip when there is no `/proc`.

## Known limitations

- **The state letter is the main thread's.** `/proc/[pid]/stat` reports the state of the thread whose id is the
  PID, while `utime` and `stime` add up all threads. A process burning CPU on another thread can show `S` in the
  table. Observed with a test program whose main thread sleeps while another thread spins: the process showed
  `S`, the spinning thread `R`. This is not guaranteed in either direction (a similar program without pinning
  its main goroutine showed `R`); the detail view lists the state of every thread.
- **`Verify` narrows the PID race, it does not close it** (see the previous section). A pidfd would.
- **Signalling a zombie succeeds and does nothing.** `kill(2)` returns success on a zombie, which stays in state
  Z until its parent reads its exit status. The status line will still say the signal was sent.
- **%CPU is coarse at short intervals** (section 6): the counters tick in units of 10 ms.
- **Priority acts on one thread.** `n` changes the nice of the main thread, not of the other threads of the
  process.
- **Permissions.** Another user's process may hide `status`, `cmdline` or its threads. The row then keeps what
  could be read (USER `?`, command `[comm]`) and the detail view says "permission denied" for the threads.
- **USER needs `/etc/passwd`,** the only file read outside `/proc`. If it is unreadable or a UID is missing, the
  number is shown.
- **Counts by state.** Tasks counts every process read; the other counts cover R, S/I, D, T/t and Z, so a rare
  state (for example X) appears only in the total.
- **History is in memory only:** 60 samples per process (one minute at the default interval), dropped when the
  process exits or the program ends.
- **Suspending under tmux.** The tmux server resumes a stopped pane process of its own right away (checked:
  a direct child of the tmux server was back in state S 0.3 s after SIGSTOP). To demonstrate `s` inside tmux,
  pick a process that is not a direct child of the tmux server, for example run `sh -c 'sleep 300 & wait'` and
  select the `sleep`.
- **Linux only,** because of `/proc` and the Linux behaviour of `getpriority` and `setpriority`.
