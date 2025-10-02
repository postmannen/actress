package actress

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/fxamacker/cbor/v2"
)

type supervisorProcesses struct {
	mu      sync.Mutex
	procMap map[EventName]*Process
}

// Prepare and return a new *processes structure.
func newsuperVisorProcesses() *supervisorProcesses {
	p := supervisorProcesses{
		procMap: make(map[EventName]*Process),
	}
	return &p
}

// ------------------------------------------------------------------------------
// Events and event functions, ESRouter
// ------------------------------------------------------------------------------

// Router for supervisor events.
const ESRouter EventName = "ESRouter"

// Process function for routing and handling supervisor events. Will check
// and route the event to the correct process.
func esRouterFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

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
					ev.NextEvent.PreviousEvent = CopyEventFields(&ev)
				}

				eventNr++
				ev.Nr = eventNr

				// Check if process is registred and valid.
				p.supervisorProcesses.mu.Lock()
				_, ok := p.supervisorProcesses.procMap[ev.Name]
				p.supervisorProcesses.mu.Unlock()

				if !ok {
					slog.Error("", "msg", fmt.Errorf("etRouter: on %v found no process registered for the event type : %v", p.Config.NodeName, ev.Name))
				}

				// // Process was registered. Deliver the event to the process InCh.

				p.supervisorProcesses.mu.Lock()
				inCh := p.supervisorProcesses.procMap[ev.Name].InCh
				p.supervisorProcesses.mu.Unlock()

				slog.Debug("", "msg", fmt.Errorf("esRouterFn on %v, Routing event, %v, node: %v, name: %v, .Inch: %v", p.Config.NodeName, p.Event, p.Config.NodeName, ev.Name, inCh))

				inCh <- ev

			case <-p.Ctx.Done():
				slog.Debug("", "msg", fmt.Errorf("info: esRouterFn, got ctx.Done"))

				return
			}
		}
	}

	return fn
}

// ------------------------------------------------------------------------------
// Events and event functions, Process handling
// ------------------------------------------------------------------------------

// Handles information about the currently running processes in the local Actress system.
const ESProcesses EventName = "ESProcesses"

// Will instruct to get all information about all processes.
const InstructionESProcessesAdd Instruction = "InstructionESProcessesAdd"
const InstructionESProcessesDelete Instruction = "InstructionESProcessesDelete"
const InstructionESProcessesGetAll Instruction = "InstructionESProcessesGetAll"

type esProcessesMapDataIn struct {
	Name EventName
}

// TODO: Figure out what the value should be here after we have removed the Kind type.
type ESProcessesMap map[EventName]string

// ETFunc for handling information about the currently running processes in the local Actress system.
func esProcessesFn() ETFunc {
	ETfn := func(ctx context.Context, p *Process) func() {
		fn := func() {
			defer p.Stop()

			// The map of all the running processes.
			processMap := make(ESProcessesMap)

			for {
				select {
				case ev := <-p.InCh:

					switch ev.Instruction {
					// Add The received data about a process to the map.
					case InstructionESProcessesAdd:
						md := esProcessesMapDataIn{}

						err := cbor.Unmarshal(ev.Data, &md)
						if err != nil {
							slog.Error("", "msg", fmt.Errorf("failed to unmarshal esProcesses map in data: %v", err))
						}

						// TODO: Figure out what the value should be here after we have removed the Kind type.
						processMap[md.Name] = string(md.Name)

						slog.Debug("", "msg", fmt.Errorf("esProcessesfn on %v, processesMap: %+v", p.Config.NodeName, processMap))

						// Nothing to output are produced so we just add for the .NextEvent if defined.
						if ev.NextEvent != nil {
							p.AddEvent(*ev.NextEvent)
						}

					case InstructionESProcessesDelete:
						md := esProcessesMapDataIn{}

						err := cbor.Unmarshal(ev.Data, &md)
						if err != nil {
							slog.Error("", "msg", fmt.Errorf("failed to unmarshal esProcesses map in data: %v", err))
						}

						delete(processMap, p.Event)

						// Nothing to output are produced so we just add for the .NextEvent if defined.
						if ev.NextEvent != nil {
							p.AddEvent(*ev.NextEvent)
						}

					// Dump the content of the whole processes map, and send it with .NextEvent.
					case InstructionESProcessesGetAll:
						b, err := cbor.Marshal(processMap)
						if err != nil {
							slog.Error("", "msg", fmt.Errorf("failed to marshal esProcesses for push all: %v", err))
						}

						nEv := ev.NextEvent
						nEv.Data = b

						if ev.NextEvent != nil {
							p.AddEvent(*nEv)
						}

					default:
						slog.Error("", "msg", fmt.Errorf("esProcesses: not a defined instruction: %v", ev.Instruction))
					}

				case <-p.Ctx.Done():
					slog.Debug("", "msg", fmt.Errorf("info: esProcessesFn, got ctx.Done"))

					return
				}
			}
		}

		return fn
	}

	return ETfn
}
