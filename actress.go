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

type pid struct {
	mu   sync.Mutex
	next int
}

func newPid() *pid {
	p := pid{
		next: 0,
	}

	return &p
}

func (p *pid) GetNext() int {
	p.mu.Lock()
	nr := p.next
	p.next++
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
	pid *pid
	// PID of the process
	PID int
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

	a := Process{
		fn:        nil,
		InCh:      make(chan Event),
		EventCh:   make(chan Event),
		ErrorCh:   make(chan Event),
		Event:     ETRoot,
		Processes: newProcesses(),
		isRoot:    true,
		Config:    conf,
		pid:       newPid(),
	}

	a.PID = a.pid.next

	// Register all the standard child processes that should
	// spawn off the root process
	if a.Config.Profiling {
		NewProcess(ctx, a, ETProfiling, procProfilingFunc).Act()
	}

	NewProcess(ctx, a, ETRouter, procRouterFunc).Act()

	NewProcess(ctx, a, ETOsSignal, procOsSignalFunc).Act()
	NewProcess(ctx, a, ETDone, procDoneFunc).Act()
	NewProcess(ctx, a, ETPrint, procPrintFunc).Act()
	NewProcess(ctx, a, ETExit, procExitFunc).Act()

	NewProcess(ctx, a, ERRouter, procErrorRouterFunc).Act()
	NewProcess(ctx, a, ERLog, procErrorLogFunc).Act()
	NewProcess(ctx, a, ERDebug, procDebugLogFunc).Act()
	NewProcess(ctx, a, ERFatal, procFatalLogFunc).Act()

	return &a
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
		pid:       parentP.pid,
		PID:       parentP.pid.GetNext(),
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
