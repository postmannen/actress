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
const Instruction = eventspkg.Instruction;
const ETFunc = eventspkg.ETFunc;
const Process = actress.Process;
const setPreviousEvent = @import("staticprocesses.zig").setPreviousEvent;

// processes holds information about what process functions
// who belongs to what event, and also a map of the started
// processes.
pub const errorProcesses = struct {
    procMap: std.StringHashMap(*Process),

    // // Delete an Event and it's process from the processes map.
    // fn delete(p: *errorProcesses, et: EventName, proc: *Process) void {
    //     p.procMap.get(et).?.Cancel();
    //     _ = p.procMap.remove(et);
    // }

    // Checks if the event is defined in the processes map, and returns true if it is.
    pub fn IsEventDefined(p: *errorProcesses, ev: EventName) bool {
        if (p.procMap.get(ev) == null) {
            return false;
        }

        return true;
    }
};

// Prepare and return a new *processes structure.
pub fn newErrorProcesses(allocator: std.mem.Allocator) !*errorProcesses {
    const p = try allocator.create(errorProcesses);
    p.* = errorProcesses{
        .procMap = std.StringHashMap(*Process).init(allocator),
    };
    return p;
}

// Router for error events.
pub const ERRouter: EventName = "ERRouter";

// Process function for routing and handling events.
pub const erRouterFn = ETFunc{ .run = erRouterFnRun };

fn erRouterFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        var e = p.ErrorEventCh.getOne(p.io) catch {
            slog.Debug("erRouterFn", .{ "got ctx.Done, on", p.Config.NodeName });
            return;
        };

        // If there is a next event defined, we make a copy of all the fields  of the current event,
        // and put that as the previousEvent on the next event. We can use this information later
        // if need to check something in the previous event.
        if (e.NextEvent) |ne| {
            // Keep the information about the current event, so we are able to check for things
            // like ackTimeout and what node to reply back to if ack should be given.
            setPreviousEvent(p.allocator, ne, &e);
        }

        // NB: Like the Go version there is no check here if a process is
        // registered for the event name, a missing process is a panic.
        const inCh = p.ErrorProcesses.procMap.get(e.Name).?.InCh;

        slog.Debug("erRouterFn", .{ "Routing event", p.Event, "node", p.Config.NodeName, "name", e.Name, "Inch", inCh });
        inCh.putOne(p.io, e) catch |err| switch (err) {
            error.Canceled => {
                e.deinit(p.allocator);
                return;
            },
            error.Closed => {
                e.deinit(p.allocator);
                continue;
            },
        };
    }
}

// Instructions for error logging.
pub const InstructionError: Instruction = "InstructionError";
pub const InstructionInfo: Instruction = "InstructionInfo";
pub const InstructionDebug: Instruction = "InstructionDebug";
pub const InstructionFatal: Instruction = "InstructionFatal";

// Log errors.
pub const ERLog: EventName = "ERLog";

// Will log errors to the console based on the Instruction field of the event.
//
// NB: The "none" is handled in the AddEvent function, to drop the event as
// early as possible, instead of sending it all the way to be dropped here.
pub const erLogFn = ETFunc{ .run = erLogFnRun };

fn erLogFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        p.SignalReady();

        const er = p.InCh.getOne(p.io) catch return;
        defer er.deinit(p.allocator);

        // The Go version switches on the Instruction value. Zig can not
        // switch on strings, so an if/else chain is used instead.
        if (std.mem.eql(u8, er.Instruction, InstructionError)) {
            slog.Error("erLogFn", .{ "instructionError, msg", er.Err });
        } else if (std.mem.eql(u8, er.Instruction, InstructionInfo)) {
            slog.Info("erLogFn", .{ "instructionInfo, msg", er.Err });
        } else if (std.mem.eql(u8, er.Instruction, InstructionDebug)) {
            slog.Debug("erLogFn", .{ "instructionDebug, msg", er.Err });
        } else if (std.mem.eql(u8, er.Instruction, InstructionFatal)) {
            slog.Error("erLogFn", .{ "instructionFatal, msg", er.Err });
            std.process.exit(1);
        } else {
            slog.Error("erLogFn", .{ "default, msg", er.Err });
        }
    }
}

// Log and exit system.
pub const ERTest: EventName = "ERTest";

pub const erTestFn = ETFunc{ .run = erTestFnRun };

fn erTestFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        p.SignalReady();

        const er = p.InCh.getOne(p.io) catch return;
        defer er.deinit(p.allocator);

        // The Go version formats the message in a goroutine and drops it.
        // The formatted value is just discarded here as well.
        _ = er.Err;
    }
}

// Will drop the event if it is an error event.
pub const ERNone: EventName = "ERNone";

// Process function for dropping error events. Primarily used for testing.
pub const erNoneFn = ETFunc{ .run = erNoneFnRun };

fn erNoneFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        p.SignalReady();

        const er = p.InCh.getOne(p.io) catch return;
        defer er.deinit(p.allocator);

        _ = er.Err;
    }
}
