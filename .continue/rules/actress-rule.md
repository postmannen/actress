---
trigger: always_on
---

# Actress Actor System Architecture and Functionality

## Overview

The Actress actor system is a concurrent actor framework written in Go designed for building modular and event-driven applications. It uses actors (processes) to handle specific tasks and processes events in a distributed manner, making it suitable for applications requiring high concurrency and extensibility.

## Core Architecture Components

### Processes (actors)

- **Definition**: Like modules capable of performing specific tasks
- **Characteristics**:
  - Each process has a unique EventName
  - Has an InCh channel for receiving events
  - Has an AddEvent function for sending events
  - Can send event messages to other processes
  - Can hold state within the Process Function
  - **Note**: Processes are created externally via `NewProcess(ctx, parent, eventName, etFunc)` and started with `Act()`.

### Events

- **Structure**: The Event struct contains multiple fields for communication and data handling:
  - `Nr`: Event sequence number
  - `Name`: Unique identifier for event type (EventName)
  - `Cmd`: Instructions or parameters for what an event should do (slice of strings)
  - `Instruction`: Single-word instruction (string type), used in switch statements at the receiving actor to add more instructions for what to do with an event.
  - `Data`: Carry data between processes (e.g., file content)
  - `Err`: Error messages for error events
  - `NextEvent`: Chain of events to create workflows
  - `PreviousEvent`: Allows keeping information about the previous event in a chain
  - `DstNode/SrcNode`: Node identification for distributed processing
  - `InternalCh`: Channel for internal data transfer between actors

### Event Types and Routing

The system categorizes events by their **first two characters**:

- `ET*`: Static events (defined at startup)
- `ED*`: Dynamic events (runtime defined actors, auto-assigned UUID names like `ED-<uuid>`)
- `ER*`: Error events (for error logging and handling)
- `EC*`: Custom events ()
- `ES*`: Supervisor events (future control logic for the system)

### Process Lifecycle Management

- **Root Process**: Central process that holds core system elements and channels
- **Process Creation**: New processes are created by copying channels and map structures from parent processes via `NewProcess(ctx, parent, eventName, etFunc)`
- **Process Registration**: Processes are registered in their respective maps (StaticProcesses, DynamicProcesses, etc.)
- **Process Management**: Processes can be started with `Act()` and stopped with `Stop()`
- **Important**: Static, error, and supervisor processes **cannot be deleted** from their maps. Only dynamic (`ED*`) and custom (`EC*`) processes can be removed.

## Key Features

### Event-Driven Architecture

- Events are passed between actors and processes
- Processes can communicate via event routing

### Modular Design

- System can be easily extended by adding new actors to handle additional event types
- Each actor is responsible for a specific task

### Distributed Processing

- Events can be handled by actors running on different nodes
- `ETRemote` is a pre-defined event name constant (`const ETRemote EventName = "ETRemote"`), **not a separate package**. The user must implement their own `ETFunc` for `ETRemote` to handle remote transport (e.g., NATS, MQTT)
- Node identification and tracking capabilities

### Process Types

1. **Static Processes**: For processes/actors defined at startup
2. **Dynamic Processes**: Runtime-defined actors (auto-assigned UUID names)
3. **Error Processes**: For error logging and handling
4. **Custom Processes**: User-defined event types (mutex-protected)
5. **Supervisor Processes**: For control logic and system information

## Core Functionality

### Event Handling Flow

1. Events are sent through `AddEvent()` method
2. Based on event name second character, events are routed to appropriate channels:
   - `ET*` → StaticEventCh
   - `ER*` → ErrorEventCh
   - `ED*` → DynamicEventCh
   - `EC*` → CustomEventCh
   - `ES*` → SupervisorEventCh
3. Events are processed by the appropriate router process

### Process Creation and Management

- Root process initializes all core components via `NewRootProcess(ctx, fn, conf)`
- Processes inherit channels and configuration from parent defined when created with the function: func NewProcess(ctx context.Context, parentP *Process, event EventName, fn ETFunc) *Process
- Each process has unique PID (Process ID) stored in the process map
- Context management with cancellation support

### Distributed Communication (ETRemote)

- When `DstNode` is set to a different node name, events are wrapped in `ETRemote` event
- The user must implement their own `ETFunc` for `ETRemote` to handle remote transport
- Source node tracking maintained for debugging and logging

## Key Implementation Details

### Process Maps and Mutex Protection

- **Static processes**: use mutex-protected maps for process management
- **Dynamic processes**: use mutex-protected maps for runtime process management
- **Custom processes**: use mutex-protected maps
- **Supervisor processes**: use mutex-protected maps
- **Error processes**: **do NOT use mutex protection** (no mutex in `errorProcesses` struct)

### Process Lifecycle Methods

- `Act()`: Starts the process function (spawns goroutine, calls `WaitForReady()`)
- `Stop()`: Cancels context, removes from process maps, removes PID
- `SignalReady()`: Signals that the process function has started (use `sync.Once` for safety)
- `WaitForReady()`: Waits for process readiness (5ms timeout fallback)

### Built-in Processes

The system includes several built-in processes:

**Routers (one per event type):**
- `ETRouter`: Routes static events (`ET*`) to correct processes
- `ERRouter`: Routes error events (`ER*`) to correct processes
- `EDRouter`: Routes dynamic events (`ED*`) to correct processes
- `ECRouter`: Routes custom events (`EC*`) to correct processes
- `ESRouter`: Routes supervisor events (`ES*`) to correct processes

**Static processes:**
- `ETRoot`: Root process
- `ETRemote`: Handles remote node communication (**requires user implementation**)
- `ETOsSignal`: Handles OS signals like CTRL+C
- `ETTestCh`: Forwards events to TestCh channel
- `ETPrint`: Prints event data to stdout
- `ETExit`: Exits the system
- `ETTest`: Testing helper (accepts `chan string` via `ETTestfn`)
- `ETPid`: Handles PID queries (get single or all)
- `ETPidGetAll`: Returns CBOR-encoded PID map
- `ETReadFile`: Reads file from path in `Event.Cmd[0]`
- `ETDone`: Placeholder/dummy process

**Error processes:**
- `ERLog`: Logs errors based on `Instruction` field (Error, Info, Debug, Fatal)
- `ERTest`: Error testing handler
- `ERNone`: Drops error events (for testing)

**Dynamic processes:**
- `EDSync`: Synchronizes events via a signal channel

**Custom processes:**
- `ECGeneralDelivery`: Forward data to NextEvent (testing)

**Supervisor processes:**
- `ESProcesses`: Tracks running processes in the system

### Event Routing Logic

- Events are routed through **five separate router processes** (one per event type)
- Process lookups are protected by mutexes for dynamic, custom, and supervisor processes (but **not** for error processes)
- The `NextEvent` feature enables workflow creation through event chaining
- When `NextEvent` is set, `PreviousEvent` is automatically populated with a copy of the current event fields by the routers

## Configuration

The system supports configuration via:

- Command-line flags (with environment variable overrides)
- Log levels (debug, info, error, fatal, none)
- Node names for distributed processing
- Custom events and metrics support

## Key Principles

1. **Modularity**: Each actor handles specific tasks with clear responsibilities
2. **Event-driven**: Communication happens via events and channels
3. **Distributed**: Supports remote processing with node identification
4. **Thread-safe**: Uses mutexes where necessary for concurrent access (**except error processes**)

## Important Notes for Implementation

- All event names must start with 'E' followed by a type character (T, D, R, C, S)
- `NewProcess` does not start the process; you must call `Act()` separately
- `ETRemote` requires user-provided implementation of the ETFunc, only the ETRemote type is defined. The user shall create the ETRemote process with the newProcess function, assigning it the ETRemote type and it's own etRemote function, and start it with the .act() method of the process.
- Error processes (`errorProcesses`) lack mutex protection — be aware when accessing their maps concurrently
- Dynamic processes get UUID-based names via `NewUUID()` (format: `ED-<uuid>`)
- The `RegisterProcessesInESProcesses` function exists but is currently a no-op (body commented out)
