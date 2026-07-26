const std = @import("std");
const actress = @import("actress");

// Define the first function that will be attached to the ETTest1 EventType process.
const test1Func = actress.ETFunc{ .run = test1FuncRun };

fn test1FuncRun(userdata: ?*anyopaque, ctx: *actress.Context, p: *actress.Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        // Reading the InCh blocks until an event arrives, or until the task
        // is canceled, which is the Zig version of the Go select on the
        // channel and <-ctx.Done().
        const ev = p.InCh.getOne(p.io) catch return;
        // The received event is owned here, the defer frees it at the end
        // of the loop iteration, also on the early returns below.
        defer ev.deinit(p.allocator);

        const upper = std.ascii.allocUpperString(p.allocator, ev.Data) catch return;
        // AddEvent takes a deep copy, so the upper cased string is still
        // owned here and freed by the defer.
        defer p.allocator.free(upper);

        // Pass on the processing to the next process, and use the NextEvent we have specified in main
        // for the EventType, and add the result of ToUpper to the data field.
        p.AddEvent(actress.Event{
            .Name = ev.NextEvent.?.Name,
            .Data = upper,
        }) catch return;
    }
}

// Define the second function that will be attached to the ETTest2 EventType process.
const test2Func = actress.ETFunc{ .run = test2FuncRun };

fn test2FuncRun(userdata: ?*anyopaque, ctx: *actress.Context, p: *actress.Process) void {
    _ = userdata;
    _ = ctx;

    while (true) {
        const result = p.InCh.getOne(p.io) catch return;
        defer result.deinit(p.allocator);

        const dots = std.fmt.allocPrint(p.allocator, "{s}...", .{result.Data}) catch return;

        // All actresses have a TestCh, which can be just used for
        // passing data via the routers. It is primarily intended
        // for use in tests.
        // Since the main code are aware of the actress using this
        // function, we can then put a value on the channel here
        // and read it in main.
        // The ownership of dots is handed over to the reader of TestCh on
        // success, so it is only freed here when the put fails.
        p.TestCh.putOne(p.io, actress.Event{ .Data = dots }) catch {
            p.allocator.free(dots);
            return;
        };

        // Also create an informational error message.
        p.AddEvent(actress.Event{
            .Name = actress.ERLog,
            .Instruction = actress.InstructionDebug,
            .Err = "info: done with the acting",
        }) catch return;
    }
}

pub fn main(init: std.process.Init) !void {
    const io = init.io;
    const gpa = init.gpa;

    const ctx = try actress.Context.Background(gpa, io);
    defer ctx.deinit();

    // Create a new root process.
    const cfg = try actress.NewConfig(gpa, init.environ_map, "info");
    defer gpa.destroy(cfg);
    const rootAct = try actress.NewRootProcess(io, gpa, ctx, null, cfg);

    // The defers run in reverse order at exit: first the cancel of the
    // context which stops all the process tasks, like the deferred
    // cancel() in the Go version, and then the freeing of all the memory
    // of the process tree, which the garbage collector handles in Go.
    defer rootAct.Deinit();
    defer ctx.cancel();

    // Define two event typess for two processes.
    const ETTest1: actress.EventName = "ETTest1";
    const ETTest2: actress.EventName = "ETTest2";

    // Register the event types and event function to processes.
    const ac1 = try actress.NewProcess(ctx, rootAct, ETTest1, test1Func);
    const ac2 = try actress.NewProcess(ctx, rootAct, ETTest2, test2Func);
    try ac1.Act();
    try ac2.Act();

    // Start all the registered processes.
    try rootAct.Act();

    // Pass in an event destined for an ETTest1 EventType process, and also specify
    // the next event to be used when passing the result on from ETTest1 to the next
    // process which here is ETTest2.
    var nextEv = actress.Event{ .Name = ETTest2 };
    try rootAct.AddEvent(actress.Event{
        .Name = ETTest1,
        .Data = "test",
        .NextEvent = &nextEv,
    });

    // Wait and receive the result from the ETTest2 process.
    const ev = try ac2.TestCh.getOne(io);
    // The data was allocated by the ETTest2 process function and handed
    // over on the TestCh, so it is owned and freed here.
    defer gpa.free(ev.Data);
    const out = try std.fmt.allocPrint(gpa, "The result: {s}\n", .{ev.Data});
    defer gpa.free(out);
    try std.Io.File.stdout().writeStreamingAll(io, out);

    try io.sleep(.fromSeconds(2), .awake);
}
