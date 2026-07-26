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

const Node = @import("events.zig").Node;

// Config holds all the configuration settings for the actress system.
pub const Config = struct {
    CustomEvents: bool = false,
    Metrics: bool = false,
    CustomEventsPath: []const u8 = "customevents",
    NodeName: Node = "replaceme",
    LogLevel: []const u8 = "info",

    // Zig specific: environment variables are not globally reachable like
    // os.LookupEnv in Go, they are handed to main by the runtime. The map
    // is kept here so NewRootProcess can check the LOGLEVEL env variable.
    environ: ?*std.process.Environ.Map = null,
};

// New config prepare a *Config and return the resulting actress *Config.
// The values are checked for env variables, and if not found, the default
// value is used.
//
// The Go version also returns a *flag.FlagSet with command line flags for
// the same values. Zig got no flag package in the standard library, so the
// flag parsing is not ported, only the env variable handling.
//
// The logLevel is the default log level for the system, and can be provided
// as an input argument.
pub fn NewConfig(allocator: std.mem.Allocator, environ: ?*std.process.Environ.Map, logLevel: []const u8) !*Config {
    // The config with default values set.
    const c = try allocator.create(Config);
    c.* = Config{
        .CustomEvents = false,
        .Metrics = false,
        .CustomEventsPath = "customevents",
        .NodeName = "replaceme",
        .LogLevel = logLevel,
        .environ = environ,
    };

    c.CustomEvents = CheckEnv(bool, environ, "CUSTOMEVENTS", c.CustomEvents);
    c.Metrics = CheckEnv(bool, environ, "METRICS", c.Metrics);
    c.CustomEventsPath = CheckEnv([]const u8, environ, "CUSTOMEVENTSPATH", c.CustomEventsPath);
    c.NodeName = CheckEnv([]const u8, environ, "NODENAME", c.NodeName);
    c.LogLevel = CheckEnv([]const u8, environ, "LOGLEVEL", c.LogLevel);

    return c;
}

// Check if an env variable is set. If found, return the value.
// Takes the name of the env variable, and the actual variable
// containing a default value as it's input.
// The Go version takes any type and does a runtime type switch. The Zig
// version takes the type as a comptime parameter instead.
pub fn CheckEnv(comptime T: type, environ: ?*std.process.Environ.Map, key: []const u8, v: T) T {
    const env = environ orelse return v;
    const val = env.get(key) orelse return v;

    switch (T) {
        i64 => {
            const n = std.fmt.parseInt(i64, val, 10) catch {
                std.debug.print("error: failed to convert env to int: {s}\n", .{val});
                std.process.exit(1);
            };
            return n;
        },
        []const u8 => {
            return val;
        },
        bool => {
            if (std.mem.eql(u8, val, "1") or std.mem.eql(u8, val, "true")) {
                return true;
            } else {
                return false;
            }
        },
        else => {},
    }

    return v;
}
