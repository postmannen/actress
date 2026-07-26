# Actress — Zig port

A Zig 0.16 port of the Go actress actor framework in the parent directory.
The port keeps the same file structure, names of files, functions, variables
and comments as the Go version, so the two can be compared side by side.
Where Go and Zig fundamentally differ (garbage collection, closures, the
concurrency primitives), the differences are kept as small and as local as
possible, and are documented in comments at the place they occur.

## Building and running

```
zig build                          # builds the library and the examples
zig build run                      # runs the 2actresses example
zig build test                     # runs the ported *_test.go test files
zig build test -Dtest-filter=Name  # runs a single test, like go test -run
./zig-out/bin/smoketest            # runs the smoke test of routers, cbor, factories
```

NB: `zig build test` can print a `failed command: ...` line at the end even
when all tests pass (check the exit code, or use `--summary all`). This is
cosmetic and comes from the Zig 0.16 build runner when a passing test
writes a lot to stderr, which these tests do since they use the debug log
level like the Go tests.

## File mapping

| Go | Zig | |
|---|---|---|
| actress.go | src/actress.zig | Process, NewRootProcess, NewProcess, Act, AddEvent, Stop |
| events.go | src/events.zig | Event, EventName, ETFunc |
| staticprocesses.go | src/staticprocesses.zig | ETRouter and the ET* builtins |
| errorprocesses.go | src/errorprocesses.zig | ERRouter and the ER* builtins |
| dynamicprocesses.go | src/dynamicprocesses.zig | EDRouter, EDSync |
| customprocesses.go | src/customprocesses.zig | ECRouter, ECGeneralDelivery |
| supervisorprocesses.go | src/supervisorprocesses.zig | ESRouter, ESProcesses |
| config.go | src/config.zig | Config, CheckEnv |
| buffer.go | src/buffer.zig | Buffer |
| eventrw.go | src/eventrw.zig | EventRW |
| actress_test.go | src/actress_test.zig | Tests and benchmarks |
| staticProcesses_test.go | src/staticProcesses_test.zig | (empty in Go as well) |
| customprocesses_test.go | src/customprocesses_test.zig | TestECRouter |
| supervisorprocesses_test.go | src/supervisorprocesses_test.zig | TestESProcesses |
| (context stdlib pkg) | src/context.zig | Go-like Context on top of std.Io task cancelation |
| (log/slog stdlib pkg) | src/slog.zig | Runtime leveled logger, slog text format |

## How the Go concurrency maps to Zig std.Io

The port uses the Zig 0.16 `std.Io` interface. The example gets the `Io`
instance and the allocator from `std.process.Init` in main, and every
process carries them in the `io` and `allocator` fields.

| Go | Zig |
|---|---|
| `chan Event` (buffered 10) | `Io.Queue(Event)` with a 10 element buffer |
| `chan Event` (unbuffered) | `Io.Queue(Event)` with a zero length buffer |
| `go fn()` | task spawned into the `Io.Group` of the process context |
| `ctx, cancel := context.WithCancel(ctx)` | `Context.WithCancel(ctx)`, `ctx.cancel()` |
| `select { case ev := <-ch: ...; case <-ctx.Done(): return }` | `const ev = ch.getOne(io) catch return;` — canceling the context cancels the task, and the blocking queue read returns `error.Canceled` |
| `select { case ch <- ev: ...; case <-time.After(5s): ... }` | `Io.Select` with a put task and a sleep task (see `addEventStatic`) |
| `close(readyCh)` + `sync.Once` | `Io.Event.set`, which is idempotent |
| `sync.Mutex` | `Io.Mutex` with `lockUncancelable` |
| `time.Sleep` | `io.sleep` |

## Differences forced by Zig

- **Memory ownership.** Go is garbage collected. In the Zig version
  `AddEvent` takes a deep copy of the event, so the caller keeps ownership
  of what it passes in and can use literals and stack values. A process
  function owns the events it receives from `p.InCh` and must free them
  with `ev.deinit(p.allocator)`, expressed with a `defer` right after the
  event is received so the free also runs on the early return paths. The
  root process got a `Deinit` method that frees the whole process tree at
  shutdown, after the context is canceled.
- **Stop reclaims dynamic and custom processes, through the reaper.** In
  Go a stopped `ED*` or `EC*` process is removed from the processes map
  and the pids map, nothing references it anymore, and the garbage
  collector reclaims it. The Zig `Stop` does the same bookkeeping as the
  Go version, and then hands the process to the reaper task, which does
  the job of the garbage collector: it cancels the process context
  (waiting for the process tasks to finish) and frees the memory of the
  `ED*`/`EC*` process. `ET*`, `ER*` and `ES*` processes can not be
  deleted from their maps, so they stay reachable in Go as well, and
  their memory is freed by `Deinit` at shutdown. Since the cancel and the
  freeing happen in the reaper task and not in `Stop` itself, `Stop` can
  be called from anywhere, also from the process own process function,
  just like in Go. A second call to `Stop` is a no-op. The pointer to a
  stopped `ED*`/`EC*` process is invalid once the reaper has freed it.
  Three rules make the freeing safe against a delivery in flight:
  - The ED/EC routers increment an `inflight` counter on the process
    while holding the map lock of the lookup, and decrement it when the
    delivery is done. The reaper waits for the counter to reach zero
    before freeing.
  - The reaper closes the process `InCh`, so a router blocked putting an
    event to it wakes up with `error.Closed` and frees the event.
  - The reaper removes the process from the registry, so `Deinit` does
    not free it a second time.
  Note that removing a process with
  `DynamicProcesses.Delete`/`CustomProcesses.Delete` only removes the map
  entry, in Zig and in Go alike: the Go version keeps the process
  reachable through the pids map so the garbage collector never frees it
  either. Use `Stop` when the memory of the process should be reclaimed.
- **Closures.** `ETFunc` is a struct of a function pointer plus an optional
  `userdata` pointer instead of a returned closure. Factory functions like
  `ETTestfn(testCh)` and `EDSyncFn(syncCh)` put the captured channel in
  `userdata`. Call sites look the same as in Go.
- **NextEvent pointers.** `Event.NextEvent` is `?*Event`, so an event used
  as the next event must be declared as a `var` and passed with `&`.
- **Event.Err** carries the error message text instead of a Go error value.
- **CBOR** uses the zbor package (fetched by the build). The maps that Go
  encodes with process pointer values are encoded as name/pid to process
  name maps instead, since raw pointers have no meaning without a GC. The
  pid builtins use `copyOfNamesMap`, which dupes the process names while
  the pid map lock is held, so the copy stays valid also when a process
  is stopped and freed while the copy is in use. `copyOfMap` still hands
  out the raw process pointers and is not safe against a concurrent Stop
  of a dynamic or custom process.
- **Config** ports the env variable handling, but not the flag package
  part, since Zig got no flag package in the standard library.
- **Cancel waits, Go cancel signals.** `Cancel` waits for the tasks of
  the process to finish, unlike the Go cancel which just signals. This is
  why `Stop` does not call `Cancel` directly like the Go version does,
  the cancel is done by the reaper task instead, see the Stop bullet
  above.
- **Cleanup with defer.** Owned values are freed with `defer`/`errdefer`
  at the place they are acquired: a process function does
  `defer ev.deinit(p.allocator)` right after `p.InCh.getOne`, which frees
  the event at the end of each loop iteration including the early return
  paths. The exception is a value whose ownership is handed over on
  success, like a buffer put on a test channel, there the value is only
  freed in the failure branch of the put.
- **Tests.** The Go test closures are file scope functions in the test
  files, with captured channels passed via `userdata`, and each test
  creates it's own `Io.Threaded` and uses the leak checking
  `std.testing.allocator`. Direct channel sends in the tests (Go
  `p.StaticEventCh <- ev`) put an owned deep copy on the queue, since the
  receiving process frees what it gets. The Go `Benchmark*` functions are
  ported as tests with a fixed iteration count that print ns/op, since
  Zig got no benchmark framework in the standard library.
