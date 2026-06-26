// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

use crate::actress::{Process, ProcFn};
use crate::context::Context;
use crate::events::EventName;
use crate::staticprocesses::CopyEventFields;

use crossbeam_channel::select;
use std::collections::HashMap;
use std::sync::{Arc, Mutex};

// Holds information about what process functions belong to what event, and a
// map of the started processes.
pub struct customProcesses {
    pub procMap: Mutex<HashMap<EventName, Arc<Process>>>,
}

impl customProcesses {
    // Add a new Event and its process to the processes map. Cancels any
    // existing process for the same event first.
    //
    // NB: This mirrors the Go method that the comment marks as currently
    // unused; kept for parity.
    pub fn Add(&self, et: EventName, proc: Arc<Process>) {
        let mut m = self.procMap.lock().unwrap();
        if let Some(old) = m.get(&et) {
            (old.Cancel)();
        }
        m.insert(et, proc);
    }

    // Delete an Event and its process from the processes map.
    pub fn Delete(&self, et: &EventName) {
        let mut m = self.procMap.lock().unwrap();
        if let Some(old) = m.get(et) {
            (old.Cancel)();
        }
        m.remove(et);
        println!("deleted process {}", et);
    }

    // Checks if the event is defined in the processes map.
    pub fn IsEventDefined(&self, ev: &EventName) -> bool {
        self.procMap.lock().unwrap().contains_key(ev)
    }
}

// Prepare and return a new customProcesses structure.
pub fn newCustomProcesses() -> customProcesses {
    customProcesses {
        procMap: Mutex::new(HashMap::new()),
    }
}

// ------------------------------------------------------------------------------
// Events and event functions.
// ------------------------------------------------------------------------------

// Router for custom events.
pub const ECRouter: EventName = EventName::new("ECRouter");

// Process function for routing and handling custom events.
pub fn ecRouterFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            select! {
                recv(p.CustomEventCh.rx) -> msg => {
                    let mut ev = match msg { Ok(e) => e, Err(_) => return };

                    // If there is a next event defined, copy the current event
                    // fields into the next event's PreviousEvent.
                    if ev.NextEvent.is_some() {
                        let prev = CopyEventFields(&ev);
                        if let Some(ne) = ev.NextEvent.as_mut() {
                            ne.PreviousEvent = Some(Box::new(prev));
                        }
                    }

                    // Custom processes can take a little longer to start up and
                    // be registered in the map. If not registered yet, we retry
                    // in a separate thread so the router does not block, and
                    // continue with the next event in the queue.
                    let registered = p.CustomProcesses.procMap.lock().unwrap().contains_key(&ev.Name);

                    if !registered {
                        let p2 = p.clone();
                        std::thread::spawn(move || {
                            // Try 3 times to deliver the message.
                            for _ in 0..3 {
                                let in_tx = {
                                    let m = p2.CustomProcesses.procMap.lock().unwrap();
                                    m.get(&ev.Name).map(|proc| proc.InCh.tx.clone())
                                };

                                match in_tx {
                                    None => {
                                        crate::log_error!(
                                            "ecRouterFn: on {}: found no process registered for the event type {}, ev.DstNode {}",
                                            p2.Config.NodeName, ev.Name, ev.DstNode
                                        );
                                        continue;
                                    }
                                    Some(tx) => {
                                        let _ = tx.send(ev);
                                        return;
                                    }
                                }
                            }
                        });
                        continue;
                    }

                    // Process was registered. Deliver the event to its InCh.
                    let in_tx = {
                        let m = p.CustomProcesses.procMap.lock().unwrap();
                        m.get(&ev.Name).map(|proc| proc.InCh.tx.clone())
                    };
                    if let Some(tx) = in_tx {
                        let _ = tx.send(ev);
                    }
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}

// Primarily used for testing to check that the ECRouter properly routes events,
// and that custom processes start up correctly. Forwards the event Data to the
// defined NextEvent.
pub const ECGeneralDelivery: EventName = EventName::new("ECGeneralDelivery");

pub fn ecGeneralDeliveryFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        p.SignalReady();

        loop {
            select! {
                recv(p.InCh.rx) -> msg => {
                    let ev = match msg { Ok(e) => e, Err(_) => return };
                    if let Some(mut ne) = ev.NextEvent {
                        ne.Data = ev.Data;
                        p.AddEvent(*ne);
                    }
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}
