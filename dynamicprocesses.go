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
type dynamicProcesses struct {
	procMap map[EventType]*Process
	mu      sync.Mutex
}

// Add a new Event and it's process to the processes map.
//
// *******************************************************************
// TODO: Consider if we still need this function. It is not in use.
// *******************************************************************
func (p *dynamicProcesses) Add(et EventType, proc *Process) {
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
func (p *dynamicProcesses) Delete(et EventType) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// p.procMap[et].Cancel()
	delete(p.procMap, et)
	log.Printf("deleted process %v\n", et)
}

// Checks if the event is defined in the processes map, and returns true if it is.
func (p *dynamicProcesses) IsEventDefined(ev EventType) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.procMap[ev]; !ok {
		return false
	}

	return true
}

// Prepare and return a new *dynamicProcesses structure.
func newDynamicProcesses() *dynamicProcesses {
	p := dynamicProcesses{
		procMap: make(map[EventType]*Process),
	}
	return &p
}

// Will create and return a new UUID.
func NewUUID() string {
	u := uuid.New()
	return u.String()
}

// ------------------------------------------------------------------------------
// Events and event functions.
// ------------------------------------------------------------------------------

// Router for normal events.
const EDRouter EventType = "EDRouter"

// Process function for routing and handling events. Will check
// and route the event to the correct process.
func edRouterFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

		eventNr := 0

		for {
			select {
			case ev := <-p.DynamicEventCh:
				// If there is a next event defined, we make a copy of all the fields  of the current event,
				// and put that as the previousEvent on the next event. We can use this information later
				// if need to check something in the previous event.
				if ev.NextEvent != nil {
					// Keep the information about the current event, so we are able to check for things
					// like ackTimeout and what node to reply back to if ack should be given.
					ev.NextEvent.PreviousEvent = CopyEventFields(ev)
				}

				eventNr++
				ev.Nr = eventNr

				// Dynamic processes can take a little longer to start up and be
				// registered in the map. We check here if process is registred,
				// and if it is not we retry.
				// The checking is done in a go routine so the router don't block
				// here waiting, and we continue with the next event in the queue.
				p.DynamicProcesses.mu.Lock()
				_, ok := p.DynamicProcesses.procMap[ev.EventType]
				p.DynamicProcesses.mu.Unlock()
				if !ok {
					go func(ev Event) {
						// Try to 3 times to deliver the message.
						for i := 0; i < 3; i++ {
							p.DynamicProcesses.mu.Lock()
							_, ok := p.DynamicProcesses.procMap[ev.EventType]
							p.DynamicProcesses.mu.Unlock()

							if !ok {
								p.AddEvent(Event{EventType: ERLog,
									EventKind: EventKindError,
									Err:       fmt.Errorf("edRouter: on %v found no process registered for the event type : %v, ev.DstNode: %v", p.Config.NodeName, ev.EventType, ev.DstNode)})
								time.Sleep(time.Millisecond * 250)
								continue
							}

							// Process is now registred, so we can safely put
							//the event on the InCh of the process.
							p.DynamicProcesses.mu.Lock()
							p.DynamicProcesses.procMap[ev.EventType].InCh <- ev
							p.DynamicProcesses.mu.Unlock()

							return
						}

					}(ev)

					// The above go routine will wait, and check if the process becomes
					// available, and send the event if it the process eventually found,
					// so we can continue with the next event in the queue.
					continue
				}

				// Process was registered. Deliver the event to the process InCh.
				p.DynamicProcesses.mu.Lock()
				inCh := p.DynamicProcesses.procMap[ev.EventType].InCh
				p.DynamicProcesses.mu.Unlock()

				fmt.Printf("DEBUG: Routing event, %v, node: %v, eventType: %v, .Inch: %v\n", p.Event, p.Config.NodeName, ev.EventType, inCh)
				inCh <- ev

			case <-p.Ctx.Done():
				p.AddEvent(Event{
					EventType: ERLog,
					EventKind: EventKindError,
					Err:       fmt.Errorf("info: got ctx.Done"),
				})

				return
			}
		}
	}

	return fn
}
