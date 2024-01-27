package actress

import (
	"context"
	"sync"
)

// processes holds information about what process functions
// who belongs to what event, and also a map of the started
// processes.
type processes struct {
	procMap map[EventType]*Process
	mu      sync.Mutex
}

func newProcesses() *processes {
	p := processes{
		procMap: make(map[EventType]*Process),
	}
	return &p
}

type pidnr int
type PidVsProcMap map[pidnr]*Process

// Holds information about the relation between a pid and it's process.
type pidToProc struct {
	mu sync.Mutex
	mp PidVsProcMap
}

// Add a pid and process to the map.
func (p *pidToProc) add(pid pidnr, proc *Process) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mp[pid] = proc
}

// Get the *Process based on the pid.
func (p *pidToProc) getProc(pid pidnr) *Process {
	p.mu.Lock()
	defer p.mu.Unlock()
	proc, ok := p.mp[pid]
	if !ok {
		return nil
	}
	return proc
}

// Return a copy of the pid vs *Processes map.
func (p *pidToProc) copyOfMap() *PidVsProcMap {
	m := make(PidVsProcMap)

	p.mu.Lock()
	defer p.mu.Unlock()

	for pid, proc := range p.mp {
		m[pid] = proc
	}

	return &m
}

func newPidToProc() *pidToProc {
	p := pidToProc{
		mp: make(PidVsProcMap),
	}
	return &p
}

type pids struct {
	mu     sync.Mutex
	nr     pidnr
	toProc pidToProc
}

func newPids() *pids {
	p := pids{
		nr:     0,
		toProc: *newPidToProc(),
	}

	return &p
}

func (p *pids) next() pidnr {
	p.mu.Lock()
	nr := p.nr
	p.nr++
	p.mu.Unlock()

	return nr
}

// Process the essential parts of an process.
type Process struct {
	// Process function.
	fn func()
	// Channel to receive events into the process function.
	InCh chan Event `json:"-"`
	// Channel to send events to be picked up by other processes.
	EventCh chan Event `json:"-"`
	// Channel to send error events.
	ErrorCh chan Event `json:"-"`
	// Channel for getting the result in tests.
	TestCh chan Event `json:"-"`
	// The event type for the process.
	Event EventType
	// Maps for various process information.
	Processes *processes
	// Is this the root process.
	isRoot bool
	// Holding all configuration settings.
	Config *Config
	// process ID struct
	pids *pids
	// PID of the process
	PID pidnr
	// Cancel func
	cancel context.CancelFunc
}

// Checks if the event is defined in the map, and returns true if it is.
func (p *Process) IsEventDefined(ev EventType) bool {
	p.Processes.mu.Lock()
	defer p.Processes.mu.Unlock()
	if _, ok := p.Processes.procMap[ev]; !ok {
		return false
	}

	return true
}

// NewRootProcess will prepare and return the root process
// which holds all the core elements needed, like the main
// channels for events and errors, and varouis registers or
// maps holding information about the system. Later created
// processes will reference these elements when they are
// created.
// The root process will also start up all the essential other
// processes needed, like the event router, and various standard
// error handling processes.
func NewRootProcess(ctx context.Context) *Process {
	ctx, cancel := context.WithCancel(ctx)
	conf := NewConfig()

	p := Process{
		fn:        nil,
		InCh:      make(chan Event),
		EventCh:   make(chan Event),
		ErrorCh:   make(chan Event),
		TestCh:    make(chan Event, 1),
		Event:     ETRoot,
		Processes: newProcesses(),
		isRoot:    true,
		Config:    conf,
		pids:      newPids(),
		cancel:    cancel,
	}

	p.PID = p.pids.nr

	// Register all the standard child processes that should
	// spawn off the root process
	if p.Config.Profiling {
		NewProcess(ctx, p, ETProfiling, etProfilingFn).Act()
	}

	NewProcess(ctx, p, ETRouter, etRouterFn).Act()
	NewProcess(ctx, p, ETOsSignal, etOsSignalFn).Act()
	NewProcess(ctx, p, ETTestCh, etTestChFn).Act()
	NewProcess(ctx, p, ETPid, etPidFn).Act()
	NewProcess(ctx, p, ETWatchEventFile, wrapperETWatchEventFileFn("tmp", ".json")).Act()
	NewProcess(ctx, p, ETReadFile, ETReadFileFn).Act()
	NewProcess(ctx, p, ETCustomEvent, ETCustomEventFn).Act()

	NewProcess(ctx, p, ETDone, etDoneFn).Act()
	NewProcess(ctx, p, ETPrint, etPrintFn).Act()
	NewProcess(ctx, p, ETExit, etExitFn).Act()
	NewProcess(ctx, p, ETPidGetAll, etPidGetAllFn).Act()

	NewProcess(ctx, p, ERRouter, erRouterFn).Act()
	NewProcess(ctx, p, ERLog, erLogFn).Act()
	NewProcess(ctx, p, ERDebug, erDebugFn).Act()
	NewProcess(ctx, p, ERFatal, erFatalFn).Act()

	return &p
}

// NewProcess will prepare and return a *Process. It will copy
// channels and map structures from the root process.
func NewProcess(ctx context.Context, parentP Process, event EventType, fn ETFunc) *Process {
	ctx, cancel := context.WithCancel(ctx)
	p := Process{
		fn:        nil,
		InCh:      make(chan Event),
		EventCh:   parentP.EventCh,
		ErrorCh:   parentP.ErrorCh,
		TestCh:    parentP.TestCh,
		Event:     event,
		Processes: parentP.Processes,
		isRoot:    false,
		Config:    parentP.Config,
		pids:      parentP.pids,
		PID:       parentP.pids.next(),
		cancel:    cancel,
	}

	// Register the InCh of the process in the inchMap so the root
	// process router func are able to route the Events to the
	// correct process
	p.Processes.mu.Lock()
	p.Processes.procMap[event] = &p
	p.Processes.mu.Unlock()

	if fn != nil {
		p.fn = fn(ctx, &p)
	}
	return &p
}

// Will add an event to be handled by the processes.
func (p *Process) AddEvent(event Event) {
	p.EventCh <- event
}

// Will add an error to be handled by the error processes.
func (p *Process) AddError(event Event) {
	p.ErrorCh <- event
}

// Will start the current process.
func (p *Process) Act() error {
	if p.fn == nil {
		//go p.fn()
		return nil
	}

	p.pids.toProc.add(p.PID, p)

	go p.fn()

	return nil
}
