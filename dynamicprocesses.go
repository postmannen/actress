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
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Holds information about what process functions who belongs to what
// event, and also a map of the started processes.
type dynProcesses struct {
	procMap map[EventType]*Process
	mu      sync.Mutex
}

// Add a new Event and it's process to the processes map.
func (p *dynProcesses) Add(et EventType, proc *Process) {
	// Check if a process for the same event is defined, and if so we
	// cancel the current process before we replace it with a new one.
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.procMap[et]; ok {
		p.procMap[et].Cancel()
	}
	p.procMap[et] = proc
}

// Delete an Event and it's process from the processes map.
func (p *dynProcesses) Delete(et EventType) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.procMap[et].Cancel()
	delete(p.procMap, et)
	log.Printf("deleted process %v\n", et)
}

// Checks if the event is defined in the processes map, and returns true if it is.
func (p *dynProcesses) IsEventDefined(ev EventType) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.procMap[ev]; !ok {
		return false
	}

	return true
}

// Prepare and return a new *dynProcesses structure.
func newDynProcesses() *dynProcesses {
	p := dynProcesses{
		procMap: make(map[EventType]*Process),
	}
	return &p
}

// Will create and return a new UUID.
func NewUUID() string {
	u := uuid.New()
	return u.String()
}

// NewDynProcess will prepare and return a *Process. It will copy
// channels and map structures from the root process.
// The purpose of dynamic processes is to have short lived processes
// that can be quickly started, and removed again when it's job is done.
// The only difference between a process and a dynamic process are that
// the dynamic processes have a mutex in processes map DynProcesses so
// we also can delete the processes when they are no longer needed.

func NewDynProcess(ctx context.Context, parentP Process, event EventType, fn ETFunc) *Process {
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

	p.DynProcesses.Add(event, &p)

	if fn != nil {
		p.fn = fn(ctx, &p)
	}
	return &p
}

// ------------------------------------------------------

// Router for normal events.
const EDRouter EventType = "EDRouter"

// Process function for routing and handling events. Will check
// and route the event to the correct process.
func edRouterFn(ctx context.Context, p *Process) func() {
	fn := func() {
		eventNr := 0

		for {
			select {
			case e := <-p.DynCh:
				eventNr++
				e.Nr = eventNr

				// Dynamic processes can take a little longer to start up and be
				// registered in the map. We check here if process is registred,
				// and if it is not we retry.
				if _, ok := p.DynProcesses.procMap[e.EventType]; !ok {
					go func(ev Event) {
						// Try to 3 times to deliver the message.
						for i := 0; i < 3; i++ {
							_, ok := p.DynProcesses.procMap[e.EventType]

							if !ok {
								p.AddError(Event{EventType: ERLog, Err: fmt.Errorf("found no process registered for the event type : %v", ev.EventType)})
								time.Sleep(time.Millisecond * 1000)
								continue
							}

							// Process is now registred, so we can safely put
							//the event on the InCh of the process.
							p.DynProcesses.procMap[e.EventType].InCh <- e

							return
						}

					}(e)
					continue
				}

				// Process was registered. Deliver the event to the process InCh.
				p.DynProcesses.procMap[e.EventType].InCh <- e

			case <-ctx.Done():
				p.AddError(Event{
					EventType: ERLog,
					Err:       fmt.Errorf("info: got ctx.Done"),
				})

				return
			}
		}
	}

	return fn
}
