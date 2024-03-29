// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

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
		p.procMap[et].Cancel()
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
		fn:           nil,
		InCh:         make(chan Event, 1),
		EventCh:      parentP.EventCh,
		ErrorCh:      parentP.ErrorCh,
		TestCh:       parentP.TestCh,
		DynCh:        parentP.DynCh,
		Event:        event,
		Processes:    parentP.Processes,
		DynProcesses: parentP.DynProcesses,
		ErrProcesses: parentP.ErrProcesses,
		isRoot:       false,
		Config:       parentP.Config,
		pids:         parentP.pids,
		PID:          parentP.pids.next(),
		Cancel:       cancel,
	}

	p.ErrProcesses.add(event, &p)

	if fn != nil {
		p.fn = fn(ctx, &p)
	}
	return &p
}
