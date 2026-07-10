// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

use crate::actress::{Process, ProcFn};
use crate::context::Context;
use crate::events::{EventName, Instruction};
use crate::staticprocesses::CopyEventFields;

use crossbeam_channel::select;
use std::collections::HashMap;
use std::sync::{Arc, Mutex};

// errorProcesses holds information about what process functions belong to what
// event, and a map of the started processes.
//
// NB: The Go errorProcesses struct intentionally has no mutex; the comment in
// the Go code documents that this is on purpose. Rust requires synchronized
// access to share a map across threads, so a Mutex is used here.
pub struct errorProcesses {
    pub procMap: Mutex<HashMap<EventName, Arc<Process>>>,
}

impl errorProcesses {
    // Checks if the event is defined in the processes map.
    pub fn IsEventDefined(&self, ev: &EventName) -> bool {
        self.procMap.lock().unwrap().contains_key(ev)
    }
}

// Prepare and return a new errorProcesses structure.
pub fn newErrorProcesses() -> errorProcesses {
    errorProcesses {
        procMap: Mutex::new(HashMap::new()),
    }
}

// Router for error events.
pub const ERRouter: EventName = EventName::new("ERRouter");

// Process function for routing and handling error events.
pub fn erRouterFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            select! {
                recv(p.ErrorEventCh.rx) -> msg => {
                    let mut e = match msg { Ok(e) => e, Err(_) => return };

                    // If there is a next event defined, copy the current event
                    // fields into the next event's PreviousEvent.
                    if e.NextEvent.is_some() {
                        let prev = CopyEventFields(&e);
                        if let Some(ne) = e.NextEvent.as_mut() {
                            ne.PreviousEvent = Some(Box::new(prev));
                        }
                    }

                    let in_tx = {
                        let m = p.ErrorProcesses.procMap.lock().unwrap();
                        match m.get(&e.Name) {
                            None => {
                                crate::log_error!(
                                    "erRouterFn: on {}: found no process registered for the event type {}",
                                    p.Config.NodeName, e.Name
                                );
                                continue;
                            }
                            Some(proc) => proc.InCh.tx.clone(),
                        }
                    };

                    let _ = in_tx.send(e);
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}

// Instructions for error logging.
pub const InstructionError: Instruction = Instruction::new("InstructionError");
pub const InstructionInfo: Instruction = Instruction::new("InstructionInfo");
pub const InstructionDebug: Instruction = Instruction::new("InstructionDebug");
pub const InstructionFatal: Instruction = Instruction::new("InstructionFatal");

// Log errors.
pub const ERLog: EventName = EventName::new("ERLog");

// Logs errors to the console based on the Instruction field of the event.
//
// NB: The "none" log level is handled where the level is set, to drop the event
// as early as possible instead of sending it all the way here.
pub fn erLogFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            p.SignalReady();

            select! {
                recv(p.InCh.rx) -> msg => {
                    let er = match msg { Ok(e) => e, Err(_) => return };
                    let m = er.Err.clone().unwrap_or_default();

                    match er.Instruction {
                        ref i if *i == InstructionError => crate::log_error!("erLogFn: instructionError, msg {}", m),
                        ref i if *i == InstructionInfo => crate::log_info!("erLogFn: instructionInfo, msg {}", m),
                        ref i if *i == InstructionDebug => crate::log_debug!("erLogFn: instructionDebug, msg {}", m),
                        ref i if *i == InstructionFatal => {
                            crate::log_error!("erLogFn: instructionFatal, msg {}", m);
                            std::process::exit(1);
                        }
                        _ => crate::log_error!("erLogFn: default, msg {}", m),
                    }
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}

// Log and exit system (used for testing).
pub const ERTest: EventName = EventName::new("ERTest");

pub fn erTestFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            p.SignalReady();

            select! {
                recv(p.InCh.rx) -> msg => {
                    let er = match msg { Ok(e) => e, Err(_) => return };
                    let _drop = format!(
                        "erTestFn: error for fatal logging received: {}",
                        er.Err.clone().unwrap_or_default()
                    );
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}

// Will drop the event if it is an error event. Primarily used for testing.
pub const ERNone: EventName = EventName::new("ERNone");

pub fn erNoneFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            p.SignalReady();

            select! {
                recv(p.InCh.rx) -> msg => {
                    let _er = match msg { Ok(e) => e, Err(_) => return };
                    // Drop the event.
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}
