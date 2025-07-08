# Actress Zig

A Concurrent Actor framework for Zig applications, based on the Go Actress library by Bjørn Tore Svinningen.

## Overview

Actress Zig is an opinionated take on an actor system. Create custom processes where each process performs specific tasks, communicating through events to pass results and chain operations together as workflows.

### Key Features

- **Event-driven Architecture**: Processes communicate via events
- **Process Management**: Create, manage, and coordinate multiple concurrent processes
- **Event Chaining**: Chain events together to create workflows using NextEvent
- **Error Handling**: Built-in error handling and routing system
- **Thread Safety**: Thread-safe channel-based communication
- **Zero Shared State**: All communication via message passing
- **Memory Safe**: Proper memory management with Zig's allocator system

## Quick Start

### Basic Setup

```zig
const std = @import("std");
const actress = @import("actress");

const ETCustom = "ETCustom";

fn customProcessFn(allocator: std.mem.Allocator, process: *actress.Process) !void {
    _ = allocator;
    
    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                defer {
                    var mut_event = event;
                    mut_event.deinit();
                }
                
                if (event.data) |data| {
                    std.debug.print("Processing: {s}\n", .{data});
                    
                    // Chain to next process if specified
                    if (event.next_event) |next_event| {
                        const new_event = try actress.Event.initWithData(
                            process.allocator, 
                            next_event.event_type, 
                            "Processed data"
                        );
                        process.addEvent(new_event);
                    }
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("Process error: {}", .{err});
            break;
        }
    }
}

pub fn main() !void {
    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    defer _ = gpa.deinit();
    const allocator = gpa.allocator();
    
    // Initialize the actress system
    const root = try actress.initActressSystem(allocator);
    defer {
        root.deinit();
        allocator.destroy(root);
    }
    
    // Create a custom process
    const custom_process = try actress.newProcess(allocator, &root.process, ETCustom, customProcessFn);
    try custom_process.act();
    
    // Send an event
    const event = try actress.Event.initWithData(allocator, ETCustom, "Hello World");
    root.process.addEvent(event);
    
    // Wait for processing
    std.time.sleep(1 * std.time.ns_per_s);
}
```

### Two Processes Example

```zig
const std = @import("std");
const actress = @import("actress");

const ETTest1 = "ETTest1";
const ETTest2 = "ETTest2";

// First process: convert to uppercase
fn test1Fn(allocator: std.mem.Allocator, process: *actress.Process) !void {
    // Implementation that converts data to uppercase and chains to next process
}

// Second process: add dots
fn test2Fn(allocator: std.mem.Allocator, process: *actress.Process) !void {
    // Implementation that adds dots and prints result
}

pub fn main() !void {
    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    defer _ = gpa.deinit();
    const allocator = gpa.allocator();
    
    const root = try actress.initActressSystem(allocator);
    defer {
        root.deinit();
        allocator.destroy(root);
    }
    
    const test1_process = try actress.newProcess(allocator, &root.process, ETTest1, test1Fn);
    const test2_process = try actress.newProcess(allocator, &root.process, ETTest2, test2Fn);
    
    try test1_process.act();
    try test2_process.act();
    
    // Create event chain
    var next_event = actress.newEvent(allocator, ETTest2);
    var initial_event = try actress.Event.initWithData(allocator, ETTest1, "hello world");
    initial_event.setNextEvent(&next_event);
    
    root.process.addEvent(initial_event);
    
    std.time.sleep(2 * std.time.ns_per_s);
}
```

## Core Concepts

### Events

Events are the communication mechanism between processes:

```zig
const event = actress.Event.init(allocator, "MyEventType");

// Event with data
const data_event = try actress.Event.initWithData(allocator, "MyEventType", "some data");

// Event with command
const cmd = [_][]const u8{"param1", "param2"};
const cmd_event = try actress.Event.initWithCmd(allocator, "MyEventType", &cmd);

// Event with error
const err_event = try actress.Event.initWithErr(allocator, "ErrorEventType", "error message");
```

### Processes

Processes are actors that handle specific event types:

```zig
fn myProcessFn(allocator: std.mem.Allocator, process: *actress.Process) !void {
    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                defer {
                    var mut_event = event;
                    mut_event.deinit(); // Clean up event memory
                }
                
                // Process the event
                if (event.data) |data| {
                    // Do something with data
                    std.debug.print("Received: {s}\n", .{data});
                }
                
                // Chain to next event if specified
                if (event.next_event) |next_event| {
                    const new_event = try actress.Event.initWithData(
                        process.allocator, 
                        next_event.event_type, 
                        "result"
                    );
                    process.addEvent(new_event);
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("Process error: {}", .{err});
            break;
        }
    }
}
```

### Event Types

Event types are unique string identifiers for routing events to processes:

```zig
const ETCustom = "ETCustom";
const ETProcess = "ETProcess";
```

## Built-in Event Types

Actress Zig includes several built-in event types:

- **ETPrint**: Prints data to stdout
- **ETDone**: Signals process completion
- **ETExit**: Shuts down the system
- **ETTestCh**: Routes events to test queue
- **ERLog**: Error logging
- **ERDebug**: Debug logging
- **ERFatal**: Fatal error handling

## Built-in Process Functions

### Print Process

```zig
const print_event = try actress.Event.initWithData(allocator, actress.ETPrint, "Hello World");
root.process.addEvent(print_event);
```

### Error Handling

```zig
// Log an error
const log_event = try actress.Event.initWithErr(allocator, actress.ERLog, "Something went wrong");
process.addError(log_event);

// Debug message
const debug_event = try actress.Event.initWithErr(allocator, actress.ERDebug, "Debug info");
process.addError(debug_event);

// Fatal error (exits system)
const fatal_event = try actress.Event.initWithErr(allocator, actress.ERFatal, "Critical error");
process.addError(fatal_event);
```

## Dynamic Processes

Create and manage dynamic processes:

```zig
// Create dynamic process
const dyn_process = try actress.newDynProcess(allocator, &parent_process, "ETDynamic", dynamicProcessFn);
try dyn_process.act();

// Remove dynamic process when done
actress.removeDynProcess(&parent_process, "ETDynamic");
```

## Configuration

Configure the actress system via environment variables:

- `PROFILING`: Set profiling mode ("none", "cpu", "mem", "mutex", "block", "trace")
- `CUSTOMEVENTS`: Enable custom events from files ("1" or "true")
- `METRICS`: Enable metrics collection ("1" or "true")
- `CUSTOMEVENTSPATH`: Path for custom events files
- `DEBUG`: Enable debug mode ("1" or "true")
- `MAX_RETRIES`: Maximum retries for event delivery
- `RETRY_DELAY_MS`: Retry delay in milliseconds

## Building

### Prerequisites

- Zig 0.13.0 or later

### Build Commands

```bash
# Build the library
zig build

# Run tests
zig build test

# Build examples
zig build

# Run examples
./zig-out/bin/two_actresses
./zig-out/bin/httpget
```

## Examples

See the `examples/` directory for complete working examples:

- `two_actresses.zig`: Demonstrates event chaining between two processes
- `httpget.zig`: Shows HTTP-like processing with file writing

## Architecture

The Actress Zig system consists of:

1. **Root Process**: Main system coordinator
2. **Event Router**: Routes events to correct processes based on EventType
3. **Dynamic Router**: Handles dynamic process events
4. **Error Router**: Routes error events to error handlers
5. **Built-in Handlers**: Standard processes for printing, logging, etc.
6. **Custom Processes**: User-defined actors for specific tasks

## Thread Safety

All communication is thread-safe using:

- Channel-based message passing
- Atomic operations for cancellation
- Mutex-protected shared data structures

## Memory Management

- Events own their data and must be cleaned up with `deinit()`
- Processes are cleaned up automatically on cancellation
- Use arena allocators for temporary allocations in process functions

## Key Principles

- **No Shared State**: All communication via message passing
- **Actor Model**: Processes are independent actors
- **Event-Driven**: Everything happens through events
- **Composable**: Chain events to create workflows
- **Fault Tolerant**: Error handling is built into the system

## License

This implementation follows the same license as the original Go library.
Based on the Go Actress library by Bjørn Tore Svinningen.
