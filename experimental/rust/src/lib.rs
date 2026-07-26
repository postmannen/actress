// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

// Actress is a concurrent actor framework. A system is modelled as a hierarchy
// of processes (actors) that communicate exclusively by passing events through
// channels. This is a Rust port of the Go package, kept structurally as close
// to the original as practical.
//
// The Go code leans heavily on Go's naming conventions (exported PascalCase
// identifiers, lowercase package private ones). To keep the structure familiar
// for readers of the Go code, we keep those names verbatim and silence the
// Rust naming lints rather than renaming everything to snake_case.
#![allow(non_snake_case)]
#![allow(non_upper_case_globals)]
#![allow(non_camel_case_types)]

pub mod logging;

pub mod buffer;
pub mod config;
pub mod context;
pub mod customprocesses;
pub mod dynamicprocesses;
pub mod errorprocesses;
pub mod eventrw;
pub mod events;
pub mod staticprocesses;
pub mod supervisorprocesses;

// Kept as a private module so its name does not collide with the crate name
// when users glob-import the crate. Its public items are re-exported below.
#[path = "actress.rs"]
mod actress;

// Re-export the public API at the crate root, so users can write
// actress::Event, actress::NewRootProcess, and so on, mirroring the Go
// package's flat namespace.
pub use buffer::*;
pub use config::*;
pub use context::Context;
pub use customprocesses::*;
pub use dynamicprocesses::*;
pub use errorprocesses::*;
pub use eventrw::*;
pub use events::*;
pub use staticprocesses::*;
pub use supervisorprocesses::*;

pub use actress::*;
