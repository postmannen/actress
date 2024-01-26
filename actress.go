package actress

import (
	"context"
	"sync"
)

// processes holds information about what process functions
// who belongs to what event, and also a map of the started
// processes.
type processes struct {
	inChMap  map[EventType]chan Event
	pFuncMap map[EventType]pFunc
	// mu   sync.Mutex
}

func newProcesses() *processes {
	p := processes{
		inChMap:  make(map[EventType]chan Event),
		pFuncMap: make(map[EventType]pFunc),
	}
	return &p
}

type pidnr int
type pidVsProcMap map[pidnr]*Process

// Holds information about the relation between a pid and it's process.
type pidToProc struct {
	mu sync.Mutex
	mp pidVsProcMap
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
func (p *pidToProc) copyOfMap() *pidVsProcMap {
	m := make(pidVsProcMap)

	p.mu.Lock()
	defer p.mu.Unlock()

	for pid, proc := range p.mp {
		m[pid] = proc
	}

	return &m
}

func newPidToProc() *pidToProc {
	p := pidToProc{
		mp: make(pidVsProcMap),
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
	InCh chan Event
	// Channel to send events to be picked up by other processes.
	EventCh chan Event
	// Channel to send error events.
	ErrorCh chan Event
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
}

// Checks if the event is defined in the map, and returns true if it is.
func (p *Process) IsEventDefined(ev EventType) bool {
	if _, ok := p.Processes.inChMap[ev]; !ok {
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
	conf := NewConfig()

	p := Process{
		fn:        nil,
		InCh:      make(chan Event),
		EventCh:   make(chan Event),
		ErrorCh:   make(chan Event),
		Event:     ETRoot,
		Processes: newProcesses(),
		isRoot:    true,
		Config:    conf,
		pids:      newPids(),
	}

	p.PID = p.pids.nr

	// Register all the standard child processes that should
	// spawn off the root process
	if p.Config.Profiling {
		NewProcess(ctx, p, ETProfiling, procProfilingFunc).Act()
	}

	NewProcess(ctx, p, ETRouter, procRouterFunc).Act()
	NewProcess(ctx, p, ETOsSignal, procOsSignalFunc).Act()
	NewProcess(ctx, p, ETPid, procPidFunc).Act()

	NewProcess(ctx, p, ETDone, procDoneFunc).Act()
	NewProcess(ctx, p, ETPrint, procPrintFunc).Act()
	NewProcess(ctx, p, ETExit, procExitFunc).Act()

	NewProcess(ctx, p, ERRouter, procErrorRouterFunc).Act()
	NewProcess(ctx, p, ERLog, procErrorLogFunc).Act()
	NewProcess(ctx, p, ERDebug, procDebugLogFunc).Act()
	NewProcess(ctx, p, ERFatal, procFatalLogFunc).Act()

	return &p
}

// newProcess will prepare and return a *Process. If empty parentP process
// and empty event EventType are provided as input it indicates that this
// is the first process to be created, and a root process are returned. A
// root process is the first process to create. When new child processes
// are created later they should reference the root process by providing it
// as the parentP argument.
func NewProcess(ctx context.Context, parentP Process, event EventType, fn pFunc) *Process {
	p := Process{
		fn:        nil,
		InCh:      make(chan Event),
		EventCh:   parentP.EventCh,
		ErrorCh:   parentP.ErrorCh,
		Event:     event,
		Processes: parentP.Processes,
		isRoot:    false,
		pids:      parentP.pids,
		PID:       parentP.pids.next(),
	}

	// Register the InCh of the process in the inchMap so the root
	// process router func are able to route the Events to the
	// correct process
	p.Processes.inChMap[event] = p.InCh

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

	go p.fn()

	return nil
}
