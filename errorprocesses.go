package actress

import (
	"context"
)

// processes holds information about what process functions
// who belongs to what event, and also a map of the started
// processes.
type errProcesses struct {
	procMap map[EventType]*Process
}

// Add a new Event and it's process to the processes map.
func (p *errProcesses) add(et EventType, proc *Process) {
	// Check if a process for the same event is defined, and if so we
	// cancel the current process before we replace it with a new one.
	if _, ok := p.procMap[et]; ok {
		p.procMap[et].cancel()
	}
	p.procMap[et] = proc
}

// // Delete an Event and it's process from the processes map.
// func (p *processes) delete(et EventType, proc *Process) {
// 	p.mu.Lock()
// 	defer p.mu.Unlock()
// 	p.procMap[et].cancel()
// 	delete(p.procMap, et)
// }

// Checks if the event is defined in the processes map, and returns true if it is.
func (p *errProcesses) IsEventDefined(ev EventType) bool {
	if _, ok := p.procMap[ev]; !ok {
		return false
	}

	return true
}

// Prepare and return a new *processes structure.
func newErrProcesses() *errProcesses {
	p := errProcesses{
		procMap: make(map[EventType]*Process),
	}
	return &p
}

// NewErrProcess will prepare and return a *Process. It will copy
// channels and map structures from the root process.
func NewErrProcess(ctx context.Context, parentP Process, event EventType, fn ETFunc) *Process {
	ctx, cancel := context.WithCancel(ctx)
	p := Process{
		fn:              nil,
		InCh:            make(chan Event),
		EventCh:         parentP.EventCh,
		ErrorCh:         parentP.ErrorCh,
		CustomCh:        parentP.CustomCh,
		TestCh:          parentP.TestCh,
		Event:           event,
		Processes:       parentP.Processes,
		ErrProcesses:    parentP.ErrProcesses,
		CustomProcesses: parentP.CustomProcesses,
		isRoot:          false,
		Config:          parentP.Config,
		pids:            parentP.pids,
		PID:             parentP.pids.next(),
		cancel:          cancel,
	}

	p.ErrProcesses.add(event, &p)

	if fn != nil {
		p.fn = fn(ctx, &p)
	}
	return &p
}
