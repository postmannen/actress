const std = @import("std");
const Io = std.Io;

const actress = @import("actress.zig");
const Context = actress.Context;
const Event = actress.Event;
const NewConfig = actress.NewConfig;
const NewRootProcess = actress.NewRootProcess;
const NewProcess = actress.NewProcess;
const ETTestfn = actress.ETTestfn;
const ECGeneralDelivery = actress.ECGeneralDelivery;
const ecGeneralDeliveryFn = actress.ecGeneralDeliveryFn;

test "TestECRouter" {
    var threaded: Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    const gpa = std.testing.allocator;

    const ctx = try Context.Background(gpa, io);
    defer ctx.deinit();

    var testChBuf: [0][]const u8 = undefined;
    var testCh = Io.Queue([]const u8).init(&testChBuf);

    std.debug.print("--------------------------------------------------------\n", .{});

    const cfg = try NewConfig(gpa, null, "debug");
    defer gpa.destroy(cfg);
    const rootp = try NewRootProcess(io, gpa, ctx, null, cfg);
    defer rootp.Deinit();
    defer ctx.cancel();
    try rootp.Act();

    try (try NewProcess(ctx, rootp, actress.ETTest, ETTestfn(&testCh))).Act();
    try (try NewProcess(ctx, rootp, ECGeneralDelivery, ecGeneralDeliveryFn)).Act();

    const testStr = "some custom data";

    var nextEv = Event{ .Name = actress.ETTest };
    try rootp.AddEvent(Event{
        .Name = ECGeneralDelivery,
        .Data = testStr,
        .NextEvent = &nextEv,
    });

    // The Go version selects on the test channel and ctx.Done. The Done
    // branch only fires when the test itself is canceled, so a plain
    // blocking read is the equivalent here.
    const s = try testCh.getOne(io);
    defer gpa.free(s);
    if (!std.mem.eql(u8, s, testStr)) {
        std.debug.print("string were not equal\n", .{});
        return error.TestFailed;
    }
}
