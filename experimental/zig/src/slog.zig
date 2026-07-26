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

// This file has no counterpart in the Go version. The Go version uses the
// log/slog package from the standard library. Zig's std.log filters the
// log level at compile time, but actress needs the level to be settable
// at runtime from the config or the LOGLEVEL env variable, so a minimal
// slog equivalent with a runtime level is implemented here. The output
// format mimics the slog text handler: level=INFO msg=... key=value ...

const std = @import("std");

// The log levels with the same numeric values as Go log/slog.
pub const Level = enum(i16) {
    debug = -4,
    info = 0,
    err = 8,
    // slog has no fatal or none level. none is used to disable all logs by
    // setting a very high level, like the Go version does with slog.Level(1000).
    none = 1000,
};

// The current log level for the system. Set once when the root process is
// created, like slog.SetDefault in the Go version.
var levelVar: Level = .info;

// Set the log level. Allowed values: debug, info, error, fatal, none.
// Map unknown values to info. Mirrors the level setup in NewRootProcess.
pub fn SetLevelFromString(levelStr: []const u8) void {
    if (std.ascii.eqlIgnoreCase(levelStr, "debug")) {
        levelVar = .debug;
        return;
    }
    if (std.ascii.eqlIgnoreCase(levelStr, "info")) {
        levelVar = .info;
        return;
    }
    if (std.ascii.eqlIgnoreCase(levelStr, "error")) {
        levelVar = .err;
        return;
    }
    if (std.ascii.eqlIgnoreCase(levelStr, "fatal")) {
        // slog has no fatal level; use error and rely on explicit exits
        // for fatal paths.
        levelVar = .err;
        return;
    }
    if (std.ascii.eqlIgnoreCase(levelStr, "none")) {
        // Disable all logs by setting a very high level.
        levelVar = .none;
        return;
    }
    levelVar = .info;
}

// Enabled reports whether a message at the given level would be logged.
// The equivalent of slog.Default().Enabled in the Go version.
pub fn Enabled(l: Level) bool {
    return @intFromEnum(l) >= @intFromEnum(levelVar);
}

pub fn Debug(msg: []const u8, kv: anytype) void {
    logAt(.debug, "DEBUG", msg, kv);
}

pub fn Info(msg: []const u8, kv: anytype) void {
    logAt(.info, "INFO", msg, kv);
}

pub fn Error(msg: []const u8, kv: anytype) void {
    logAt(.err, "ERROR", msg, kv);
}

// Write the log line to stderr. The kv argument is a tuple with
// alternating keys and values, like the variadic arguments of the
// Go slog functions.
fn logAt(l: Level, levelName: []const u8, msg: []const u8, kv: anytype) void {
    if (!Enabled(l)) {
        return;
    }

    // std.debug is used for the actual printing since it writes to stderr
    // and holds a lock so lines from different tasks don't interleave.
    std.debug.print("level={s} msg={s}", .{ levelName, msg });

    const fields = std.meta.fields(@TypeOf(kv));
    comptime var i: usize = 0;
    inline while (i < fields.len) : (i += 2) {
        // Print the key.
        printValue(" ", kv[i]);
        // Print the value if there is one, keys without a value are
        // printed alone like slog prints !BADKEY entries.
        if (i + 1 < fields.len) {
            printValue("=", kv[i + 1]);
        }
    }
    std.debug.print("\n", .{});
}

// Print a single key or value with the given prefix. Strings are printed
// verbatim, everything else with the generic any format.
fn printValue(prefix: []const u8, value: anytype) void {
    const T = @TypeOf(value);
    if (comptime isString(T)) {
        std.debug.print("{s}{s}", .{ prefix, value });
        return;
    }
    if (comptime isOptionalString(T)) {
        if (value) |v| {
            std.debug.print("{s}{s}", .{ prefix, v });
        } else {
            std.debug.print("{s}null", .{prefix});
        }
        return;
    }
    std.debug.print("{s}{any}", .{ prefix, value });
}

fn isString(comptime T: type) bool {
    return switch (@typeInfo(T)) {
        .pointer => |ptr| switch (ptr.size) {
            .slice => ptr.child == u8,
            .one => switch (@typeInfo(ptr.child)) {
                .array => |arr| arr.child == u8,
                else => false,
            },
            else => false,
        },
        else => false,
    };
}

fn isOptionalString(comptime T: type) bool {
    return switch (@typeInfo(T)) {
        .optional => |opt| isString(opt.child),
        else => false,
    };
}
