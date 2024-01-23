package actress

import (
	"context"
	"fmt"
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
}

// Checks if the event is defined in the map, and returns true if it is.
func (p *Process) IsEventDefined(ev EventType) bool {
	if _, ok := p.Processes.inChMap[ev]; !ok {
		return false
	}

	return true
}

func NewActress(ctx context.Context) *Process {
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
	}

	//a.Processes.inChMap[Event] = a.InCh
	a.fn = procProcessesStartFunc(ctx, &a)

	// Register all the standard child processes that should
	// spawn off the root process
	if a.Config.Profiling {
		a.RegisterProcess(ETProfiling, procProfilingFunc)
	}
	a.RegisterProcess(ETRouter, procRouterFunc)
	a.RegisterProcess(ETOsSignal, procOsSignalFunc)
	a.RegisterProcess(ETDone, procDoneFunc)
	a.RegisterProcess(ETPrint, procPrintFunc)
	a.RegisterProcess(ETExit, procExitFunc)

	a.RegisterProcess(ERRouter, procErrorRouterFunc)
	a.RegisterProcess(ERLog, procErrorLogFunc)
	a.RegisterProcess(ERDebug, procDebugLogFunc)
	a.RegisterProcess(ERFatal, procFatalLogFunc)

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
	}

	p.Processes.inChMap[event] = p.InCh

	if fn != nil {
		p.fn = fn(ctx, &p)
	}
	return &p
}

// Register a new event type and it's process function.
func (p *Process) RegisterProcess(et EventType, ep func(context.Context, *Process) func()) {
	p.Processes.pFuncMap[et] = ep
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
		return fmt.Errorf("error: process '%v' are missing an process function", p.Event)
	}
	go p.fn()

	return nil
}
