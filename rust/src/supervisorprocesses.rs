// Actress Copyright (C) 2024  Bjørn Tore Svinningen

use crate::actress::{Process, ProcFn, ETFunc};
use crate::context::Context;
use crate::events::{Event, EventName, Instruction};
use crate::staticprocesses::CopyEventFields;

use ciborium::{from_reader, into_writer};
use crossbeam_channel::select;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::{Arc, Mutex};

pub struct supervisorProcesses {
    pub procMap: Mutex<HashMap<EventName, Arc<Process>>>,
}

// Prepare and return a new supervisorProcesses structure.
pub fn newsuperVisorProcesses() -> supervisorProcesses {
    supervisorProcesses {
        procMap: Mutex::new(HashMap::new()),
    }
}

// ------------------------------------------------------------------------------
// Events and event functions, ESRouter
// ------------------------------------------------------------------------------

// Router for supervisor events.
pub const ESRouter: EventName = EventName::new("ESRouter");

// Process function for routing and handling supervisor events.
pub fn esRouterFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            select! {
                recv(p.SupervisorEventCh.rx) -> msg => {
                    let mut ev = match msg { Ok(e) => e, Err(_) => return };

                    // If there is a next event defined, copy the current event
                    // fields into the next event's PreviousEvent.
                    if ev.NextEvent.is_some() {
                        let prev = CopyEventFields(&ev);
                        if let Some(ne) = ev.NextEvent.as_mut() {
                            ne.PreviousEvent = Some(Box::new(prev));
                        }
                    }

                    let in_tx = {
                        let m = p.supervisorProcesses.procMap.lock().unwrap();
                        match m.get(&ev.Name) {
                            None => {
                                crate::log_error!(
                                    "esRouterFn: on {}: found no process registered for the event type {}",
                                    p.Config.NodeName, ev.Name
                                );
                                continue;
                            }
                            Some(proc) => proc.InCh.tx.clone(),
                        }
                    };

                    let _ = in_tx.send(ev);
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}

// ------------------------------------------------------------------------------
// Events and event functions, Process handling
// ------------------------------------------------------------------------------

// Handles information about the currently running processes in the local
// actress system.
pub const ESProcesses: EventName = EventName::new("ESProcesses");

pub const InstructionESProcessesAdd: Instruction = Instruction::new("InstructionESProcessesAdd");
pub const InstructionESProcessesDelete: Instruction =
    Instruction::new("InstructionESProcessesDelete");
pub const InstructionESProcessesGetAll: Instruction =
    Instruction::new("InstructionESProcessesGetAll");

#[derive(Serialize, Deserialize, Clone)]
pub struct esProcessesMapDataIn {
    pub Name: EventName,
}

pub type ESProcessesMap = HashMap<EventName, String>;

// ETFunc for handling information about the currently running processes in the
// local actress system.
pub fn esProcessesFn() -> ETFunc {
    Box::new(move |_ctx: Context, p: Arc<Process>| -> ProcFn {
        Box::new(move || {
            // The map of all the running processes.
            let mut processMap: ESProcessesMap = HashMap::new();

            loop {
                p.SignalReady();

                let ev = match p.InCh.rx.recv() {
                    Ok(e) => e,
                    Err(_) => return,
                };

                match ev.Instruction {
                    // Add the received data about a process to the map.
                    ref i if *i == InstructionESProcessesAdd => {
                        let md: esProcessesMapDataIn = match from_reader(ev.Data.as_slice()) {
                            Ok(md) => md,
                            Err(e) => {
                                crate::log_error!("esProcessesFn: failed to unmarshal esProcesses map in data: {}", e);
                                continue;
                            }
                        };

                        processMap.insert(md.Name.clone(), md.Name.to_string());

                        // Nothing to output, so just add the NextEvent if defined.
                        if let Some(ne) = ev.NextEvent {
                            p.AddEvent(*ne);
                        }
                    }

                    ref i if *i == InstructionESProcessesDelete => {
                        let _md: esProcessesMapDataIn = match from_reader(ev.Data.as_slice()) {
                            Ok(md) => md,
                            Err(e) => {
                                crate::log_error!("esProcessesFn: failed to unmarshal esProcesses map in data: {}", e);
                                continue;
                            }
                        };

                        processMap.remove(&p.Event);

                        if let Some(ne) = ev.NextEvent {
                            p.AddEvent(*ne);
                        }
                    }

                    // Dump the content of the whole processes map and send it
                    // with NextEvent.
                    ref i if *i == InstructionESProcessesGetAll => {
                        let mut buf: Vec<u8> = Vec::new();
                        if let Err(e) = into_writer(&processMap, &mut buf) {
                            crate::log_error!("esProcessesFn: failed to marshal esProcesses for push all: {}", e);
                        }

                        if let Some(mut ne) = ev.NextEvent {
                            ne.Data = buf;
                            p.AddEvent(*ne);
                        }
                    }

                    _ => {
                        crate::log_error!("esProcessesFn: not a defined instruction: {}", ev.Instruction);
                    }
                }
            }
        })
    })
}

// Silence unused warning for the convenience constructor of the dump event.
#[allow(dead_code)]
fn _dump_event(name: EventName) -> Event {
    Event {
        Name: name,
        ..Default::default()
    }
}
