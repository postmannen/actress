const event = @import("event.zig");

/// Built-in Event Types - matching the Go implementation
/// The main Root process
pub const ETRoot: event.EventType = "ETRoot";

/// Router for normal events
pub const ETRouter: event.EventType = "ETRouter";

/// Router for dynamic events
pub const EDRouter: event.EventType = "EDRouter";

/// Router for error events
pub const ERRouter: event.EventType = "ERRouter";

/// Press ctrl+c to exit
pub const ETOsSignal: event.EventType = "ETOsSignal";

/// Will forward the incoming event to the Process.TestCh
pub const ETTestCh: event.EventType = "ETTestCh";

/// Get information about a given PID
pub const ETPid: event.EventType = "ETPid";

/// Get all PIDs
pub const ETPidGetAll: event.EventType = "ETPidGetAll";

/// Enable profiling
pub const ETProfiling: event.EventType = "ETProfiling";

/// Process done signal
pub const ETDone: event.EventType = "ETDone";

/// Print to stdout
pub const ETPrint: event.EventType = "ETPrint";

/// Exit the system
pub const ETExit: event.EventType = "ETExit";

/// Watch for file changes
pub const ETWatchEventFile: event.EventType = "ETWatchEventFile";

/// Read file
pub const ETReadFile: event.EventType = "ETReadFile";

/// Custom event from file
pub const ETCustomEvent: event.EventType = "ETCustomEvent";

/// Execute OS command
pub const ETOsCmd: event.EventType = "ETOsCmd";

/// HTTP Get request
pub const ETHttpGet: event.EventType = "ETHttpGet";

/// Timer event
pub const ETTimer: event.EventType = "ETTimer";

// Error Event Types

/// Error log
pub const ERLog: event.EventType = "ERLog";

/// Debug message
pub const ERDebug: event.EventType = "ERDebug";

/// Fatal error
pub const ERFatal: event.EventType = "ERFatal";

/// Test error
pub const ERTest: event.EventType = "ERTest";
