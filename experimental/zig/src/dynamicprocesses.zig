// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

const std = @import("std");
const Io = std.Io;

const actress = @import("actress.zig");
const slog = @import("slog.zig");
const Context = @import("context.zig").Context;
const Event = @import("events.zig").Event;
const EventName = @import("events.zig").EventName;
const ETFunc = @import("events.zig").ETFunc;
const Process = actress.Process;
const setPreviousEvent = @import("staticprocesses.zig").setPreviousEvent;

// Holds information about what process functions who belongs to what
// event, and also a map of the started processes.
pub const dynamicProcesses = struct {
    procMap: std.StringHashMap(*Process),
    mu: Io.Mutex = .init,

    // Add a new Event and it's process to the processes map.
    //
    // *******************************************************************
    // TODO: Consider if we still need this function. It is not in use.
    // *******************************************************************
    pub fn Add(p: *dynamicProcesses, io: Io, et: EventName, proc: *Process) !void {
        // Check if a process for the same event is defined, and if so we
        // cancel the current process before we replace it with a new one.
        p.mu.lockUncancelable(io);
        defer p.mu.unlock(io);
        if (p.procMap.get(et)) |existing| {
            existing.Cancel();
        }
        try p.procMap.put(et, proc);
    }

    // Delete an Event and it's process from the processes map.
    pub fn Delete(p: *dynamicProcesses, io: Io, et: EventName) void {
        p.mu.lockUncancelable(io);
        defer p.mu.unlock(io);
        // p.procMap.get(et).?.Cancel();
        _ = p.procMap.remove(et);
        std.debug.print("deleted process {s}\n", .{et});
    }

    // Checks if the event is defined in the processes map, and returns true if it is.
    pub fn IsEventDefined(p: *dynamicProcesses, io: Io, ev: EventName) bool {
        p.mu.lockUncancelable(io);
        defer p.mu.unlock(io);
        if (p.procMap.get(ev) == null) {
            return false;
        }

        return true;
    }
};

// Prepare and return a new *dynamicProcesses structure.
pub fn newDynamicProcesses(allocator: std.mem.Allocator) !*dynamicProcesses {
    const p = try allocator.create(dynamicProcesses);
    p.* = dynamicProcesses{
        .procMap = std.StringHashMap(*Process).init(allocator),
    };
    return p;
}

// Will create and return a new UUID prefix with "ED-".
// The caller owns the returned memory.
pub fn NewUUID(allocator: std.mem.Allocator, io: Io) ![]const u8 {
    // Generate a random version 4 UUID like the Go version does with
    // the google/uuid package.
    var b: [16]u8 = undefined;
    io.random(&b);
    b[6] = (b[6] & 0x0f) | 0x40;
    b[8] = (b[8] & 0x3f) | 0x80;

    const u = try std.fmt.allocPrint(allocator, "ED-{x:0>2}{x:0>2}{x:0>2}{x:0>2}-{x:0>2}{x:0>2}-{x:0>2}{x:0>2}-{x:0>2}{x:0>2}-{x:0>2}{x:0>2}{x:0>2}{x:0>2}{x:0>2}{x:0>2}", .{ b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7], b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15] });
    return u;
}

// ------------------------------------------------------------------------------
// Events and event functions.
// ------------------------------------------------------------------------------

// Router for normal events.
pub const EDRouter: EventName = "EDRouter";

// Process function for routing and handling events. Will check
// and route the event to the correct process.
pub const edRouterFn = ETFunc{ .run = edRouterFnRun };

fn edRouterFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;

    defer {
        slog.Debug("edRouterFn", .{ "stopping process with defered stop", "" });
        p.Stop();
    }

    while (true) {
        var ev = p.DynamicEventCh.getOne(p.io) catch {
            slog.Debug("edRouterFn", .{ "got ctx.Done, on", p.Config.NodeName });
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

        // Dynamic processes can take a little longer to start up and be
        // registered in the map. We check here if process is registred,
        // and if it is not we retry.
        // The checking is done in a task so the router don't block
        // here waiting, and we continue with the next event in the queue.
        //
        // NB, Zig specific: the inflight counter of the process is
        // incremented while the map lock is held, so a concurrent Stop of
        // the process waits with freeing it's memory until the delivery
        // below is done. The Go version needs no such pin, the garbage
        // collector keeps the process alive through the pointer.
        p.DynamicProcesses.mu.lockUncancelable(p.io);
        const procOpt = p.DynamicProcesses.procMap.get(ev.Name);
        if (procOpt) |proc| {
            _ = proc.inflight.fetchAdd(1, .acq_rel);
        }
        p.DynamicProcesses.mu.unlock(p.io);

        const proc = procOpt orelse {
            ctx.spawn(edRouterRetryDeliver, .{ p, ev }) catch {
                ev.deinit(p.allocator);
                return;
            };

            // The above task will wait, and check if the process becomes
            // available, and send the event if it the process eventually found,
            // so we can continue with the next event in the queue.
            continue;
        };
        defer _ = proc.inflight.fetchSub(1, .release);

        // Process was registered. Deliver the event to the process InCh.
        slog.Debug("edRouterFn", .{ "on", p.Config.NodeName, "Routing event", p.Event, "node", p.Config.NodeName, "name", ev.Name, "Inch", proc.InCh });

        proc.InCh.putOne(p.io, ev) catch |err| switch (err) {
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

// The retry task started by the router when no process was registered for
// the event yet. The equivalent of the `go func(ev Event)` in the Go version.
fn edRouterRetryDeliver(p: *Process, evIn: Event) void {
    const ev = evIn;

    // Try to 3 times to deliver the message.
    var i: usize = 0;
    while (i < 3) : (i += 1) {
        // The inflight counter is incremented while the map lock is held,
        // so a concurrent Stop of the found process waits with freeing it
        // until the delivery is done. See the comment in edRouterFnRun.
        p.DynamicProcesses.mu.lockUncancelable(p.io);
        const procOpt = p.DynamicProcesses.procMap.get(ev.Name);
        if (procOpt) |proc| {
            _ = proc.inflight.fetchAdd(1, .acq_rel);
        }
        p.DynamicProcesses.mu.unlock(p.io);

        const proc = procOpt orelse {
            slog.Error("edRouterFn", .{ "on", p.Config.NodeName, "found no process registered for the event type", ev.Name, "ev.DstNode", ev.DstNode });
            p.io.sleep(.fromSeconds(1), .awake) catch {
                ev.deinit(p.allocator);
                return;
            };
            continue;
        };
        defer _ = proc.inflight.fetchSub(1, .release);

        // Process is now registred, so we can safely put
        //the event on the InCh of the process.
        proc.InCh.putOne(p.io, ev) catch {
            ev.deinit(p.allocator);
            return;
        };

        return;
    }

    // All retries failed, the event is dropped and freed.
    ev.deinit(p.allocator);
}

// EDSync is used to syncronize events.
// The EDSyncFn that is to be used with this event type takes a
// signal channel, we can then use this event type to signal that
// another event is done before we continue by setting this event
// type as the NextEvent.
pub const EDSync: EventName = "EDSync";

// EtSyncFn is the function that will be used to syncronize events.
// It takes a channel that will be used to send a signal on when the
// EDSync event is executed.
//
// In general EDSync is used to syncronize one-off events, so delete
// the process after it is done with it's sync job.
//
// The Go version uses a chan struct{}, the Zig version uses a queue of
// u8 since a queue element can not be zero sized.
pub fn EDSyncFn(syncCh: *Io.Queue(u8)) ETFunc {
    return ETFunc{ .userdata = syncCh, .run = edSyncFnRun };
}

fn edSyncFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const syncCh: *Io.Queue(u8) = @ptrCast(@alignCast(userdata.?));

    while (true) {
        p.SignalReady();

        const ev = p.InCh.getOne(p.io) catch return;
        defer ev.deinit(p.allocator);

        syncCh.putOne(p.io, 0) catch return;
    }
}
