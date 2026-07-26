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

const actress = @import("actress.zig");
const Event = @import("events.zig").Event;
const Process = actress.Process;

pub const EventRW = struct {
    P: *Process,
    Ev: *Event,
    Info: []const u8,
    Pos: usize,

    // Write the data into Event.Data, and put the event into the StaticEventCh to be processed.
    pub fn Write(m: *EventRW, b: []const u8) !usize {
        std.debug.print(" *2 DEBUG WRITE: ev nr: {d}, info: {s} , name: {s}, PREPARING for Writing to erw, len(b): {d}, {s}\n", .{ m.Ev.Nr, m.Info, m.Ev.Name, b.len, b });

        if (b.len > 0) {
            const ev = m.Ev;
            ev.Data = b;

            try m.P.AddEvent(ev.*);
            std.debug.print(" *2 DEBUG WRITE: ev nr: {d}, info: {s} , name: {s}, DONE Writing to erw, len(b): {d}, {s}\n", .{ m.Ev.Nr, m.Info, m.Ev.Name, b.len, b });
        }

        std.debug.print("{s}, len: {d} when writing, not adding an event\n", .{ m.Info, b.len });
        return b.len;
    }

    // Read the data into b.
    pub fn Read(m: *EventRW, b: []u8) !usize {
        if (m.Pos >= m.Ev.Data.len) {
            std.debug.print(" *1 DEBUG READ: ev nr: {d}, info: {s}, name: {s}, EOF reading from erw, len(m.ev.Data): {d}, {s}\n", .{ m.Ev.Nr, m.Info, m.Ev.Name, m.Ev.Data.len, m.Ev.Data });
            return error.EOF;
        }

        std.debug.print(" *1 DEBUG READ: ev nr: {d}, info: {s}, name: {s}, PREPARING reading from erw, len(m.ev.Data): {d}, {s}\n", .{ m.Ev.Nr, m.Info, m.Ev.Name, m.Ev.Data.len, m.Ev.Data });

        const n = @min(b.len, m.Ev.Data.len - m.Pos);
        @memcpy(b[0..n], m.Ev.Data[m.Pos .. m.Pos + n]);

        std.debug.print(" *1 DEBUG READ: ev nr: {d}, info: {s}, name: {s}, DONE reading from erw, len(m.ev.Data): {d}, {s}\n", .{ m.Ev.Nr, m.Info, m.Ev.Name, m.Ev.Data.len, m.Ev.Data });

        m.Pos += n;
        return n;
    }
};

// NewEventRW will return a type that adds Read and Write methods
// to the Event type.
pub fn NewEventRW(allocator: std.mem.Allocator, p: *Process, ev: *Event, info: []const u8) !*EventRW {
    const m = try allocator.create(EventRW);
    m.* = EventRW{
        .P = p,
        .Ev = ev,
        .Info = info,
        .Pos = 0,
    };
    return m;
}
