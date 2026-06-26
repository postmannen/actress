// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

use std::io::{self, Read, Write};
use std::sync::Mutex;

// Buffer is a thread safe FIFO byte buffer, the equivalent of the Go Buffer
// that wraps bytes.Buffer with a mutex. Writes append to the back, reads
// consume from the front.
pub struct Buffer {
    inner: Mutex<BufferInner>,
}

struct BufferInner {
    buf: Vec<u8>,
    pos: usize,
}

impl Buffer {
    pub fn new() -> Buffer {
        Buffer {
            inner: Mutex::new(BufferInner {
                buf: Vec::new(),
                pos: 0,
            }),
        }
    }
}

impl Default for Buffer {
    fn default() -> Self {
        Buffer::new()
    }
}

// NewBuffer returns a new Buffer.
pub fn NewBuffer() -> Buffer {
    Buffer::new()
}

impl Buffer {
    pub fn Read(&self, p: &mut [u8]) -> io::Result<usize> {
        let mut inner = self.inner.lock().unwrap();
        if inner.pos >= inner.buf.len() {
            // Nothing buffered: reset and report EOF, like bytes.Buffer.
            inner.buf.clear();
            inner.pos = 0;
            return Ok(0);
        }
        let pos = inner.pos;
        let available = &inner.buf[pos..];
        let n = std::cmp::min(p.len(), available.len());
        p[..n].copy_from_slice(&available[..n]);
        inner.pos += n;
        Ok(n)
    }

    pub fn Write(&self, b: &[u8]) -> io::Result<usize> {
        let mut inner = self.inner.lock().unwrap();
        inner.buf.extend_from_slice(b);
        Ok(b.len())
    }
}

// Also expose the std::io traits for ergonomic use with the io ecosystem.
impl Read for &Buffer {
    fn read(&mut self, p: &mut [u8]) -> io::Result<usize> {
        Buffer::Read(self, p)
    }
}

impl Write for &Buffer {
    fn write(&mut self, b: &[u8]) -> io::Result<usize> {
        Buffer::Write(self, b)
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}
