const std = @import("std");
const Allocator = std.mem.Allocator;
const Mutex = std.Thread.Mutex;
const Condition = std.Thread.Condition;
const ArrayList = std.ArrayList;

/// A thread-safe channel for passing messages between processes
pub fn Channel(comptime T: type) type {
    return struct {
        const Self = @This();

        allocator: Allocator,
        buffer: ArrayList(T),
        capacity: usize,
        mutex: Mutex,
        not_empty: Condition,
        not_full: Condition,
        closed: bool,

        /// Create a new channel with the specified capacity
        pub fn init(allocator: Allocator, capacity: usize) Self {
            return Self{
                .allocator = allocator,
                .buffer = ArrayList(T).init(allocator),
                .capacity = capacity,
                .mutex = Mutex{},
                .not_empty = Condition{},
                .not_full = Condition{},
                .closed = false,
            };
        }

        /// Clean up the channel
        pub fn deinit(self: *Self) void {
            self.mutex.lock();
            defer self.mutex.unlock();

            self.closed = true;
            self.not_empty.broadcast();
            self.not_full.broadcast();
            self.buffer.deinit();
        }

        /// Send a value to the channel (blocking if full)
        pub fn send(self: *Self, value: T) !void {
            self.mutex.lock();
            defer self.mutex.unlock();

            while (self.buffer.items.len >= self.capacity and !self.closed) {
                self.not_full.wait(&self.mutex);
            }

            if (self.closed) {
                return error.ChannelClosed;
            }

            try self.buffer.append(value);
            self.not_empty.signal();
        }

        /// Try to send a value to the channel (non-blocking)
        pub fn trySend(self: *Self, value: T) !bool {
            self.mutex.lock();
            defer self.mutex.unlock();

            if (self.closed) {
                return error.ChannelClosed;
            }

            if (self.buffer.items.len >= self.capacity) {
                return false; // Channel is full
            }

            try self.buffer.append(value);
            self.not_empty.signal();
            return true;
        }

        /// Receive a value from the channel (blocking if empty)
        pub fn recv(self: *Self) !?T {
            self.mutex.lock();
            defer self.mutex.unlock();

            while (self.buffer.items.len == 0 and !self.closed) {
                self.not_empty.wait(&self.mutex);
            }

            if (self.buffer.items.len == 0 and self.closed) {
                return null; // Channel is closed and empty
            }

            const value = self.buffer.orderedRemove(0);
            self.not_full.signal();
            return value;
        }

        /// Try to receive a value from the channel (non-blocking)
        pub fn tryRecv(self: *Self) !?T {
            self.mutex.lock();
            defer self.mutex.unlock();

            if (self.buffer.items.len == 0) {
                if (self.closed) {
                    return null; // Channel is closed and empty
                }
                return error.WouldBlock; // Channel is empty but not closed
            }

            const value = self.buffer.orderedRemove(0);
            self.not_full.signal();
            return value;
        }

        /// Close the channel
        pub fn close(self: *Self) void {
            self.mutex.lock();
            defer self.mutex.unlock();

            self.closed = true;
            self.not_empty.broadcast();
            self.not_full.broadcast();
        }

        /// Check if the channel is closed
        pub fn isClosed(self: *Self) bool {
            self.mutex.lock();
            defer self.mutex.unlock();
            return self.closed;
        }

        /// Get the current length of the channel
        pub fn len(self: *Self) usize {
            self.mutex.lock();
            defer self.mutex.unlock();
            return self.buffer.items.len;
        }

        /// Check if the channel is empty
        pub fn isEmpty(self: *Self) bool {
            return self.len() == 0;
        }

        /// Check if the channel is full
        pub fn isFull(self: *Self) bool {
            self.mutex.lock();
            defer self.mutex.unlock();
            return self.buffer.items.len >= self.capacity;
        }
    };
}

/// A simple select-like mechanism for channels
pub const ChannelSelect = struct {
    /// Channel operation type
    pub const OpType = enum {
        recv,
        send,
    };

    /// Channel operation
    pub const Op = struct {
        op_type: OpType,
        channel_ptr: *anyopaque,
        value_ptr: ?*anyopaque = null, // For send operations
        result_ptr: ?*anyopaque = null, // For recv operations
        ready: bool = false,
    };

    /// Simplified select for a single receive operation with timeout
    pub fn selectRecv(comptime T: type, channel: *Channel(T), timeout_ms: ?u64) !?T {
        if (timeout_ms) |timeout| {
            const start_time = std.time.milliTimestamp();
            while (std.time.milliTimestamp() - start_time < timeout) {
                if (channel.tryRecv()) |value| {
                    return value;
                } else |err| switch (err) {
                    error.WouldBlock => {
                        std.time.sleep(1_000_000); // Sleep 1ms
                        continue;
                    },
                    else => return err,
                }
            }
            return error.Timeout;
        } else {
            return channel.recv();
        }
    }
};

test "channel basic operations" {
    const testing = std.testing;
    var arena = std.heap.ArenaAllocator.init(testing.allocator);
    defer arena.deinit();
    const allocator = arena.allocator();

    var ch = Channel(i32).init(allocator, 2);
    defer ch.deinit();

    // Test send and receive
    try ch.send(42);
    try ch.send(24);

    const val1 = try ch.recv();
    try testing.expect(val1.? == 42);

    const val2 = try ch.recv();
    try testing.expect(val2.? == 24);

    // Test try operations
    const sent = try ch.trySend(100);
    try testing.expect(sent);

    const received = try ch.tryRecv();
    try testing.expect(received.? == 100);
}
