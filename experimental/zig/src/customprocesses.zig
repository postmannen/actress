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
const eventspkg = @import("events.zig");
const Event = eventspkg.Event;
const EventName = eventspkg.EventName;
const ETFunc = eventspkg.ETFunc;
const Process = actress.Process;
const setPreviousEvent = @import("staticprocesses.zig").setPreviousEvent;

// Holds information about what process functions who belongs to what
// event, and also a map of the started processes.
pub const customProcesses = struct {
    procMap: std.StringHashMap(*Process),
    mu: Io.Mutex = .init,

    // Add a new Event and it's process to the processes map.
    //
    // *******************************************************************
    // TODO: Consider if we still need this function. It is not in use.
    // *******************************************************************
    pub fn Add(p: *customProcesses, io: Io, et: EventName, proc: *Process) !void {
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
    pub fn Delete(p: *customProcesses, io: Io, et: EventName) void {
        p.mu.lockUncancelable(io);
        defer p.mu.unlock(io);
        if (p.procMap.get(et)) |existing| {
            existing.Cancel();
        }
        _ = p.procMap.remove(et);
        std.debug.print("deleted process {s}\n", .{et});
    }

    // Checks if the event is defined in the processes map, and returns true if it is.
    pub fn IsEventDefined(p: *customProcesses, io: Io, ev: EventName) bool {
        p.mu.lockUncancelable(io);
        defer p.mu.unlock(io);
        if (p.procMap.get(ev) == null) {
            return false;
        }

        return true;
    }
};

// Prepare and return a new *customProcesses structure.
pub fn newCustomProcesses(allocator: std.mem.Allocator) !*customProcesses {
    const p = try allocator.create(customProcesses);
    p.* = customProcesses{
        .procMap = std.StringHashMap(*Process).init(allocator),
    };
    return p;
}

// ------------------------------------------------------------------------------
// Events and event functions.
// ------------------------------------------------------------------------------

// Router for custom events.
pub const ECRouter: EventName = "ECRouter";

// Process function for routing and handling events. Will check
// and route the event to the correct process.
pub const ecRouterFn = ETFunc{ .run = ecRouterFnRun };

fn ecRouterFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;

    while (true) {
        var ev = p.CustomEventCh.getOne(p.io) catch {
            slog.Debug("ecRouterFn", .{ "got ctx.Done, on", p.Config.NodeName });
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

        // Custom processes can take a little longer to start up and be
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
        p.CustomProcesses.mu.lockUncancelable(p.io);
        const procOpt = p.CustomProcesses.procMap.get(ev.Name);
        if (procOpt) |proc| {
            _ = proc.inflight.fetchAdd(1, .acq_rel);
        }
        p.CustomProcesses.mu.unlock(p.io);

        const proc = procOpt orelse {
            ctx.spawn(ecRouterRetryDeliver, .{ p, ev }) catch {
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
        slog.Debug("ecRouterFn", .{ "on", p.Config.NodeName, "Routing event", p.Event, "node", p.Config.NodeName, "name", ev.Name, "Inch", proc.InCh });

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
// NB: Unlike the dynamic router retry, the Go version got no sleep between
// the retries here, and that is mirrored.
fn ecRouterRetryDeliver(p: *Process, evIn: Event) void {
    const ev = evIn;

    // Try to 3 times to deliver the message.
    var i: usize = 0;
    while (i < 3) : (i += 1) {
        // The inflight counter is incremented while the map lock is held,
        // so a concurrent Stop of the found process waits with freeing it
        // until the delivery is done. See the comment in ecRouterFnRun.
        p.CustomProcesses.mu.lockUncancelable(p.io);
        const procOpt = p.CustomProcesses.procMap.get(ev.Name);
        if (procOpt) |proc| {
            _ = proc.inflight.fetchAdd(1, .acq_rel);
        }
        p.CustomProcesses.mu.unlock(p.io);

        const proc = procOpt orelse {
            slog.Error("ecRouterFn", .{ "on", p.Config.NodeName, "found no process registered for the event type", ev.Name, "ev.DstNode", ev.DstNode });
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

// Primarily used for testing to check that the ECRouter properly routes events, and that
// custom processes start up correctly.
pub const ECGeneralDelivery: EventName = "ECGeneralDelivery";

// Primarily used for testing to check that the ECRouter properly routes events, and that
// custom processes start up correctly.
pub const ecGeneralDeliveryFn = ETFunc{ .run = ecGeneralDeliveryFnRun };

fn ecGeneralDeliveryFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    p.SignalReady();

    while (true) {
        const ev = p.InCh.getOne(p.io) catch {
            slog.Info("ecGeneralDeliveryFn", .{ "got ctx.Done, on", p.Config.NodeName });
            return;
        };
        defer ev.deinit(p.allocator);

        // Primarily used for tests. Will just forward the event data to defined NextEvent.
        if (ev.NextEvent) |nextEv| {
            // Put the event data in the data field of the next event while
            // it is added (and deep copied), and put back the old value so
            // the owned event tree can be freed without a double free.
            const oldData = nextEv.Data;
            nextEv.Data = ev.Data;
            p.AddEvent(nextEv.*) catch {};
            nextEv.Data = oldData;
        }
    }
}
