# Actress (Rust port)

A Rust port of the Go `actress` actor framework. The structure follows the Go
code as closely as practical: the same files, the same type and function names,
and the same routing architecture.

## Running the example

```sh
cd rust
cargo run --example 2actresses
```

This is a port of `examples/2actresses/main.go` and prints `The result: TEST...`,
the same as the Go version.

## File mapping

| Go file                  | Rust file                  | Contents |
|--------------------------|----------------------------|----------|
| `actress.go`             | `src/actress.rs`           | `Process`, `NewRootProcess`, `NewProcess`, `AddEvent`, `Act`, the pid and event-number bookkeeping |
| `events.go`              | `src/events.rs`            | `Event`, `EventName`, `Instruction`, `Node`, `NewEvent` and options |
| `staticprocesses.go`     | `src/staticprocesses.rs`   | `ETRouter` and the built-in `ET*` processes, `CopyEventFields` |
| `errorprocesses.go`      | `src/errorprocesses.rs`    | `ERRouter` and the `ER*` processes |
| `dynamicprocesses.go`    | `src/dynamicprocesses.rs`  | `EDRouter`, `EDSync`, `NewUUID` |
| `customprocesses.go`     | `src/customprocesses.rs`   | `ECRouter`, `ECGeneralDelivery` |
| `supervisorprocesses.go` | `src/supervisorprocesses.rs` | `ESRouter`, `ESProcesses` |
| `config.go`              | `src/config.rs`            | `Config`, `NewConfig` |
| `eventrw.go`             | `src/eventrw.rs`           | `EventRW` (`io::Read`/`io::Write` over an event) |
| `buffer.go`              | `src/buffer.rs`            | thread-safe `Buffer` |
| (slog usage)             | `src/logging.rs`           | leveled logging gated by the `LogLevel` / `LOGLEVEL` env var |
| (context.Context)        | `src/context.rs`           | cancellation tree, the equivalent of `context.WithCancel` |

## Notable translation choices

- **Channels.** Go's `chan Event` is mapped to a `Chan { tx, rx }` pair built on
  `crossbeam-channel`, which provides the `select!` macro needed to mirror Go's
  `select` over an input channel plus `ctx.Done()`. Shared channels are buffered
  (capacity 10); a process `InCh` is unbuffered (a rendezvous channel), matching
  the Go `make(chan Event)`.
- **Goroutines.** Each process body runs in its own `std::thread`. The body is
  produced by an `ETFunc` exactly as in Go: a function that takes the context
  and process handle and returns the loop closure.
- **Shared state.** Where Go shares `*Process`, the registries, the config and
  the pid/event counters via pointers, the Rust port shares them via `Arc`, with
  a `Mutex` guarding each process map.
- **Cancellation.** `Context` implements a parent/child cancellation tree using a
  disconnected channel as the "done" signal, observable by `select!`.
- **`errorProcesses` mutex.** The Go map intentionally has no mutex; Rust
  requires synchronized cross-thread access, so a `Mutex` is used, with a comment
  marking the deviation.
- **`ETPidGetAll`.** The Go code CBOR-encodes the whole pid→`*Process` map.
  Encoding the full process graph is neither meaningful nor serializable, so the
  port encodes a pid→event-name map instead.
- **`Event.InternalCh`.** The Go `chan chan []byte` field is omitted; it is not
  serialized and unused by the examples.
