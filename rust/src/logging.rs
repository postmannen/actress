// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

// Minimal leveled logging that mirrors how the Go code uses slog.
//
// The Go code configures slog with one of the levels debug/info/error/fatal/
// none, where the environment variable LOGLEVEL always wins over the
// configured value. "fatal" maps to error (slog has no fatal level), and the
// explicit process exit for fatal paths is handled where the error is logged.
// "none" disables all logging by setting a very high level.

use std::sync::atomic::{AtomicU8, Ordering};

pub const DEBUG: u8 = 0;
pub const INFO: u8 = 1;
pub const ERROR: u8 = 2;
pub const NONE: u8 = 255;

// The currently active log level. A message is printed when its level is
// greater than or equal to this value.
static LEVEL: AtomicU8 = AtomicU8::new(INFO);

// Set the active level from one of the strings used by the Config and the
// LOGLEVEL environment variable. Unknown values map to info, the same way the
// Go code does.
pub fn set_level_str(s: &str) {
    let level = match s.to_lowercase().as_str() {
        "debug" => DEBUG,
        "info" => INFO,
        "error" => ERROR,
        // slog has no fatal level, so it is treated as error.
        "fatal" => ERROR,
        "none" => NONE,
        _ => INFO,
    };
    LEVEL.store(level, Ordering::Relaxed);
}

// Reports whether a message at the given level should be printed.
pub fn enabled(level: u8) -> bool {
    level >= LEVEL.load(Ordering::Relaxed)
}

#[macro_export]
macro_rules! log_error {
    ($($arg:tt)*) => {{
        if $crate::logging::enabled($crate::logging::ERROR) {
            eprintln!("ERROR: {}", format!($($arg)*));
        }
    }};
}

#[macro_export]
macro_rules! log_info {
    ($($arg:tt)*) => {{
        if $crate::logging::enabled($crate::logging::INFO) {
            eprintln!("INFO: {}", format!($($arg)*));
        }
    }};
}

#[macro_export]
macro_rules! log_debug {
    ($($arg:tt)*) => {{
        if $crate::logging::enabled($crate::logging::DEBUG) {
            eprintln!("DEBUG: {}", format!($($arg)*));
        }
    }};
}
