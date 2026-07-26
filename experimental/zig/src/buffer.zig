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

// Buffer is a thread safe buffer, the equivalent of the Go version's
// mutex protected bytes.Buffer. Read consumes bytes from the front of
// the buffer, and Write appends bytes at the end.
pub const Buffer = struct {
    buffer: std.ArrayList(u8) = .empty,
    // Read position into buffer. bytes.Buffer in Go consumes what is
    // read, here the consumed bytes are tracked with an offset instead.
    off: usize = 0,
    mu: Io.Mutex = .init,
    allocator: std.mem.Allocator,
    io: Io,

    pub fn Read(bu: *Buffer, p: []u8) !usize {
        bu.mu.lockUncancelable(bu.io);
        defer bu.mu.unlock(bu.io);

        if (bu.off >= bu.buffer.items.len) {
            // Mirrors the io.EOF returned by bytes.Buffer when empty.
            return error.EOF;
        }
        const n = @min(p.len, bu.buffer.items.len - bu.off);
        @memcpy(p[0..n], bu.buffer.items[bu.off .. bu.off + n]);
        bu.off += n;
        return n;
    }

    pub fn Write(bu: *Buffer, b: []const u8) !usize {
        bu.mu.lockUncancelable(bu.io);
        defer bu.mu.unlock(bu.io);

        try bu.buffer.appendSlice(bu.allocator, b);
        return b.len;
    }

    pub fn deinit(bu: *Buffer) void {
        bu.buffer.deinit(bu.allocator);
        bu.allocator.destroy(bu);
    }
};

pub fn NewBuffer(allocator: std.mem.Allocator, io: Io) !*Buffer {
    const b = try allocator.create(Buffer);
    b.* = Buffer{
        .allocator = allocator,
        .io = io,
    };
    return b;
}
