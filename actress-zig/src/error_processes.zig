const std = @import("std");
const Allocator = std.mem.Allocator;

const Event = @import("event.zig").Event;
const EventType = @import("event.zig").EventType;
const Process = @import("process.zig").Process;
const ETFunc = @import("process.zig").ETFunc;
const Channel = @import("channel.zig").Channel;

/// Create a new error process
/// Error processes handle specific types of error events
pub fn newErrProcess(allocator: Allocator, parent: *Process, event_type: EventType, func: ?ETFunc) !*Process {
    const process = try allocator.create(Process);

    process.* = Process{
        .process_fn = func,
        .in_ch = Channel(Event).init(allocator, 10),
        .event_ch = parent.event_ch,
        .error_ch = parent.error_ch,
        .test_ch = parent.test_ch,
        .dyn_ch = parent.dyn_ch,
        .event_type = event_type,
        .processes = parent.processes,
        .dyn_processes = parent.dyn_processes,
        .err_processes = parent.err_processes,
        .is_root = false,
        .config = parent.config,
        .pids = parent.pids,
        .pid = parent.pids.next(),
        .thread = null,
        .should_cancel = std.atomic.Value(bool).init(false),
        .allocator = allocator,
    };

    // Register the error process
    parent.err_processes.add(event_type, process);

    return process;
}
