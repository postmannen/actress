// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

use crate::config::Config;
use crate::context::{CancelFn, Context};
use crate::customprocesses::{customProcesses, newCustomProcesses};
use crate::dynamicprocesses::{dynamicProcesses, newDynamicProcesses};
use crate::errorprocesses::{errorProcesses, newErrorProcesses};
use crate::events::{Event, EventName, ETRoot};
use crate::staticprocesses::{newStaticProcesses, staticProcesses, ETRemote};
use crate::supervisorprocesses::{
    esProcessesMapDataIn, newsuperVisorProcesses, supervisorProcesses,
};

use crossbeam_channel::{after, bounded, select, Receiver, Sender};
use std::collections::HashMap;
use std::sync::{Arc, Mutex, Once};
use std::time::Duration;

// The process ID number type. Go calls this pidnr.
pub type pidnr = i64;
pub type PidVsProcMap = HashMap<pidnr, Arc<Process>>;

// The body of a process: the goroutine equivalent that runs in its own thread.
// In Go this is the inner "func()" produced by an ETFunc.
pub type ProcFn = Box<dyn FnOnce() + Send + 'static>;

// Function type describing the signature of a function that is to be used when
// creating a new process. It receives the process context and process handle,
// and returns the actual process body. This two level shape allows per process
// setup before the main loop starts, just like the Go ETFunc.
pub type ETFunc = Box<dyn FnOnce(Context, Arc<Process>) -> ProcFn + Send + 'static>;

// Chan wraps both ends of a crossbeam channel. A Go "chan Event" is a single
// value that can be both sent to and received from; crossbeam splits this into
// a Sender and a Receiver, so we keep them together here. Cloning a Chan clones
// both ends, which still refer to the same underlying channel, mirroring how a
// Go channel value is copied by reference.
#[derive(Clone)]
pub struct Chan {
    pub tx: Sender<Event>,
    pub rx: Receiver<Event>,
}

impl Chan {
    // A buffered channel of the given capacity (Go: make(chan Event, cap)).
    pub fn bounded(cap: usize) -> Chan {
        let (tx, rx) = bounded(cap);
        Chan { tx, rx }
    }

    // An unbuffered, rendezvous channel (Go: make(chan Event)).
    pub fn unbuffered() -> Chan {
        let (tx, rx) = bounded(0);
        Chan { tx, rx }
    }
}

// Holds the relation between a pid and its process.
pub struct pidToProc {
    mp: Mutex<PidVsProcMap>,
}

impl pidToProc {
    fn new() -> pidToProc {
        pidToProc {
            mp: Mutex::new(HashMap::new()),
        }
    }

    // Add a pid and process to the map.
    fn add(&self, pid: pidnr, proc: Arc<Process>) {
        self.mp.lock().unwrap().insert(pid, proc);
    }

    // Delete a pid and process from the map.
    fn remove(&self, pid: pidnr) {
        self.mp.lock().unwrap().remove(&pid);
    }

    // Return a copy of the pid vs Process map.
    fn copyOfMap(&self) -> PidVsProcMap {
        self.mp.lock().unwrap().clone()
    }
}

// The main counter used for assigning Nr to events.
pub struct eventNr {
    nr: Mutex<i64>,
}

impl eventNr {
    // Create a new eventNr struct. This is normally used only when creating the
    // root process. Child processes inherit the eventNr from the root.
    fn new(start: i64) -> eventNr {
        eventNr {
            nr: Mutex::new(start),
        }
    }
}

// Holds the pid counter and the pid to process map.
pub struct pids {
    nr: Mutex<pidnr>,
    toProc: pidToProc,
}

impl pids {
    fn new() -> pids {
        pids {
            nr: Mutex::new(0),
            toProc: pidToProc::new(),
        }
    }

    // Return the next pidnr for a new process.
    fn next(&self) -> pidnr {
        let mut g = self.nr.lock().unwrap();
        let nr = *g;
        *g += 1;
        nr
    }
}

// Process defines a process.
pub struct Process {
    // The process body, set when the process is created and taken out and run
    // by Act/actForRoot. Go stores this in the "fn" field.
    fnCell: Mutex<Option<ProcFn>>,
    // Channel to signal that the process is ready. The sender is dropped by
    // SignalReady to "close" the channel; WaitForReady waits on the receiver.
    readyTx: Mutex<Option<Sender<()>>>,
    readyRx: Receiver<()>,
    // Ensures the ready channel is closed at most once.
    readyOnce: Once,

    // Channel to receive events into the process function.
    pub InCh: Chan,
    // Channel to send events to be picked up by other processes.
    pub StaticEventCh: Chan,
    // Channel to send error events.
    pub ErrorEventCh: Chan,
    // Channel for getting the result in tests.
    pub TestCh: Chan,
    // Channel to use for routing events for dynamic processes.
    pub DynamicEventCh: Chan,
    // Channel to use for routing events for custom processes.
    pub CustomEventCh: Chan,
    // Channel to use for routing supervisor events.
    pub SupervisorEventCh: Chan,

    // The event type for the process.
    pub Event: EventName,

    // Maps for the various process registries, shared with the whole tree.
    pub StaticProcesses: Arc<staticProcesses>,
    pub DynamicProcesses: Arc<dynamicProcesses>,
    pub CustomProcesses: Arc<customProcesses>,
    pub ErrorProcesses: Arc<errorProcesses>,
    pub supervisorProcesses: Arc<supervisorProcesses>,

    // The main counter used for assigning Nr to events.
    pub EventNr: Arc<eventNr>,
    // Is this the root process. Set on creation; kept for parity with the Go
    // struct, which also stores but does not currently read it.
    #[allow(dead_code)]
    isRoot: bool,
    // Holding all configuration settings.
    pub Config: Arc<Config>,
    // Process ID allocator and registry, shared with the whole tree.
    pids: Arc<pids>,
    // PID of the process.
    pub PID: pidnr,

    // The context of the process.
    pub Ctx: Context,
    // Cancel func for the context of the process.
    pub Cancel: CancelFn,
}

impl Process {
    // Add a new Event and its process to the processes map. Checks if a process
    // for the same event is defined, and if so cancels the current process
    // before replacing it with a new one.
    fn addToProcessesMap(self: &Arc<Self>) {
        let s = self.Event.as_str().as_bytes();
        match s[1] {
            b'T' => {
                let mut m = self.StaticProcesses.procMap.lock().unwrap();
                if let Some(old) = m.get(&self.Event) {
                    (old.Cancel)();
                }
                m.insert(self.Event.clone(), self.clone());
            }
            b'D' => {
                let mut m = self.DynamicProcesses.procMap.lock().unwrap();
                if let Some(old) = m.get(&self.Event) {
                    (old.Cancel)();
                }
                m.insert(self.Event.clone(), self.clone());
            }
            b'C' => {
                let mut m = self.CustomProcesses.procMap.lock().unwrap();
                if let Some(old) = m.get(&self.Event) {
                    (old.Cancel)();
                }
                m.insert(self.Event.clone(), self.clone());
            }
            b'R' => {
                // NB: The Go errorProcesses map intentionally has no mutex. Rust
                // requires synchronized access to share the map across threads,
                // so we use a Mutex here, but the behaviour is otherwise the
                // same.
                let mut m = self.ErrorProcesses.procMap.lock().unwrap();
                if let Some(old) = m.get(&self.Event) {
                    (old.Cancel)();
                }
                m.insert(self.Event.clone(), self.clone());
            }
            b'S' => {
                let mut m = self.supervisorProcesses.procMap.lock().unwrap();
                if let Some(old) = m.get(&self.Event) {
                    (old.Cancel)();
                }
                m.insert(self.Event.clone(), self.clone());
            }
            _ => {}
        }
    }

    // Delete an Event and its process from the processes map. Only dynamic and
    // custom processes can be deleted; static, error and supervisor processes
    // cannot, mirroring the empty switch cases in the Go code.
    fn deleteFromProcessesMap(&self) {
        let s = self.Event.as_str().as_bytes();
        match s[1] {
            b'T' => {
                // Not allowed to delete static process.
            }
            b'D' => {
                self.DynamicProcesses.procMap.lock().unwrap().remove(&self.Event);
            }
            b'C' => {
                self.CustomProcesses.procMap.lock().unwrap().remove(&self.Event);
            }
            b'R' => {
                // Not allowed to delete error process.
            }
            b'S' => {
                // Not allowed to delete supervisor process.
            }
            _ => {}
        }
    }

    // AddEvent will deliver the event to the correct router based on the second
    // character of the Event Name. If the event is to be delivered to a remote
    // node, AddEvent wraps it in an ETRemote event and ships it off via the
    // static event channel.
    pub fn AddEvent(&self, mut event: Event) {
        let eventnr = self.IncrementEventNr();
        event.Nr = eventnr;

        if !event.DstNode.is_empty() && event.DstNode != self.Config.NodeName {
            let remote = Event {
                Name: ETRemote,
                Nr: eventnr,
                NextEvent: Some(Box::new(event)),
                ..Default::default()
            };
            self.addEventStatic(remote);
            return;
        }

        let s = event.Name.as_str();
        if s.len() < 2 {
            panic!("unknown event.Name: {}", event.Name);
        }
        let bytes = s.as_bytes();
        if bytes[0] != b'E' {
            panic!("unknown event.Name, should start with E: {}", event.Name);
        }
        match bytes[1] {
            b'T' => self.addEventStatic(event),
            b'R' => self.addEventError(event),
            b'D' => self.addEventDynamic(event),
            b'C' => self.addEventCustom(event),
            b'S' => self.addEventSuperVisor(event),
            _ => panic!(
                "unknown event.Name, got default case, should start with E: {}",
                event.Name
            ),
        }
    }

    fn addEventSuperVisor(&self, event: Event) {
        let _ = self.SupervisorEventCh.tx.send(event);
    }

    // Adds a static event, with a 5 second timeout so a stuck downstream
    // process does not block forever, matching the Go select on a timer.
    fn addEventStatic(&self, event: Event) {
        select! {
            send(self.StaticEventCh.tx, event) -> _res => {}
            recv(after(Duration::from_secs(5))) -> _ => {
                crate::log_error!(
                    "addEventStatic TIMEOUT: one of the later AddEvent commands probably is not working well, check the debug output"
                );
            }
        }
    }

    fn addEventDynamic(&self, event: Event) {
        let _ = self.DynamicEventCh.tx.send(event);
    }

    fn addEventCustom(&self, event: Event) {
        let _ = self.CustomEventCh.tx.send(event);
    }

    fn addEventError(&self, event: Event) {
        let _ = self.ErrorEventCh.tx.send(event);
    }

    // Act will start the process body attached to the process. If no body is
    // defined the function just returns; the process can still be communicated
    // with via its channels.
    pub fn Act(self: &Arc<Self>) -> Result<(), String> {
        if self.Config.LogLevel != "none" {
            println!(
                "on node {}: Starting actor for Name: {}",
                self.Config.NodeName, self.Event
            );
        }

        self.pids.toProc.add(self.PID, self.clone());
        self.addToProcessesMap();

        let body = self.fnCell.lock().unwrap().take();
        if let Some(body) = body {
            // The ready channel is created when the process is created. It is
            // closed either by the user inside the body via SignalReady, or by
            // the timeout in WaitForReady if the user forgot to call it.
            std::thread::spawn(move || body());
            self.WaitForReady();
        }

        Ok(())
    }

    // actForRoot is similar to Act, but used for starting the root's built in
    // processes. It skips the readiness mechanism, because the routers and
    // error processes need to be available before other processes can register.
    fn actForRoot(self: &Arc<Self>, pi: &mut Vec<esProcessesMapDataIn>) {
        if self.Config.LogLevel != "none" {
            println!(
                "on node {}: ROOT ACTOR: Starting actor for Name: {}",
                self.Config.NodeName, self.Event
            );
        }

        self.pids.toProc.add(self.PID, self.clone());
        self.addToProcessesMap();

        let body = self.fnCell.lock().unwrap().take();
        if let Some(body) = body {
            std::thread::spawn(move || body());
        }

        pi.push(esProcessesMapDataIn {
            Name: self.Event.clone(),
        });
    }

    // Stop cancels the context attached to the process, removes the process
    // from the processes map, and removes the pid from the pids map.
    pub fn Stop(&self) {
        (self.Cancel)();
        self.deleteFromProcessesMap();
        self.pids.toProc.remove(self.PID);
    }

    // Signal that the process body is ready.
    pub fn SignalReady(&self) {
        // Close the ready channel exactly once by dropping the sender. Multiple
        // calls are no-ops.
        self.readyOnce.call_once(|| {
            *self.readyTx.lock().unwrap() = None;
        });
    }

    // Wait for the process body to be ready. If the user forgot to call
    // SignalReady, the 5ms timeout fires and we assume the process is ready.
    pub fn WaitForReady(&self) {
        select! {
            recv(self.readyRx) -> _ => {}
            recv(after(Duration::from_millis(5))) -> _ => {}
        }
    }

    // IncrementEventNr atomically increments the shared event counter and
    // returns the new value.
    pub fn IncrementEventNr(&self) -> i64 {
        let mut g = self.EventNr.nr.lock().unwrap();
        *g += 1;
        *g
    }

    // CurrentEventNr returns the current value of the shared event counter
    // without modifying it.
    pub fn CurrentEventNr(&self) -> i64 {
        *self.EventNr.nr.lock().unwrap()
    }

    // Returns a snapshot of the running pids and the event name of each
    // process. Used by the pid related processes.
    pub fn pidSnapshot(&self) -> HashMap<pidnr, String> {
        let m = self.pids.toProc.copyOfMap();
        let mut out = HashMap::new();
        for (pid, proc) in m.iter() {
            out.insert(*pid, proc.Event.to_string());
        }
        out
    }
}

// Builds a Process struct ready to be wrapped in an Arc. Used by both
// NewRootProcess and NewProcess.
#[allow(clippy::too_many_arguments)]
fn newProcessStruct(
    event: EventName,
    in_ch: Chan,
    static_ch: Chan,
    error_ch: Chan,
    test_ch: Chan,
    dynamic_ch: Chan,
    custom_ch: Chan,
    supervisor_ch: Chan,
    static_p: Arc<staticProcesses>,
    dynamic_p: Arc<dynamicProcesses>,
    custom_p: Arc<customProcesses>,
    error_p: Arc<errorProcesses>,
    supervisor_p: Arc<supervisorProcesses>,
    event_nr: Arc<eventNr>,
    is_root: bool,
    config: Arc<Config>,
    pids_: Arc<pids>,
    pid: pidnr,
    ctx: Context,
    cancel: CancelFn,
) -> Process {
    let (rtx, rrx) = bounded::<()>(0);
    Process {
        fnCell: Mutex::new(None),
        readyTx: Mutex::new(Some(rtx)),
        readyRx: rrx,
        readyOnce: Once::new(),
        InCh: in_ch,
        StaticEventCh: static_ch,
        ErrorEventCh: error_ch,
        TestCh: test_ch,
        DynamicEventCh: dynamic_ch,
        CustomEventCh: custom_ch,
        SupervisorEventCh: supervisor_ch,
        Event: event,
        StaticProcesses: static_p,
        DynamicProcesses: dynamic_p,
        CustomProcesses: custom_p,
        ErrorProcesses: error_p,
        supervisorProcesses: supervisor_p,
        EventNr: event_nr,
        isRoot: is_root,
        Config: config,
        pids: pids_,
        PID: pid,
        Ctx: ctx,
        Cancel: cancel,
    }
}

// Used for storing information about the processes to be registered in
// ESProcesses when starting the root process.
type registerProcessInfo = Vec<esProcessesMapDataIn>;

// NewRootProcess prepares and returns the root process, which holds all the
// core elements needed: the main channels for events and errors, and the
// registries holding information about the system. Later created processes
// reference these elements.
//
// The root process also starts up all the essential other processes, like the
// event routers and the standard error handling processes.
//
// The fn parameter is optional; if None, the root has no process body.
pub fn NewRootProcess(ctx: &Context, fnc: Option<ETFunc>, conf: Config) -> Arc<Process> {
    // Initialize logging based on the configured log level. The environment
    // variable LOGLEVEL always wins to allow global overrides.
    let level = std::env::var("LOGLEVEL").unwrap_or_else(|_| conf.LogLevel.clone());
    crate::logging::set_level_str(&level);

    let (cctx, cancel) = Context::with_cancel(ctx);

    let pids_ = Arc::new(pids::new());
    // The Go code sets root.PID to pids.nr (0) without consuming a pid, so the
    // first child created later also receives pid 0. We reproduce that here.
    let root_pid: pidnr = 0;

    let p = newProcessStruct(
        ETRoot,
        Chan::bounded(10),
        Chan::bounded(10),
        Chan::bounded(10),
        Chan::bounded(10),
        Chan::bounded(10),
        Chan::bounded(10),
        Chan::bounded(10),
        Arc::new(newStaticProcesses()),
        Arc::new(newDynamicProcesses()),
        Arc::new(newCustomProcesses()),
        Arc::new(newErrorProcesses()),
        Arc::new(newsuperVisorProcesses()),
        Arc::new(eventNr::new(0)),
        true,
        Arc::new(conf),
        pids_,
        root_pid,
        cctx,
        cancel,
    );

    let root = Arc::new(p);

    if let Some(f) = fnc {
        let body = f(root.Ctx.clone(), root.clone());
        *root.fnCell.lock().unwrap() = Some(body);
    }

    // Register and start all the standard child processes of root.
    //
    // When starting Root, all the processes needed to handle the system are not
    // yet started. For example both the ESRouter and ESProcesses, and the error
    // processes, are needed before other processes can register in ESProcesses.
    // We store the information about each process started in a slice, so we can
    // register all of them in ESProcesses once everything is up.
    let mut pi: registerProcessInfo = Vec::new();

    // Starting error handling processes.
    NewProcess(&root.Ctx, &root, crate::errorprocesses::ERLog, Box::new(crate::errorprocesses::erLogFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::errorprocesses::ERTest, Box::new(crate::errorprocesses::erTestFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::errorprocesses::ERNone, Box::new(crate::errorprocesses::erNoneFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::staticprocesses::ETPrint, Box::new(crate::staticprocesses::etPrintFn))
        .actForRoot(&mut pi);

    // Starting the routers.
    NewProcess(&root.Ctx, &root, crate::staticprocesses::ETRouter, Box::new(crate::staticprocesses::etRouterFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::errorprocesses::ERRouter, Box::new(crate::errorprocesses::erRouterFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::dynamicprocesses::EDRouter, Box::new(crate::dynamicprocesses::edRouterFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::customprocesses::ECRouter, Box::new(crate::customprocesses::ecRouterFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::supervisorprocesses::ESRouter, Box::new(crate::supervisorprocesses::esRouterFn))
        .actForRoot(&mut pi);

    // Starting the supervisor process.
    NewProcess(&root.Ctx, &root, crate::supervisorprocesses::ESProcesses, crate::supervisorprocesses::esProcessesFn())
        .actForRoot(&mut pi);

    // Starting the remaining processes.
    NewProcess(&root.Ctx, &root, crate::staticprocesses::ETOsSignal, Box::new(crate::staticprocesses::etOsSignalFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::staticprocesses::ETTestCh, Box::new(crate::staticprocesses::etTestChFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::staticprocesses::ETPid, Box::new(crate::staticprocesses::etPidFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::staticprocesses::ETReadFile, Box::new(crate::staticprocesses::ETReadFileFn))
        .actForRoot(&mut pi);

    NewProcess(&root.Ctx, &root, crate::staticprocesses::ETDone, Box::new(crate::staticprocesses::etDoneFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::staticprocesses::ETExit, Box::new(crate::staticprocesses::etExitFn))
        .actForRoot(&mut pi);
    NewProcess(&root.Ctx, &root, crate::staticprocesses::ETPidGetAll, Box::new(crate::staticprocesses::etPidGetAllFn))
        .actForRoot(&mut pi);

    RegisterProcessesInESProcesses(&root, &pi);

    root
}

// Register all the processes in ESProcesses. Currently a no-op, mirroring the
// Go code where the body is commented out. The infrastructure (collecting the
// process info in actForRoot) is in place but not active.
pub fn RegisterProcessesInESProcesses(_p: &Arc<Process>, _pi: &registerProcessInfo) {}

// NewProcess prepares and returns a Process. It copies the channels and map
// references from the parent process. Only InCh is unique per process.
//
// NewProcess does not start the process; call Act separately.
pub fn NewProcess(
    ctx: &Context,
    parentP: &Arc<Process>,
    event: EventName,
    fnc: ETFunc,
) -> Arc<Process> {
    let (cctx, cancel) = Context::with_cancel(ctx);
    let pid = parentP.pids.next();

    let p = newProcessStruct(
        event,
        Chan::unbuffered(),
        parentP.StaticEventCh.clone(),
        parentP.ErrorEventCh.clone(),
        parentP.TestCh.clone(),
        parentP.DynamicEventCh.clone(),
        parentP.CustomEventCh.clone(),
        parentP.SupervisorEventCh.clone(),
        parentP.StaticProcesses.clone(),
        parentP.DynamicProcesses.clone(),
        parentP.CustomProcesses.clone(),
        parentP.ErrorProcesses.clone(),
        parentP.supervisorProcesses.clone(),
        parentP.EventNr.clone(),
        false,
        parentP.Config.clone(),
        parentP.pids.clone(),
        pid,
        cctx,
        cancel,
    );

    let arc = Arc::new(p);

    // Build the process body, giving it its own context and a handle to itself.
    let body = fnc(arc.Ctx.clone(), arc.clone());
    *arc.fnCell.lock().unwrap() = Some(body);

    arc
}
