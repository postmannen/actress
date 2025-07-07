const std = @import("std");
const Allocator = std.mem.Allocator;
const Thread = std.Thread;
const Mutex = std.Thread.Mutex;
const HashMap = std.HashMap;
const ArrayList = std.ArrayList;

const Event = @import("event.zig").Event;
const EventType = @import("event.zig").EventType;
const Config = @import("config.zig").Config;
const Channel = @import("channel.zig").Channel;
const builtin_events = @import("builtin_events.zig");

/// Process ID type
pub const PidNr = u32;

/// Function type for process functions
pub const ETFunc = *const fn (allocator: Allocator, process: *Process) anyerror!void;

/// Context for passing to process wrapper
const ProcessContext = struct {
    func: ETFunc,
    allocator: Allocator,
    process: *Process,
};

/// Wrapper function for spawning threads
fn processWrapper(context: ProcessContext) void {
    context.func(context.allocator, context.process) catch |err| {
        std.log.err("Process function error: {}", .{err});
    };
}

/// Process Map type
const ProcessMap = HashMap(EventType, *Process, EventTypeContext, std.hash_map.default_max_load_percentage);

/// Context for EventType keys in HashMap
const EventTypeContext = struct {
    pub fn hash(self: @This(), s: EventType) u64 {
        _ = self;
        return std.hash_map.hashString(s);
    }

    pub fn eql(self: @This(), a: EventType, b: EventType) bool {
        _ = self;
        return std.mem.eql(u8, a, b);
    }
};

/// Holds information about what process functions belong to what event
pub const Processes = struct {
    proc_map: ProcessMap,
    mutex: Mutex,

    const Self = @This();

    pub fn init(allocator: Allocator) Self {
        return Self{
            .proc_map = ProcessMap.init(allocator),
            .mutex = Mutex{},
        };
    }

    pub fn deinit(self: *Self) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        self.proc_map.deinit();
    }

    /// Add a new Event and its process to the processes map
    pub fn add(self: *Self, et: EventType, proc: *Process) void {
        self.mutex.lock();
        defer self.mutex.unlock();

        // Check if a process for the same event is defined
        if (self.proc_map.get(et)) |existing_proc| {
            existing_proc.cancel();
        }

        self.proc_map.put(et, proc) catch |err| {
            std.log.err("Failed to add process for event {s}: {}", .{ et, err });
            return;
        };
    }

    /// Check if the event is defined in the processes map
    pub fn isEventDefined(self: *Self, et: EventType) bool {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.proc_map.contains(et);
    }

    /// Get process for event type
    pub fn getProcess(self: *Self, et: EventType) ?*Process {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.proc_map.get(et);
    }
};

/// Dynamic Processes type
pub const DynProcesses = struct {
    proc_map: ProcessMap,
    mutex: Mutex,

    const Self = @This();

    pub fn init(allocator: Allocator) Self {
        return Self{
            .proc_map = ProcessMap.init(allocator),
            .mutex = Mutex{},
        };
    }

    pub fn deinit(self: *Self) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        self.proc_map.deinit();
    }

    pub fn add(self: *Self, et: EventType, proc: *Process) void {
        self.mutex.lock();
        defer self.mutex.unlock();

        if (self.proc_map.get(et)) |existing_proc| {
            existing_proc.cancel();
        }

        self.proc_map.put(et, proc) catch |err| {
            std.log.err("Failed to add dynamic process for event {s}: {}", .{ et, err });
            return;
        };
    }

    pub fn delete(self: *Self, et: EventType) void {
        self.mutex.lock();
        defer self.mutex.unlock();

        if (self.proc_map.get(et)) |proc| {
            proc.cancel();
            _ = self.proc_map.remove(et);
            std.log.info("Deleted dynamic process {s}", .{et});
        }
    }

    pub fn isEventDefined(self: *Self, et: EventType) bool {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.proc_map.contains(et);
    }

    pub fn getProcess(self: *Self, et: EventType) ?*Process {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.proc_map.get(et);
    }
};

/// Error Processes type
pub const ErrProcesses = struct {
    proc_map: ProcessMap,
    mutex: Mutex,

    const Self = @This();

    pub fn init(allocator: Allocator) Self {
        return Self{
            .proc_map = ProcessMap.init(allocator),
            .mutex = Mutex{},
        };
    }

    pub fn deinit(self: *Self) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        self.proc_map.deinit();
    }

    pub fn add(self: *Self, et: EventType, proc: *Process) void {
        self.mutex.lock();
        defer self.mutex.unlock();

        if (self.proc_map.get(et)) |existing_proc| {
            existing_proc.cancel();
        }

        self.proc_map.put(et, proc) catch |err| {
            std.log.err("Failed to add error process for event {s}: {}", .{ et, err });
            return;
        };
    }

    pub fn isEventDefined(self: *Self, et: EventType) bool {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.proc_map.contains(et);
    }

    pub fn getProcess(self: *Self, et: EventType) ?*Process {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.proc_map.get(et);
    }
};

/// PID management
pub const Pids = struct {
    nr: PidNr,
    mutex: Mutex,

    const Self = @This();

    pub fn init() Self {
        return Self{
            .nr = 0,
            .mutex = Mutex{},
        };
    }

    pub fn next(self: *Self) PidNr {
        self.mutex.lock();
        defer self.mutex.unlock();

        const nr = self.nr;
        self.nr += 1;
        return nr;
    }
};

/// Process defines a process (actor)
pub const Process = struct {
    /// Process function
    process_fn: ?ETFunc,

    /// Channel to receive events into the process function
    in_ch: Channel(Event),

    /// Channel to send events to be picked up by other processes
    event_ch: *Channel(Event),

    /// Channel to send error events
    error_ch: *Channel(Event),

    /// Channel for getting the result in tests
    test_ch: *Channel(Event),

    /// Channel for routing events for dynamic processes
    dyn_ch: *Channel(Event),

    /// The event type for the process
    event_type: EventType,

    /// Maps for various process information
    processes: *Processes,

    /// Map of dynamic processes
    dyn_processes: *DynProcesses,

    /// Maps for various error process information
    err_processes: *ErrProcesses,

    /// Is this the root process
    is_root: bool,

    /// Holding all configuration settings
    config: *const Config,

    /// Process ID management
    pids: *Pids,

    /// PID of the process
    pid: PidNr,

    /// Thread handle
    thread: ?Thread,

    /// Cancellation flag
    should_cancel: std.atomic.Value(bool),

    /// Allocator
    allocator: Allocator,

    const Self = @This();

    /// Add an event to the event channel
    pub fn addEvent(self: *Self, event: Event) void {
        self.event_ch.send(event) catch |err| {
            std.log.err("Failed to send event: {}", .{err});
        };
    }

    /// Add an event to the dynamic event channel
    pub fn addDynEvent(self: *Self, event: Event) void {
        self.dyn_ch.send(event) catch |err| {
            std.log.err("Failed to send dynamic event: {}", .{err});
        };
    }

    /// Add an error event to the error channel
    pub fn addError(self: *Self, event: Event) void {
        self.error_ch.send(event) catch |err| {
            std.log.err("Failed to send error event: {}", .{err});
        };
    }

    /// Add an event to the test channel
    pub fn addTestEvent(self: *Self, event: Event) void {
        self.test_ch.send(event) catch |err| {
            std.log.err("Failed to send test event: {}", .{err});
        };
    }

    /// Start the process
    pub fn act(self: *Self) !void {
        if (self.process_fn) |func| {
            const context = ProcessContext{
                .func = func,
                .allocator = self.allocator,
                .process = self,
            };
            self.thread = try Thread.spawn(.{}, processWrapper, .{context});
        } else {
            std.log.warn("No process function defined for process with event type: {s}", .{self.event_type});
        }
    }

    /// Cancel the process
    pub fn cancel(self: *Self) void {
        self.should_cancel.store(true, .monotonic);
        self.in_ch.close();

        if (self.thread) |thread| {
            thread.join();
            self.thread = null;
        }
    }

    /// Check if process should be cancelled
    pub fn shouldCancel(self: *const Self) bool {
        return self.should_cancel.load(.monotonic);
    }

    /// Wait for the process to finish
    pub fn join(self: *Self) void {
        if (self.thread) |thread| {
            thread.join();
            self.thread = null;
        }
    }

    /// Clean up the process
    pub fn deinit(self: *Self) void {
        self.cancel();
        self.in_ch.deinit();
    }
};

/// Root Process - special process that manages the entire system
pub const RootProcess = struct {
    process: Process,
    event_ch: Channel(Event),
    error_ch: Channel(Event),
    test_ch: Channel(Event),
    dyn_ch: Channel(Event),
    processes: Processes,
    dyn_processes: DynProcesses,
    err_processes: ErrProcesses,
    pids: Pids,
    config: Config,
    allocator: Allocator,

    const Self = @This();

    pub fn deinit(self: *Self) void {
        self.process.deinit();
        self.event_ch.deinit();
        self.error_ch.deinit();
        self.test_ch.deinit();
        self.dyn_ch.deinit();
        self.processes.deinit();
        self.dyn_processes.deinit();
        self.err_processes.deinit();
    }
};

/// Create a new root process
pub fn newRootProcess(allocator: Allocator, func: ?ETFunc) !*RootProcess {
    const root = try allocator.create(RootProcess);

    root.* = RootProcess{
        .event_ch = Channel(Event).init(allocator, 100),
        .error_ch = Channel(Event).init(allocator, 100),
        .test_ch = Channel(Event).init(allocator, 100),
        .dyn_ch = Channel(Event).init(allocator, 100),
        .processes = Processes.init(allocator),
        .dyn_processes = DynProcesses.init(allocator),
        .err_processes = ErrProcesses.init(allocator),
        .pids = Pids.init(),
        .config = Config.initFromEnv(),
        .allocator = allocator,
        .process = undefined,
    };

    root.process = Process{
        .process_fn = func,
        .in_ch = Channel(Event).init(allocator, 10),
        .event_ch = &root.event_ch,
        .error_ch = &root.error_ch,
        .test_ch = &root.test_ch,
        .dyn_ch = &root.dyn_ch,
        .event_type = builtin_events.ETRoot,
        .processes = &root.processes,
        .dyn_processes = &root.dyn_processes,
        .err_processes = &root.err_processes,
        .is_root = true,
        .config = &root.config,
        .pids = &root.pids,
        .pid = root.pids.next(),
        .thread = null,
        .should_cancel = std.atomic.Value(bool).init(false),
        .allocator = allocator,
    };

    return root;
}

/// Create a new process
pub fn newProcess(allocator: Allocator, parent: *Process, event_type: EventType, func: ?ETFunc) !*Process {
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

    // Register the process
    parent.processes.add(event_type, process);

    return process;
}
