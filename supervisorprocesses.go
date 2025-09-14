package actress

import (
	"context"
	"fmt"
	"log"

	"github.com/fxamacker/cbor/v2"
)

type supervisorProcesses struct {
	procMap map[EventType]*Process
}

// Prepare and return a new *processes structure.
func newsuperVisorProcesses() *supervisorProcesses {
	p := supervisorProcesses{
		procMap: make(map[EventType]*Process),
	}
	return &p
}

// ----------------------------------------------------------------------------

// Router for supervisor events.
const ESRouter EventType = "ESRouter"

// Process function for routing and handling supervisor events. Will check
// and route the event to the correct process.
func esRouterFn(ctx context.Context, p *Process) func() {
	fn := func() {
		eventNr := 0

		for {
			select {
			case ev := <-p.SupervisorEventCh:
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

				// Check if process is registred and valid.
				if _, ok := p.supervisorProcesses.procMap[ev.EventType]; !ok {
					p.AddEvent(Event{EventType: ERLog,
						EventKind: EventKindError,
						Err:       fmt.Errorf("etRouter: on %v found no process registered for the event type : %v", p.Config.NodeName, ev.EventType)})
				}

				// Process was registered. Deliver the event to the process InCh.
				log.Printf(" -------- DEBUG1 %v ---------p.supervisorProcesses.procMap[ev.EventType] : %v\n", p.Config.NodeName, p.supervisorProcesses.procMap[ev.EventType])
				log.Printf(" -------- DEBUG2 %v---------p ev.EventType : %v\n", p.Config.NodeName, ev.EventType)
				log.Printf(" -------- DEBUG3 %v---------p.supervisorProcesses.procMap[ev.EventType].InCh : %v\n", p.Config.NodeName, p.supervisorProcesses.procMap[ev.EventType].InCh)
				p.supervisorProcesses.procMap[ev.EventType].InCh <- ev

			case <-ctx.Done():
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

// Handles information about the currently running processes in the local Actress system.
const ESProcesses EventType = "ESProcesses"

// ProcessesAllMarshaled are used when we want to to send or receive the content of all
// procecess maps by serializing the whole structure.
type ProcessesAllMarshaled struct {
	Static     []byte
	Dynamic    []byte
	Error      []byte
	Supervisor []byte
}

// Will instruct to get all information about all processes.
const InstructionGetAllProcesses Instruction = "getAllProcesses"
const InstructionNone Instruction = ""

// ETFunc for handling information about the currently running processes in the local Actress system.
func esProcessesFn(rootP *Process) ETFunc {
	ETfn := func(ctx context.Context, p *Process) func() {
		fn := func() {

			for {
				select {
				case ev := <-p.InCh:
					m := ProcessesAllMarshaled{}
					var err error

					switch ev.Instruction {

					// Will get all information about all processes
					case InstructionGetAllProcesses:
						m.Static, err = cbor.Marshal(rootP.StaticProcesses.procMap)
						if err != nil {
							p.AddEvent(Event{EventType: ERFatal, Err: fmt.Errorf("esProcesses: failed to marshal static procMap: %v", err)})
						}
						rootP.DynamicProcesses.mu.Lock()
						m.Dynamic, err = cbor.Marshal(rootP.DynamicProcesses.procMap)
						rootP.DynamicProcesses.mu.Unlock()
						if err != nil {
							p.AddEvent(Event{EventType: ERFatal, Err: fmt.Errorf("esProcesses: failed to marshal dynamic procMap: %v", err)})
						}
						m.Error, err = cbor.Marshal(rootP.ErrorProcesses.procMap)
						if err != nil {
							p.AddEvent(Event{EventType: ERFatal, Err: fmt.Errorf("esProcesses: failed to marshal error procMap: %v", err)})
						}
						m.Supervisor, err = cbor.Marshal(rootP.supervisorProcesses.procMap)
						if err != nil {
							p.AddEvent(Event{EventType: ERFatal, Err: fmt.Errorf("esProcesses: failed to marshal supervisor procMap: %v", err)})
						}

						bAll, err := cbor.Marshal(m)
						if err != nil {
							p.AddEvent(Event{EventType: ERFatal, Err: fmt.Errorf("esProcesses: failed to marshal bAll map: %v", err)})
						}

						fmt.Printf("%v\n", bAll)

						// For now just check the next event, and do it as the next thing.
						if ev.NextEvent != nil {
							nextEv := ev.NextEvent
							nextEv.Data = ev.Data
							p.AddEvent(*nextEv)
						}

					// Primarily used for tests. Will just forward the event data to defined NextEvent.
					case InstructionNone:
						if ev.NextEvent != nil {
							nextEv := ev.NextEvent
							nextEv.Data = ev.Data
							p.AddEvent(*nextEv)
						}

					default:
						p.AddEvent(Event{
							EventType: ERLog,
							EventKind: EventKindError,
							Err:       fmt.Errorf("supervisorprocesses: not recognized instruction"),
						})
					}

				case <-ctx.Done():
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

	return ETfn
}
