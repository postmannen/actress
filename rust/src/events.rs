// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

use serde::{Deserialize, Serialize};
use std::borrow::Cow;
use std::fmt;

// EventName is a unique name used to identify events. It is used both for
// creating processes and also for routing messages to the correct process.
//
// We back it by Cow<'static, str> so that the built in event names can be
// declared as compile time constants (Cow::Borrowed), while names built at
// runtime, like the UUID based dynamic process names, can own their string
// (Cow::Owned).
#[derive(Clone, PartialEq, Eq, Hash, Debug, Default, Serialize, Deserialize)]
pub struct EventName(pub Cow<'static, str>);

impl EventName {
    pub const fn new(s: &'static str) -> EventName {
        EventName(Cow::Borrowed(s))
    }

    pub fn from_string(s: String) -> EventName {
        EventName(Cow::Owned(s))
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl fmt::Display for EventName {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0)
    }
}

// Instruction has the underlying type of string. This field can be used to
// give for example an instruction of a single word, for example in switch
// statements at the receiving actor.
#[derive(Clone, PartialEq, Eq, Hash, Debug, Default, Serialize, Deserialize)]
pub struct Instruction(pub Cow<'static, str>);

impl Instruction {
    pub const fn new(s: &'static str) -> Instruction {
        Instruction(Cow::Borrowed(s))
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl fmt::Display for Instruction {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0)
    }
}

// Node identifies a node for distributed processing.
#[derive(Clone, PartialEq, Eq, Hash, Debug, Default, Serialize, Deserialize)]
pub struct Node(pub Cow<'static, str>);

impl Node {
    pub const fn new(s: &'static str) -> Node {
        Node(Cow::Borrowed(s))
    }

    pub fn from_string(s: String) -> Node {
        Node(Cow::Owned(s))
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }

    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }
}

impl fmt::Display for Node {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0)
    }
}

// Event defines an event. It holds:
//   - The Name, which specifies the process it is meant for.
//   - The Cmd, meant to but not limited to be a way to give instructions for
//     what a process should do. The receiving process is responsible for
//     parsing the slice into something useful.
//   - The Data field is meant to carry the result from the work done by a
//     process, to the next process.
//   - Both Cmd and Data can be used interchangeably if it makes more sense for
//     a given scenario. No strict rules exist.
//   - Err is used by the error event type (ER).
//   - NextEvent is used when we want to define a chain of events to be executed
//     like a workflow. The processes must make use of the field for this to
//     work.
//   - PreviousEvent allows for keeping information about the previous event. It
//     is filled in by the routers with a metadata copy of the triggering event.
//
// NB: The Go Event also has an InternalCh field of type "chan chan []byte" for
// transferring a channel directly between actors. It is tagged as not
// serialized and is not used by the examples, so it is omitted from this port.
#[derive(Clone, Default)]
pub struct Event {
    pub Nr: i64,
    pub Name: EventName,
    pub Cmd: Vec<String>,
    pub Instruction: Instruction,
    pub Data: Vec<u8>,
    // Err is used for defining the error message when the event is used as an
    // error event.
    pub Err: Option<String>,
    // NextEvent defines a series of events to be executed like a workflow.
    pub NextEvent: Option<Box<Event>>,
    // PreviousEvent keeps information about the previous event if needed.
    pub PreviousEvent: Option<Box<Event>>,
    pub DstNode: Node,
    pub SrcNode: Node,
}

// The main Root process. By default the root process does not have an ETFunc
// registered with it to handle the ETRoot event type, but one can be created
// with the normal ETFunc function signature and defined when creating a new
// root process.
pub const ETRoot: EventName = EventName::new("ETRoot");

// A functional option used by NewEvent to set fields on a new Event.
pub type EventOpt = Box<dyn FnOnce(&mut Event)>;

// NewEvent constructs an Event with the given name and applies the options.
pub fn NewEvent(et: EventName, opts: Vec<EventOpt>) -> Event {
    let mut ev = Event {
        Name: et,
        ..Default::default()
    };
    for opt in opts {
        opt(&mut ev);
    }
    ev
}

// EvCmd sets the Cmd field.
pub fn EvCmd(cmd: Vec<String>) -> EventOpt {
    Box::new(move |ev: &mut Event| {
        ev.Cmd = cmd;
    })
}

// EVData sets the Data field.
pub fn EVData(b: Vec<u8>) -> EventOpt {
    Box::new(move |ev: &mut Event| {
        ev.Data = b;
    })
}

// EvNext sets the NextEvent field.
pub fn EvNext(nev: Event) -> EventOpt {
    Box::new(move |ev: &mut Event| {
        ev.NextEvent = Some(Box::new(nev));
    })
}
