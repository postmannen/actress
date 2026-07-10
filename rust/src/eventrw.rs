// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

use crate::actress::Process;
use crate::events::Event;
use std::io::{self, Read, Write};
use std::sync::Arc;

// EventRW adds Read and Write methods to an Event, so it can be plugged into
// Rust's std::io ecosystem the same way the Go EventRW implements io.Reader and
// io.Writer.
pub struct EventRW {
    pub P: Arc<Process>,
    pub Ev: Event,
    pub Info: String,
    pub Pos: usize,
}

// NewEventRW returns an EventRW wrapping the given process and event.
pub fn NewEventRW(p: Arc<Process>, ev: Event, info: &str) -> EventRW {
    EventRW {
        P: p,
        Ev: ev,
        Info: info.to_string(),
        Pos: 0,
    }
}

impl Write for EventRW {
    // Write sets the data into Event.Data and adds the event to be processed.
    fn write(&mut self, b: &[u8]) -> io::Result<usize> {
        if !b.is_empty() {
            let mut ev = self.Ev.clone();
            ev.Data = b.to_vec();
            self.P.AddEvent(ev);
        }
        Ok(b.len())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

impl Read for EventRW {
    // Read reads sequentially from Event.Data into b, returning 0 (EOF) once the
    // data is exhausted.
    fn read(&mut self, b: &mut [u8]) -> io::Result<usize> {
        if self.Pos >= self.Ev.Data.len() {
            return Ok(0);
        }

        let src = &self.Ev.Data[self.Pos..];
        let n = std::cmp::min(b.len(), src.len());
        b[..n].copy_from_slice(&src[..n]);
        self.Pos += n;
        Ok(n)
    }
}
