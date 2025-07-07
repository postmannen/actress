// Actress Zig Implementation
// Based on the Go Actress library by Bjørn Tore Svinningen
//
// A Concurrent Actor framework for Zig applications.
// Create custom processes where each process performs specific tasks,
// communicating through events to pass results and chain operations
// together as workflows.

const std = @import("std");
const Thread = std.Thread;
const Allocator = std.mem.Allocator;
const ArrayList = std.ArrayList;
const HashMap = std.HashMap;
const Mutex = std.Thread.Mutex;
const Condition = std.Thread.Condition;

// Export main types and functions
pub const Event = @import("event.zig").Event;
pub const EventType = @import("event.zig").EventType;
pub const Process = @import("process.zig").Process;
pub const RootProcess = @import("process.zig").RootProcess;
pub const Config = @import("config.zig").Config;
pub const Channel = @import("channel.zig").Channel;

// Export process management
pub const newRootProcess = @import("process.zig").newRootProcess;
pub const newProcess = @import("process.zig").newProcess;
pub const newTrackedProcess = @import("process.zig").newTrackedProcess;
pub const newDynProcess = @import("dynamic_processes.zig").newDynProcess;
pub const newErrProcess = @import("error_processes.zig").newErrProcess;
pub const newTrackedErrProcess = @import("error_processes.zig").newTrackedErrProcess;
pub const removeDynProcess = @import("dynamic_processes.zig").removeDynProcess;

// Export built-in event types
pub const builtin_events = @import("builtin_events.zig");
pub const ETRoot = builtin_events.ETRoot;
pub const ETRouter = builtin_events.ETRouter;
pub const EDRouter = builtin_events.EDRouter;
pub const ERRouter = builtin_events.ERRouter;
pub const ETPrint = builtin_events.ETPrint;
pub const ETDone = builtin_events.ETDone;
pub const ETExit = builtin_events.ETExit;
pub const ETTestCh = builtin_events.ETTestCh;
pub const ERLog = builtin_events.ERLog;
pub const ERDebug = builtin_events.ERDebug;
pub const ERFatal = builtin_events.ERFatal;
pub const ERTest = builtin_events.ERTest;

// Export built-in process functions
pub const builtin_functions = @import("builtin_functions.zig");
pub const etRouterFn = builtin_functions.etRouterFn;
pub const edRouterFn = builtin_functions.edRouterFn;
pub const erRouterFn = builtin_functions.erRouterFn;
pub const etPrintFn = builtin_functions.etPrintFn;
pub const etDoneFn = builtin_functions.etDoneFn;
pub const etExitFn = builtin_functions.etExitFn;
pub const etTestChFn = builtin_functions.etTestChFn;
pub const erLogFn = builtin_functions.erLogFn;
pub const erDebugFn = builtin_functions.erDebugFn;
pub const erFatalFn = builtin_functions.erFatalFn;

/// Initialize a complete actress system with all essential processes
pub fn initActressSystem(allocator: Allocator) !*RootProcess {
    const root = try newRootProcess(allocator, null);

    // Create essential system processes
    const router_process = try newTrackedProcess(allocator, root, &root.process, ETRouter, etRouterFn);
    const dyn_router_process = try newTrackedProcess(allocator, root, &root.process, EDRouter, edRouterFn);
    const err_router_process = try newTrackedErrProcess(allocator, root, &root.process, ERRouter, erRouterFn);

    // Create built-in event handlers
    const print_process = try newTrackedProcess(allocator, root, &root.process, ETPrint, etPrintFn);
    const done_process = try newTrackedProcess(allocator, root, &root.process, ETDone, etDoneFn);
    const exit_process = try newTrackedProcess(allocator, root, &root.process, ETExit, etExitFn);
    const test_process = try newTrackedProcess(allocator, root, &root.process, ETTestCh, etTestChFn);

    // Create error handlers
    const log_process = try newTrackedErrProcess(allocator, root, &root.process, ERLog, erLogFn);
    const debug_process = try newTrackedErrProcess(allocator, root, &root.process, ERDebug, erDebugFn);
    const fatal_process = try newTrackedErrProcess(allocator, root, &root.process, ERFatal, erFatalFn);

    // Start all essential processes
    try router_process.act();
    try dyn_router_process.act();
    try err_router_process.act();
    try print_process.act();
    try done_process.act();
    try exit_process.act();
    try test_process.act();
    try log_process.act();
    try debug_process.act();
    try fatal_process.act();

    // Start the root process only if it has a function
    if (root.process.process_fn != null) {
        try root.process.act();
    }

    return root;
}

/// Helper function to create an event with data
pub fn newEventWithData(allocator: Allocator, event_type: EventType, data: []const u8) !Event {
    return Event.initWithData(allocator, event_type, data);
}

/// Helper function to create an event with command
pub fn newEventWithCmd(allocator: Allocator, event_type: EventType, cmd: []const []const u8) !Event {
    return Event.initWithCmd(allocator, event_type, cmd);
}

/// Helper function to create an event with error
pub fn newEventWithErr(allocator: Allocator, event_type: EventType, err: []const u8) !Event {
    return Event.initWithErr(allocator, event_type, err);
}

/// Helper function to create a basic event
pub fn newEvent(allocator: Allocator, event_type: EventType) Event {
    return Event.init(allocator, event_type);
}

test {
    std.testing.refAllDecls(@This());
}
