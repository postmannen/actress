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

pub const staticProcesses = struct {
    procMap: std.StringHashMap(*Process),
    mu: Io.Mutex = .init,

    // Checks if the event is defined in the processes map, and returns true if it is.
    pub fn IsEventDefined(p: *staticProcesses, ev: EventName) bool {
        if (p.procMap.get(ev) == null) {
            return false;
        }

        return true;
    }
};

// Prepare and return a new *processes structure.
pub fn newStaticProcesses(allocator: std.mem.Allocator) !*staticProcesses {
    const p = try allocator.create(staticProcesses);
    p.* = staticProcesses{
        .procMap = std.StringHashMap(*Process).init(allocator),
    };
    return p;
}

// -----------------------------------------------------------------------------
// Builtin standard Name's and their ETfunc's.
// -----------------------------------------------------------------------------

// ETRemote is an Name that will be used if
// an event should be delivered to a remote node.
//
// There are no ETFunc defined for ETRemote in Actress,
// so it is up to the user to write this function, and
// attach their own ETFunc when they create the process
// to handle the ETRemote Name.
//
// ETRemote are for example used in the AddEvent function,
// and will be prepended to the current event if it should
// not be handled locally.
pub const ETRemote: EventName = "ETRemote";

// Router for normal events.
pub const ETRouter: EventName = "ETRouter";

// Process function for routing and handling events. Will check
// and route the event to the correct process.
pub const etRouterFn = ETFunc{ .run = etRouterFnRun };

fn etRouterFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    defer {
        // fmt.Printf("STOPPED ETRouter!!!")
        p.Stop();
    }

    while (true) {
        // Reading the channel blocks until an event arrives, or until the
        // task is canceled, which is the Zig version of the Go select on
        // the channel and <-p.Ctx.Done().
        var ev = p.StaticEventCh.getOne(p.io) catch {
            slog.Debug("etRouterFn", .{ "got ctx.Done, on", p.Config.NodeName });
            return;
        };

        if (std.mem.eql(u8, ev.Name, ETRemote)) {
            p.StaticProcesses.mu.lockUncancelable(p.io);
            if (p.StaticProcesses.procMap.get(ev.Name) == null) {
                slog.Error("etRouterFn", .{ "on", p.Config.NodeName, "found no process registered for the event type ETRemote, and you need to register an ETFunc for how to handle remote connections with the EventName ", ev.Name });
            }
            p.StaticProcesses.mu.unlock(p.io);
        }
        if (slog.Enabled(.debug)) {
            slog.Debug("etRouterFn", .{ "event nr", ev.Nr, "received on StaticEventCh on", p.Config.NodeName });
        }
        // If there is a next event defined, we make a copy of all the fields  of the current event,
        // and put that as the previousEvent on the next event. We can use this information later
        // if need to check something in the previous event.
        if (ev.NextEvent) |ne| {
            // Keep the information about the current event, so we are able to check for things
            // like ackTimeout and what node to reply back to if ack should be given.
            setPreviousEvent(p.allocator, ne, &ev);
        }

        if (slog.Enabled(.debug)) {
            slog.Debug("etRouterFn", .{ "event nr", ev.Nr, "after CopyEventFields on", p.Config.NodeName });
        }

        // Check if process is registred and valid.
        p.StaticProcesses.mu.lockUncancelable(p.io);
        if (p.StaticProcesses.procMap.get(ev.Name) == null) {
            slog.Error("etRouterFn", .{ "on", p.Config.NodeName, "found no process registered for the event type", ev.Name });
        }
        p.StaticProcesses.mu.unlock(p.io);

        if (slog.Enabled(.debug)) {
            slog.Debug("etRouterFn", .{ "event nr", ev.Nr, "after checking if process is registred in procMap", ev.Name });
        }

        p.StaticProcesses.mu.lockUncancelable(p.io);
        const proc = p.StaticProcesses.procMap.get(ev.Name);
        if (proc == null) {
            slog.Error("etRouterFn", .{ "on", p.Config.NodeName, "found no process registered for the event type", ev.Name });
            p.StaticProcesses.mu.unlock(p.io);
            // The event can not be delivered anywhere, so it is dropped and freed.
            ev.deinit(p.allocator);
            continue;
        }
        const inCh = proc.?.InCh;
        p.StaticProcesses.mu.unlock(p.io);

        if (slog.Enabled(.debug)) {
            slog.Debug("etRouterFn", .{ "event nr", ev.Nr, "after getting the process InCh from procMap", ev.Name });
            slog.Debug("etRouterFn", .{ "event nr", ev.Nr, "before putting event on process InCh", p.Event, "node", p.Config.NodeName, "name", ev.Name, "Inch", inCh });
        }
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

        if (slog.Enabled(.debug)) {
            slog.Debug("etRouterFn", .{ "event nr", ev.Nr, "after routing event to process InCh", ev.Name });
        }
    }
}

// Copy all the descriptive meta data fields of the Event, not
// channels or Data.
//
// NB, Zig specific: the copy owns all it's memory, where the Go version
// shares the Cmd slice and Err value with the original. This is needed so
// the event tree can be freed without double frees.
pub fn CopyEventFields(allocator: std.mem.Allocator, ev: ?*const Event) ?*Event {
    const in = ev orelse return null;

    const e = allocator.create(Event) catch |err| {
        std.debug.panic("CopyEventFields: allocation failed: {s}", .{@errorName(err)});
    };

    const fieldsOnly = Event{
        .Nr = in.Nr,
        .Name = in.Name,

        .Cmd = in.Cmd,
        .Instruction = in.Instruction,
        .Err = in.Err,
        .DstNode = in.DstNode,
        .SrcNode = in.SrcNode,
    };

    e.* = eventspkg.CopyEvent(allocator, fieldsOnly) catch |err| {
        std.debug.panic("CopyEventFields: allocation failed: {s}", .{@errorName(err)});
    };

    return e;
}

// Helper used by the routers to attach a copy of the current event as the
// PreviousEvent of the next event. If a previous event already was set it
// is freed first, since the owned event tree must stay a tree.
pub fn setPreviousEvent(allocator: std.mem.Allocator, nextEvent: *Event, ev: *const Event) void {
    if (nextEvent.PreviousEvent) |old| {
        old.deinit(allocator);
        allocator.destroy(old);
    }
    nextEvent.PreviousEvent = CopyEventFields(allocator, ev);
}

// Press ctrl+c to exit.
pub const ETOsSignal: EventName = "ETOsSignal";

// Process function for handling CTRL+C pressed.
pub const etOsSignalFn = ETFunc{ .run = etOsSignalFnRun };

// Set from the signal handler when SIGINT is received. A plain atomic
// store is one of the few things that are safe to do inside a signal
// handler.
var osSignalReceived = std.atomic.Value(bool).init(false);

fn osSignalHandler(sig: std.posix.SIG) callconv(.c) void {
    _ = sig;
    osSignalReceived.store(true, .seq_cst);
}

fn etOsSignalFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    // Wait for ctrl+c to stop the server. Go blocks on a signal channel,
    // Zig got no such channel so a handler sets an atomic flag which is
    // polled here. The sleep is a cancelation point so the task stops
    // when the system is shut down.
    var act = std.posix.Sigaction{
        .handler = .{ .handler = osSignalHandler },
        .mask = std.posix.sigemptyset(),
        .flags = 0,
    };
    std.posix.sigaction(.INT, &act, null);

    // Block and wait for CTRL+C
    while (true) {
        p.io.sleep(.fromMilliseconds(100), .awake) catch return;
        if (osSignalReceived.load(.seq_cst)) {
            std.debug.print("Got terminate signal, terminating all processes, interrupt\n", .{});
            std.process.exit(0);
        }
    }
}

// The ETTest eventype are used for testing.
pub const ETTest: EventName = "ETTest";
pub const InstructionCmdEOF: Instruction = "InstructionCmdEOF";

// etTestFn accepts an 'chan string' as it's input argument, and
// it will return the data field of the previous event on that
// channel. You can then listen on that channel, check the
// value delivered, and see if it contains the value you expected
// it to hold.
pub fn ETTestfn(testCh: *Io.Queue([]const u8)) ETFunc {
    return ETFunc{ .userdata = testCh, .run = etTestFnRun };
}

fn etTestFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const testCh: *Io.Queue([]const u8) = @ptrCast(@alignCast(userdata.?));

    p.SignalReady();

    while (true) {
        const result = p.InCh.getOne(p.io) catch return;
        // The received event is owned here, the defer frees it at the end
        // of the loop iteration, also on the early returns below.
        defer result.deinit(p.allocator);

        if (std.mem.eql(u8, result.Instruction, InstructionCmdEOF)) {
            testCh.close(p.io);
            return;
        }
        // The data is duplicated since the received event is freed here,
        // and the reader of testCh owns and frees what it receives.
        const data = p.allocator.dupe(u8, result.Data) catch |err| {
            std.debug.panic("etTestFn: allocation failed: {s}", .{@errorName(err)});
        };
        testCh.putOne(p.io, data) catch {
            p.allocator.free(data);
            return;
        };

        // Check if there is a next event defined
        if (result.NextEvent) |ne| {
            p.AddEvent(ne.*) catch return;
        }
    }
}

// Will forward the incomming event to the builtin .TestCh
// of the process.
pub const ETTestCh: EventName = "ETTestCh";

// Will forward the incomming event to the builtin .TestCh
// of the process.
pub const etTestChFn = ETFunc{ .run = etTestChFnRun };

fn etTestChFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    p.SignalReady();

    while (true) {
        const e = p.InCh.getOne(p.io) catch {
            slog.Debug("etTestChFn", .{ "got ctx.Done, on", p.Config.NodeName });
            return;
        };

        p.TestCh.putOne(p.io, e) catch {
            e.deinit(p.allocator);
            return;
        };
    }
}

// Get all the current processes running. Will return a
// cbor encoded PidVsProcMap.
pub const ETPidGetAll: EventName = "ETPidGetAll";

// Get all the pids and processes, encode it into cbor.
pub const etPidGetAllFn = ETFunc{ .run = etPidGetAllFnRun };

fn etPidGetAllFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    p.SignalReady();

    while (true) {
        const e = p.InCh.getOne(p.io) catch {
            slog.Debug("etPidGetAllFn", .{ "got ctx.Done, on", p.Config.NodeName });
            return;
        };
        defer e.deinit(p.allocator);

        // The names map is used instead of the process pointer map, so
        // the names stay valid also if a process is stopped and freed
        // while the map copy is in use here.
        const pMap = p.pids.toProc.copyOfNamesMap(p.allocator) catch |err| {
            slog.Error("etPidGetAllFn", .{ "failed to copy pid to proc map", @errorName(err) });
            std.debug.panic("etPidGetAllFn: {s}", .{@errorName(err)});
        };
        defer {
            var it = pMap.iterator();
            while (it.next()) |entry| {
                p.allocator.free(entry.value_ptr.*);
            }
            pMap.deinit();
            p.allocator.destroy(pMap);
        }

        // The Go version cbor encodes map[pidnr]*Process directly. The Zig
        // version encodes the map as pid -> process event name, since
        // encoding raw process pointers got no meaning without a GC.
        const b = marshalPidMap(p.allocator, pMap) catch |err| {
            slog.Error("etPidGetAllFn", .{ "failed to marshal pid to proc map", @errorName(err) });
            std.debug.panic("etPidGetAllFn: {s}", .{@errorName(err)});
        };
        defer p.allocator.free(b);

        p.AddEvent(Event{ .Name = e.NextEvent.?.Name, .Data = b }) catch {};
    }
}

// Encode the pid vs process name map as a cbor map of pid -> process name.
fn marshalPidMap(allocator: std.mem.Allocator, pMap: *actress.PidVsNameMap) ![]u8 {
    var b = try zbor.Builder.withType(allocator, .Map);
    var it = pMap.iterator();
    while (it.next()) |entry| {
        try b.pushInt(entry.key_ptr.*);
        try b.pushTextString(entry.value_ptr.*);
    }
    const out = try b.finish();
    return out;
}

// Done don't currently do anything.
pub const ETDone: EventName = "ETDone";

pub const etDoneFn = ETFunc{ .run = etDoneFnRun };

fn etDoneFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        p.SignalReady();

        const d = p.InCh.getOne(p.io) catch return;
        defer d.deinit(p.allocator);

        // The Go version logs from a goroutine, logging directly here
        // gives the same result.
        slog.Info("etDoneFn", .{ "got event ETDone", d.Data });
        slog.Error("etDoneFn", .{ "got etDone, on", p.Config.NodeName });
    }
}

// Print the content of the .Data field of the event to stdout.
pub const ETPrint: EventName = "ETPrint";

// Print the content of the .Data field of the event to stdout.
pub const etPrintFn = ETFunc{ .run = etPrintFnRun };

fn etPrintFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        p.SignalReady();

        const d = p.InCh.getOne(p.io) catch return;
        defer d.deinit(p.allocator);

        printStdout(p, "{s}\n", .{d.Data});
    }
}

// Will exit and kill all processes.
pub const ETExit: EventName = "ETExit";

// Will exit and kill all processes.
pub const etExitFn = ETFunc{ .run = etExitFnRun };

fn etExitFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    p.SignalReady();

    while (true) {
        const d = p.InCh.getOne(p.io) catch return;

        printStdout(p, "etExitFn: got event ETExit: {s}\n", .{d.Data});
        std.process.exit(0);
    }
}

// Format and print to stdout.
fn printStdout(p: *Process, comptime fmt: []const u8, args: anytype) void {
    const s = std.fmt.allocPrint(p.allocator, fmt, args) catch return;
    defer p.allocator.free(s);
    Io.File.stdout().writeStreamingAll(p.io, s) catch {};
}

// Handling pids within the system.
// The structure of the ev.Cmd is a slice of string:
// []string{"action","pid","process name"}
pub const ETPid: EventName = "ETPid";

const pidAction = []const u8;

const pidGet: pidAction = "pidGet";
const pidGetAll: pidAction = "pidGetAll";

// Handle pids.
// The structure of the ev.Cmd is a slice of string:
// []string{"action","pid","process name"}
pub const etPidFn = ETFunc{ .run = etPidFnRun };

fn etPidFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    p.SignalReady();

    while (true) {
        const ev = p.InCh.getOne(p.io) catch return;
        defer ev.deinit(p.allocator);

        const action = ev.Cmd[0];
        const pid = std.fmt.parseInt(actress.pidnr, ev.Cmd[1], 10) catch |err| {
            std.debug.print("etPidFn: failed to convert pid from string to int: {s}\n", .{@errorName(err)});
            std.process.exit(1);
        };
        const procName = ev.Cmd[2];

        // Check the type of action we got.
        if (std.mem.eql(u8, action, pidGet)) {
            const data = std.fmt.allocPrint(p.allocator, "pid: {d}, process name: {s}", .{ pid, procName }) catch |err| {
                std.debug.panic("etPidFn: allocation failed: {s}", .{@errorName(err)});
            };
            defer p.allocator.free(data);
            p.AddEvent(Event{ .Name = ev.NextEvent.?.Name, .Data = data }) catch {};
        } else if (std.mem.eql(u8, action, pidGetAll)) {
            // The names map is used instead of the process pointer map,
            // so the names stay valid also if a process is stopped and
            // freed while the map copy is in use here.
            const pidProcMap = p.pids.toProc.copyOfNamesMap(p.allocator) catch |err| {
                std.debug.panic("etPidFn: allocation failed: {s}", .{@errorName(err)});
            };
            defer {
                var fit = pidProcMap.iterator();
                while (fit.next()) |entry| {
                    p.allocator.free(entry.value_ptr.*);
                }
                pidProcMap.deinit();
                p.allocator.destroy(pidProcMap);
            }
            var it = pidProcMap.iterator();
            while (it.next()) |entry| {
                const data = std.fmt.allocPrint(p.allocator, "pid: {d}, process name: {s}", .{ entry.key_ptr.*, entry.value_ptr.* }) catch |err| {
                    std.debug.panic("etPidFn: allocation failed: {s}", .{@errorName(err)});
                };
                defer p.allocator.free(data);
                p.AddEvent(Event{ .Name = ev.NextEvent.?.Name, .Data = data }) catch {};
            }
        }
    }
}

// Read file. The path path to read should be in Event.Cmd[0].
pub const ETReadFile: EventName = "ETReadFile";

pub const ETReadFileFn = ETFunc{ .run = etReadFileFnRun };

fn etReadFileFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;

    p.SignalReady();

    while (true) {
        const ev = p.InCh.getOne(p.io) catch return;

        // The file read is done in it's own task, the equivalent of the
        // `go func()` in the Go version, so the process can continue to
        // pick up new events while the file is read.
        ctx.spawn(readFileTask, .{ p, ev }) catch {
            ev.deinit(p.allocator);
            return;
        };
    }
}

fn readFileTask(p: *Process, evIn: Event) void {
    var ev = evIn;
    // The task owns the event, the defer frees it when the task exits.
    defer ev.deinit(p.allocator);

    const b = Io.Dir.cwd().readFileAlloc(p.io, ev.Cmd[0], p.allocator, .unlimited) catch |err| {
        std.debug.print("etReadFileFn: failed to open file: {s}\n", .{@errorName(err)});
        std.process.exit(1);
    };
    // AddEvent below takes a deep copy of the event, so the file buffer
    // is owned here and freed when the task exits.
    defer p.allocator.free(b);

    const nEv = ev.NextEvent.?;
    // Put the file content in the data field of the next event while it
    // is added (and deep copied), and put back the old value so the owned
    // event tree can be freed without freeing the file buffer twice.
    const oldData = nEv.Data;
    nEv.Data = b;
    p.AddEvent(nEv.*) catch {};
    nEv.Data = oldData;
}
