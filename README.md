# procmon

Un monitor de procesos a pantalla completa para la terminal (al estilo de `top` y `btop`), escrito en Go para la
asignatura de Sistemas Operativos (URU, 2026C, Proyecto 1). Lista todos los procesos con su CPU, memoria, estado y
prioridad, resume el estado de la máquina, muestra un árbol padre/hijo, los hilos y el historial de CPU de un
proceso, y permite terminar, matar, suspender, reanudar y cambiar la prioridad (renice) de los procesos.

Restricciones del enunciado del proyecto, y cómo las cumple el código:

| Restricción | Cómo |
|---|---|
| Los datos de los procesos salen de `/proc` | `internal/procfs` analiza `/proc/[pid]/{stat,status,cmdline}`, `/proc/[pid]/task/*/stat` y `/proc/{stat,meminfo,loadavg,uptime,self/auxv}`. Una lectura adicional: `/etc/passwd`, solo para convertir un UID en un nombre de usuario (ver «De dónde sale cada dato»). |
| Ninguna biblioteca que liste procesos o calcule el % de CPU | `go.mod` solo requiere bibliotecas de interfaz: Bubble Tea, Lip Gloss y `x/ansi` (ancho y truncado de texto). Ni gopsutil ni un paquete procfs. |
| Sin programas externos | No hay `os/exec` en el código que no es de prueba. Las señales usan `kill(2)`, la prioridad usa `getpriority(2)` y `setpriority(2)`, todo mediante el paquete `syscall`. |
| El %CPU lo calcula el programa | `internal/monitor/cpu.go`, a partir de dos muestras de los contadores del kernel (ver «Cómo se calcula el %CPU»). |

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

## Arquitectura

Cuatro paquetes y un paquete compartido de errores, conectados en `cmd/procmon/main.go`:

| Paquete | Rol | API principal |
|---|---|---|
| `internal/procfs` | Puerta de lectura. Analiza los archivos de `/proc` y los convierte en structs simples. Sin porcentajes ni deltas. La raíz de `/proc` es inyectable. | `FS` con `PIDs`, `Stat`, `Status`, `Cmdline`, `Threads`, `SystemStat`, `MemInfo`, `LoadAvg`, `Uptime`, `ClockTicks` |
| `internal/control` | Puerta de acción. Señales y prioridad mediante llamadas al sistema, con validación. | `Terminate`, `Kill`, `Suspend`, `Resume`, `Priority`, `SetPriority` |
| `internal/monitor` | Lógica. Snapshots, %CPU a partir de deltas, resumen, orden, filtro, árbol, historial de CPU, UID a nombre. | `Monitor` con `Refresh`, `History`, `Threads`, `Verify`; `Sort`, `Filter`, `BuildTree`, `WithAncestors` |
| `internal/tui` | Pantalla. Modelo de Bubble Tea, diseño, teclas, vistas. Nunca lee `/proc` y nunca envía señales por sí misma. | `New`, `Model`, puertos `Monitor` y `Control` |
| `internal/oserr` | Vocabulario compartido para los dos fallos esperables: el proceso ya no existe, o se denegó el permiso. | `ErrProcessGone`, `ErrPermission`, `Classify` |

Dirección de las dependencias. Una flecha continua significa «importa»; una punteada significa «solo a través de
una interfaz». `cmd/procmon` importa los cuatro paquetes para conectarlos; `internal/oserr` es importado por procfs,
control, monitor y tui.

```
cmd/procmon     solo ensamblaje: importa tui, monitor, procfs y control

internal/tui ------> internal/monitor ------> internal/procfs ------> /proc
     :
     :  Puerto Control (una interfaz; el adaptador vive en cmd/procmon)
     v
internal/control ------> kill(2), getpriority(2), setpriority(2)
```

`internal/tui` no importa ni `procfs` ni `control`: solo conoce las dos interfaces siguientes, y `main.go` conecta
las implementaciones reales.

- **`monitor.Source`** (`internal/monitor/source.go`) es todo lo que el monitor lee del sistema:
  `PIDs`, `Stat`, `Status`, `Cmdline`, `Threads`, `SystemStat`, `MemInfo`, `LoadAvg`, `Uptime`, `ClockTicks`.
  `procfs.FS` es el adaptador; `var _ Source = procfs.FS{}` hace que la compilación falle si deja de encajar.
  Las pruebas usan una fuente falsa (fake) cuyos contadores cambian entre dos refrescos.
- **`tui.Monitor`** (`Refresh`, `History`, `Threads`, `Verify`) lo satisface `*monitor.Monitor`.
  **`tui.Control`** (`Terminate`, `Kill`, `Suspend`, `Resume`, `Priority`, `SetPriority`) lo satisface un pequeño
  adaptador `controller` en `main.go` que reenvía a las funciones de `control`.
  Las pruebas conectan fakes que registran las llamadas, de modo que ninguna prueba de la pantalla envía una señal
  real.

La pantalla sigue la arquitectura Elm de Bubble Tea: un modelo, un `Update`, un `View`.

```
Init           -> Cmd: refresh (en segundo plano) + Cmd: tick tras min(interval, 0.5 s)
tickMsg        -> Update -> Cmd: refresh + Cmd: siguiente tick   (un tick de un intervalo anterior se descarta)
refreshedMsg   -> Update -> guardar el snapshot, reconstruir las filas: filtro -> orden -> árbol
KeyPressMsg    -> Update -> cambiar modo, orden, filtro o selección, o llamar a Control
WindowSizeMsg  -> Update -> nuevo tamaño, se recalcula el diseño
cada mensaje   -> View   -> un frame de exactamente `height` líneas, ninguna más ancha que la terminal
```

`Update` nunca se bloquea: `Refresh` se ejecuta dentro de un `tea.Cmd`, y solo puede haber un refresco en curso. Un
refresco fallido conserva el snapshot anterior en pantalla y muestra "refresh failed: ..." (refresco fallido) en la
línea de estado. El renderer de Bubble Tea dibuja a partir de un buffer de celdas y escribe solo lo que cambió entre
frames, que es lo que evita el parpadeo.

## De dónde sale cada dato

Los números de campo de `stat` siguen `proc(5)`. `comm` (campo 2) puede contener espacios y `)`, por lo que el
parser divide la línea en el último `)` (uno de los fixtures es un proceso llamado `my (weird) app`).

**Tabla de procesos**

| Columna | Fuente |
|---|---|
| PID | Nombres de directorio numéricos bajo `/proc` (`self`, `sys`... se ignoran) |
| PPID | Campo 4 de `/proc/[pid]/stat` |
| USER | Línea `Uid:` de `/proc/[pid]/status` (el primer valor, el UID real), luego el nombre desde `/etc/passwd`. Muestra el número si el UID no figura en la lista, `?` si no se puede leer `status` |
| S | Campo 3 de `stat` (letra de estado) |
| %CPU | Por proceso: campos 14 (`utime`) + 15 (`stime`) de `stat`. Sistema: la línea `cpu` de `/proc/stat`. Véase la sección siguiente |
| %MEM | `VmRSS` de `status` dividido entre `MemTotal` de `/proc/meminfo`, por 100 |
| RES | Línea `VmRSS:` de `status` (kB). Los hilos del kernel no tienen y muestran `0K` |
| THR | Campo 20 de `stat` (`num_threads`) |
| PRI | Campo 18 de `stat` (`priority`, 20 + nice en las tareas normales). Los valores menores que -99 (tiempo real) se muestran como `rt` |
| NI | Campo 19 de `stat` (`nice`) |
| TIME | (`utime` + `stime`) / CLK_TCK. CLK_TCK se lee de `/proc/self/auxv` (entrada `AT_CLKTCK`), 100 si no está disponible |
| COMMAND | `/proc/[pid]/cmdline` con los separadores NUL convertidos en espacios; cuando está vacío (hilos del kernel, zombis) `[comm]` |

**Encabezado**

| Elemento | Fuente |
|---|---|
| `Tasks` | Número de procesos leídos en este snapshot |
| `running`, `sleeping`, `disk-sleep`, `stopped`, `zombie` | Conteo de las letras de estado R, S más I, D, T más t, Z (los hilos del kernel inactivos, estado I, cuentan como sleeping igual que en `top`) |
| `Load` | Campos 1 a 3 de `/proc/loadavg` |
| `up` | Campo 1 de `/proc/uptime` |
| `Mem` used / total, `avail` | `MemTotal` y `MemAvailable` de `/proc/meminfo`; used = `MemTotal` - `MemAvailable` |
| `Swap` | `SwapTotal`, y used = `SwapTotal` - `SwapFree` |
| Cores (factor del %CPU por núcleo) | Número de líneas `cpuN` de `/proc/stat` |

**Vista de detalle**

| Elemento | Fuente |
|---|---|
| Threads: TID, state, TIME, name | `/proc/[pid]/task/[tid]/stat` (campos 1, 3, 14 + 15, 2) |
| Historial de CPU | Las últimas 60 muestras de %CPU del proceso, una por refresco, guardadas en memoria por `monitor` en la unidad de la fórmula del enunciado y dibujadas en el modo de %CPU activo, en un gráfico con autoescala (véase la sección siguiente) |

## Cómo se calcula el %CPU

El enunciado de la asignatura da la fórmula: dividir la diferencia del proceso entre la diferencia del sistema. Eso
es lo que calcula el programa y lo que muestra por defecto:

```
%CPU = (delta de utime+stime del proceso) / (delta de los jiffies totales de /proc/stat) x 100
```

El resultado es la fracción de **toda la máquina**, de 0 a 100. La línea `cpu` de `/proc/stat` suma todos los
núcleos, así que en 4 núcleos un proceso que mantiene un núcleo al máximo da 25. Ejemplo: 4 núcleos, 1 s, 100
ticks/s: el sistema crece 400 ticks, un proceso que creció 100 ticks muestra 100 / 400 x 100 = 25%.

- **Dos muestras.** `Refresh` recuerda, para cada PID, su `utime+stime` y el total del sistema del refresco
  anterior. El total es la suma de las ocho columnas de la línea `cpu` (user, nice, system, idle, iowait, irq,
  softirq, steal; guest se ignora porque el kernel ya lo cuenta dentro de user).
- **Un valor canónico.** `monitor.CPUPercent` devuelve la fórmula anterior y nada más; el ring buffer del historial,
  el orden y toda comparación usan ese valor. El modo de visualización descrito más abajo solo cambia cómo lo dibuja
  la pantalla, así que cambiarlo nunca reordena la tabla ni reescribe el historial.
- **Modo por núcleo (`I`).** Con `I` (Shift+i) se ve lo que `top` muestra por defecto, su modo Irix: la misma cifra
  multiplicada por el número de núcleos (`monitor.PerCore`), de modo que 100 significa un núcleo al máximo y un
  proceso multihilo puede superarlo (hasta 100 x núcleos). El mismo ejemplo da 100 / 400 x 100 x 4 = 100%. `top`
  tiene el mismo interruptor en Shift+I: con el modo Irix activado es por núcleo; desactivado (llamado modo Solaris)
  es la fracción de toda la máquina. Al pulsar `I` de nuevo se vuelve a la fórmula del enunciado. La pill del
  encabezado dice `cpu machine` o `cpu per-core`, un aviso de una línea explica qué significa 100, y `-cpu-mode core`
  arranca en modo por núcleo. El modo se aplica a la tabla, el árbol, la línea de CPU de la vista de detalle y su
  gráfico de historial; en una máquina con 1 núcleo los dos modos son iguales.
- **Escala del gráfico de historial.** La vista de detalle dibuja las últimas 60 muestras como un gráfico de barras
  de una línea. Su tope es la mayor muestra redondeada hacia arriba a un paso de 1, 2, 5, 10, 20, 50, 100, 200,
  500... (1, 2 o 5 veces una potencia de diez), con un mínimo de 1%: un proceso ligero llena el gráfico, uno inactivo
  se queda en el bloque más bajo en lugar de amplificar el ruido de los contadores. La etiqueta `scale 0-10%` junto
  a min, avg y max indica qué representa un bloque de altura completa, y esas tres cifras son siempre las reales.
  El tope nunca supera la máquina completa en el modo activo (100, o 100 x núcleos en modo por núcleo): una muestra
  por encima de eso es una desviación entre las dos lecturas de `/proc` y se dibuja recortada. Ejemplo: las muestras
  1, 6 y 3 dan `scale 0-10%`, y el 6 es la columna más alta.
- **La primera muestra es 0.** Sin una muestra previa no hay delta. Lo mismo vale para un proceso que aparece por
  primera vez en un refresco posterior. La pantalla programa el segundo refresco a más tardar 0.5 s después del
  arranque, para que la columna no se quede en cero durante todo un intervalo.
- **Un PID reciclado no es el mismo proceso.** Las muestras se emparejan por PID y hora de inicio (campo 22 de
  `stat`). El mismo PID con otra hora de inicio es un proceso nuevo: su delta parte de cero y su historial se
  reinicia.
- **Los contadores que retroceden dan 0**, no un número enorme sin signo (resta saturada).
- **Un refresco fallido conserva la línea base anterior**, de modo que el siguiente refresco exitoso todavía tiene
  algo que restar.
- **Sin recorte.** Los dos archivos se leen con un instante de diferencia, así que un proceso muy ocupado puede
  mostrar una fracción de punto por encima de 100 en el modo por defecto; el código muestra lo que da la fórmula.
- **La resolución** es un tick de reloj del proceso. En modo por núcleo mueve el %CPU en 100 / (CLK_TCK x intervalo
  en segundos) puntos: 1 punto a 1 s, 5 puntos en el mínimo de 0.2 s. En el modo por defecto se divide eso entre el
  número de núcleos (0.25 puntos a 1 s con 4 núcleos).

## Actuar sobre procesos

Todas las acciones pasan por `internal/control`, que llama a `syscall.Kill`, `syscall.Getpriority` y
`syscall.Setpriority`. Los números de abajo son para Linux en x86-64 y arm64.

| Tecla | Función | Señal | Efecto | ¿Puede el proceso impedirlo? |
|---|---|---|---|---|
| `x` | `Terminate` | SIGTERM (15) | Le pide al proceso que termine; puede limpiar antes | Sí: puede manejarla o ignorarla |
| `X` | `Kill` | SIGKILL (9) | El kernel termina el proceso; no se ejecuta nada en él | No |
| `s` | `Suspend` | SIGSTOP (19) | Detiene el proceso; se muestra en estado T | No: no se puede capturar, bloquear ni ignorar |
| `r` | `Resume` | SIGCONT (18) | Continúa un proceso detenido | Puede ejecutarse un handler, pero el proceso continúa de todos modos |

**Prioridad.** `n` cambia el valor nice, de -20 (mayor prioridad) a 19 (menor). El propietario puede subir el
valor; bajarlo requiere root o `CAP_SYS_NICE` (el kernel también lo permite dentro del `RLIMIT_NICE` del objetivo).
En Linux el valor nice pertenece a un solo hilo, así que `getpriority`/`setpriority` con un PID actúan sobre el hilo
principal, el que reporta `/proc/[pid]/stat`.

**El detalle de `20 - nice`.** La llamada al sistema `getpriority` cruda devuelve `20 - nice` (1 a 40), para que -1
nunca parezca un error. glibc lo convierte de vuelta; el paquete `syscall` de Go no, así que `control.Priority`
devuelve `20 - raw`. Nice 10 es el punto fijo de esa fórmula, por eso la prueba de ida y vuelta (round-trip) usa
también 5 y 15.

**Salvaguardas**

1. `control` rechaza `pid <= 0` antes de cualquier llamada al sistema: `kill(0, sig)` envía la señal a todo el grupo
   de procesos del llamador y `kill(-1, sig)` a todos los procesos a los que el llamador puede enviarla.
2. La pantalla se niega a actuar sobre el PID del propio procmon (`WithSelfPID`, por defecto `os.Getpid()`). Lo
   comprueba al pulsar la tecla y de nuevo justo antes de la señal o de `setpriority`.
3. El objetivo (PID, hora de inicio, nombre) se captura cuando comienza la acción, de modo que un refresco con una
   pregunta abierta no puede redirigir la respuesta. Justo antes de cada señal o `setpriority` la pantalla llama a
   `Monitor.Verify`, que vuelve a leer `/proc/[pid]/stat` y compara la hora de inicio. Si el proceso ya no existe o
   el PID ahora pertenece a otro proceso, no se envía nada y el mensaje dice "process N no longer exists" (el
   proceso N ya no existe).
4. Toda tecla que envía una señal pregunta primero, nombrando la acción, el PID y el comando. El kill forzado tiene
   su propio texto: se dibuja en el color de alarma y dice "the process cannot clean up" (el proceso no puede
   limpiar). Mientras hay una pregunta abierta, solo `y`, `n` y `Esc` la responden. En una línea estrecha se recorta
   primero la explicación y luego el final del nombre; el PID y la respuesta se conservan (el "Press y to confirm, n
   to cancel" del kill forzado pasa a `y/n` por debajo de 60 columnas).
5. El texto que viene de `/proc` se sanea antes de dibujarse: los caracteres de control (un ESC iniciaría una
   secuencia de terminal) y el UTF-8 inválido se convierten en `?`.

**Lo que `Verify` no cierra.** Reduce la ventana de la fila desactualizada (stale) de "hasta un intervalo de
refresco" a los pocos microsegundos entre su lectura de `stat` y la llamada a `kill(2)`. No puede cerrarla: el PID
todavía podría reciclarse entre ambos momentos. Solo un pidfd (`pidfd_open(2)` más `pidfd_send_signal(2)`), que se
refiere a un proceso específico, lo lograría.

**Mensajes.** Toda acción informa qué ocurrió, nombrando la acción, el PID y el comando, por ejemplo
`SIGSTOP sent to 4321 (firefox) — stopped` o `nice of 4321 (firefox) set from 0 to 5`. Los fallos nunca hacen caer
el programa: `process 4321 no longer exists`, `permission denied: only root or the owner of 4321 (firefox) can send
SIGSTOP`, `permission denied: lowering nice needs root (CAP_SYS_NICE); only the owner can raise it`, `invalid
nice "abc": not a whole number`. (La interfaz está en inglés; en orden, los cuatro mensajes dicen: el proceso ya no
existe; permiso denegado para enviar la señal; permiso denegado para bajar el nice; nice inválido, no es un número
entero.)

## Requisitos del enunciado

| Req | Qué | Dónde |
|---|---|---|
| 1 | Tabla: PID, PPID, usuario, estado, %CPU, %MEM, RSS, hilos, prioridad, tiempo de CPU, comando | `internal/procfs/process.go` (análisis), `internal/monitor/monitor.go` (`readProcess`), `internal/tui/table.go` |
| 2 | Resumen: total, en ejecución, detenidos, zombis, carga, memoria | `internal/procfs/system.go`, `internal/monitor/monitor.go` (`summarize`), `internal/tui/header.go` |
| 3 | %CPU por delta, coherente con el sistema | `internal/monitor/cpu.go` (`CPUPercent`, `PerCore`), `internal/monitor/monitor.go` (`Refresh`), modo de visualización en `internal/tui/cpumode.go` |
| 4 | Ordenar por CPU y por memoria | `internal/monitor/sort.go`; teclas `c`, `m`, `p` en `internal/tui/keys.go` |
| 5 | Árbol de procesos | `internal/monitor/tree.go` (`BuildTree`, `WithAncestors`); `t` en `internal/tui/keys.go`, `rebuild` en `internal/tui/model.go`, conectores en `internal/tui/table.go` |
| 6 | Vista de hilos con el estado de cada hilo | `Threads` en `internal/procfs/process.go`, `internal/monitor/threads.go`, `internal/tui/detail.go` |
| 7 | Historial de CPU por proceso | `internal/monitor/history.go`; sparkline en `internal/tui/chart.go`, mostrado por `internal/tui/detail.go` |
| 8 | Resaltar zombis (Z) y procesos bloqueados por E/S (D) | `internal/tui/style.go` (`state`), `styleRow` y `paint` en `internal/tui/table.go`, contadores en `internal/tui/header.go` |
| 9 | Terminar, matar a la fuerza (confirmación aparte), suspender, reanudar | `internal/control/control.go`; `internal/tui/actions.go` |
| 10 | Mostrar y cambiar la prioridad | `Priority` y `SetPriority` en `internal/control/control.go`; columnas PRI y NI; `n` en `internal/tui/actions.go` |
| 11 | TUI a pantalla completa, tamaño real, redimensionado, sin parpadeo, encabezado/tabla/línea de ayuda, terminal restaurada | `internal/tui/view.go` (pantalla alternativa, diseño, frames de tamaño exacto), `internal/tui/footer.go`, `cmd/procmon/main.go` |
| 12 | Intervalo de refresco configurable desde la interfaz | `internal/tui/interval.go`; teclas `i`, `+`, `-`; flag `-interval` |
| 13 | Teclas de acceso rápido: salir, intervalo, orden, filtro, árbol, hilos, actuar, prioridad (y `I`, el modo de %CPU) | `internal/tui/keys.go`, línea de ayuda en `internal/tui/footer.go` |
| 14 | Mensaje de confirmación que nombra el proceso y la acción | `confirmLine` y los avisos (notices) en `internal/tui/actions.go` |
| 15 | Mensaje claro, sin caída del programa, cuando el proceso ya no existe o se deniega el permiso | `internal/oserr/oserr.go`; llamadas a `Classify` en `internal/procfs` y `internal/control`; `failure` en `internal/tui/actions.go`; `Refresh` omite los procesos que terminan mientras se leen |

## Pruebas

```sh
go test ./...                                   # todos los paquetes
go test -cover ./...                            # con cobertura de sentencias
go test -race ./internal/tui/ ./internal/monitor/
go vet ./... && gofmt -l .                      # gofmt no imprime nada si todo está limpio
```

Hay 271 funciones de prueba. Cobertura observada en la última ejecución de `go test -cover ./...`:

| Paquete | Sentencias cubiertas |
|---|---|
| `internal/control` | 100.0% |
| `internal/monitor` | 100.0% |
| `internal/tui` | 99.9% |
| `internal/procfs` | 97.5% |
| `internal/oserr` | 83.3% |
| `cmd/procmon` | 39.5% |

Lo que no está cubierto: en `procfs` y `oserr`, algunos retornos de error (un archivo del sistema que no se puede
leer, un error que ya está clasificado); en `cmd/procmon`, `main` y el delgado adaptador `controller`, que necesitan
una terminal real. Esos se ejercitaron a mano en un multiplexor de terminal.

- **TDD.** Cada unidad empezó en rojo (una prueba que falla, o un fallo de compilación por un tipo que aún no
  existía), luego verde y luego refactor. El registro de progreso de `odd/tasks/process-monitor.md` enumera las
  pruebas que fallaron primero y las comprobaciones de mutación (romper el código a propósito y confirmar que una
  prueba falla).
- **Fixtures.** `internal/procfs/testdata/proc/` es un árbol pequeño con la misma estructura que `/proc` (19
  archivos): un proceso con tres hilos, un hilo del kernel, un proceso cuyo nombre contiene paréntesis, un directorio
  no numérico que debe ignorarse y los archivos del sistema. `procfs.NewFS(root)` apunta los parsers hacia él, de
  modo que las pruebas no dependen de la máquina.
- **Fakes para los puertos.** `internal/monitor/fake_test.go` es un `Source` cuyos contadores cambian entre dos
  refrescos, que es como funcionan las pruebas de %CPU sin esperas. `internal/tui/fakes_test.go` tiene un `Monitor`
  falso y un `Control` falso que registra las llamadas. La pantalla se prueba enviando `tea.KeyPressMsg` y
  `tea.WindowSizeMsg` al modelo y leyendo el frame como texto, comprobando que ninguna línea es más ancha que la
  terminal.
- **Pruebas de `control`.** Las tres llamadas al sistema están detrás de variables para que una prueba pueda
  comprobar exactamente qué llegaría al kernel (y simular `EPERM`). Las pruebas con procesos reales envían señales a
  un hijo `sleep` iniciado por la prueba; la prueba de "el proceso ya no existe" usa un PID por encima de `pid_max`,
  de modo que ninguna señal real puede alcanzar un PID reciclado.
- **`/proc` real.** `TestRealProc`, `TestRefreshOnTheRealProc` y `TestThreadsOnTheRealProc` leen el sistema en vivo
  y se omiten (skip) cuando no hay `/proc`.

## Limitaciones conocidas

- **La letra de estado es la del hilo principal.** `/proc/[pid]/stat` reporta el estado del hilo cuyo id es el PID,
  mientras que `utime` y `stime` suman todos los hilos. Un proceso que consume CPU en otro hilo puede mostrar `S` en
  la tabla. Observado con un programa de prueba cuyo hilo principal duerme mientras otro hilo gira (spin): el proceso
  mostró `S`, el hilo que giraba `R`. Esto no está garantizado en ningún sentido (un programa similar sin fijar su
  goroutine principal mostró `R`); la vista de detalle lista el estado de cada hilo.
- **`Verify` reduce la carrera del PID, no la cierra** (véase la sección anterior). Un pidfd sí la cerraría.
- **Señalar un zombi tiene éxito y no hace nada.** `kill(2)` devuelve éxito sobre un zombi, que permanece en estado
  Z hasta que su padre lee su estado de salida. La línea de estado igualmente dirá que se envió la señal.
- **El %CPU es grueso con intervalos cortos** (ver «Cómo se calcula el %CPU»): los contadores avanzan en unidades de 10 ms.
- **La prioridad actúa sobre un hilo.** `n` cambia el nice del hilo principal, no el de los demás hilos del
  proceso.
- **Permisos.** El proceso de otro usuario puede ocultar `status`, `cmdline` o sus hilos. La fila conserva entonces
  lo que se pudo leer (USER `?`, comando `[comm]`) y la vista de detalle dice "permission denied" para los hilos.
- **USER necesita `/etc/passwd`,** el único archivo leído fuera de `/proc`. Si no se puede leer o falta un UID, se
  muestra el número.
- **Conteos por estado.** `Tasks` cuenta todos los procesos leídos; los demás conteos cubren R, S/I, D, T/t y Z, así
  que un estado raro (por ejemplo X) aparece solo en el total.
- **El historial solo está en memoria:** 60 muestras por proceso (un minuto con el intervalo por defecto), se
  descartan cuando el proceso termina o el programa finaliza.
- **Suspender bajo tmux.** El servidor de tmux reanuda de inmediato, por su cuenta, un proceso de panel detenido
  (comprobado: un hijo directo del servidor de tmux volvió al estado S 0.3 s después de SIGSTOP). Para demostrar `s`
  dentro de tmux, se elige un proceso que no sea hijo directo del servidor de tmux, por ejemplo se ejecuta
  `sh -c 'sleep 300 & wait'` y se selecciona el `sleep`.
- **Solo Linux,** por `/proc` y por el comportamiento de Linux de `getpriority` y `setpriority`.
