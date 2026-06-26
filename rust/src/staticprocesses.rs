// Actress Copyright (C) 2024  Bjørn Tore Svinningen

use crate::actress::{pidnr, Process, ProcFn};
use crate::context::Context;
use crate::events::{Event, EventName};

use ciborium::into_writer;
use crossbeam_channel::select;
use std::collections::HashMap;
use std::io::Read;
use std::sync::{Arc, Once};

// Holds information about what process functions belong to what event, and a
// map of the started processes.
pub struct staticProcesses {
    pub procMap: std::sync::Mutex<HashMap<EventName, Arc<Process>>>,
}

impl staticProcesses {
    // Checks if the event is defined in the processes map.
    pub fn IsEventDefined(&self, ev: &EventName) -> bool {
        self.procMap.lock().unwrap().contains_key(ev)
    }
}

// Prepare and return a new staticProcesses structure.
pub fn newStaticProcesses() -> staticProcesses {
    staticProcesses {
        procMap: std::sync::Mutex::new(HashMap::new()),
    }
}

// -----------------------------------------------------------------------------
// Builtin standard Names and their ETfuncs.
// -----------------------------------------------------------------------------

// ETRemote is a Name used if an event should be delivered to a remote node.
// There is no ETFunc defined for ETRemote in actress, so it is up to the user
// to write this function and attach it when they create the process to handle
// the ETRemote Name. ETRemote is prepended to the current event by AddEvent
// when the event should not be handled locally.
pub const ETRemote: EventName = EventName::new("ETRemote");

// Router for normal events.
pub const ETRouter: EventName = EventName::new("ETRouter");

// Process function for routing and handling events. Checks and routes the
// event to the correct static process.
pub fn etRouterFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            select! {
                recv(p.StaticEventCh.rx) -> msg => {
                    let mut ev = match msg {
                        Ok(e) => e,
                        Err(_) => break,
                    };

                    if ev.Name == ETRemote {
                        let m = p.StaticProcesses.procMap.lock().unwrap();
                        if !m.contains_key(&ev.Name) {
                            crate::log_error!(
                                "etRouterFn: on {}: found no process registered for the event type ETRemote; register an ETFunc for it: {}",
                                p.Config.NodeName, ev.Name
                            );
                        }
                    }

                    // If there is a next event defined, copy all the fields of
                    // the current event and put that as the PreviousEvent on the
                    // next event, so the next process can check things like what
                    // node to reply back to.
                    if ev.NextEvent.is_some() {
                        let prev = CopyEventFields(&ev);
                        if let Some(ne) = ev.NextEvent.as_mut() {
                            ne.PreviousEvent = Some(Box::new(prev));
                        }
                    }

                    // Look up the target process and grab a clone of its InCh
                    // sender, then release the lock before sending.
                    let in_tx = {
                        let m = p.StaticProcesses.procMap.lock().unwrap();
                        match m.get(&ev.Name) {
                            None => {
                                crate::log_error!(
                                    "etRouterFn: on {}: found no process registered for the event type {}",
                                    p.Config.NodeName, ev.Name
                                );
                                continue;
                            }
                            Some(proc) => proc.InCh.tx.clone(),
                        }
                    };

                    let _ = in_tx.send(ev);
                }
                recv(p.Ctx.done()) -> _ => {
                    break;
                }
            }
        }

        p.Stop();
    })
}

// Copy all the descriptive metadata fields of the Event, not channels or Data,
// NextEvent or PreviousEvent.
pub fn CopyEventFields(ev: &Event) -> Event {
    Event {
        Nr: ev.Nr,
        Name: ev.Name.clone(),
        Cmd: ev.Cmd.clone(),
        Instruction: ev.Instruction.clone(),
        Err: ev.Err.clone(),
        DstNode: ev.DstNode.clone(),
        SrcNode: ev.SrcNode.clone(),
        ..Default::default()
    }
}

// Press ctrl+c to exit.
pub const ETOsSignal: EventName = EventName::new("ETOsSignal");

// Guards the global signal handler so it is installed only once, even if more
// than one root process is created in the same program.
static SIGNAL_ONCE: Once = Once::new();

// Process function for handling CTRL+C. Installs a SIGINT handler that exits
// the program, then blocks until the context is cancelled.
pub fn etOsSignalFn(ctx: Context, _p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        SIGNAL_ONCE.call_once(|| {
            let res = ctrlc::set_handler(|| {
                println!("Got terminate signal, terminating all processes");
                std::process::exit(0);
            });
            if let Err(e) = res {
                crate::log_error!("etOsSignalFn: failed to set signal handler: {}", e);
            }
        });

        // Block until the context is cancelled.
        let _ = ctx.done().recv();
    })
}

// The ETTest event type is used for testing.
pub const ETTest: EventName = EventName::new("ETTest");
pub const InstructionCmdEOF: crate::events::Instruction =
    crate::events::Instruction::new("InstructionCmdEOF");

// ETTestfn accepts a channel as its input argument and returns the Data field
// of received events on that channel as a String. You can then listen on that
// channel and check the value. Receiving an event with InstructionCmdEOF closes
// the channel.
pub fn ETTestfn(testCh: crossbeam_channel::Sender<String>) -> crate::actress::ETFunc {
    Box::new(move |ctx: Context, p: Arc<Process>| -> ProcFn {
        Box::new(move || {
            p.SignalReady();

            loop {
                select! {
                    recv(p.InCh.rx) -> msg => {
                        let result = match msg { Ok(e) => e, Err(_) => return };
                        if result.Instruction == InstructionCmdEOF {
                            // Dropping the sender closes the channel.
                            drop(testCh);
                            return;
                        }
                        let _ = testCh.send(String::from_utf8_lossy(&result.Data).to_string());

                        if let Some(ne) = result.NextEvent {
                            p.AddEvent(*ne);
                        }
                    }
                    recv(ctx.done()) -> _ => return,
                }
            }
        })
    })
}

// Will forward the incoming event to the builtin TestCh of the process.
pub const ETTestCh: EventName = EventName::new("ETTestCh");

pub fn etTestChFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        p.SignalReady();

        loop {
            select! {
                recv(p.InCh.rx) -> msg => {
                    let e = match msg { Ok(e) => e, Err(_) => return };
                    let _ = p.TestCh.tx.send(e);
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}

// Get all the current processes running. Returns a CBOR encoded map of pid to
// process name in the NextEvent Data.
pub const ETPidGetAll: EventName = EventName::new("ETPidGetAll");

// NB: The Go version CBOR encodes the full pid to *Process map. Encoding the
// whole Process graph is neither meaningful nor serializable here (it holds
// channels, contexts and back references), so this port encodes a map of pid to
// the process event name, which is the production sensible representation.
pub fn etPidGetAllFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        p.SignalReady();

        loop {
            select! {
                recv(p.InCh.rx) -> msg => {
                    let e = match msg { Ok(e) => e, Err(_) => return };
                    let snapshot: HashMap<pidnr, String> = p.pidSnapshot();
                    let mut buf: Vec<u8> = Vec::new();
                    if let Err(err) = into_writer(&snapshot, &mut buf) {
                        crate::log_error!("etPidGetAllFn: failed to marshal pid to proc map: {}", err);
                        panic!("etPidGetAllFn: failed to marshal pid to proc map: {}", err);
                    }

                    if let Some(ne) = e.NextEvent.as_ref() {
                        p.AddEvent(Event { Name: ne.Name.clone(), Data: buf, ..Default::default() });
                    }
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}

// Done logs received events.
pub const ETDone: EventName = EventName::new("ETDone");

pub fn etDoneFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            p.SignalReady();

            let d = match p.InCh.rx.recv() {
                Ok(e) => e,
                Err(_) => return,
            };

            crate::log_info!("etDoneFn: got event ETDone: {}", String::from_utf8_lossy(&d.Data));
            crate::log_error!("etDoneFn: got etDone, on {}", p.Config.NodeName);
        }
    })
}

// Print the content of the Data field of the event to stdout.
pub const ETPrint: EventName = EventName::new("ETPrint");

pub fn etPrintFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            p.SignalReady();

            select! {
                recv(p.InCh.rx) -> msg => {
                    let d = match msg { Ok(e) => e, Err(_) => return };
                    println!("{}", String::from_utf8_lossy(&d.Data));
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}

// Will exit and kill all processes.
pub const ETExit: EventName = EventName::new("ETExit");

pub fn etExitFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        p.SignalReady();

        loop {
            select! {
                recv(p.InCh.rx) -> msg => {
                    let d = match msg { Ok(e) => e, Err(_) => return };
                    println!("etExitFn: got event ETExit: {}", String::from_utf8_lossy(&d.Data));
                    std::process::exit(0);
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}

// Handling pids within the system. The structure of the ev.Cmd is a slice:
// [action, pid, process name].
pub const ETPid: EventName = EventName::new("ETPid");

const pidGet: &str = "pidGet";
const pidGetAll: &str = "pidGetAll";

pub fn etPidFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        p.SignalReady();

        loop {
            select! {
                recv(p.InCh.rx) -> msg => {
                    let ev = match msg { Ok(e) => e, Err(_) => return };
                    let action = ev.Cmd[0].as_str();
                    let pid: pidnr = match ev.Cmd[1].parse() {
                        Ok(n) => n,
                        Err(e) => {
                            crate::log_error!("etPidFn: failed to convert pid from string to int: {}", e);
                            panic!("etPidFn: failed to convert pid from string to int: {}", e);
                        }
                    };
                    let procName = ev.Cmd[2].clone();

                    match action {
                        x if x == pidGet => {
                            if let Some(ne) = ev.NextEvent.as_ref() {
                                p.AddEvent(Event {
                                    Name: ne.Name.clone(),
                                    Data: format!("pid: {}, process name: {}", pid, procName).into_bytes(),
                                    ..Default::default()
                                });
                            }
                        }
                        x if x == pidGetAll => {
                            let snapshot = p.pidSnapshot();
                            for (pid, name) in snapshot.iter() {
                                if let Some(ne) = ev.NextEvent.as_ref() {
                                    p.AddEvent(Event {
                                        Name: ne.Name.clone(),
                                        Data: format!("pid: {}, process name: {}", pid, name).into_bytes(),
                                        ..Default::default()
                                    });
                                }
                            }
                        }
                        _ => {}
                    }
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}

// Read file. The path to read should be in Event.Cmd[0]. The content is put in
// the NextEvent Data, and that event is added.
pub const ETReadFile: EventName = EventName::new("ETReadFile");

pub fn ETReadFileFn(_ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        p.SignalReady();

        loop {
            select! {
                recv(p.InCh.rx) -> msg => {
                    let ev = match msg { Ok(e) => e, Err(_) => return };
                    let path = ev.Cmd[0].clone();

                    let mut fh = match std::fs::File::open(&path) {
                        Ok(f) => f,
                        Err(e) => {
                            crate::log_error!("etReadFileFn: failed to open file: {}", e);
                            panic!("etReadFileFn: failed to open file: {}", e);
                        }
                    };
                    let mut b = Vec::new();
                    if let Err(e) = fh.read_to_end(&mut b) {
                        crate::log_error!("etReadFileFn: failed to read file: {}", e);
                        panic!("etReadFileFn: failed to read file: {}", e);
                    }

                    if let Some(mut ne) = ev.NextEvent {
                        ne.Data = b;
                        p.AddEvent(*ne);
                    }
                }
                recv(p.Ctx.done()) -> _ => return,
            }
        }
    })
}
