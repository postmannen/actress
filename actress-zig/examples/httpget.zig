const std = @import("std");
const print = std.debug.print;
const actress = @import("actress");

const Allocator = std.mem.Allocator;
const Event = actress.Event;
const EventType = actress.EventType;
const Process = actress.Process;

// Define event types
const ETHttpGet: EventType = "ETHttpGet";
const ETWriteToFile: EventType = "ETWriteToFile";

/// HTTP GET process function
fn httpGetFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                defer {
                    var mut_event = event;
                    mut_event.deinit();
                }

                if (event.cmd) |cmd| {
                    if (cmd.len > 0) {
                        // Simulate HTTP GET with a simple mock response
                        // In a real implementation, you'd use an HTTP client
                        var arena = std.heap.ArenaAllocator.init(process.allocator);
                        defer arena.deinit();
                        const temp_allocator = arena.allocator();

                        const response = try std.fmt.allocPrint(temp_allocator, "<html><head><title>Mock Response</title></head><body><h1>Mock HTTP Response for {s}</h1><p>This is a simulated HTTP GET response.</p></body></html>", .{cmd[0]});

                        // Send to write file process
                        const write_event = try Event.initWithData(process.allocator, ETWriteToFile, response);
                        process.addEvent(write_event);

                        print("HTTP GET completed for: {s}\n", .{cmd[0]});
                    }
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("HTTP GET process error: {}", .{err});
            break;
        }
    }
}

/// Write to file process function
fn writeToFileFn(allocator: Allocator, process: *Process) !void {
    _ = allocator;

    while (!process.shouldCancel()) {
        if (process.in_ch.recv()) |maybe_event| {
            if (maybe_event) |event| {
                defer {
                    var mut_event = event;
                    mut_event.deinit();
                }

                if (event.data) |data| {
                    // Write data to file
                    std.fs.cwd().writeFile(.{
                        .sub_path = "web.html",
                        .data = data,
                    }) catch |err| {
                        std.log.err("Failed to write file: {}", .{err});
                        return;
                    };

                    print("File written successfully: web.html\n", .{});

                    // Create a done event
                    const done_event = try Event.initWithData(process.allocator, actress.ETDone, "File write completed");
                    process.addEvent(done_event);
                }
            } else {
                break;
            }
        } else |err| {
            std.log.err("Write to file process error: {}", .{err});
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

    // Create the processes
    const httpget_process = try actress.newProcess(allocator, &root.process, ETHttpGet, httpGetFn);
    const writefile_process = try actress.newProcess(allocator, &root.process, ETWriteToFile, writeToFileFn);

    // Start the processes
    try httpget_process.act();
    try writefile_process.act();

    // Give the system a moment to initialize
    std.time.sleep(100 * std.time.ns_per_ms);

    // Create and send the HTTP GET event
    const cmd = [_][]const u8{"http://example.com"};
    const http_event = try Event.initWithCmd(allocator, ETHttpGet, &cmd);

    root.process.addEvent(http_event);

    // Wait for processing to complete
    std.time.sleep(2 * std.time.ns_per_s);

    print("HTTP GET example completed!\n", .{});
}
