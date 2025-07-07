const std = @import("std");
const print = std.debug.print;
const actress = @import("actress");

const Allocator = std.mem.Allocator;
const Event = actress.Event;
const EventType = actress.EventType;
const Process = actress.Process;

// Define two event types for two processes
const ETTest1: EventType = "ETTest1";
const ETTest2: EventType = "ETTest2";

/// First process function that converts data to uppercase
fn test1Fn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                defer {
                    // Clean up the received event
                    var mut_event = event;
                    mut_event.deinit();
                }

                if (event.data) |data| {
                    // Convert to uppercase
                    var arena = std.heap.ArenaAllocator.init(process.allocator);
                    defer arena.deinit();
                    const temp_allocator = arena.allocator();

                    const upper = try std.ascii.allocUpperString(temp_allocator, data);

                    // Pass on the processing to the next process
                    if (event.next_event) |next_event| {
                        const new_event = try Event.initWithData(process.allocator, next_event.event_type, upper);
                        process.addEvent(new_event);
                    }
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("Test1 process error: {}", .{err});
            break;
        }
    }
}

/// Second process function that adds dots to the data
fn test2Fn(allocator: Allocator, process: *Process) !void {
    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                defer {
                    // Clean up the received event
                    var mut_event = event;
                    mut_event.deinit();
                }

                if (event.data) |data| {
                    // Add dots
                    var arena = std.heap.ArenaAllocator.init(allocator);
                    defer arena.deinit();
                    const temp_allocator = arena.allocator();

                    const dots = try std.fmt.allocPrint(temp_allocator, "{s}...", .{data});

                    // Print the result
                    print("The result: {s}\n", .{dots});

                    // Also create an informational debug message
                    const debug_event = try Event.initWithErr(process.allocator, actress.ERDebug, "info: done with the acting");
                    process.addError(debug_event);
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("Test2 process error: {}", .{err});
            break;
        }
    }
}

pub fn main() !void {
    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    defer _ = gpa.deinit();
    const allocator = gpa.allocator();

    // Initialize the actress system
    const root = try actress.initActressSystem(allocator);
    defer {
        root.deinit();
        allocator.destroy(root);
    }

    // Create the two processes
    const test1_process = try actress.newProcess(allocator, &root.process, ETTest1, test1Fn);
    const test2_process = try actress.newProcess(allocator, &root.process, ETTest2, test2Fn);

    // Start the processes
    try test1_process.act();
    try test2_process.act();

    // Give the system a moment to initialize
    std.time.sleep(100 * std.time.ns_per_ms);

    // Create the next event for chaining
    var next_event = actress.newEvent(allocator, ETTest2);

    // Create and send the initial event with chaining
    var initial_event = try Event.initWithData(allocator, ETTest1, "test");
    initial_event.setNextEvent(&next_event);

    root.process.addEvent(initial_event);

    // Wait for processing to complete
    std.time.sleep(2 * std.time.ns_per_s);

    print("Example completed!\n", .{});
}
