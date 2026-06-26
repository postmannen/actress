// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

use crate::actress::{Process, ProcFn, ETFunc};
use crate::context::Context;
use crate::events::EventName;
use crate::staticprocesses::CopyEventFields;

use crossbeam_channel::select;
use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::Duration;

// Holds information about what process functions belong to what event, and a
// map of the started processes. The purpose of dynamic processes is to have
// short lived processes that can be quickly started and removed again when
// their job is done. The only difference from a static process is that dynamic
// processes can be deleted from the map.
pub struct dynamicProcesses {
    pub procMap: Mutex<HashMap<EventName, Arc<Process>>>,
}

impl dynamicProcesses {
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
        self.procMap.lock().unwrap().remove(et);
        println!("deleted process {}", et);
    }

    // Checks if the event is defined in the processes map.
    pub fn IsEventDefined(&self, ev: &EventName) -> bool {
        self.procMap.lock().unwrap().contains_key(ev)
    }
}

// Prepare and return a new dynamicProcesses structure.
pub fn newDynamicProcesses() -> dynamicProcesses {
    dynamicProcesses {
        procMap: Mutex::new(HashMap::new()),
    }
}

// Will create and return a new UUID prefixed with "ED-".
pub fn NewUUID() -> String {
    format!("ED-{}", uuid::Uuid::new_v4())
}

// ------------------------------------------------------------------------------
// Events and event functions.
// ------------------------------------------------------------------------------

// Router for dynamic events.
pub const EDRouter: EventName = EventName::new("EDRouter");

// Process function for routing and handling dynamic events.
pub fn edRouterFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            select! {
                recv(p.DynamicEventCh.rx) -> msg => {
                    let mut ev = match msg { Ok(e) => e, Err(_) => break };

                    // If there is a next event defined, copy the current event
                    // fields into the next event's PreviousEvent.
                    if ev.NextEvent.is_some() {
                        let prev = CopyEventFields(&ev);
                        if let Some(ne) = ev.NextEvent.as_mut() {
                            ne.PreviousEvent = Some(Box::new(prev));
                        }
                    }

                    // Dynamic processes can take a little longer to start up and
                    // be registered in the map. If the process is not registered
                    // yet, we retry in a separate thread so the router does not
                    // block, and continue with the next event in the queue.
                    let registered = p.DynamicProcesses.procMap.lock().unwrap().contains_key(&ev.Name);

                    if !registered {
                        let p2 = p.clone();
                        std::thread::spawn(move || {
                            // Try 3 times to deliver the message.
                            for _ in 0..3 {
                                let in_tx = {
                                    let m = p2.DynamicProcesses.procMap.lock().unwrap();
                                    m.get(&ev.Name).map(|proc| proc.InCh.tx.clone())
                                };

                                match in_tx {
                                    None => {
                                        crate::log_error!(
                                            "edRouterFn: on {}: found no process registered for the event type {}, ev.DstNode {}",
                                            p2.Config.NodeName, ev.Name, ev.DstNode
                                        );
                                        std::thread::sleep(Duration::from_secs(1));
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
                        let m = p.DynamicProcesses.procMap.lock().unwrap();
                        m.get(&ev.Name).map(|proc| proc.InCh.tx.clone())
                    };
                    if let Some(tx) = in_tx {
                        let _ = tx.send(ev);
                    }
                }
                recv(p.Ctx.done()) -> _ => break,
            }
        }

        p.Stop();
    })
}

// EDSync is used to synchronize events. The EDSyncFn takes a signal channel; we
// can then use this event type to signal that another event is done before we
// continue, by setting EDSync as the NextEvent.
pub const EDSync: EventName = EventName::new("EDSync");

// EDSyncFn is the function used to synchronize events. It takes a channel that
// will be used to send a signal when the EDSync event is executed.
//
// In general EDSync is used to synchronize one-off events, so delete the
// process after it is done with its sync job.
pub fn EDSyncFn(syncCh: crossbeam_channel::Sender<()>) -> ETFunc {
    Box::new(move |ctx: Context, p: Arc<Process>| -> ProcFn {
        Box::new(move || {
            loop {
                p.SignalReady();

                select! {
                    recv(p.InCh.rx) -> msg => {
                        if msg.is_err() { return; }
                        let _ = syncCh.send(());
                    }
                    recv(ctx.done()) -> _ => return,
                }
            }
        })
    })
}
