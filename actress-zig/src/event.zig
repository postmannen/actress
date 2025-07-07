const std = @import("std");
const Allocator = std.mem.Allocator;
const ArrayList = std.ArrayList;

/// EventType is a unique name used to identify events.
/// It is used both for creating processes and also for routing messages to the correct process.
pub const EventType = []const u8;

/// Event defines an event. It holds:
/// - The EventType, which specifies which process the event is meant for.
/// - The cmd, meant to be a way to give instructions for what a process should do.
///   The receiving process is responsible for parsing the string slice.
/// - The data field is meant to carry the result from the work done by a process,
///   to the next process.
/// - Both cmd and data can be used interchangeably if it makes more sense for a
///   given scenario. No strict rules exist for this.
/// - err is used by the error event types.
/// - next_event is used when we want to define a chain of events to be executed.
///   The processes must make use of the field for this to work.
pub const Event = struct {
    /// Sequential number for event ordering
    nr: i32 = 0,

    /// EventType is a unique name to identify the type of the event
    event_type: EventType,

    /// cmd is usually used for giving instructions or parameters for what an event shall do
    cmd: ?[]const []const u8 = null,

    /// data usually carries the data from one process to the next
    data: ?[]const u8 = null,

    /// err is used for defining the error when the event is used as an error event
    err: ?[]const u8 = null,

    /// next_event defines a series of events to be executed like a workflow
    next_event: ?*Event = null,

    allocator: Allocator,

    const Self = @This();

    /// Create a new Event with the given EventType
    pub fn init(allocator: Allocator, event_type: EventType) Self {
        return Self{
            .event_type = event_type,
            .allocator = allocator,
        };
    }

    /// Create a new Event with command
    pub fn initWithCmd(allocator: Allocator, event_type: EventType, cmd: []const []const u8) !Self {
        const owned_cmd = try allocator.alloc([]const u8, cmd.len);
        for (cmd, 0..) |arg, i| {
            owned_cmd[i] = try allocator.dupe(u8, arg);
        }

        return Self{
            .event_type = event_type,
            .cmd = owned_cmd,
            .allocator = allocator,
        };
    }

    /// Create a new Event with data
    pub fn initWithData(allocator: Allocator, event_type: EventType, data: []const u8) !Self {
        const owned_data = try allocator.dupe(u8, data);
        return Self{
            .event_type = event_type,
            .data = owned_data,
            .allocator = allocator,
        };
    }

    /// Create a new Event with error
    pub fn initWithErr(allocator: Allocator, event_type: EventType, err: []const u8) !Self {
        const owned_err = try allocator.dupe(u8, err);
        return Self{
            .event_type = event_type,
            .err = owned_err,
            .allocator = allocator,
        };
    }

    /// Set the next event in the chain
    pub fn setNextEvent(self: *Self, next_event: *Event) void {
        self.next_event = next_event;
    }

    /// Set command for the event
    pub fn setCmd(self: *Self, cmd: []const []const u8) !void {
        if (self.cmd) |old_cmd| {
            for (old_cmd) |arg| {
                self.allocator.free(arg);
            }
            self.allocator.free(old_cmd);
        }

        const owned_cmd = try self.allocator.alloc([]const u8, cmd.len);
        for (cmd, 0..) |arg, i| {
            owned_cmd[i] = try self.allocator.dupe(u8, arg);
        }
        self.cmd = owned_cmd;
    }

    /// Set data for the event
    pub fn setData(self: *Self, data: []const u8) !void {
        if (self.data) |old_data| {
            self.allocator.free(old_data);
        }
        self.data = try self.allocator.dupe(u8, data);
    }

    /// Set error for the event
    pub fn setErr(self: *Self, err: []const u8) !void {
        if (self.err) |old_err| {
            self.allocator.free(old_err);
        }
        self.err = try self.allocator.dupe(u8, err);
    }

    /// Clean up allocated memory
    pub fn deinit(self: *Self) void {
        if (self.cmd) |cmd| {
            for (cmd) |arg| {
                self.allocator.free(arg);
            }
            self.allocator.free(cmd);
        }
        if (self.data) |data| {
            self.allocator.free(data);
        }
        if (self.err) |err| {
            self.allocator.free(err);
        }
        // Note: next_event is not owned by this event, so we don't free it
    }

    /// Clone the event (deep copy)
    pub fn clone(self: *const Self) !Self {
        var new_event = Self{
            .nr = self.nr,
            .event_type = self.event_type,
            .allocator = self.allocator,
        };

        if (self.cmd) |cmd| {
            try new_event.setCmd(cmd);
        }
        if (self.data) |data| {
            try new_event.setData(data);
        }
        if (self.err) |err| {
            try new_event.setErr(err);
        }
        // Note: We don't clone next_event to avoid infinite recursion

        return new_event;
    }
};
