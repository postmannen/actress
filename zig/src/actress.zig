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

pub const slog = @import("slog.zig");
pub const Context = @import("context.zig").Context;

const events = @import("events.zig");
pub const Event = events.Event;
pub const EventName = events.EventName;
pub const EventOpt = events.EventOpt;
pub const NewEvent = events.NewEvent;
pub const CopyEvent = events.CopyEvent;
pub const Node = events.Node;
pub const Instruction = events.Instruction;
pub const ETFunc = events.ETFunc;
pub const ETRoot = events.ETRoot;

const configpkg = @import("config.zig");
pub const Config = configpkg.Config;
pub const NewConfig = configpkg.NewConfig;
pub const CheckEnv = configpkg.CheckEnv;

const bufferpkg = @import("buffer.zig");
pub const Buffer = bufferpkg.Buffer;
pub const NewBuffer = bufferpkg.NewBuffer;

const eventrwpkg = @import("eventrw.zig");
pub const EventRW = eventrwpkg.EventRW;
pub const NewEventRW = eventrwpkg.NewEventRW;

const staticpkg = @import("staticprocesses.zig");
pub const staticProcesses = staticpkg.staticProcesses;
pub const CopyEventFields = staticpkg.CopyEventFields;
pub const ETRemote = staticpkg.ETRemote;
pub const ETRouter = staticpkg.ETRouter;
pub const ETOsSignal = staticpkg.ETOsSignal;
pub const ETTest = staticpkg.ETTest;
pub const InstructionCmdEOF = staticpkg.InstructionCmdEOF;
pub const ETTestfn = staticpkg.ETTestfn;
pub const ETTestCh = staticpkg.ETTestCh;
pub const ETPidGetAll = staticpkg.ETPidGetAll;
pub const ETDone = staticpkg.ETDone;
pub const ETPrint = staticpkg.ETPrint;
pub const ETExit = staticpkg.ETExit;
pub const ETPid = staticpkg.ETPid;
pub const ETReadFile = staticpkg.ETReadFile;
pub const ETReadFileFn = staticpkg.ETReadFileFn;

const errorpkg = @import("errorprocesses.zig");
pub const errorProcesses = errorpkg.errorProcesses;
pub const ERRouter = errorpkg.ERRouter;
pub const InstructionError = errorpkg.InstructionError;
pub const InstructionInfo = errorpkg.InstructionInfo;
pub const InstructionDebug = errorpkg.InstructionDebug;
pub const InstructionFatal = errorpkg.InstructionFatal;
pub const ERLog = errorpkg.ERLog;
pub const ERTest = errorpkg.ERTest;
pub const ERNone = errorpkg.ERNone;

const dynamicpkg = @import("dynamicprocesses.zig");
pub const dynamicProcesses = dynamicpkg.dynamicProcesses;
pub const NewUUID = dynamicpkg.NewUUID;
pub const EDRouter = dynamicpkg.EDRouter;
pub const EDSync = dynamicpkg.EDSync;
pub const EDSyncFn = dynamicpkg.EDSyncFn;

const custompkg = @import("customprocesses.zig");
pub const customProcesses = custompkg.customProcesses;
pub const ECRouter = custompkg.ECRouter;
pub const ECGeneralDelivery = custompkg.ECGeneralDelivery;
pub const ecGeneralDeliveryFn = custompkg.ecGeneralDeliveryFn;

const supervisorpkg = @import("supervisorprocesses.zig");
pub const supervisorProcesses = supervisorpkg.supervisorProcesses;
pub const ESRouter = supervisorpkg.ESRouter;
pub const ESProcesses = supervisorpkg.ESProcesses;
pub const InstructionESProcessesAdd = supervisorpkg.InstructionESProcessesAdd;
pub const InstructionESProcessesDelete = supervisorpkg.InstructionESProcessesDelete;
pub const InstructionESProcessesGetAll = supervisorpkg.InstructionESProcessesGetAll;
pub const esProcessesMapDataIn = supervisorpkg.esProcessesMapDataIn;
pub const ESProcessesMap = supervisorpkg.ESProcessesMap;

pub const pidnr = i64;
pub const PidVsProcMap = std.AutoHashMap(pidnr, *Process);
pub const PidVsNameMap = std.AutoHashMap(pidnr, []const u8);

// Holds information about the relation between a pid and it's process.
pub const pidToProc = struct {
    mu: Io.Mutex = .init,
    mp: PidVsProcMap,
    io: Io,

    // Add a pid and process to the map.
    fn add(p: *pidToProc, pid: pidnr, proc: *Process) !void {
        p.mu.lockUncancelable(p.io);
        defer p.mu.unlock(p.io);
        try p.mp.put(pid, proc);
    }

    // Delete a pid and process from the map.
    fn remove(p: *pidToProc, pid: pidnr) void {
        p.mu.lockUncancelable(p.io);
        defer p.mu.unlock(p.io);
        _ = p.mp.remove(pid);
    }

    // Return a copy of the pid vs *Processes map. The caller owns the
    // returned map and must call deinit on it, and destroy the pointer.
    //
    // NB, Zig specific: the process pointers in the returned copy are not
    // protected against a concurrent Stop of a dynamic or custom process,
    // which frees the process memory. Use copyOfNamesMap when the process
    // names are what is needed, it takes the copy of the names while the
    // map lock is held.
    pub fn copyOfMap(p: *pidToProc, allocator: std.mem.Allocator) !*PidVsProcMap {
        const m = try allocator.create(PidVsProcMap);
        m.* = PidVsProcMap.init(allocator);

        p.mu.lockUncancelable(p.io);
        defer p.mu.unlock(p.io);

        var it = p.mp.iterator();
        while (it.next()) |entry| {
            try m.put(entry.key_ptr.*, entry.value_ptr.*);
        }

        return m;
    }

    // Return a copy of the pid vs process name map. The names are duped
    // while the map lock is held, so the copy stays valid also when a
    // process is stopped and freed after this function returns. The
    // caller owns the returned map, and must free the name values, call
    // deinit on the map, and destroy the pointer. Zig specific, the Go
    // version can safely hand out the process pointers since the garbage
    // collector keeps them alive.
    pub fn copyOfNamesMap(p: *pidToProc, allocator: std.mem.Allocator) !*PidVsNameMap {
        const m = try allocator.create(PidVsNameMap);
        errdefer allocator.destroy(m);
        m.* = PidVsNameMap.init(allocator);
        errdefer {
            var eit = m.iterator();
            while (eit.next()) |entry| {
                allocator.free(entry.value_ptr.*);
            }
            m.deinit();
        }

        p.mu.lockUncancelable(p.io);
        defer p.mu.unlock(p.io);

        var it = p.mp.iterator();
        while (it.next()) |entry| {
            const name = try allocator.dupe(u8, entry.value_ptr.*.Event);
            errdefer allocator.free(name);
            try m.put(entry.key_ptr.*, name);
        }

        return m;
    }
};

// Is the main counter used for assigning Nr to events.
pub const eventNr = struct {
    mu: Io.Mutex = .init,
    nr: i64 = 0,
    io: Io,
};

// Create and return a new *pidToProc struct.
fn newPidToProc(allocator: std.mem.Allocator, io: Io) pidToProc {
    const p = pidToProc{
        .mp = PidVsProcMap.init(allocator),
        .io = io,
    };
    return p;
}

// Create and return a new *eventNr struct.
// This function is normally used only when creatiing the root process.
// The child processes will inherit the eventNr from the root process
// when the newProcess function is called.
fn newEventNr(allocator: std.mem.Allocator, io: Io, start: i64) !*eventNr {
    const e = try allocator.create(eventNr);
    e.* = eventNr{ .nr = start, .io = io };
    return e;
}

pub const pids = struct {
    mu: Io.Mutex = .init,
    nr: pidnr = 0,
    toProc: pidToProc,
    io: Io,

    // Will return the next pidnr for a new process.
    fn next(p: *pids) pidnr {
        p.mu.lockUncancelable(p.io);
        const nr = p.nr;
        p.nr += 1;
        p.mu.unlock(p.io);

        return nr;
    }
};

// Create and return a new *pids struct.
fn newPids(allocator: std.mem.Allocator, io: Io) !*pids {
    const p = try allocator.create(pids);
    p.* = pids{
        .nr = 0,
        .toProc = newPidToProc(allocator, io),
        .io = io,
    };

    return p;
}

// Zig specific: a registry of every process created, so all the memory can
// be freed with Deinit when the system is shut down. Go relies on the
// garbage collector for this. A stopped process that is freed by the
// reaper is removed from the registry first, so Deinit does not free it
// twice.
const processRegistry = struct {
    mu: Io.Mutex = .init,
    list: std.ArrayList(*Process) = .empty,
    io: Io,

    fn add(r: *processRegistry, allocator: std.mem.Allocator, proc: *Process) !void {
        r.mu.lockUncancelable(r.io);
        defer r.mu.unlock(r.io);
        try r.list.append(allocator, proc);
    }

    fn remove(r: *processRegistry, proc: *Process) void {
        r.mu.lockUncancelable(r.io);
        defer r.mu.unlock(r.io);
        for (r.list.items, 0..) |item, i| {
            if (item == proc) {
                _ = r.list.swapRemove(i);
                return;
            }
        }
    }
};

// Process defines a process.
pub const Process = struct {
    // Process function.
    @"fn": ?ETFunc = null,
    // Signals that the process is ready. The Go version uses a channel
    // that is closed with a sync.Once, the Io.Event set here is idempotent
    // in the same way. Created in Act, like the readyCh in Go.
    readyCh: ?*Io.Event = null,
    // Channel to receive events into the process function.
    InCh: *Io.Queue(Event),
    // Channel to send events to be picked up by other processes.
    StaticEventCh: *Io.Queue(Event),
    // Channel to send error events.
    ErrorEventCh: *Io.Queue(Event),
    // Channel for getting the result in tests.
    TestCh: *Io.Queue(Event),
    // Channel to use for routing events for dynamic processes.
    DynamicEventCh: *Io.Queue(Event),
    // Channel to use for routing events for custom processes.
    CustomEventCh: *Io.Queue(Event),
    // Channel to use for routing supervisor events
    SupervisorEventCh: *Io.Queue(Event),
    // The event type for the process.
    Event: EventName,
    // Maps for various staticProcess information.
    // NB: Added a Mutex on this structure, though it should really not be needed,
    //    since there is only reads from the static procMap. Decide later if we should
    //    remove it again.
    StaticProcesses: *staticProcesses,
    // Map of dynamic processes
    DynamicProcesses: *dynamicProcesses,
    // Map of custom processes
    CustomProcesses: *customProcesses,
    // Maps for various errProcess information
    ErrorProcesses: *errorProcesses,
    // Map of supervisor processes
    supervisorProcesses: *supervisorProcesses,
    // The main counter used for assigning Nr to events.
    EventNr: *eventNr,
    // Is this the root process.
    isRoot: bool = false,
    // Holding all configuration settings.
    Config: *Config,
    // process ID struct
    pids: *pids,
    // PID of the process
    PID: pidnr,
    // The context of the process
    Ctx: *Context,
    // Zig specific: the allocator and io implementation used by the whole
    // process tree, and the registry used to free all processes at shutdown.
    allocator: std.mem.Allocator,
    io: Io,
    registry: *processRegistry,
    // Zig specific: the number of deliveries to this process InCh that are
    // in flight from the routers. The dynamic and custom routers increment
    // the counter while holding the map lock of the process lookup, and
    // decrement it when the delivery is done. The reaper waits for the
    // counter to reach zero before the memory of the process is freed, so
    // a delivery can never touch a freed process.
    inflight: std.atomic.Value(usize) = .init(0),
    // Zig specific: set by Stop so a second call to Stop is a no-op, and
    // the process can not be handed to the reaper twice.
    stopped: std.atomic.Value(bool) = .init(false),
    // Zig specific: the channel to the reaper task, shared by all
    // processes in the tree like the event channels. Stop hands the
    // process over on this channel, and the reaper cancels it and frees
    // the memory. Go leaves both to the garbage collector.
    reapCh: *Io.Queue(*Process),
    // Zig specific: the group holding the reaper task. Only set on the
    // root process. The reaper runs outside the context tree, so a
    // shutdown cancel of the tree never waits on the reaper while the
    // reaper waits on the shutdown.
    reapGroup: ?*Io.Group = null,

    // Will Cancel the context attached to the process. The Go version
    // stores the context.CancelFunc in the Cancel field, here it is a
    // method that cancels the process context.
    pub fn Cancel(p: *Process) void {
        p.Ctx.cancel();
    }

    // AddEvent will deliver the event to the correct router based
    // on the specified Kind of the Event.
    // If the Kind are missing the event will be handled as a static
    // event.
    // If the event is to be delivered to a remote node, AddEvent will also
    // take care of that and ship the event off to the ETRemote process.
    //
    // NB, Zig specific: AddEvent takes a deep copy of the event, so the
    // caller keeps ownership of the memory referenced by the input event.
    // The copy is owned by the routing system, and the process that
    // receives it from it's InCh must free it with ev.deinit().
    pub fn AddEvent(p: *Process, event: Event) !void {
        const evNr = p.IncrementEventNr();

        if (!std.mem.eql(u8, event.DstNode, p.Config.NodeName) and event.DstNode.len != 0) {
            var ev = try CopyEvent(p.allocator, event);
            ev.Nr = evNr;
            const inner = try p.allocator.create(Event);
            inner.* = ev;

            const remoteEv = Event{
                .Name = try p.allocator.dupe(u8, ETRemote),
                .Nr = evNr,
                .NextEvent = inner,
            };

            try p.addEventStatic(remoteEv);
            return;
        }
        // -------------------------------------------------------------
        const s = event.Name;
        if (s.len < 2) {
            std.debug.panic("unknown event.Name: {s}", .{event.Name});
        }
        if (s[0] != 'E') { // all your kinds start with 'E'
            std.debug.panic("unknown event.Name, should start with E: {s}", .{event.Name});
        }

        var ev = try CopyEvent(p.allocator, event);
        ev.Nr = evNr;

        switch (s[1]) {
            'T' => try p.addEventStatic(ev), // ET*
            'R' => try p.addEventError(ev), // ER*
            'D' => try p.addEventDynamic(ev), // ED*
            'C' => try p.addEventCustom(ev), // EC*
            'S' => try p.addEventSuperVisor(ev), // ES*
            else => std.debug.panic("unknown event.Name, got default case, should start with E: {s}", .{event.Name}),
        }
    }

    // Will add an event to be handled by the processes.
    fn addEventSuperVisor(p: *Process, event: Event) !void {
        p.SupervisorEventCh.putOne(p.io, event) catch |err| switch (err) {
            error.Closed => event.deinit(p.allocator),
            error.Canceled => {
                event.deinit(p.allocator);
                return err;
            },
        };
    }

    // Will add an event to be handled by the processes.
    // Mirrors the Go select with a time.After of 5 seconds: first a
    // non-blocking put is tried, and if the channel is full an Io.Select
    // with a put task and a timeout task is used, which is the Zig
    // equivalent of the Go select statement.
    fn addEventStatic(p: *Process, event: Event) !void {
        const n = p.StaticEventCh.put(p.io, &.{event}, 0) catch |err| switch (err) {
            error.Closed => {
                event.deinit(p.allocator);
                return;
            },
            error.Canceled => {
                event.deinit(p.allocator);
                return err;
            },
        };
        if (n == 1) {
            return;
        }

        const U = union(enum) {
            put: (Io.QueueClosedError || Io.Cancelable)!void,
            timeout: Io.Cancelable!void,
        };
        var selBuf: [2]U = undefined;
        var sel = Io.Select(U).init(p.io, &selBuf);

        sel.concurrent(.put, addEventStaticPut, .{ p.StaticEventCh, p.io, event }) catch |err| {
            event.deinit(p.allocator);
            return err;
        };
        sel.concurrent(.timeout, addEventStaticTimeout, .{p.io}) catch {
            // Could not start the timeout task, just wait for the put task
            // to finish like a plain blocking send.
            finishSelect(U, &sel, p, event, false);
            return;
        };

        const first = sel.await() catch |err| {
            finishSelect(U, &sel, p, event, false);
            return err;
        };
        switch (first) {
            .put => |res| {
                sel.cancelDiscard();
                if (res) |_| {} else |_| {
                    event.deinit(p.allocator);
                }
            },
            .timeout => {
                slog.Error("addEventStatic", .{ "TIMEOUT: reason...one of the later AddEvent commands probably are not working well. Check the debug output", event.Name });
                finishSelect(U, &sel, p, event, false);
            },
        }
    }

    // Cancel the remaining select tasks and check if the put task managed
    // to deliver the event. If it did not, the event is dropped and freed.
    fn finishSelect(comptime U: type, sel: *Io.Select(U), p: *Process, event: Event, deliveredIn: bool) void {
        var delivered = deliveredIn;
        while (sel.cancel()) |u| {
            switch (u) {
                .put => |res| {
                    if (res) |_| {
                        delivered = true;
                    } else |_| {}
                },
                .timeout => {},
            }
        }
        if (!delivered) {
            event.deinit(p.allocator);
        }
    }

    // Will add an event to be handled by the processes.
    fn addEventDynamic(p: *Process, event: Event) !void {
        p.DynamicEventCh.putOne(p.io, event) catch |err| switch (err) {
            error.Closed => event.deinit(p.allocator),
            error.Canceled => {
                event.deinit(p.allocator);
                return err;
            },
        };
    }

    // Will add an event to be handled by the processes.
    fn addEventCustom(p: *Process, event: Event) !void {
        p.CustomEventCh.putOne(p.io, event) catch |err| switch (err) {
            error.Closed => event.deinit(p.allocator),
            error.Canceled => {
                event.deinit(p.allocator);
                return err;
            },
        };
    }

    // Will add an error to be handled by the error processes.
    fn addEventError(p: *Process, event: Event) !void {
        p.ErrorEventCh.putOne(p.io, event) catch |err| switch (err) {
            error.Closed => event.deinit(p.allocator),
            error.Canceled => {
                event.deinit(p.allocator);
                return err;
            },
        };
    }

    // Will start the ETFunc attached to the process.
    //
    // If no ETFunc is defined for the process will just return
    // after calling this function. The process can still be used
    // and we can communicate with it via it's channels.
    pub fn Act(p: *Process) !void {
        if (!std.mem.eql(u8, p.Config.LogLevel, "none")) {
            std.debug.print("on node {s}: Starting actor for Name: {s}\n", .{ p.Config.NodeName, p.Event });
        }

        try p.pids.toProc.add(p.PID, p);

        try p.addToProcessesMap();

        if (p.@"fn" != null) {
            // Create a channel to signal that the process is ready.
            // This will either be close by the user inside the p.fn function,
            // or by timeout, incase the user forgot to put in the calling of
            // theSignalReady function.
            const rc = try p.allocator.create(Io.Event);
            rc.* = .unset;
            p.readyCh = rc;

            if (!p.Ctx.isCanceled()) {
                try p.Ctx.spawn(runProcessFn, .{p});
            }

            p.WaitForReady();
        }
    }

    // Signal that the function is ready.
    pub fn SignalReady(p: *Process) void {
        // Setting the Io.Event is idempotent, so multiple calls are no-ops
        // just like the sync.Once protected channel close in the Go version.
        if (p.readyCh) |rc| {
            rc.set(p.io);
        }
    }

    // Wait for the process function to be ready and started for the specific process.
    pub fn WaitForReady(p: *Process) void {
        if (p.readyCh) |rc| {
            rc.waitTimeout(p.io, .{ .duration = .{ .raw = .fromMilliseconds(5), .clock = .awake } }) catch |err| switch (err) {
                // If the timeout is reached, there the user have most likely forgotten to call the SignalReady function,
                // so we wait for the defined amout of time, and assume that the process is ready.
                error.Timeout => {
                    slog.Debug("WaitForReady", .{ "should be ready, signal ready based on TIMEOUT", p.Event });
                    return;
                },
                error.Canceled => return,
            };
            // If the event is set by the user inside the p.fn function, the process function is started.
            slog.Debug("WaitForReady", .{ "is ready", p.Event });
        }
    }

    // actForRoot is similar to the Act function, but it is used for starting the root process
    // so we can register the processes in ESProcesses after all the processes are started.
    fn actForRoot(p: *Process, pi: *registerProcessInfo) !void {
        if (!std.mem.eql(u8, p.Config.LogLevel, "none")) {
            std.debug.print("on node {s}: ROOT ACTOR: Starting actor for Name: {s}\n", .{ p.Config.NodeName, p.Event });
        }
        try p.pids.toProc.add(p.PID, p);

        try p.addToProcessesMap();

        if (p.@"fn" != null) {
            try p.Ctx.spawn(runProcessFn, .{p});
        }

        const toRegister = esProcessesMapDataIn{
            .Name = p.Event,
        };

        try pi.append(p.allocator, toRegister);
    }

    // Will Cancel the context attached to the process.
    // Will delete the process from the processes map.
    // Will delete the pid from the pids map.
    //
    // NB, Zig specific: the Go version calls p.Cancel() directly here,
    // which just signals, and the garbage collector later frees the
    // process when nothing references it anymore. The Zig Cancel waits
    // for the tasks of the process to finish, which would deadlock when
    // a process function calls Stop on it's own process. The cancel and
    // the freeing of the memory are therefore both done by the reaper
    // task, so Stop can be called from anywhere, like in Go. A second
    // call to Stop is a no-op, since the process must not be handed to
    // the reaper twice.
    pub fn Stop(p: *Process) void {
        if (p.stopped.swap(true, .acq_rel)) {
            return;
        }

        p.deleteFromProcessesMap();
        p.pids.toProc.remove(p.PID);

        // Hand the process over to the reaper, see reaperFn. If the
        // channel is closed the system is shutting down, and the memory
        // is freed by Deinit through the registry instead.
        p.reapCh.putOneUncancelable(p.io, p) catch {};
    }

    // IncrementEventNr atomically increments the shared event counter and returns the new value.
    pub fn IncrementEventNr(p: *Process) i64 {
        p.EventNr.mu.lockUncancelable(p.io);
        defer p.EventNr.mu.unlock(p.io);
        p.EventNr.nr += 1;
        return p.EventNr.nr;
    }

    // CurrentEventNr returns the current value of the shared event counter without modifying it.
    pub fn CurrentEventNr(p: *Process) i64 {
        p.EventNr.mu.lockUncancelable(p.io);
        defer p.EventNr.mu.unlock(p.io);
        return p.EventNr.nr;
    }

    // Add a new Event and it's process to the processes map.
    // Check if a process for the same event is defined, if so we
    // cancel the current process before we replace it with a new one.
    //
    // NB, Zig specific: in Go the cancel of an existing process happens while
    // the map mutex is held, since a Go cancel just signals and returns. The
    // Zig cancel waits for the tasks of the process to finish, and a finishing
    // task may itself need the map mutex, so here the existing process is
    // taken out under the lock and canceled after the lock is released.
    fn addToProcessesMap(p: *Process) !void {
        const s = p.Event;
        var existing: ?*Process = null;

        switch (s[1]) {
            'T' => {
                p.StaticProcesses.mu.lockUncancelable(p.io);
                defer p.StaticProcesses.mu.unlock(p.io);
                existing = p.StaticProcesses.procMap.get(p.Event);
                try p.StaticProcesses.procMap.put(p.Event, p);
            },
            'D' => {
                p.DynamicProcesses.mu.lockUncancelable(p.io);
                defer p.DynamicProcesses.mu.unlock(p.io);
                existing = p.DynamicProcesses.procMap.get(p.Event);
                try p.DynamicProcesses.procMap.put(p.Event, p);
            },
            'C' => {
                p.CustomProcesses.mu.lockUncancelable(p.io);
                defer p.CustomProcesses.mu.unlock(p.io);
                existing = p.CustomProcesses.procMap.get(p.Event);
                try p.CustomProcesses.procMap.put(p.Event, p);
            },
            'R' => {
                existing = p.ErrorProcesses.procMap.get(p.Event);
                try p.ErrorProcesses.procMap.put(p.Event, p);
            },
            'S' => {
                p.supervisorProcesses.mu.lockUncancelable(p.io);
                defer p.supervisorProcesses.mu.unlock(p.io);
                existing = p.supervisorProcesses.procMap.get(p.Event);
                try p.supervisorProcesses.procMap.put(p.Event, p);
            },
            else => {},
        }

        if (existing) |e| {
            e.Cancel();
        }
    }

    // Delete an Event and it's process from the processes map.
    fn deleteFromProcessesMap(p: *Process) void {
        // Check if a process for the same event is defined, and if so we
        // cancel the current process before we replace it with a new one.

        const s = p.Event;
        switch (s[1]) {
            'T' => {
                // slog.Error("", .{ "msg", "not allowed to delete static process" });
            },
            'D' => {
                p.DynamicProcesses.mu.lockUncancelable(p.io);
                _ = p.DynamicProcesses.procMap.remove(p.Event);
                p.DynamicProcesses.mu.unlock(p.io);
            },
            'C' => {
                p.CustomProcesses.mu.lockUncancelable(p.io);
                _ = p.CustomProcesses.procMap.remove(p.Event);
                p.CustomProcesses.mu.unlock(p.io);
            },
            'R' => {
                // slog.Error("", .{ "msg", "not allowed to delete error process" });
            },
            'S' => {
                // slog.Error("", .{ "msg", "not allowed to delete supervisor process" });
            },
            else => {},
        }
    }

    // Deinit frees all the memory of the whole process tree. Zig specific,
    // Go leaves this to the garbage collector. Must be called on the root
    // process, after the context of the system have been canceled so no
    // tasks are running anymore.
    pub fn Deinit(p: *Process) void {
        const allocator = p.allocator;
        const io = p.io;

        // Shut down the reaper first, so it is not freeing a process
        // while the rest of the teardown runs. Closing the channel makes
        // the reaper finish the processes already handed to it and then
        // return, the group cancel waits for the reaper task to be done.
        p.reapCh.close(io);
        if (p.reapGroup) |g| {
            g.cancel(io);
            allocator.destroy(g);
        }

        // Drain and free the reap channel. Processes still sitting in it
        // are freed through the registry below.
        var rbuf: [1]*Process = undefined;
        while ((p.reapCh.getUncancelable(io, &rbuf, 0) catch 0) != 0) {}
        const rcap = p.reapCh.capacity();
        if (rcap > 0) {
            const relems: [*]*Process = @ptrCast(@alignCast(p.reapCh.type_erased.buffer.ptr));
            allocator.free(relems[0..rcap]);
        }
        allocator.destroy(p.reapCh);

        // Drain and free the shared event channels. Events still sitting
        // in these channels are deep copies owned by the routing system,
        // so they are freed here. The TestCh can hold events put there
        // directly by user code which the system does not own, those are
        // drained but not freed.
        drainAndFreeEventQueue(allocator, io, p.StaticEventCh, true);
        drainAndFreeEventQueue(allocator, io, p.ErrorEventCh, true);
        drainAndFreeEventQueue(allocator, io, p.DynamicEventCh, true);
        drainAndFreeEventQueue(allocator, io, p.CustomEventCh, true);
        drainAndFreeEventQueue(allocator, io, p.SupervisorEventCh, true);
        drainAndFreeEventQueue(allocator, io, p.TestCh, false);

        // Free the process maps.
        p.StaticProcesses.procMap.deinit();
        allocator.destroy(p.StaticProcesses);
        p.DynamicProcesses.procMap.deinit();
        allocator.destroy(p.DynamicProcesses);
        p.CustomProcesses.procMap.deinit();
        allocator.destroy(p.CustomProcesses);
        p.ErrorProcesses.procMap.deinit();
        allocator.destroy(p.ErrorProcesses);
        p.supervisorProcesses.procMap.deinit();
        allocator.destroy(p.supervisorProcesses);

        // Free the pid and event counters.
        p.pids.toProc.mp.deinit();
        allocator.destroy(p.pids);
        allocator.destroy(p.EventNr);

        // Free every process still in the registry, including the root
        // process itself, together with their input channels and contexts.
        // Stopped dynamic and custom processes that were freed by the
        // reaper are no longer in the registry.
        const registry = p.registry;
        for (registry.list.items) |proc| {
            freeProcessOwned(allocator, io, proc);
        }
        registry.list.deinit(allocator);
        allocator.destroy(registry);
    }
};

// Free the memory owned by one process: the context, the events still in
// it's input channel and the channel itself, the ready signal, the event
// name and the process struct. Zig specific, used both by the reaper and
// by Deinit at shutdown.
fn freeProcessOwned(allocator: std.mem.Allocator, io: Io, proc: *Process) void {
    // The context is freed first: it's deinit waits for a cancel of the
    // process that may still be in flight from a system shutdown, so the
    // tasks of the process are known to be done before the rest of the
    // memory they use is freed.
    proc.Ctx.deinit();
    drainAndFreeEventQueue(allocator, io, proc.InCh, true);
    if (proc.readyCh) |rc| {
        allocator.destroy(rc);
    }
    allocator.free(proc.Event);
    allocator.destroy(proc);
}

// The reaper task. Zig specific, no Go counterpart: in Go the garbage
// collector frees a stopped process when nothing references it anymore.
// The reaper does that job explicitly for the processes handed to it by
// Stop. It cancels the context of the process, which waits for the
// process tasks to finish. Doing the cancel here instead of in Stop is
// what makes it safe for a process function to call Stop on it's own
// process.
//
// The memory is then freed for the dynamic (ED) and custom (EC)
// processes, the categories where Stop removes the process from the
// processes map, so nothing references them anymore, just like in Go.
// Static, error and supervisor processes stay in their maps, they stay
// reachable in Go as well, and their memory is freed by Deinit at
// shutdown.
//
// A delivery that found the process in the map right before Stop removed
// it is handled in two steps: closing the InCh wakes a router that is
// blocked putting an event to it (the router gets error.Closed and frees
// the event), and the inflight counter is then waited on, so the last
// router holding a reference to the process is known to be done with it.
// The counter can only decrease at this point, since the process is out
// of the maps.
//
// The reaper runs until the reap channel is closed by Deinit, and is not
// part of the context tree, so a shutdown cancel of the tree never waits
// on it.
fn reaperFn(p: *Process) void {
    while (true) {
        const proc = p.reapCh.getOneUncancelable(p.io) catch return;

        proc.Ctx.cancel();

        switch (proc.Event[1]) {
            'D', 'C' => {
                proc.InCh.close(p.io);

                while (proc.inflight.load(.acquire) != 0) {
                    p.io.sleep(.fromMilliseconds(1), .awake) catch {};
                }

                p.registry.remove(proc);
                freeProcessOwned(p.allocator, p.io, proc);
            },
            else => {},
        }
    }
}

// The task body started by Act and actForRoot. The equivalent of the
// `go func() { p.fn() }()` in the Go version.
fn runProcessFn(p: *Process) void {
    const f = p.@"fn".?;
    f.run(f.userdata, p.Ctx, p);
}

// The put branch of the select in addEventStatic.
fn addEventStaticPut(q: *Io.Queue(Event), io: Io, event: Event) (Io.QueueClosedError || Io.Cancelable)!void {
    return q.putOne(io, event);
}

// The timeout branch of the select in addEventStatic, the equivalent of
// the time.After of 5 seconds in the Go version.
fn addEventStaticTimeout(io: Io) Io.Cancelable!void {
    return io.sleep(.fromSeconds(5), .awake);
}

// Allocate a new event queue with the given buffer capacity. A capacity
// of 0 gives the same rendezvous behavior as an unbuffered Go channel.
fn newEventQueue(allocator: std.mem.Allocator, cap: usize) !*Io.Queue(Event) {
    const q = try allocator.create(Io.Queue(Event));
    errdefer allocator.destroy(q);
    const buf = try allocator.alloc(Event, cap);
    q.* = .init(buf);
    return q;
}

// Empty the queue, optionally freeing the events found in it, and free
// the queue buffer and the queue itself.
fn drainAndFreeEventQueue(allocator: std.mem.Allocator, io: Io, q: *Io.Queue(Event), deinitEvents: bool) void {
    var buf: [1]Event = undefined;
    while (true) {
        const n = q.getUncancelable(io, &buf, 0) catch 0;
        if (n == 0) {
            break;
        }
        if (deinitEvents) {
            buf[0].deinit(allocator);
        }
    }

    const cap = q.capacity();
    if (cap > 0) {
        const elems: [*]Event = @ptrCast(@alignCast(q.type_erased.buffer.ptr));
        allocator.free(elems[0..cap]);
    }
    allocator.destroy(q);
}

// NewRootProcess will prepare and return the root process
// which holds all the core elements needed, like the main
// channels for events and errors, and varouis registers or
// maps holding information about the system. Later created
// processes will reference these elements when they are
// created.
// The root process will also start up all the essential other
// processes needed, like the event router, and various standard
// error handling processes.
pub fn NewRootProcess(io: Io, allocator: std.mem.Allocator, ctx: *Context, @"fn": ?ETFunc, conf: *Config) !*Process {
    const rootCtx = try Context.WithCancel(ctx);

    // Initialize the logger based on configured log level string
    // Allowed values: debug, info, error, fatal, none.
    // Map unknown values to info.
    var levelStr: []const u8 = conf.LogLevel;
    if (conf.environ) |env| {
        if (env.get("LOGLEVEL")) |v| {
            // Environment always wins to allow global overrides in tests/runs
            levelStr = v;
        }
    }
    slog.SetLevelFromString(levelStr);

    const registry = try allocator.create(processRegistry);
    registry.* = processRegistry{ .io = io };

    // The channel to the reaper task, see reaperFn.
    const reapCh = try allocator.create(Io.Queue(*Process));
    errdefer allocator.destroy(reapCh);
    const reapBuf = try allocator.alloc(*Process, 10);
    reapCh.* = .init(reapBuf);

    const p = try allocator.create(Process);
    p.* = Process{
        .@"fn" = @"fn",
        .InCh = try newEventQueue(allocator, 10),
        .StaticEventCh = try newEventQueue(allocator, 10),
        .ErrorEventCh = try newEventQueue(allocator, 10),
        .TestCh = try newEventQueue(allocator, 10),
        .DynamicEventCh = try newEventQueue(allocator, 10),
        .CustomEventCh = try newEventQueue(allocator, 10),
        .SupervisorEventCh = try newEventQueue(allocator, 10),
        .Event = try allocator.dupe(u8, ETRoot),
        .StaticProcesses = try staticpkg.newStaticProcesses(allocator),
        .DynamicProcesses = try dynamicpkg.newDynamicProcesses(allocator),
        .CustomProcesses = try custompkg.newCustomProcesses(allocator),
        .ErrorProcesses = try errorpkg.newErrorProcesses(allocator),
        .supervisorProcesses = try supervisorpkg.newsuperVisorProcesses(allocator),
        .EventNr = try newEventNr(allocator, io, 0),
        .isRoot = true,
        .Config = conf,
        .pids = try newPids(allocator, io),
        .PID = 0,
        .Ctx = rootCtx,
        .allocator = allocator,
        .io = io,
        .registry = registry,
        .reapCh = reapCh,
    };

    p.PID = p.pids.nr;

    try registry.add(allocator, p);

    // Start the reaper task, see reaperFn. It runs in it's own group
    // outside the context tree, and lives until Deinit shuts it down.
    const reapGroup = try allocator.create(Io.Group);
    reapGroup.* = .init;
    p.reapGroup = reapGroup;
    try reapGroup.concurrent(io, reaperFn, .{p});

    // Register and start all the standard child processes of root.
    //
    // When starting Root, all the needed processes to handle the system are not
    // yet started. For example both the ESRouter and the ESProcesses, and the
    // Error processes are needed to be able to register processes in the
    // ESProcesses process.
    // To be able to register the processes with the esProcesses actor we store
    // the information about each process started in a slice, so when all
    // processes are started we can go through each element of the slice and
    // use the information to register all the processes in ESProcesses.
    var pi = newRegisterProcessInfo();
    defer pi.deinit(allocator);

    // Starting error handling processes.
    try (try NewProcess(rootCtx, p, ERLog, errorpkg.erLogFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, ERTest, errorpkg.erTestFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, ERNone, errorpkg.erNoneFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, ETPrint, staticpkg.etPrintFn)).actForRoot(&pi);

    try (try NewProcess(rootCtx, p, ETRouter, staticpkg.etRouterFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, ERRouter, errorpkg.erRouterFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, EDRouter, dynamicpkg.edRouterFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, ECRouter, custompkg.ecRouterFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, ESRouter, supervisorpkg.esRouterFn)).actForRoot(&pi);

    try (try NewProcess(rootCtx, p, ESProcesses, supervisorpkg.esProcessesFn())).actForRoot(&pi);

    // Starting the remainding processes.
    try (try NewProcess(rootCtx, p, ETOsSignal, staticpkg.etOsSignalFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, ETTestCh, staticpkg.etTestChFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, ETPid, staticpkg.etPidFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, ETReadFile, staticpkg.ETReadFileFn)).actForRoot(&pi);

    try (try NewProcess(rootCtx, p, ETDone, staticpkg.etDoneFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, ETExit, staticpkg.etExitFn)).actForRoot(&pi);
    try (try NewProcess(rootCtx, p, ETPidGetAll, staticpkg.etPidGetAllFn)).actForRoot(&pi);

    RegisterProcessesInESProcesses(p, &pi);

    return p;
}

// Register all the processes in ESProcesses.
pub fn RegisterProcessesInESProcesses(p: *Process, pi: *registerProcessInfo) void {
    _ = p;
    _ = pi;
    // // Register all the processes in ESProcesses.
    // for (pi.items) |md| {
    //     const syncCh = ...;
    //     const syncP = try NewProcess(p.Ctx, p, EDSync, EDSyncFn(syncCh));
    //     try syncP.Act();
    //     defer syncP.Stop();
    //
    //     const b = try cbor.Marshal(md);
    //
    //     try p.AddEvent(Event{
    //         .Name = ESProcesses,
    //
    //         .Instruction = InstructionESProcessesAdd,
    //         .Data = b,
    //         .NextEvent = &Event{
    //             .Name = EDSync,
    //         },
    //     });
    //
    //     <-syncCh
    // }
}

// Used for storing information about the processes to be registered in ESProcesses
// when starting the root process.
pub const registerProcessInfo = std.ArrayList(esProcessesMapDataIn);

// Create and return a new *registerProcessInfo struct.
fn newRegisterProcessInfo() registerProcessInfo {
    const pi = registerProcessInfo.empty;
    return pi;
}

// NewProcess will prepare and return a *Process. It will copy
// channels and map structures from the root process.
pub fn NewProcess(ctx: *Context, parentP: *Process, event: EventName, @"fn": ?ETFunc) !*Process {
    const childCtx = try Context.WithCancel(ctx);
    const p = try parentP.allocator.create(Process);
    p.* = Process{
        // The Go version calls fn(ctx, &p) here and stores the returned
        // closure. Zig got no closures, so the ETFunc struct is stored as
        // is, and it's run function is called when the process is started
        // with Act.
        .@"fn" = @"fn",
        .InCh = try newEventQueue(parentP.allocator, 0),
        .StaticEventCh = parentP.StaticEventCh,
        .ErrorEventCh = parentP.ErrorEventCh,
        .TestCh = parentP.TestCh,
        .DynamicEventCh = parentP.DynamicEventCh,
        .CustomEventCh = parentP.CustomEventCh,
        .SupervisorEventCh = parentP.SupervisorEventCh,
        .Event = try parentP.allocator.dupe(u8, event),

        .StaticProcesses = parentP.StaticProcesses,
        .DynamicProcesses = parentP.DynamicProcesses,
        .CustomProcesses = parentP.CustomProcesses,
        .ErrorProcesses = parentP.ErrorProcesses,
        .supervisorProcesses = parentP.supervisorProcesses,
        .EventNr = parentP.EventNr,
        .isRoot = false,
        .Config = parentP.Config,
        .pids = parentP.pids,
        .PID = parentP.pids.next(),
        .Ctx = childCtx,
        .allocator = parentP.allocator,
        .io = parentP.io,
        .registry = parentP.registry,
        .reapCh = parentP.reapCh,
    };

    try parentP.registry.add(parentP.allocator, p);

    return p;
}

// Pull in the test files, the Zig equivalent of the *_test.go files,
// so they are found by `zig build test`.
test {
    _ = @import("actress_test.zig");
    _ = @import("staticProcesses_test.zig");
    _ = @import("customprocesses_test.zig");
    _ = @import("supervisorprocesses_test.zig");
}
