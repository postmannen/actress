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
	"unsafe"
)

// processes holds information about what process functions
// who belongs to what event, and also a map of the started
// processes.
type errorProcesses struct {
	procMap map[EventType]*Process
}

// // Delete an Event and it's process from the processes map.
// func (p *processes) delete(et EventType, proc *Process) {
// 	p.mu.Lock()
// 	defer p.mu.Unlock()
// 	p.procMap[et].cancel()
// 	delete(p.procMap, et)
// }

// Checks if the event is defined in the processes map, and returns true if it is.
func (p *errorProcesses) IsEventDefined(ev EventType) bool {
	if _, ok := p.procMap[ev]; !ok {
		return false
	}

	return true
}

// Prepare and return a new *processes structure.
func newErrorProcesses() *errorProcesses {
	p := errorProcesses{
		procMap: make(map[EventType]*Process),
	}
	return &p
}

// Router for error events.
const ERRouter EventType = "ERRouter"

// Process function for routing and handling events.
func erRouterFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

		eventNr := 0

		for {
			select {
			case e := <-p.ErrorEventCh:
				// If there is a next event defined, we make a copy of all the fields  of the current event,
				// and put that as the previousEvent on the next event. We can use this information later
				// if need to check something in the previous event.
				if e.NextEvent != nil {
					// Keep the information about the current event, so we are able to check for things
					// like ackTimeout and what node to reply back to if ack should be given.
					e.NextEvent.PreviousEvent = CopyEventFields(e)
				}

				eventNr++
				e.Nr = eventNr

				inCh := p.ErrorProcesses.procMap[e.EventType].InCh

				fmt.Printf("DEBUG: Routing event, %v, node: %v, eventType: %v, .Inch: %v\n", p.Event, p.Config.NodeName, e.EventType, inCh)
				inCh <- e

			case <-p.Ctx.Done():
				// NB: Bevare of this one getting stuck if for example the error
				// handling is down. Maybe add a timeout if blocking to long,
				// and then send elsewhere if it becomes a problem.
				p.AddEvent(Event{
					EventType: ERLog,
					EventKind: EventKindError,
					Err:       fmt.Errorf("info: got ctx.Done"),
				})
			}
		}
	}

	return fn
}

// Log errors.
const ERLog EventType = "ERLog"

func erLogFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

		for {
			select {
			case er := <-p.InCh:

				go func() {
					log.Printf("error for logging received: %v\n", er.Err)
				}()
			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

// Log debug errors.
const ERDebug EventType = "ERDebug"

func erDebugFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

		for {
			select {
			case er := <-p.InCh:

				go func() {
					log.Printf("error for debug logging received: %v\n", er.Err)
				}()
			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

// Log and exit system.
const ERFatal EventType = "ERFatal"

func erFatalFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

		for {
			select {
			case er := <-p.InCh:

				go func() {
					log.Fatalf("error for fatal logging received: %v\n", er.Err)
				}()
			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

// Log and exit system.
const ERTest EventType = "ERTest"

func erTestFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

		for {
			select {
			case er := <-p.InCh:

				go func() {
					drop := fmt.Sprintf("error for fatal logging received: %v\n", er.Err)
					_ = drop
				}()
			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

const ERNone EventType = "ERNone"

func erNoneFn(ctx context.Context, p *Process) func() {
	use := func(p unsafe.Pointer) {}

	fn := func() {
		defer p.Stop()

		for {
			select {
			case er := <-p.InCh:
				use(unsafe.Pointer(&er.Err))
			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}
