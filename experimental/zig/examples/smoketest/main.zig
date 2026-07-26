// Smoke test exercising the parts of the library the 2actresses example
// does not touch: the dynamic and custom routers with the retry path,
// EDSync, ESProcesses with cbor in both directions, ETPidGetAll and
// ETReadFile.
const std = @import("std");
const actress = @import("actress");
const zbor = @import("zbor");

pub fn main(init: std.process.Init) !void {
    const io = init.io;
    const gpa = init.gpa;

    const ctx = try actress.Context.Background(gpa, io);
    defer ctx.deinit();

    const cfg = try actress.NewConfig(gpa, init.environ_map, "none");
    defer gpa.destroy(cfg);
    const rootAct = try actress.NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootAct.Deinit();
    defer ctx.cancel();
    try rootAct.Act();

    var failed = false;

    // 1. Custom process routing through the ECRouter. The section is a
    // block so the deferred free of the received event runs at the end of
    // the section.
    {
        const ecProc = try actress.NewProcess(ctx, rootAct, actress.ECGeneralDelivery, actress.ecGeneralDeliveryFn);
        try ecProc.Act();
        var nextTestCh1 = actress.Event{ .Name = actress.ETTestCh };
        try rootAct.AddEvent(actress.Event{
            .Name = actress.ECGeneralDelivery,
            .Data = "custom-data",
            .NextEvent = &nextTestCh1,
        });

        const ev1 = try rootAct.TestCh.getOne(io);
        defer ev1.deinit(gpa);
        if (!std.mem.eql(u8, ev1.Data, "custom-data")) {
            std.debug.print("FAIL custom: got {s}\n", .{ev1.Data});
            failed = true;
        } else {
            std.debug.print("OK custom router + general delivery\n", .{});
        }
    }

    // 2. Dynamic process with EDSync. The event is sent BEFORE the process
    // is started, so the EDRouter retry task (which sleeps 1 second between
    // the retries) must deliver it once the process shows up. The Stop at
    // the end also frees the memory of the dynamic process, which the leak
    // detection of the Debug build verifies.
    {
        var syncBuf: [1]u8 = undefined;
        var syncCh = std.Io.Queue(u8).init(&syncBuf);
        try rootAct.AddEvent(actress.Event{ .Name = actress.EDSync });
        const syncP = try actress.NewProcess(ctx, rootAct, actress.EDSync, actress.EDSyncFn(&syncCh));
        try syncP.Act();
        _ = try syncCh.getOne(io);
        std.debug.print("OK dynamic router + retry + EDSync\n", .{});
        syncP.Stop();
    }

    // 3. ESProcesses: add a process name via cbor, then get all back as cbor.
    {
        var nameBuf = std.Io.Writer.Allocating.init(gpa);
        defer nameBuf.deinit();
        try zbor.stringify(actress.esProcessesMapDataIn{ .Name = "ETSmoke" }, .{ .slice_serialization_type = .TextString }, &nameBuf.writer);

        try rootAct.AddEvent(actress.Event{
            .Name = actress.ESProcesses,
            .Instruction = actress.InstructionESProcessesAdd,
            .Data = nameBuf.written(),
        });
        var nextTestCh2 = actress.Event{ .Name = actress.ETTestCh };
        try rootAct.AddEvent(actress.Event{
            .Name = actress.ESProcesses,
            .Instruction = actress.InstructionESProcessesGetAll,
            .NextEvent = &nextTestCh2,
        });
        const ev2 = try rootAct.TestCh.getOne(io);
        defer ev2.deinit(gpa);
        // The result is a cbor map name -> name, check that ETSmoke is in there.
        if (std.mem.indexOf(u8, ev2.Data, "ETSmoke") == null) {
            std.debug.print("FAIL esprocesses: ETSmoke not found in cbor\n", .{});
            failed = true;
        } else {
            std.debug.print("OK supervisor router + ESProcesses cbor roundtrip\n", .{});
        }
    }

    // 4. ETPidGetAll returns a cbor map of pid -> process name.
    {
        var nextTestCh3 = actress.Event{ .Name = actress.ETTestCh };
        try rootAct.AddEvent(actress.Event{
            .Name = actress.ETPidGetAll,
            .NextEvent = &nextTestCh3,
        });
        const ev3 = try rootAct.TestCh.getOne(io);
        defer ev3.deinit(gpa);
        if (std.mem.indexOf(u8, ev3.Data, "ETRouter") == null) {
            std.debug.print("FAIL pidgetall: ETRouter not found in cbor\n", .{});
            failed = true;
        } else {
            std.debug.print("OK ETPidGetAll cbor\n", .{});
        }
    }

    // 5. ETReadFile reads a file and passes the content to the next event.
    {
        try std.Io.Dir.cwd().writeFile(io, .{ .sub_path = "smoketest.txt", .data = "file-content-here" });
        var nextTestCh4 = actress.Event{ .Name = actress.ETTestCh };
        try rootAct.AddEvent(actress.Event{
            .Name = actress.ETReadFile,
            .Cmd = &.{"smoketest.txt"},
            .NextEvent = &nextTestCh4,
        });
        const ev4 = try rootAct.TestCh.getOne(io);
        defer ev4.deinit(gpa);
        if (!std.mem.eql(u8, ev4.Data, "file-content-here")) {
            std.debug.print("FAIL readfile: got {s}\n", .{ev4.Data});
            failed = true;
        } else {
            std.debug.print("OK ETReadFile\n", .{});
        }
        try std.Io.Dir.cwd().deleteFile(io, "smoketest.txt");
    }

    // 6. ETTest factory with testCh and EOF instruction.
    {
        var testChBuf: [4][]const u8 = undefined;
        var testCh = std.Io.Queue([]const u8).init(&testChBuf);
        const testP = try actress.NewProcess(ctx, rootAct, actress.ETTest, actress.ETTestfn(&testCh));
        try testP.Act();
        try rootAct.AddEvent(actress.Event{ .Name = actress.ETTest, .Data = "hello-test" });
        const s = try testCh.getOne(io);
        defer gpa.free(s);
        if (!std.mem.eql(u8, s, "hello-test")) {
            std.debug.print("FAIL ettest: got {s}\n", .{s});
            failed = true;
        } else {
            std.debug.print("OK ETTest factory with testCh\n", .{});
        }
        try rootAct.AddEvent(actress.Event{ .Name = actress.ETTest, .Instruction = actress.InstructionCmdEOF });
        // The channel is closed by the process when it got the EOF instruction.
        if (testCh.getOne(io)) |_| {
            std.debug.print("FAIL ettest: channel not closed\n", .{});
            failed = true;
        } else |err| switch (err) {
            error.Closed => std.debug.print("OK ETTest EOF closes channel\n", .{}),
            else => return err,
        }
    }

    if (failed) {
        std.debug.print("SMOKETEST FAILED\n", .{});
        std.process.exit(1);
    }
    std.debug.print("SMOKETEST PASSED\n", .{});
}
