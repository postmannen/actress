const std = @import("std");
const Allocator = std.mem.Allocator;

const Event = @import("event.zig").Event;
const EventType = @import("event.zig").EventType;
const Process = @import("process.zig").Process;
const builtin_events = @import("builtin_events.zig");

/// Router for normal events
/// Routes events to the correct process based on EventType
pub fn etRouterFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.event_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                // Check if process is registered
                if (process.processes.getProcess(event.event_type)) |target_process| {
                    target_process.in_ch.send(event) catch |err| {
                        var error_event = Event.initWithErr(process.allocator, builtin_events.ERLog, "Failed to deliver event to process") catch continue;
                        defer error_event.deinit();
                        process.addError(error_event);
                        std.log.err("Failed to deliver event to process {s}: {}", .{ event.event_type, err });
                    };
                } else {
                    // Try to deliver with retries
                    retryEventDelivery(process, event);
                }
            } else {
                // Channel closed, exit
                break;
            }
        } else |err| {
            std.log.err("Router error receiving event: {}", .{err});
            break;
        }
    }
}

/// Retry event delivery for processes that might not be registered yet
fn retryEventDelivery(process: *Process, event: Event) void {
    const retries = process.config.max_retries;
    const delay_ms = process.config.retry_delay_ms;

    var i: u32 = 0;
    while (i < retries) : (i += 1) {
        if (process.processes.getProcess(event.event_type)) |target_process| {
            target_process.in_ch.send(event) catch |err| {
                std.log.err("Failed to deliver event after retry {}: {}", .{ i, err });
                return;
            };
            return;
        }

        var error_event = Event.initWithErr(process.allocator, builtin_events.ERLog, "No process registered for event type") catch return;
        defer error_event.deinit();
        process.addError(error_event);

        std.time.sleep(delay_ms * std.time.ns_per_ms);
    }
}

/// Router for dynamic events
pub fn edRouterFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.dyn_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                if (process.dyn_processes.getProcess(event.event_type)) |target_process| {
                    target_process.in_ch.send(event) catch |err| {
                        var error_event = Event.initWithErr(process.allocator, builtin_events.ERLog, "Failed to deliver dynamic event to process") catch continue;
                        defer error_event.deinit();
                        process.addError(error_event);
                        std.log.err("Failed to deliver dynamic event to process {s}: {}", .{ event.event_type, err });
                    };
                } else {
                    // Try to deliver with retries for dynamic processes too
                    retryDynEventDelivery(process, event);
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("Dynamic router error receiving event: {}", .{err});
            break;
        }
    }
}

fn retryDynEventDelivery(process: *Process, event: Event) void {
    const retries = process.config.max_retries;
    const delay_ms = process.config.retry_delay_ms;

    var i: u32 = 0;
    while (i < retries) : (i += 1) {
        if (process.dyn_processes.getProcess(event.event_type)) |target_process| {
            target_process.in_ch.send(event) catch |err| {
                std.log.err("Failed to deliver dynamic event after retry {}: {}", .{ i, err });
                return;
            };
            return;
        }

        var error_event = Event.initWithErr(process.allocator, builtin_events.ERLog, "No dynamic process registered for event type") catch return;
        defer error_event.deinit();
        process.addError(error_event);

        std.time.sleep(delay_ms * std.time.ns_per_ms);
    }
}

/// Router for error events
pub fn erRouterFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.error_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                if (process.err_processes.getProcess(event.event_type)) |target_process| {
                    target_process.in_ch.send(event) catch |err| {
                        std.log.err("Failed to deliver error event to process {s}: {}", .{ event.event_type, err });
                    };
                } else {
                    // Default error handling - just log to console
                    handleDefaultError(event);
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("Error router error receiving event: {}", .{err});
            break;
        }
    }
}

fn handleDefaultError(event: Event) void {
    if (std.mem.eql(u8, event.event_type, builtin_events.ERLog)) {
        if (event.err) |err_msg| {
            std.log.err("Log: {s}", .{err_msg});
        }
    } else if (std.mem.eql(u8, event.event_type, builtin_events.ERDebug)) {
        if (event.err) |err_msg| {
            std.log.debug("Debug: {s}", .{err_msg});
        }
    } else if (std.mem.eql(u8, event.event_type, builtin_events.ERFatal)) {
        if (event.err) |err_msg| {
            std.log.err("Fatal: {s}", .{err_msg});
            std.process.exit(1);
        }
    }
}

/// Print event handler
pub fn etPrintFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                if (event.data) |data| {
                    std.debug.print("{s}\n", .{data});
                } else if (event.cmd) |cmd| {
                    for (cmd) |arg| {
                        std.debug.print("{s} ", .{arg});
                    }
                    std.debug.print("\n", .{});
                } else {
                    std.debug.print("Print event with no data\n", .{});
                }

                // Handle chained events
                if (event.next_event) |next_event| {
                    process.addEvent(next_event.*);
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("Print process error: {}", .{err});
            break;
        }
    }
}

/// Done event handler
pub fn etDoneFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                if (event.data) |data| {
                    std.log.info("Process done: {s}", .{data});
                } else {
                    std.log.info("Process done", .{});
                }

                // Handle chained events
                if (event.next_event) |next_event| {
                    process.addEvent(next_event.*);
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("Done process error: {}", .{err});
            break;
        }
    }
}

/// Exit event handler
pub fn etExitFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                _ = event;
                std.log.info("Exit event received, shutting down...", .{});
                std.process.exit(0);
            } else {
                break;
            }
        } else |err| {
            std.log.err("Exit process error: {}", .{err});
            break;
        }
    }
}

/// Test channel event handler
pub fn etTestChFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                process.addTestEvent(event);
            } else {
                break;
            }
        } else |err| {
            std.log.err("TestCh process error: {}", .{err});
            break;
        }
    }
}

/// Error log handler
pub fn erLogFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                if (event.err) |err_msg| {
                    std.log.err("Error Log: {s}", .{err_msg});
                } else if (event.data) |data| {
                    std.log.err("Error Log: {s}", .{data});
                } else {
                    std.log.err("Error Log: Unknown error", .{});
                }

                // Handle chained events
                if (event.next_event) |next_event| {
                    process.addEvent(next_event.*);
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("Error log process error: {}", .{err});
            break;
        }
    }
}

/// Debug handler
pub fn erDebugFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                if (event.err) |err_msg| {
                    std.log.debug("Debug: {s}", .{err_msg});
                } else if (event.data) |data| {
                    std.log.debug("Debug: {s}", .{data});
                } else {
                    std.log.debug("Debug: Unknown message", .{});
                }

                // Handle chained events
                if (event.next_event) |next_event| {
                    process.addEvent(next_event.*);
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("Debug process error: {}", .{err});
            break;
        }
    }
}

/// Fatal error handler
pub fn erFatalFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                if (event.err) |err_msg| {
                    std.log.err("Fatal Error: {s}", .{err_msg});
                } else if (event.data) |data| {
                    std.log.err("Fatal Error: {s}", .{data});
                } else {
                    std.log.err("Fatal Error: Unknown fatal error", .{});
                }

                std.process.exit(1);
            } else {
                break;
            }
        } else |err| {
            std.log.err("Fatal error process error: {}", .{err});
            std.process.exit(1);
        }
    }
}
