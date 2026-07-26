const std = @import("std");
const Io = std.Io;
const zbor = @import("zbor");

const actress = @import("actress.zig");
const Context = actress.Context;
const Event = actress.Event;
const NewConfig = actress.NewConfig;
const NewRootProcess = actress.NewRootProcess;
const NewProcess = actress.NewProcess;
const ETTestfn = actress.ETTestfn;
const ESProcesses = actress.ESProcesses;
const InstructionESProcessesAdd = actress.InstructionESProcessesAdd;
const InstructionESProcessesDelete = actress.InstructionESProcessesDelete;
const InstructionESProcessesGetAll = actress.InstructionESProcessesGetAll;
const esProcessesMapDataIn = actress.esProcessesMapDataIn;

test "TestESProcesses" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();

    try (try NewProcess(ctx, rootp, actress.ETTest, ETTestfn(&testCh))).Act();
    // try (try NewProcess(ctx, rootp, ESProcesses, esProcessesFn())).Act();

    const md = esProcessesMapDataIn{
        .Name = "TestType1",
    };

    // ---------- Add item to the esprocesses map

    var bBuf = std.Io.Writer.Allocating.init(gpa);
    defer bBuf.deinit();
    zbor.stringify(md, .{ .slice_serialization_type = .TextString }, &bBuf.writer) catch |err| {
        std.debug.print("error: failed to marshal map data: {s}", .{@errorName(err)});
        return error.TestFailed;
    };
    const b = bBuf.written();

    var nextEv1 = Event{ .Name = actress.ETTest };
    try rootp.AddEvent(Event{
        .Name = ESProcesses,
        .Instruction = InstructionESProcessesAdd,
        .Data = b,
        .NextEvent = &nextEv1,
    });

    // Wait for the the reply back, which are the ETTest .NextEvent
    const reply1 = try testCh.getOne(io);
    gpa.free(reply1);

    // ---------- Get item to the esprocesses map

    var nextEv2 = Event{ .Name = actress.ETTest };
    try rootp.AddEvent(Event{
        .Name = ESProcesses,
        .Instruction = InstructionESProcessesGetAll,
        .NextEvent = &nextEv2,
    });

    // Wait for the the reply back, which are the ETTest .NextEvent
    const ev = try testCh.getOne(io);
    defer gpa.free(ev);

    // The Go version decodes the cbor into an ESProcessesMap. Here the
    // cbor map is iterated directly to check for the expected entry.
    const di = zbor.DataItem.new(ev) catch |err| {
        std.debug.print("error: failed to unmarshal process map: {s}", .{@errorName(err)});
        return error.TestFailed;
    };
    var it = di.map() orelse {
        std.debug.print("error: failed to unmarshal process map: not a map", .{});
        return error.TestFailed;
    };

    // TODO: FIX TEST, so it actually tests for something.
    var found = false;
    while (it.next()) |pair| {
        const k = pair.key.string() orelse continue;
        const v = pair.value.string() orelse continue;
        if (std.mem.eql(u8, k, "TestType1") and std.mem.eql(u8, v, "TestType1")) {
            found = true;
        }
    }
    if (!found) {
        std.debug.print("The received map did not contain the expected value\n", .{});
        return error.TestFailed;
    }

    std.debug.print("\n###########################################################################\n", .{});
    std.debug.print("# Map contained: TestType1:TestType1\n", .{});
    std.debug.print("###########################################################################\n", .{});

    // ---------- Delete item from the esprocesses map

    var nextEv3 = Event{ .Name = actress.ETTest };
    try rootp.AddEvent(Event{
        .Name = ESProcesses,
        .Instruction = InstructionESProcessesDelete,
        .Data = b,
        .NextEvent = &nextEv3,
    });

    // Wait for the the reply back, which are the ETTest .NextEvent
    const reply3 = try testCh.getOne(io);
    gpa.free(reply3);
}
