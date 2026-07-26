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
const Context = @import("context.zig").Context;
const Process = actress.Process;

// Event defines an event. It holds:
//   - The Name, which specifies the process are meant for.
//   - The Cmd, are meant to but not limited to be a way to give
//     instructions for what a process should do. The receiving
//     process are responsible for parsing the string slice into
//     something useful.
//   - The Data field are ment to carry the result from the work
//     done by a process, to the next process.
//   - Both Cmd and Data can be used interchangeably if it makes
//     more sense for a given scenario. No strict rules for this
//     exist. Just make sure to document the use of the given
//     Name, so the structure of how to use the fields exist.
//   - Err, are used by the error event type (ER).
//   - NextEvent are used when we want to define a chain of events
//     to be executed. The processes must make use of the field
//     for this to work. Check out the examples folder for a simple
//     example for how it could be implemented.
//
// NB, Zig specific: Go is garbage collected, so the Go version never
// frees events. In the Zig version AddEvent takes a deep copy of the
// event given as input, so the caller keeps ownership of what it passes
// in, and the copy is owned by the routing system. A process function
// that receives an event from it's InCh owns that event, and must free
// it with ev.deinit(p.allocator) when it is done with it.
pub const Event = struct {
    Nr: i64 = 0,
    // Name is a unique name to identify the type of the event.
    Name: EventName = "",
    // Cmd is usually used for giving instructions or parameters for
    // what an event shall do.
    Cmd: []const []const u8 = &.{},
    // Instruction got the underlying type of string. This field can
    // be used to give for example an instruction of a single word.
    // For example in switch statements at the receiving actor, or other.
    Instruction: Instruction = "",
    // Data usually carries the data from one process to the next. Example
    // could be a file read on process1 is put in the Data field, and
    // passed on to process2 to be unmarshaled.
    Data: []const u8 = &.{},
    // Data to be transfered internally. Example is to send config directly via
    // the channel between internal actors. Not serialized, and not part of the
    // deep copy taken by AddEvent, only the reference is copied.
    InternalCh: ?*Io.Queue([]const u8) = null,
    // Err is used for defining the error message when the event is used
    // as an error event. Go uses the error interface here, the Zig version
    // carries the error message text.
    Err: ?[]const u8 = null,
    // NextEvent defines a series of events to be executed like a workflow.
    // The receiving process should check this field for what kind of event
    // to create as the next step in the workflow.
    NextEvent: ?*Event = null,
    // PreviousEvent allows for keeping information about the previous event if needed.
    PreviousEvent: ?*Event = null,
    // Dst node.
    DstNode: Node = "",
    // Src node.
    SrcNode: Node = "",

    // Free all the memory owned by the event, including the NextEvent and
    // PreviousEvent chains. Must only be used on events that came out of
    // the routing system (from a process InCh), since those are always
    // deep copies owned by the receiver. Events constructed by the user
    // reference the user's own memory and must not be freed with deinit.
    pub fn deinit(ev: *const Event, allocator: std.mem.Allocator) void {
        freeString(allocator, ev.Name);
        for (ev.Cmd) |c| {
            freeString(allocator, c);
        }
        if (ev.Cmd.len > 0) {
            allocator.free(ev.Cmd);
        }
        freeString(allocator, ev.Instruction);
        freeString(allocator, ev.Data);
        if (ev.Err) |e| {
            freeString(allocator, e);
        }
        if (ev.NextEvent) |ne| {
            ne.deinit(allocator);
            allocator.destroy(ne);
        }
        if (ev.PreviousEvent) |pe| {
            pe.deinit(allocator);
            allocator.destroy(pe);
        }
        freeString(allocator, ev.DstNode);
        freeString(allocator, ev.SrcNode);
    }
};

// Zero length strings are the default field values and are never
// allocated, so they are skipped when freeing.
fn freeString(allocator: std.mem.Allocator, s: []const u8) void {
    if (s.len == 0) {
        return;
    }
    allocator.free(s);
}

fn dupeString(allocator: std.mem.Allocator, s: []const u8) ![]const u8 {
    if (s.len == 0) {
        return "";
    }
    const d = try allocator.dupe(u8, s);
    return d;
}

// CopyEvent takes a deep copy of the event given as input, so the copy
// owns all it's memory. Used by AddEvent when an event enters the routing
// system. The InternalCh reference is copied as is, it is not owned.
pub fn CopyEvent(allocator: std.mem.Allocator, ev: Event) std.mem.Allocator.Error!Event {
    var cmd: []const []const u8 = &.{};
    if (ev.Cmd.len > 0) {
        const c = try allocator.alloc([]const u8, ev.Cmd.len);
        for (ev.Cmd, 0..) |s, i| {
            c[i] = try dupeString(allocator, s);
        }
        cmd = c;
    }

    var nextEvent: ?*Event = null;
    if (ev.NextEvent) |ne| {
        nextEvent = try copyEventPtr(allocator, ne.*);
    }
    var previousEvent: ?*Event = null;
    if (ev.PreviousEvent) |pe| {
        previousEvent = try copyEventPtr(allocator, pe.*);
    }

    const e = Event{
        .Nr = ev.Nr,
        .Name = try dupeString(allocator, ev.Name),
        .Cmd = cmd,
        .Instruction = try dupeString(allocator, ev.Instruction),
        .Data = try dupeString(allocator, ev.Data),
        .InternalCh = ev.InternalCh,
        .Err = if (ev.Err) |err| try dupeString(allocator, err) else null,
        .NextEvent = nextEvent,
        .PreviousEvent = previousEvent,
        .DstNode = try dupeString(allocator, ev.DstNode),
        .SrcNode = try dupeString(allocator, ev.SrcNode),
    };

    return e;
}

// Allocate an event on the heap and deep copy the input event into it.
fn copyEventPtr(allocator: std.mem.Allocator, ev: Event) std.mem.Allocator.Error!*Event {
    const e = try allocator.create(Event);
    errdefer allocator.destroy(e);
    e.* = try CopyEvent(allocator, ev);
    return e;
}

pub const Instruction = []const u8;

// NewDynProcess will prepare and return a *Process. It will copy
// channels and map structures from the root process.
// The purpose of dynamic processes is to have short lived processes
// that can be quickly started, and removed again when it's job is done.
// The only difference between a process and a dynamic process are that
// the dynamic processes have a mutex in processes map DynamicProcesses so
// we also can delete the processes when they are no longer needed.

pub const Node = []const u8;

pub const EventOpt = *const fn (*Event) void;

pub fn NewEvent(allocator: std.mem.Allocator, et: EventName, opts: []const EventOpt) !*Event {
    const ev = try allocator.create(Event);
    ev.* = Event{ .Name = et };
    for (opts) |opt| {
        opt(ev);
    }
    return ev;
}

// The functional options of the Go version take the value to set as an
// argument and return a closure. Zig got no closures, so the option
// functions here are written by the user as plain functions that set the
// wanted field on the event, and passed to NewEvent in the opts slice.

// Name is a unique name used to identify events. It is used both for
// creating processes and also for routing messages to the correct process.
pub const EventName = []const u8;

// The main Root process. By default the root process don't have an ETFunc
// registered with it to handle the ETRoot eventtype, but one can be created
// with the normal ETFunc function signature, and defined when creating a
// new root process.
pub const ETRoot: EventName = "ETRoot";

// Function type describing the signature of a function that is to be used
// when creating a new process.
//
// The Go version is a function that takes the context and the process and
// returns a closure to run as the goroutine body. Zig got no closures, so
// the equivalent here is a struct holding a function pointer together with
// an optional userdata pointer. Factory functions like ETTestfn and
// EDSyncFn, that in Go capture a channel in the returned closure, instead
// put the captured value in userdata. For plain functions the userdata is
// null. The run function is started as a task when Act is called on the
// process.
pub const ETFunc = struct {
    userdata: ?*anyopaque = null,
    run: *const fn (userdata: ?*anyopaque, ctx: *Context, p: *Process) void,
};
