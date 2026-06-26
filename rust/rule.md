# Actress (Rust port) — Architecture & Usage Guide

This document describes the Rust port of the `actress` actor framework living in
the `rust/` directory. It is written so the library can be used and modified
later without re-reading every source file. It assumes familiarity with the Go
version (`.claude/rules/actress-rule.md` in the repo root); where the Rust port
diverges, the difference is called out explicitly.

---

## 0. The one-paragraph mental model

A system is a tree of **processes** (actors). Each process runs its body in its
own OS thread and owns an unbuffered input channel `InCh`. Processes never call
each other directly — they build an `Event`, hand it to `AddEvent`, and a
**router** thread looks the target up by event name and pushes it onto that
process's `InCh`. The event name's second character (`T/R/D/C/S`) decides which
of five shared channels (and therefore which router and which process map) the
event travels through. The whole tree shares those channels and maps via `Arc`;
cancellation flows top-down through a `Context` tree that mirrors the process
tree.

---

## 1. Crate layout

| File | Responsibility |
|------|----------------|
| `src/lib.rs` | Module declarations + flat re-export of the public API at the crate root |
| `src/actress.rs` | `Process`, `NewRootProcess`, `NewProcess`, `AddEvent`, `Act`/`actForRoot`, `Stop`, pids, event counter, `Chan`, `ETFunc`/`ProcFn` |
| `src/events.rs` | `Event`, `EventName`, `Instruction`, `Node`, `NewEvent` + option helpers |
| `src/context.rs` | `Context` — the cancellation tree (replaces Go `context.Context`) |
| `src/logging.rs` | Leveled logging + `log_error!`/`log_info!`/`log_debug!` macros |
| `src/config.rs` | `Config`, `NewConfig` |
| `src/staticprocesses.rs` | `ETRouter` + all built-in `ET*` processes, `CopyEventFields` |
| `src/errorprocesses.rs` | `ERRouter` + `ER*` processes |
| `src/dynamicprocesses.rs` | `EDRouter`, `EDSync`, `NewUUID` |
| `src/customprocesses.rs` | `ECRouter`, `ECGeneralDelivery` |
| `src/supervisorprocesses.rs` | `ESRouter`, `ESProcesses` |
| `src/eventrw.rs` | `EventRW` — `io::Read`/`io::Write` over an event |
| `src/buffer.rs` | thread-safe FIFO `Buffer` |
| `examples/2actresses/main.rs` | worked example; the canonical "how a user uses this" |

Everything is re-exported flat, so users write `actress::Event`,
`actress::NewRootProcess`, etc. — mirroring Go's single-package namespace.

**Naming:** Go's exported PascalCase names are kept verbatim (`AddEvent`, `Act`,
`Process`, `Event`, field names like `InCh`, `NextEvent`). The crate sets
`#![allow(non_snake_case, non_upper_case_globals, non_camel_case_types)]` so this
compiles cleanly. Do the same (`#![allow(non_snake_case, non_upper_case_globals)]`)
at the top of any binary/example that uses Go-style names.

---

## 2. How to use the library

Minimal flow (see `examples/2actresses/main.rs` for the full version):

```rust
#![allow(non_snake_case, non_upper_case_globals)]
use std::sync::Arc;
use actress::*;

// An ETFunc: takes (Context, Arc<Process>) and returns the loop body (ProcFn).
fn myFunc(ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            crossbeam_channel::select! {
                recv(p.InCh.rx) -> msg => {
                    let ev = match msg { Ok(e) => e, Err(_) => return };
                    // ... do work, then forward:
                    p.AddEvent(Event { Name: ERLog, /* ... */ ..Default::default() });
                }
                recv(ctx.done()) -> _ => return,
            }
        }
    })
}

fn main() {
    let ctx = Context::background();
    let cfg = NewConfig("info");
    let root = NewRootProcess(&ctx, None, cfg);

    const MyEvent: EventName = EventName::new("ETMine"); // must start with E + T/R/D/C/S

    let p = NewProcess(&root.Ctx, &root, MyEvent, Box::new(myFunc));
    p.Act().unwrap();        // start the user process
    root.Act().unwrap();     // start root (registers ETRoot)

    root.AddEvent(Event { Name: MyEvent, Data: b"hi".to_vec(), ..Default::default() });
}
```

Rules that matter when writing a process body:
- **Always `select!` on both `p.InCh.rx` and `ctx.done()`** (or `p.Ctx.done()`,
  same context). A `recv` returning `Err` means the channel disconnected — treat
  it as shutdown and `return`.
- **Forward work via `p.AddEvent(event)`**, never by touching another process.
- **To synchronize with tests**, send on `p.TestCh.tx` and read `p.TestCh.rx` in
  the test/main code.
- **Call `p.SignalReady()`** once near the top of the body if `Act` should not
  fall back to its 5 ms readiness timeout (see §6).

### Constructing events

Struct literal with `..Default::default()` is the idiomatic form (the Go code
uses `Event{...}` with zero-value fields):

```rust
Event { Name: ETPrint, Data: b"x".to_vec(), ..Default::default() }
```

`NextEvent` is `Option<Box<Event>>`:

```rust
Event {
    Name: ETTest1,
    NextEvent: Some(Box::new(Event { Name: ETTest2, ..Default::default() })),
    ..Default::default()
}
```

There is also the functional-options constructor mirroring Go's `NewEvent`:
`NewEvent(name, vec![EvCmd(...), EVData(...), EvNext(...)])`.

---

## 3. Event types & process maps

Identical taxonomy to Go. The event name's **second byte** selects everything:

| Prefix | Category | Map type | Deletable |
|--------|----------|----------|-----------|
| `ET*` | Static | `staticProcesses` | No |
| `ED*` | Dynamic | `dynamicProcesses` | Yes |
| `ER*` | Error | `errorProcesses` | No |
| `EC*` | Custom | `customProcesses` | Yes |
| `ES*` | Supervisor | `supervisorProcesses` | No |

Each map type holds `procMap: Mutex<HashMap<EventName, Arc<Process>>>`. An event
name **must** be `E` + one of `T/R/D/C/S` + more; `AddEvent` panics otherwise.

**Deviation from Go:** the Go `errorProcesses` struct intentionally has *no*
mutex. Rust cannot share a `HashMap` across threads without synchronization, so
this port wraps it in a `Mutex` like the others. Behavior is otherwise identical.

---

## 4. Routing pipeline

`AddEvent(event)`:
1. Assign `event.Nr` from the shared counter (`IncrementEventNr`).
2. If `event.DstNode` is non-empty **and** differs from `Config.NodeName`: wrap
   the original event inside a new `ETRemote` event (original goes in
   `NextEvent`) and send it to the static channel. Return.
3. Otherwise match `name[1]`:
   `T → StaticEventCh`, `R → ErrorEventCh`, `D → DynamicEventCh`,
   `C → CustomEventCh`, `S → SupervisorEventCh`.

Each shared channel is consumed by exactly one **router thread**. Every router:
1. `select!`s on its channel + `ctx.done()`.
2. If `ev.NextEvent` is set, copies the current event's metadata into
   `ev.NextEvent.PreviousEvent` via `CopyEventFields` (Nr, Name, Cmd,
   Instruction, Err, DstNode, SrcNode — **not** Data/NextEvent/PreviousEvent).
3. Looks up the target in the relevant map, clones its `InCh.tx`, releases the
   lock, then sends.

Router-specific behavior preserved from Go:
- **`ETRouter`** also handles `ETRemote`: logs an error if no `ETRemote` process
  is registered, otherwise routes normally. Has `defer p.Stop()` equivalent: it
  calls `p.Stop()` after the loop breaks.
- **`EDRouter` / `ECRouter`**: if the target isn't registered yet, spawn a thread
  that retries up to 3×. `EDRouter` sleeps 1 s between tries; `ECRouter` spins
  without sleeping (faithful to the Go code). The router itself does **not**
  block — it `continue`s to the next event.
- **`ERRouter` / `ESRouter`**: direct lookup, log + `continue` if missing.

**Why the lookup clones `InCh.tx` and drops the lock before sending:** `InCh` is
unbuffered, so the send blocks until the target receives. Holding the map mutex
across that blocking send would be a deadlock risk (and would serialize the whole
system). Clone the sender, unlock, then send. This is the single most important
implementation detail to preserve when editing a router.

---

## 5. The `Chan` abstraction (Go `chan Event` → Rust)

Go's `chan Event` is one value usable for both send and receive. crossbeam
splits a channel into a `Sender` and a `Receiver`, so the port bundles them:

```rust
pub struct Chan { pub tx: Sender<Event>, pub rx: Receiver<Event> }
```

- Cloning a `Chan` clones both ends; both still refer to the **same** underlying
  channel (like copying a Go channel value by reference). This is how children
  share the root's channels.
- `Chan::bounded(10)` — shared event channels (`StaticEventCh`, etc.), matching
  Go's `make(chan Event, 10)`.
- `Chan::unbuffered()` = `bounded(0)` — a process's `InCh`, matching Go's
  `make(chan Event)` (rendezvous: the router's send blocks until the body
  receives).

When you need to read a channel in user code (e.g. the test channel), use the
`.rx` / `.tx` halves: `ac2.TestCh.rx.recv()` is the Rust form of Go's
`<-ac2.TestCh`; `p.TestCh.tx.send(ev)` is the Go `p.TestCh <- ev`.

**Why crossbeam and not `std::sync::mpsc` or tokio:** the process loops need to
`select!` over the input channel *and* a cancellation channel simultaneously,
exactly like Go's `select`. `std::mpsc` has no select; tokio would force the
whole codebase async and change the structure. crossbeam-channel gives MPMC
channels + a `select!` macro that maps almost line-for-line onto Go's `select`.

---

## 6. Process lifecycle

### `NewRootProcess(ctx, fn: Option<ETFunc>, conf) -> Arc<Process>`
1. Sets the log level (env `LOGLEVEL` wins over `conf.LogLevel`).
2. Builds the root `Process` with fresh buffered channels (cap 10), fresh maps,
   pid allocator, and event counter; `Event = ETRoot`.
3. Wraps it in `Arc`, then (if `fn` is `Some`) builds the body by calling the
   `ETFunc` with `(root.Ctx, root.clone())` and stores it.
4. Starts all built-ins via `actForRoot` (routers, error processes, supervisor,
   misc `ET*`). Order matters: routers + error processes must exist before other
   processes register.
5. `RegisterProcessesInESProcesses` — a **no-op** (the Go body is commented out;
   kept as a no-op here for parity).

### `NewProcess(ctx, parent, event, fn: ETFunc) -> Arc<Process>`
1. Derives a child `Context` via `Context::with_cancel(ctx)`.
2. Creates a **fresh unbuffered `InCh`**; clones the parent's shared channels and
   `Arc`-shares all maps, the config, pid allocator, and event counter.
3. Assigns a new pid from the shared allocator.
4. Wraps in `Arc`, builds the body (`fn(arc.Ctx.clone(), arc.clone())`), stores
   it. **Does not start the process** — call `Act`.

### `Act(self: &Arc<Self>)` (user processes)
Adds pid → map, registers in the process map, takes the body out of its cell,
spawns it on a thread, then `WaitForReady()`.

### `actForRoot` (root built-ins, internal)
Same registration, spawns the body, but **skips** the readiness wait (routers
must come up without anyone to signal them).

### `Stop(&self)`
Cancels the context, deletes from the process map (no-op for `ET`/`ER`/`ES`),
removes the pid.

### Readiness — `SignalReady` / `WaitForReady`
- Each process has a ready channel created at construction.
- `SignalReady()` drops the sender exactly once (via `std::sync::Once`), which
  disconnects the receiver — the "channel closed" signal.
- `WaitForReady()` `select!`s the ready receiver against a 5 ms timer. If the
  body forgot to call `SignalReady`, the timer fires and we proceed anyway
  (matches Go's 5 ms fallback).

---

## 7. `Context` — the cancellation tree

Replaces the slice of Go `context.Context` behavior actually used:
- `Context::background()` — a never-cancelled root (Go `context.Background()`).
- `Context::with_cancel(parent) -> (Context, CancelFn)` — child registered under
  the parent; cancelling the parent recursively cancels children.
- `ctx.done() -> &Receiver<()>` — use in `select!`; becomes ready when cancelled
  (Go `<-ctx.Done()`).
- `ctx.is_done() -> bool`.

**Implementation detail:** "done" is signalled by *disconnecting* a crossbeam
channel — `with_cancel` holds the `Sender` inside the cancel closure; calling
cancel drops it, which makes `done()`'s receiver report ready. There is no
payload; the disconnect *is* the signal. The process tree's `Cancel` field (an
`Arc<dyn Fn() + Send + Sync>`) is this closure, so replacing a process in a map
(`addToProcessesMap` finds an existing entry) cancels the old one by calling it.

Known minor leak (also present in Go's model): a parent keeps child contexts in a
`Vec` and never prunes them on the child's own cancel, so a long-running system
that spawns many short-lived dynamic processes grows that vector. Acceptable for
the current scope; prune in `Stop` if it ever matters.

---

## 8. `ETFunc` / `ProcFn` — the two-level function shape

```rust
pub type ProcFn = Box<dyn FnOnce() + Send + 'static>;            // the loop body (Go: inner func())
pub type ETFunc = Box<dyn FnOnce(Context, Arc<Process>) -> ProcFn + Send + 'static>; // Go: ETFunc
```

- A plain `fn(Context, Arc<Process>) -> ProcFn` (like `etPrintFn`) is passed as
  `Box::new(etPrintFn)`.
- A **factory** that captures state (like `ETTestfn(testCh)` or `esProcessesFn()`)
  returns an `ETFunc` directly — an outer closure that, when called with
  `(ctx, p)`, returns the body closure. This is the Rust form of Go's
  `func ETTestfn(ch) ETFunc { return func(ctx,p) func(){ ... } }`.
- Both are `FnOnce`: the `ETFunc` is invoked once (in `NewProcess`) to build the
  body; the body runs once on its thread. Captured `Arc<Process>` and `Context`
  are `Send + 'static`, which is what lets the body move onto a thread.

The chicken-and-egg of "the body needs `Arc<Process>`, but the process needs the
body": solved by storing the body in `fnCell: Mutex<Option<ProcFn>>`. The process
is `Arc`-wrapped *first*, then the `ETFunc` is called with `arc.clone()`, then the
result is dropped into the cell. `Act`/`actForRoot` `take()` it out to spawn it.

---

## 9. Shared state model

| Go (pointer-shared) | Rust |
|---------------------|------|
| `*Process` in maps | `Arc<Process>` |
| shared `chan Event` fields | `Chan` (cloned; same underlying channel) |
| `*staticProcesses` etc. | `Arc<staticProcesses>` with `Mutex<HashMap<…>>` |
| `*Config`, `*pids`, `*eventNr` | `Arc<Config>`, `Arc<pids>`, `Arc<eventNr>` |
| per-process unique `InCh` | fresh `Chan::unbuffered()` per process |

`Process` is `Send + Sync` (every field is), which is required to store it in
`Arc` shared across router/body threads. There are reference cycles
(`Process → pids → Arc<Process>`, and maps → `Arc<Process>`); they are the global
registry that lives for the program's duration, so the leak is intentional and
harmless. `Stop` removes a process from its map and the pid table.

---

## 10. Built-in processes (same set as Go)

Routers: `ETRouter`, `ERRouter`, `EDRouter`, `ECRouter`, `ESRouter`.

Static: `ETRoot` (user/none), `ETRemote` (user-supplied transport — no built-in),
`ETOsSignal`, `ETTestCh`, `ETPrint`, `ETExit`, `ETTest` (factory `ETTestfn`),
`ETPid`, `ETPidGetAll`, `ETReadFile`, `ETDone`.

Error: `ERLog` (switches on `Instruction`: `InstructionError/Info/Debug` →
log; `InstructionFatal` → log + `exit(1)`), `ERTest`, `ERNone` (drops).

Dynamic: `EDSync` (factory `EDSyncFn(syncCh)`).

Custom: `ECGeneralDelivery`.

Supervisor: `ESProcesses` (CBOR `Add`/`Delete`/`GetAll`).

Built-in deviations to remember:
- **`ETOsSignal`** uses the `ctrlc` crate (Go used `signal.Notify`). The handler
  is installed under a global `Once`, so creating more than one root process in a
  program won't panic on a second `set_handler`. The process then blocks on
  `ctx.done()`.
- **`ETPidGetAll`** CBOR-encodes a `HashMap<pidnr, String>` (pid → event name),
  **not** the full pid → `*Process` map. Encoding the live process graph
  (channels, contexts, back-refs) isn't serializable or meaningful; the name map
  is the production-sensible equivalent.
- **`ESProcesses`** uses `serde` + `ciborium` for the small structs
  (`esProcessesMapDataIn`, `ESProcessesMap`). `EventName` derives
  `Serialize/Deserialize` so it can be a CBOR map key.

---

## 11. The `Event` type — what's different from Go

```rust
pub struct Event {
    pub Nr: i64,
    pub Name: EventName,
    pub Cmd: Vec<String>,
    pub Instruction: Instruction,
    pub Data: Vec<u8>,
    pub Err: Option<String>,           // Go used `error`; modeled as Option<String>
    pub NextEvent: Option<Box<Event>>, // Box because Event is recursive
    pub PreviousEvent: Option<Box<Event>>,
    pub DstNode: Node,
    pub SrcNode: Node,
}
```

- `EventName`, `Instruction`, `Node` are newtypes around `Cow<'static, str>`. The
  `Cow` is the key trick: it lets built-in names be **`const`**
  (`EventName::new("ETRoot")` → `Cow::Borrowed`) while runtime names (e.g. the
  UUID dynamic names) own their `String` (`from_string` → `Cow::Owned`). Equality
  and hashing compare the string contents, so a borrowed and an owned name with
  the same text are equal and hash identically — essential for map lookups.
- `Event.InternalCh` (Go `chan chan []byte`, tagged not-serialized) is **omitted**
  — unused by the examples and not portable as a value type.
- `Err` is `Option<String>`. Build with `Some("msg".to_string())`.

---

## 12. Logging

`src/logging.rs` replaces Go's `slog`. A global `AtomicU8` holds the level
(`DEBUG < INFO < ERROR < NONE`); `set_level_str` maps the strings
`debug/info/error/fatal/none` (env `LOGLEVEL` wins, set in `NewRootProcess`).
`fatal` maps to error level (the actual `exit` happens at the fatal call site,
e.g. `ERLog` + `InstructionFatal`). `none` disables everything.

Use the macros `log_error!`, `log_info!`, `log_debug!` (format-string args).
They gate on the level *before* formatting, so disabled logs cost nothing. The
`"Starting actor for Name: …"` lines use plain `println!` gated by
`Config.LogLevel != "none"`, matching Go's `log.Printf`.

---

## 13. Gotchas / things that bit during the port (read before editing)

1. **Don't hold a map mutex across an `InCh` send.** `InCh` is unbuffered; the
   send blocks until the body receives. Clone `proc.InCh.tx`, drop the lock, then
   send. (§4)
2. **`select!` receiver references.** Pass `p.InCh.rx` and `ctx.done()` to
   `crossbeam_channel::select!{ recv(...) -> msg => ... }`. A `recv` arm yields a
   `Result`; `Err` = disconnected = shut down.
3. **Glob-import name clash.** `lib.rs` keeps the core module **private**
   (`mod actress;`) and re-exports its items, because a public `pub mod actress`
   would collide with the crate name `actress` under a user's `use actress::*`.
   Don't make it public again.
4. **`const` event names need the `Cow` newtype.** Adding a new built-in name is
   `pub const EXYZ: EventName = EventName::new("EXYZ");`. Runtime-built names use
   `EventName::from_string(s)`.
5. **Body must be `Send + 'static`.** It captures `Arc<Process>` + `Context`
   (both `Send + 'static`). If you capture something non-`Send`, the
   `thread::spawn` in `Act` won't compile.
6. **Readiness fallback is 5 ms.** If a body never calls `SignalReady`, `Act`
   returns after 5 ms regardless. Long setup before signalling can race;
   `SignalReady` early.
7. **Root and first child share pid 0.** Faithful to Go: root takes `pids.nr`
   (0) without consuming it; the first `NewProcess` call's `next()` also returns
   0. Don't "fix" this unless the Go side changes.
8. **`ETRemote` has no implementation.** It's just a constant. For distributed
   use, register your own `ETFunc` for `ETRemote` that reads `ev.NextEvent`
   (the original event), serializes it, ships it, and sets `SrcNode`. The
   receiving node deserializes and calls `AddEvent`, clearing `DstNode` to avoid
   a forwarding loop.

---

## 14. Build & run

```sh
cd rust
cargo build
cargo run --example 2actresses      # prints: The result: TEST...
```

Dependencies and why: `crossbeam-channel` (channels + `select!`), `ctrlc`
(SIGINT for `ETOsSignal`), `uuid` (`NewUUID`), `serde` + `ciborium` (CBOR for the
supervisor/pid processes).
