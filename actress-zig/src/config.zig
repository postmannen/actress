const std = @import("std");

/// Configuration structure for the actress system
pub const Config = struct {
    /// Profiling mode: "none", "cpu", "mem", "mutex", "block", "trace"
    profiling: []const u8 = "none",

    /// Enable custom events from files
    custom_events: bool = false,

    /// Enable metrics collection
    metrics: bool = false,

    /// Path for custom events files
    custom_events_path: []const u8 = "customevents",

    /// Debug mode
    debug: bool = false,

    /// Maximum retries for event delivery
    max_retries: u32 = 3,

    /// Retry delay in milliseconds
    retry_delay_ms: u64 = 1000,

    const Self = @This();

    /// Create a new Config with default values
    pub fn init() Self {
        return Self{};
    }

    /// Check environment variable and return the value or default
    pub fn checkEnv(comptime T: type, key: []const u8, default_value: T) T {
        const env_value = std.process.getEnvVarOwned(std.heap.page_allocator, key) catch return default_value;
        defer std.heap.page_allocator.free(env_value);

        switch (T) {
            []const u8 => return env_value,
            bool => {
                if (std.mem.eql(u8, env_value, "1") or
                    std.mem.eql(u8, env_value, "true"))
                {
                    return true;
                } else {
                    return false;
                }
            },
            u32 => {
                return std.fmt.parseInt(u32, env_value, 10) catch default_value;
            },
            u64 => {
                return std.fmt.parseInt(u64, env_value, 10) catch default_value;
            },
            else => @compileError("Unsupported type for checkEnv"),
        }
    }

    /// Create a new Config checking environment variables
    pub fn initFromEnv() Self {
        return Self{
            .profiling = checkEnv([]const u8, "PROFILING", "none"),
            .custom_events = checkEnv(bool, "CUSTOMEVENTS", false),
            .metrics = checkEnv(bool, "METRICS", false),
            .custom_events_path = checkEnv([]const u8, "CUSTOMEVENTSPATH", "customevents"),
            .debug = checkEnv(bool, "DEBUG", false),
            .max_retries = checkEnv(u32, "MAX_RETRIES", 3),
            .retry_delay_ms = checkEnv(u64, "RETRY_DELAY_MS", 1000),
        };
    }
};
