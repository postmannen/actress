const std = @import("std");
const Io = std.Io;
const zbor = @import("zbor");

const actress = @import("actress.zig");
const slog = @import("slog.zig");
const Context = @import("context.zig").Context;
const eventspkg = @import("events.zig");
const Event = eventspkg.Event;
const EventName = eventspkg.EventName;
const Instruction = eventspkg.Instruction;
const ETFunc = eventspkg.ETFunc;
const Process = actress.Process;
const setPreviousEvent = @import("staticprocesses.zig").setPreviousEvent;

pub const supervisorProcesses = struct {
    mu: Io.Mutex = .init,
    procMap: std.StringHashMap(*Process),
};

// Prepare and return a new *processes structure.
pub fn newsuperVisorProcesses(allocator: std.mem.Allocator) !*supervisorProcesses {
    const p = try allocator.create(supervisorProcesses);
    p.* = supervisorProcesses{
        .procMap = std.StringHashMap(*Process).init(allocator),
    };
    return p;
}

// ------------------------------------------------------------------------------
// Events and event functions, ESRouter
// ------------------------------------------------------------------------------

// Router for supervisor events.
pub const ESRouter: EventName = "ESRouter";

// Process function for routing and handling supervisor events. Will check
// and route the event to the correct process.
pub const esRouterFn = ETFunc{ .run = esRouterFnRun };

fn esRouterFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        var ev = p.SupervisorEventCh.getOne(p.io) catch {
            slog.Debug("esRouterFn", .{ "got ctx.Done, on", p.Config.NodeName });
            return;
        };

        // If there is a next event defined, we make a copy of all the fields  of the current event,
        // and put that as the previousEvent on the next event. We can use this information later
        // if need to check something in the previous event.
        if (ev.NextEvent) |ne| {
            // Keep the information about the current event, so we are able to check for things
            // like ackTimeout and what node to reply back to if ack should be given.
            setPreviousEvent(p.allocator, ne, &ev);
        }

        // Check if process is registred and valid.
        p.supervisorProcesses.mu.lockUncancelable(p.io);
        const ok = p.supervisorProcesses.procMap.get(ev.Name) != null;
        p.supervisorProcesses.mu.unlock(p.io);

        if (!ok) {
            slog.Error("esRouterFn", .{ "on", p.Config.NodeName, "found no process registered for the event type", ev.Name });
        }

        // // Process was registered. Deliver the event to the process InCh.

        p.supervisorProcesses.mu.lockUncancelable(p.io);
        // NB: Like the Go version the lookup here fails hard if the
        // process is missing, the check above only logs an error.
        const inCh = p.supervisorProcesses.procMap.get(ev.Name).?.InCh;
        p.supervisorProcesses.mu.unlock(p.io);

        slog.Debug("esRouterFn", .{ "on", p.Config.NodeName, "Routing event", p.Event, "node", p.Config.NodeName, "name", ev.Name, "Inch", inCh });

        inCh.putOne(p.io, ev) catch |err| switch (err) {
            error.Canceled => {
                ev.deinit(p.allocator);
                return;
            },
            error.Closed => {
                ev.deinit(p.allocator);
                continue;
            },
        };
    }
}

// ------------------------------------------------------------------------------
// Events and event functions, Process handling
// ------------------------------------------------------------------------------

// Handles information about the currently running processes in the local Actress system.
pub const ESProcesses: EventName = "ESProcesses";

// Will instruct to get all information about all processes.
pub const InstructionESProcessesAdd: Instruction = "InstructionESProcessesAdd";
pub const InstructionESProcessesDelete: Instruction = "InstructionESProcessesDelete";
pub const InstructionESProcessesGetAll: Instruction = "InstructionESProcessesGetAll";

pub const esProcessesMapDataIn = struct {
    Name: EventName,
};

pub const ESProcessesMap = std.StringHashMap([]const u8);

// ETFunc for handling information about the currently running processes in the local Actress system.
pub fn esProcessesFn() ETFunc {
    return ETFunc{ .run = esProcessesFnRun };
}

fn esProcessesFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    // The map of all the running processes. The keys and values are owned
    // by the map and freed when the process function returns.
    var processMap = ESProcessesMap.init(p.allocator);
    defer {
        var it = processMap.iterator();
        while (it.next()) |entry| {
            p.allocator.free(entry.key_ptr.*);
            p.allocator.free(entry.value_ptr.*);
        }
        processMap.deinit();
    }

    while (true) {
        p.SignalReady();

        const ev = p.InCh.getOne(p.io) catch {
            slog.Debug("esProcessesFn", .{ "got ctx.Done, on", p.Config.NodeName });
            return;
        };
        defer ev.deinit(p.allocator);

        // The Go version switches on the Instruction value. Zig can not
        // switch on strings, so an if/else chain is used instead.
        //
        // Add The received data about a process to the map.
        if (std.mem.eql(u8, ev.Instruction, InstructionESProcessesAdd)) {
            if (unmarshalMapDataIn(p.allocator, ev.Data)) |md| {
                defer p.allocator.free(md.Name);

                putProcessMap(p.allocator, &processMap, md.Name);

                slog.Debug("esProcessesFn", .{ "on", p.Config.NodeName, "processesMap count", processMap.count() });
            } else |err| {
                slog.Error("esProcessesFn", .{ "failed to unmarshal esProcesses map in data", @errorName(err) });
            }

            // Nothing to output are produced so we just add for the .NextEvent if defined.
            if (ev.NextEvent) |ne| {
                p.AddEvent(ne.*) catch {};
            }
        } else if (std.mem.eql(u8, ev.Instruction, InstructionESProcessesDelete)) {
            if (unmarshalMapDataIn(p.allocator, ev.Data)) |md| {
                p.allocator.free(md.Name);
            } else |err| {
                slog.Error("esProcessesFn", .{ "failed to unmarshal esProcesses map in data", @errorName(err) });
            }

            // NB: The Go version deletes p.Event here, not the name found
            // in the received data. That is mirrored.
            if (processMap.fetchRemove(p.Event)) |removed| {
                p.allocator.free(removed.key);
                p.allocator.free(removed.value);
            }

            // Nothing to output are produced so we just add for the .NextEvent if defined.
            if (ev.NextEvent) |ne| {
                p.AddEvent(ne.*) catch {};
            }
        } else if (std.mem.eql(u8, ev.Instruction, InstructionESProcessesGetAll)) {
            // Dump the content of the whole processes map, and send it with .NextEvent.
            if (marshalProcessMap(p.allocator, &processMap)) |b| {
                defer p.allocator.free(b);
                if (ev.NextEvent) |nEv| {
                    const oldData = nEv.Data;
                    nEv.Data = b;
                    p.AddEvent(nEv.*) catch {};
                    nEv.Data = oldData;
                }
            } else |err| {
                slog.Error("esProcessesFn", .{ "failed to marshal esProcesses for push all", @errorName(err) });
            }
        } else {
            slog.Error("esProcessesFn", .{ "not a defined instruction", ev.Instruction });
        }
    }
}

// Decode the cbor encoded esProcessesMapDataIn. The Name in the returned
// struct is owned by the caller.
fn unmarshalMapDataIn(allocator: std.mem.Allocator, data: []const u8) !esProcessesMapDataIn {
    const di = try zbor.DataItem.new(data);
    const md = try zbor.parse(esProcessesMapDataIn, di, .{ .allocator = allocator });
    return md;
}

// Put a copy of the name into the process map, keyed by the name itself,
// like processMap[md.Name] = string(md.Name) in the Go version.
fn putProcessMap(allocator: std.mem.Allocator, processMap: *ESProcessesMap, name: []const u8) void {
    const key = allocator.dupe(u8, name) catch |err| {
        std.debug.panic("esProcessesFn: allocation failed: {s}", .{@errorName(err)});
    };
    const value = allocator.dupe(u8, name) catch |err| {
        std.debug.panic("esProcessesFn: allocation failed: {s}", .{@errorName(err)});
    };
    if (processMap.fetchPut(key, value) catch |err| {
        std.debug.panic("esProcessesFn: allocation failed: {s}", .{@errorName(err)});
    }) |old| {
        allocator.free(key);
        allocator.free(old.value);
    }
}

// Encode the process map as a cbor map of name -> name.
fn marshalProcessMap(allocator: std.mem.Allocator, processMap: *ESProcessesMap) ![]u8 {
    var b = try zbor.Builder.withType(allocator, .Map);
    var it = processMap.iterator();
    while (it.next()) |entry| {
        try b.pushTextString(entry.key_ptr.*);
        try b.pushTextString(entry.value_ptr.*);
    }
    const out = try b.finish();
    return out;
}
