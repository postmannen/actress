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

// NB, Zig specific: the Go tests define their process functions as
// closures inside each test function. Zig got no closures, so the process
// functions are defined at file scope right before the test they belong
// to, named after the test, and the channels the Go closures capture are
// passed via the userdata field of ETFunc.
//
// Each test also creates it's own Io implementation and uses the leak
// checking std.testing.allocator, which in Go is handled by the runtime
// and the garbage collector.

const std = @import("std");
const Io = std.Io;
const zbor = @import("zbor");

const actress = @import("actress.zig");
const Context = actress.Context;
const Event = actress.Event;
const EventName = actress.EventName;
const ETFunc = actress.ETFunc;
const Process = actress.Process;
const NewConfig = actress.NewConfig;
const NewRootProcess = actress.NewRootProcess;
const NewProcess = actress.NewProcess;
const NewUUID = actress.NewUUID;
const NewEventRW = actress.NewEventRW;
const CopyEvent = actress.CopyEvent;
const pidnr = actress.pidnr;
const ERTest = actress.ERTest;
const ETPidGetAll = actress.ETPidGetAll;
const ETTestCh = actress.ETTestCh;
const ETTestfn = actress.ETTestfn;
const EDSync = actress.EDSync;
const EDSyncFn = actress.EDSyncFn;

// tFunc of TestEventProcs.
fn testEventProcsTFuncRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const testCh: *Io.Queue([]const u8) = @ptrCast(@alignCast(userdata.?));

    while (true) {
        p.SignalReady();

        const result = p.InCh.getOne(p.io) catch return;
        // The received event is owned here, the defer frees it at the
        // end of the loop iteration, also on the early return below.
        defer result.deinit(p.allocator);

        const data = p.allocator.dupe(u8, result.Data) catch |err| {
            std.debug.panic("tFunc: allocation failed: {s}", .{@errorName(err)});
        };
        // The ownership of data is handed over to the reader of testCh on
        // success, so it is only freed here when the put fails.
        testCh.putOne(p.io, data) catch {
            p.allocator.free(data);
            return;
        };
    }
}

test "TestEventProcs" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);

    const ETTest: EventName = "ETTest";

    const tFunc = ETFunc{ .userdata = &testCh, .run = testEventProcsTFuncRun };

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();

    try (try NewProcess(ctx, rootp, ETTest, tFunc)).Act();

    try rootp.AddEvent(Event{ .Name = ETTest, .Data = "test" });

    const r = try testCh.getOne(io);
    defer gpa.free(r);
    if (!std.mem.eql(u8, r, "test")) {
        std.debug.print("ETTest failed\n", .{});
        return error.TestFailed;
    }
}

// tFunc of TestDynamicProcess.
fn testDynamicProcessTFuncRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const testCh: *Io.Queue([]const u8) = @ptrCast(@alignCast(userdata.?));

    while (true) {
        p.SignalReady();

        const result = p.InCh.getOne(p.io) catch return;
        defer result.deinit(p.allocator);

        const data = p.allocator.dupe(u8, result.Data) catch |err| {
            std.debug.panic("tFunc: allocation failed: {s}", .{@errorName(err)});
        };
        testCh.putOne(p.io, data) catch {
            p.allocator.free(data);
            return;
        };
    }
}

test "TestDynamicProcess" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);

    const ETTest: EventName = "ETTest";

    const tFunc = ETFunc{ .userdata = &testCh, .run = testDynamicProcessTFuncRun };

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();

    try (try NewProcess(ctx, rootp, ETTest, tFunc)).Act();

    try rootp.AddEvent(Event{ .Name = ETTest, .Data = "test" });

    const r = try testCh.getOne(io);
    defer gpa.free(r);
    if (!std.mem.eql(u8, r, "test")) {
        std.debug.print("ETTest failed\n", .{});
        return error.TestFailed;
    }
}

// NB, Zig specific test with no Go counterpart: verifies that Stop
// reclaims the memory of a dynamic process while the system is running.
// Each stopped process is removed from the registry by the reaper, so
// the registry length is back at the starting value after the loop.
// Since a process that is out of the registry is never freed by Deinit,
// the leak checking test allocator also verifies that the reaper really
// freed the memory of every stopped process.
test "TestStopReclaimsDynamicProcess" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    const cfg = try NewConfig(gpa, null, "none");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();

    var syncChBuf: [1]u8 = undefined;
    var syncCh = Io.Queue(u8).init(&syncChBuf);

    const lenBefore = rootp.registry.list.items.len;

    var i: usize = 0;
    while (i < 10) : (i += 1) {
        const name = try NewUUID(gpa, io);
        defer gpa.free(name);

        const dynP = try NewProcess(ctx, rootp, name, EDSyncFn(&syncCh));
        try dynP.Act();

        // Send an event through the dynamic router and wait for the
        // process to signal that it received it, so the delivery is done
        // before the process is stopped.
        try rootp.AddEvent(Event{ .Name = name });
        _ = try syncCh.getOne(io);

        // Stop hands the process to the reaper, which cancels it and
        // frees it's memory. The pointer in dynP is invalid after this.
        dynP.Stop();
    }

    // The reaper frees the stopped processes asynchronously, so wait for
    // the registry to be back at the starting length.
    var waited: usize = 0;
    while (true) {
        rootp.registry.mu.lockUncancelable(io);
        const len = rootp.registry.list.items.len;
        rootp.registry.mu.unlock(io);
        if (len == lenBefore) {
            break;
        }
        waited += 1;
        if (waited > 5000) {
            std.debug.print("stopped processes were not reaped, registry length: {d}, expected: {d}\n", .{ len, lenBefore });
            return error.TestFailed;
        }
        try io.sleep(.fromMilliseconds(1), .awake);
    }
}

// The process function of TestStopFromOwnProcessFunction. Receives one
// event, signals on the sync channel, and then stops it's own process,
// like the Go version can since the Go cancel just signals.
fn testStopFromOwnFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const syncCh: *Io.Queue(u8) = @ptrCast(@alignCast(userdata.?));

    p.SignalReady();

    const ev = p.InCh.getOne(p.io) catch return;
    defer ev.deinit(p.allocator);

    syncCh.putOne(p.io, 0) catch return;

    // Stop the own process. The reaper does the cancel and the freeing
    // after this process function has returned.
    p.Stop();
}

// NB, Zig specific test with no Go counterpart: verifies that a process
// function can call Stop on it's own process, and that the process is
// freed by the reaper afterwards.
test "TestStopFromOwnProcessFunction" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    const cfg = try NewConfig(gpa, null, "none");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();

    var syncChBuf: [1]u8 = undefined;
    var syncCh = Io.Queue(u8).init(&syncChBuf);

    const lenBefore = rootp.registry.list.items.len;

    const name = try NewUUID(gpa, io);
    defer gpa.free(name);

    const dynP = try NewProcess(ctx, rootp, name, ETFunc{
        .userdata = &syncCh,
        .run = testStopFromOwnFnRun,
    });
    try dynP.Act();

    try rootp.AddEvent(Event{ .Name = name });
    _ = try syncCh.getOne(io);

    // The process stops itself after signaling. Wait for the reaper to
    // free it, which puts the registry back at the starting length.
    var waited: usize = 0;
    while (true) {
        rootp.registry.mu.lockUncancelable(io);
        const len = rootp.registry.list.items.len;
        rootp.registry.mu.unlock(io);
        if (len == lenBefore) {
            break;
        }
        waited += 1;
        if (waited > 5000) {
            std.debug.print("self stopped process was not reaped, registry length: {d}, expected: {d}\n", .{ len, lenBefore });
            return error.TestFailed;
        }
        try io.sleep(.fromMilliseconds(1), .awake);
    }
}

// The names the inner dyn2EVType process function of TestDynamicProcess2
// and TestDynamicProcessReaderWriter needs. In Go these are captured by
// the closure, here they are allocated when the process is created, and
// freed by the process function when it is done.
const dyn2Names = struct {
    dyn1EVType: []const u8,
    dyn2EVType: []const u8,
};

// The values the dyn1EVType process function needs. In Go these are
// captured by the closure defined in the main test function.
const dyn1Data = struct {
    testCh: *Io.Queue([]const u8),
    dyn2EVType: []const u8,
};

// The inner dyn2EVType process function of TestDynamicProcess2. Receives
// one event, sends an event to the dyn1EVType process, and deletes it's
// own process from the dynamic processes map.
fn testDynamicProcess2Dyn2FnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const names: *dyn2Names = @ptrCast(@alignCast(userdata.?));
    defer {
        p.allocator.free(names.dyn1EVType);
        p.allocator.free(names.dyn2EVType);
        p.allocator.destroy(names);
    }

    p.SignalReady();

    const ev = p.InCh.getOne(p.io) catch return;
    ev.deinit(p.allocator);

    // Send an event to the dyn1EVType process.
    p.AddEvent(Event{ .Name = names.dyn1EVType, .Data = "from dyn2" }) catch return;

    // We are now done with the dyn2EVType process so we delete it.
    p.DynamicProcesses.Delete(p.io, names.dyn2EVType);

    std.debug.print("successfully deleted process: {s}\n", .{names.dyn2EVType});
}

// tFunc of TestDynamicProcess2.
fn testDynamicProcess2TFuncRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;

    while (true) {
        p.SignalReady();

        const ev = p.InCh.getOne(p.io) catch return;
        defer ev.deinit(p.allocator);

        const dyn1EVType = ev.Cmd[1];
        const dyn2EVType = ev.Cmd[2];
        std.debug.print("\ndyn1EVType: {s}\ndyn2EVType: {s}\n", .{ dyn1EVType, dyn2EVType });

        // The received event is freed at the end of this iteration, so
        // the names are duplicated for the inner process, which frees
        // them when it is done.
        const names = p.allocator.create(dyn2Names) catch |err| {
            std.debug.panic("tFunc: allocation failed: {s}", .{@errorName(err)});
        };
        names.* = dyn2Names{
            .dyn1EVType = p.allocator.dupe(u8, dyn1EVType) catch |err| {
                std.debug.panic("tFunc: allocation failed: {s}", .{@errorName(err)});
            },
            .dyn2EVType = p.allocator.dupe(u8, dyn2EVType) catch |err| {
                std.debug.panic("tFunc: allocation failed: {s}", .{@errorName(err)});
            },
        };

        // Define and start the process for dyn2EVType.
        const dyn2P = NewProcess(ctx, p, names.dyn2EVType, ETFunc{
            .userdata = names,
            .run = testDynamicProcess2Dyn2FnRun,
        }) catch |err| {
            std.debug.panic("tFunc: NewProcess failed: {s}", .{@errorName(err)});
        };
        dyn2P.Act() catch |err| {
            std.debug.panic("tFunc: Act failed: {s}", .{@errorName(err)});
        };
    }
}

// The dyn1EVType process function of TestDynamicProcess2. Sends an event
// to the dyn2EVType process, and forwards the data of the event it then
// receives to the test channel.
fn testDynamicProcess2Dyn1FnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const d: *dyn1Data = @ptrCast(@alignCast(userdata.?));

    p.AddEvent(Event{ .Name = d.dyn2EVType }) catch return;

    p.SignalReady();

    const ev = p.InCh.getOne(p.io) catch return;
    defer ev.deinit(p.allocator);

    const data = p.allocator.dupe(u8, ev.Data) catch |err| {
        std.debug.panic("dyn1 fn: allocation failed: {s}", .{@errorName(err)});
    };
    d.testCh.putOne(p.io, data) catch {
        p.allocator.free(data);
    };
}

test "TestDynamicProcess2" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    const ETTest: EventName = "ETTest";
    // Test channel for receiving the final result.
    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);

    // tFunc is the function to be used with Name ETTest.
    // When receiving an ETTest event, we start up a dynamic
    // process. The Name to use for the new inner dynamic
    // process can be found in the Cmd[2] field of the event
    // to ETTest. Cmd[1] holds the other dynamic process to
    // send Event to.
    const tFunc = ETFunc{ .run = testDynamicProcess2TFuncRun };

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();

    try (try NewProcess(ctx, rootp, ETTest, tFunc)).Act();

    // Create UUID's to be used for Name's for the dynamic processes.
    // We put them in the .Cmd field of ETTest so the receiver also know
    // about them.
    const dyn1EVType = try NewUUID(gpa, io);
    defer gpa.free(dyn1EVType);
    const dyn2EVType = try NewUUID(gpa, io);
    defer gpa.free(dyn2EVType);

    var d1 = dyn1Data{ .testCh = &testCh, .dyn2EVType = dyn2EVType };
    try (try NewProcess(ctx, rootp, dyn1EVType, ETFunc{
        .userdata = &d1,
        .run = testDynamicProcess2Dyn1FnRun,
    })).Act();

    try rootp.AddEvent(Event{
        .Name = ETTest,
        .Cmd = &.{ "", dyn1EVType, dyn2EVType },
        .Data = "test",
    });

    const r = try testCh.getOne(io);
    defer gpa.free(r);
    if (!std.mem.eql(u8, r, "from dyn2")) {
        std.debug.print("ETTest failed\n", .{});
        return error.TestFailed;
    } else {
        std.debug.print("\n\u{1F602} SUCCESS\n", .{});
    }
}

// The inner dyn2EVType process function of TestDynamicProcessReaderWriter.
// Like the one in TestDynamicProcess2, but the event to the dyn1EVType
// process is written through an EventRW.
fn readerWriterDyn2FnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const names: *dyn2Names = @ptrCast(@alignCast(userdata.?));
    defer {
        p.allocator.free(names.dyn1EVType);
        p.allocator.free(names.dyn2EVType);
        p.allocator.destroy(names);
    }

    p.SignalReady();

    const ev = p.InCh.getOne(p.io) catch return;
    ev.deinit(p.allocator);

    // Send an event to the dyn1EVType process.
    var tmpEv = Event{ .Name = names.dyn1EVType };

    const erw = NewEventRW(p.allocator, p, &tmpEv, "in dyn2EVType reader writer") catch |err| {
        std.debug.panic("dyn2 fn: allocation failed: {s}", .{@errorName(err)});
    };
    defer p.allocator.destroy(erw);
    _ = erw.Write("from dyn2") catch return;
    p.AddEvent(tmpEv) catch return;

    // We are now done with the dyn2EVType process so we delete it.
    p.DynamicProcesses.Delete(p.io, names.dyn2EVType);

    std.debug.print("successfully deleted process: {s}\n", .{names.dyn2EVType});
}

// edTestFn of TestDynamicProcessReaderWriter.
fn readerWriterEdTestFnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;

    while (true) {
        p.SignalReady();

        const ev = p.InCh.getOne(p.io) catch return;
        defer ev.deinit(p.allocator);

        const dyn1EVType = ev.Cmd[1];
        const dyn2EVType = ev.Cmd[2];
        std.debug.print("\n FROM ETTestFn:dyn1EVType: {s}\ndyn2EVType: {s}\n", .{ dyn1EVType, dyn2EVType });

        // The received event is freed at the end of this iteration, so
        // the names are duplicated for the inner process, which frees
        // them when it is done.
        const names = p.allocator.create(dyn2Names) catch |err| {
            std.debug.panic("edTestFn: allocation failed: {s}", .{@errorName(err)});
        };
        names.* = dyn2Names{
            .dyn1EVType = p.allocator.dupe(u8, dyn1EVType) catch |err| {
                std.debug.panic("edTestFn: allocation failed: {s}", .{@errorName(err)});
            },
            .dyn2EVType = p.allocator.dupe(u8, dyn2EVType) catch |err| {
                std.debug.panic("edTestFn: allocation failed: {s}", .{@errorName(err)});
            },
        };

        // Define and start the process for dyn2EVType.
        const dyn2P = NewProcess(ctx, p, names.dyn2EVType, ETFunc{
            .userdata = names,
            .run = readerWriterDyn2FnRun,
        }) catch |err| {
            std.debug.panic("edTestFn: NewProcess failed: {s}", .{@errorName(err)});
        };
        dyn2P.Act() catch |err| {
            std.debug.panic("edTestFn: Act failed: {s}", .{@errorName(err)});
        };
    }
}

// The dyn1EVType process function of TestDynamicProcessReaderWriter.
// Reads the data of the received event through an EventRW, like the
// io.ReadAll in the Go version.
fn readerWriterDyn1FnRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const d: *dyn1Data = @ptrCast(@alignCast(userdata.?));

    p.AddEvent(Event{ .Name = d.dyn2EVType }) catch return;
    p.SignalReady();

    var ev = p.InCh.getOne(p.io) catch return;
    defer ev.deinit(p.allocator);

    const erw = NewEventRW(p.allocator, p, &ev, "dyn1EVType Reader/Writer") catch |err| {
        std.debug.panic("dyn1 fn: allocation failed: {s}", .{@errorName(err)});
    };
    defer p.allocator.destroy(erw);

    // io.ReadAll equivalent: read from the EventRW until EOF.
    var list: std.ArrayList(u8) = .empty;
    var buf: [512]u8 = undefined;
    while (true) {
        const n = erw.Read(&buf) catch break;
        list.appendSlice(p.allocator, buf[0..n]) catch |err| {
            std.debug.panic("dyn1 fn: allocation failed: {s}", .{@errorName(err)});
        };
    }
    const b = list.toOwnedSlice(p.allocator) catch |err| {
        std.debug.panic("dyn1 fn: allocation failed: {s}", .{@errorName(err)});
    };

    // The ownership of b is handed over to the reader of testCh on
    // success, so it is only freed here when the put fails.
    d.testCh.putOne(p.io, b) catch {
        p.allocator.free(b);
    };
}

test "TestDynamicProcessReaderWriter" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    const ETTest: EventName = "ETTest";
    // Test channel for receiving the final result.
    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();

    // edTestFn is the function to be used with Name ETTest.
    // When receiving an ETTest event, we start up a dynamic
    // process. The Name to use for the new inner dynamic
    // process can be found in the Cmd[2] field of the event
    // to ETTest. Cmd[1] holds the other dynamic process to
    // send Event to.
    const edTestFn = ETFunc{ .run = readerWriterEdTestFnRun };

    try (try NewProcess(ctx, rootp, ETTest, edTestFn)).Act();

    // Create UUID's to be used for Name's for the dynamic processes.
    // We put them in the .Cmd field of ETTest so the receiver also know
    // about them.
    const dyn1EVType = try NewUUID(gpa, io);
    defer gpa.free(dyn1EVType);
    const dyn2EVType = try NewUUID(gpa, io);
    defer gpa.free(dyn2EVType);

    std.debug.print("\n FromMaindyn1EVType: {s}\ndyn2EVType: {s}\n", .{ dyn1EVType, dyn2EVType });

    var d1 = dyn1Data{ .testCh = &testCh, .dyn2EVType = dyn2EVType };
    try (try NewProcess(ctx, rootp, dyn1EVType, ETFunc{
        .userdata = &d1,
        .run = readerWriterDyn1FnRun,
    })).Act();

    try rootp.AddEvent(Event{
        .Name = ETTest,
        .Cmd = &.{ "", dyn1EVType, dyn2EVType },
        .Data = "test",
    });

    const r = try testCh.getOne(io);
    defer gpa.free(r);
    if (!std.mem.eql(u8, r, "from dyn2")) {
        std.debug.print("ETTest failed\n", .{});
        return error.TestFailed;
    } else {
        std.debug.print("\n\u{1F602} SUCCESS\n", .{});
    }
}

// testFunc of TestNextEventProcs.
fn testNextEventProcsTestFuncRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const testCh: *Io.Queue([]const u8) = @ptrCast(@alignCast(userdata.?));

    while (true) {
        p.SignalReady();

        const result = p.InCh.getOne(p.io) catch return;
        defer result.deinit(p.allocator);

        const data = p.allocator.dupe(u8, result.Data) catch |err| {
            std.debug.panic("testFunc: allocation failed: {s}", .{@errorName(err)});
        };
        testCh.putOne(p.io, data) catch {
            p.allocator.free(data);
            return;
        };
    }
}

// nextEventFunc of TestNextEventProcs.
fn testNextEventProcsNextEventFuncRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        p.SignalReady();

        const ev = p.InCh.getOne(p.io) catch return;
        defer ev.deinit(p.allocator);

        // Pass the data from the current event into the next event.
        var nextEvent = ev.NextEvent.?.*;
        nextEvent.Data = ev.Data;
        // NB, Zig specific: the event is put directly on the StaticEventCh
        // without going through AddEvent, so a deep copy is taken here,
        // since the process that receives it from the routing system owns
        // and frees it.
        const owned = CopyEvent(p.allocator, nextEvent) catch |err| {
            std.debug.panic("nextEventFunc: allocation failed: {s}", .{@errorName(err)});
        };
        p.StaticEventCh.putOne(p.io, owned) catch {
            owned.deinit(p.allocator);
            return;
        };
    }
}

test "TestNextEventProcs" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();

    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);
    const ETTest: EventName = "ETTest";

    const testFunc = ETFunc{ .userdata = &testCh, .run = testNextEventProcsTestFuncRun };

    try (try NewProcess(ctx, rootp, ETTest, testFunc)).Act();

    const ETNextEvent: EventName = "ETNextEvent";

    const nextEventFunc = ETFunc{ .run = testNextEventProcsNextEventFuncRun };

    try (try NewProcess(ctx, rootp, ETNextEvent, nextEventFunc)).Act();
    try rootp.Act();

    var nextEv = Event{ .Name = ETTest };
    try rootp.AddEvent(Event{
        .Name = ETNextEvent,
        .Data = "test",
        .NextEvent = &nextEv,
    });

    const r = try testCh.getOne(io);
    defer gpa.free(r);
    if (!std.mem.eql(u8, r, "test")) {
        std.debug.print("ETTest failed\n", .{});
        return error.TestFailed;
    }
}

// test "TestPidToProcess" {
//     ...
//     const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
//     try rootp.Act();
//
//     // Since ETRouter is the first process to be started we can
//     // check that the first value in the map is an ETRouter.
//     if (rootp.pids.toProc.getProc(0).Event != ETRouter) {
//         return error.TestFailed;
//     }
// }

test "TestPidToProcMap" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();

    var nextEv = Event{ .Name = ETTestCh };
    try rootp.AddEvent(Event{
        .Name = ETPidGetAll,
        .NextEvent = &nextEv,
    });

    const ev = try rootp.TestCh.getOne(io);
    defer ev.deinit(gpa);

    // The Go version decodes the cbor into a PidVsProcMap. The Zig version
    // of etPidGetAllFn encodes the map as pid -> process name, so the
    // decoded map here holds the names as the values.
    var mapFromEv = std.AutoHashMap(pidnr, []const u8).init(gpa);
    defer mapFromEv.deinit();

    const di = try zbor.DataItem.new(ev.Data);
    var it = di.map() orelse return error.TestFailed;
    while (it.next()) |pair| {
        const k = pair.key.int() orelse return error.TestFailed;
        try mapFromEv.put(@intCast(k), pair.value.string() orelse "");
    }

    // Compare the map we got with the actual map.
    const mapFromActual = try rootp.pids.toProc.copyOfMap(gpa);
    defer {
        mapFromActual.deinit();
        gpa.destroy(mapFromActual);
    }

    // Check that the length of the two maps are equal
    if (mapFromActual.count() != mapFromEv.count()) {
        std.debug.print("length of maps are not equal, evMap: {d}, actualMap: {d}\n", .{ mapFromEv.count(), mapFromActual.count() });
        return error.TestFailed;
    }

    // Check all elements.
    var ait = mapFromActual.iterator();
    while (ait.next()) |entry| {
        if (mapFromEv.get(entry.key_ptr.*) == null) {
            std.debug.print("missing map value: {d}\n", .{entry.key_ptr.*});
            return error.TestFailed;
        }
    }

    // std.debug.print("got event: {s}, Data: {s}\n", .{ ev.Name, ev.Data });
}

// -------------------------------------------------------------
// Benchmarks
// -------------------------------------------------------------
//
// NB, Zig specific: Go's testing package provides a benchmark framework
// with a runtime chosen iteration count b.N. Zig got no benchmark
// framework in the standard library, so the benchmarks are ported as
// tests that run a fixed number of iterations and print the timing.

const benchmarkN = 1000;

// tFunc of BenchmarkSingleProcess.
fn benchmarkSingleProcessTFuncRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const testCh: *Io.Queue([]const u8) = @ptrCast(@alignCast(userdata.?));

    while (true) {
        p.SignalReady();

        const result = p.InCh.getOne(p.io) catch return;
        defer result.deinit(p.allocator);

        const data = p.allocator.dupe(u8, result.Data) catch |err| {
            std.debug.panic("tFunc: allocation failed: {s}", .{@errorName(err)});
        };
        testCh.putOne(p.io, data) catch {
            p.allocator.free(data);
            return;
        };
    }
}

test "BenchmarkSingleProcess" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);

    const ETTest: EventName = "ETTest";

    const tFunc = ETFunc{ .userdata = &testCh, .run = benchmarkSingleProcessTFuncRun };

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try (try NewProcess(ctx, rootp, ETTest, tFunc)).Act();
    try rootp.Act();

    const start = Io.Clock.awake.now(io);

    var n: usize = 0;
    while (n < benchmarkN) : (n += 1) {
        try rootp.AddEvent(Event{ .Name = ETTest, .Data = "test" });
        const r = try testCh.getOne(io);
        defer gpa.free(r);
        if (!std.mem.eql(u8, r, "test")) {
            std.debug.print("ETTest failed\n", .{});
            return error.TestFailed;
        }
    }

    const elapsed = start.durationTo(Io.Clock.awake.now(io));
    std.debug.print("BenchmarkSingleProcess: {d} iterations, {d} ns/op\n", .{ benchmarkN, @divTrunc(elapsed.toNanoseconds(), @as(i96, benchmarkN)) });
}

// tFunc of BenchmarkSingleProcessEventAndError.
fn benchmarkSingleProcessEventAndErrorTFuncRun(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const testCh: *Io.Queue([]const u8) = @ptrCast(@alignCast(userdata.?));

    while (true) {
        p.SignalReady();

        const result = p.InCh.getOne(p.io) catch return;
        defer result.deinit(p.allocator);

        const data = p.allocator.dupe(u8, result.Data) catch |err| {
            std.debug.panic("tFunc: allocation failed: {s}", .{@errorName(err)});
        };
        testCh.putOne(p.io, data) catch {
            p.allocator.free(data);
            return;
        };

        const msg = std.fmt.allocPrint(p.allocator, "some error:{any}", .{result}) catch |err| {
            std.debug.panic("tFunc: allocation failed: {s}", .{@errorName(err)});
        };
        // CopyEvent below takes a deep copy, so msg is still owned here
        // and freed by the defer.
        defer p.allocator.free(msg);
        // NB, Zig specific: the event is put directly on the ErrorEventCh
        // without going through AddEvent, so an owned deep copy is put on
        // the channel, since the receiving process frees what it gets.
        const errEv = CopyEvent(p.allocator, Event{ .Name = ERTest, .Err = msg }) catch |err| {
            std.debug.panic("tFunc: allocation failed: {s}", .{@errorName(err)});
        };
        p.ErrorEventCh.putOne(p.io, errEv) catch {
            errEv.deinit(p.allocator);
            return;
        };
    }
}

test "BenchmarkSingleProcessEventAndError" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);

    const ETTest: EventName = "ETTest";

    const tFunc = ETFunc{ .userdata = &testCh, .run = benchmarkSingleProcessEventAndErrorTFuncRun };

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();
    try (try NewProcess(ctx, rootp, ETTest, tFunc)).Act();

    const start = Io.Clock.awake.now(io);

    var n: usize = 0;
    while (n < benchmarkN) : (n += 1) {
        try rootp.AddEvent(Event{ .Name = ETTest, .Data = "test" });

        const msg = try std.fmt.allocPrint(gpa, "some error:{s}", .{"apekatt"});
        // NB, Zig specific: direct channel send, see the comment in the
        // process function above.
        const errEv = try CopyEvent(gpa, Event{ .Name = ERTest, .Err = msg });
        gpa.free(msg);
        rootp.ErrorEventCh.putOne(io, errEv) catch |err| {
            errEv.deinit(gpa);
            return err;
        };

        const r = try testCh.getOne(io);
        defer gpa.free(r);
        if (!std.mem.eql(u8, r, "test")) {
            std.debug.print("ETTest failed\n", .{});
            return error.TestFailed;
        }
    }

    const elapsed = start.durationTo(Io.Clock.awake.now(io));
    std.debug.print("BenchmarkSingleProcessEventAndError: {d} iterations, {d} ns/op\n", .{ benchmarkN, @divTrunc(elapsed.toNanoseconds(), @as(i96, benchmarkN)) });
}

// tFunc1 of BenchmarkTwoProcesses. Forwards the data to the ETTest2
// process. The Go version references the local ETTest2 const of the
// benchmark function, here the name is written out.
fn benchmarkTwoProcessesTFunc1Run(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        p.SignalReady();

        const result = p.InCh.getOne(p.io) catch return;
        defer result.deinit(p.allocator);

        // NB, Zig specific: the event is put directly on the StaticEventCh
        // without going through AddEvent, so an owned deep copy is put on
        // the channel, since the receiving process frees what it gets.
        const fwd = CopyEvent(p.allocator, Event{ .Name = "ETTest2", .Data = result.Data }) catch |err| {
            std.debug.panic("tFunc1: allocation failed: {s}", .{@errorName(err)});
        };
        p.StaticEventCh.putOne(p.io, fwd) catch {
            fwd.deinit(p.allocator);
            return;
        };
    }
}

// tFunc2 of BenchmarkTwoProcesses.
fn benchmarkTwoProcessesTFunc2Run(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const testCh: *Io.Queue([]const u8) = @ptrCast(@alignCast(userdata.?));

    while (true) {
        p.SignalReady();

        const result = p.InCh.getOne(p.io) catch return;
        defer result.deinit(p.allocator);

        const data = p.allocator.dupe(u8, result.Data) catch |err| {
            std.debug.panic("tFunc2: allocation failed: {s}", .{@errorName(err)});
        };
        testCh.putOne(p.io, data) catch {
            p.allocator.free(data);
            return;
        };
    }
}

test "BenchmarkTwoProcesses" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();

    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);

    const ETTest1: EventName = "ETTest1";
    const ETTest2: EventName = "ETTest2";

    const tFunc1 = ETFunc{ .run = benchmarkTwoProcessesTFunc1Run };
    try (try NewProcess(ctx, rootp, ETTest1, tFunc1)).Act();

    const tFunc2 = ETFunc{ .userdata = &testCh, .run = benchmarkTwoProcessesTFunc2Run };
    try (try NewProcess(ctx, rootp, ETTest2, tFunc2)).Act();

    try rootp.Act();

    const start = Io.Clock.awake.now(io);

    var n: usize = 0;
    while (n < benchmarkN) : (n += 1) {
        try rootp.AddEvent(Event{ .Name = ETTest1, .Data = "test" });
        const r = try testCh.getOne(io);
        defer gpa.free(r);
        if (!std.mem.eql(u8, r, "test")) {
            std.debug.print("ETTest failed\n", .{});
            return error.TestFailed;
        }
    }

    const elapsed = start.durationTo(Io.Clock.awake.now(io));
    std.debug.print("BenchmarkTwoProcesses: {d} iterations, {d} ns/op\n", .{ benchmarkN, @divTrunc(elapsed.toNanoseconds(), @as(i96, benchmarkN)) });
}

// tFunc1 of BenchmarkThreeProcesses. Forwards the data to the ETTest2 process.
fn benchmarkThreeProcessesTFunc1Run(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        p.SignalReady();

        const result = p.InCh.getOne(p.io) catch return;
        defer result.deinit(p.allocator);

        // NB, Zig specific: direct channel send, so an owned deep copy is
        // put on the channel.
        const fwd = CopyEvent(p.allocator, Event{ .Name = "ETTest2", .Data = result.Data }) catch |err| {
            std.debug.panic("tFunc1: allocation failed: {s}", .{@errorName(err)});
        };
        p.StaticEventCh.putOne(p.io, fwd) catch {
            fwd.deinit(p.allocator);
            return;
        };
    }
}

// tFunc2 of BenchmarkThreeProcesses. Forwards the data to the ETTest3 process.
fn benchmarkThreeProcessesTFunc2Run(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        p.SignalReady();

        const result = p.InCh.getOne(p.io) catch return;
        defer result.deinit(p.allocator);

        // NB, Zig specific: direct channel send, so an owned deep copy is
        // put on the channel.
        const fwd = CopyEvent(p.allocator, Event{ .Name = "ETTest3", .Data = result.Data }) catch |err| {
            std.debug.panic("tFunc2: allocation failed: {s}", .{@errorName(err)});
        };
        p.StaticEventCh.putOne(p.io, fwd) catch {
            fwd.deinit(p.allocator);
            return;
        };
    }
}

// tFunc3 of BenchmarkThreeProcesses.
fn benchmarkThreeProcessesTFunc3Run(userdata: ?*anyopaque, ctx: *Context, p: *Process) void {
    _ = ctx;
    const testCh: *Io.Queue([]const u8) = @ptrCast(@alignCast(userdata.?));

    while (true) {
        p.SignalReady();

        const result = p.InCh.getOne(p.io) catch return;
        defer result.deinit(p.allocator);

        const data = p.allocator.dupe(u8, result.Data) catch |err| {
            std.debug.panic("tFunc3: allocation failed: {s}", .{@errorName(err)});
        };
        testCh.putOne(p.io, data) catch {
            p.allocator.free(data);
            return;
        };
    }
}

test "BenchmarkThreeProcesses" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();

    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);

    const ETTest1: EventName = "ETTest1";
    const ETTest2: EventName = "ETTest2";
    const ETTest3: EventName = "ETTest3";

    const tFunc1 = ETFunc{ .run = benchmarkThreeProcessesTFunc1Run };
    try (try NewProcess(ctx, rootp, ETTest1, tFunc1)).Act();

    const tFunc2 = ETFunc{ .run = benchmarkThreeProcessesTFunc2Run };
    try (try NewProcess(ctx, rootp, ETTest2, tFunc2)).Act();

    const tFunc3 = ETFunc{ .userdata = &testCh, .run = benchmarkThreeProcessesTFunc3Run };
    try (try NewProcess(ctx, rootp, ETTest3, tFunc3)).Act();

    try rootp.Act();

    const start = Io.Clock.awake.now(io);

    var n: usize = 0;
    while (n < benchmarkN) : (n += 1) {
        try rootp.AddEvent(Event{ .Name = ETTest1, .Data = "test" });
        const r = try testCh.getOne(io);
        defer gpa.free(r);
        if (!std.mem.eql(u8, r, "test")) {
            std.debug.print("ETTest failed\n", .{});
            return error.TestFailed;
        }
    }

    const elapsed = start.durationTo(Io.Clock.awake.now(io));
    std.debug.print("BenchmarkThreeProcesses: {d} iterations, {d} ns/op\n", .{ benchmarkN, @divTrunc(elapsed.toNanoseconds(), @as(i96, benchmarkN)) });
}

test "TestETTest" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();

    // The package level ETTest event name.
    const ETTest = actress.ETTest;

    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);
    try (try NewProcess(ctx, rootp, ETTest, ETTestfn(&testCh))).Act();

    try rootp.AddEvent(Event{ .Name = ETTest, .Data = "test" });

    const r = try testCh.getOne(io);
    defer gpa.free(r);
    if (!std.mem.eql(u8, r, "test")) {
        std.debug.print("ETTest failed\n", .{});
        return error.TestFailed;
    }
}

// The two tasks of the select in TestEDSync, the sync channel read and
// the 3 second timeout.
fn testEDSyncGetTask(q: *Io.Queue(u8), io: Io) (Io.QueueClosedError || Io.Cancelable)!u8 {
    const v = try q.getOne(io);
    return v;
}

fn testEDSyncTimeoutTask(io: Io) Io.Cancelable!void {
    return io.sleep(.fromSeconds(3), .awake);
}

test "TestEDSync" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();

    var syncChBuf: [0]u8 = undefined;
    var syncCh = Io.Queue(u8).init(&syncChBuf);
    try (try NewProcess(ctx, rootp, EDSync, EDSyncFn(&syncCh))).Act();

    try rootp.AddEvent(Event{ .Name = EDSync, .Data = "test" });

    // The Go select on the sync channel and a 3 second time.After, done
    // with an Io.Select over a get task and a sleep task.
    const U = union(enum) {
        sync: (Io.QueueClosedError || Io.Cancelable)!u8,
        timeout: Io.Cancelable!void,
    };
    var selBuf: [2]U = undefined;
    var sel = Io.Select(U).init(io, &selBuf);
    defer sel.cancelDiscard();

    try sel.concurrent(.sync, testEDSyncGetTask, .{ &syncCh, io });
    try sel.concurrent(.timeout, testEDSyncTimeoutTask, .{io});

    const first = try sel.await();
    switch (first) {
        .sync => {
            std.debug.print("EDSync successful\n", .{});
        },
        .timeout => {
            std.debug.print("EDSync failed\n", .{});
            return error.TestFailed;
        },
    }
}
