package actress

import (
	"context"
	"log/slog"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// ------------------------------------------------------------------------------
// Events and event functions, ESRouter
// ------------------------------------------------------------------------------

// ESRouter for supervisor events.
const ESRouter EventName = "ESRouter"

// Process function for routing and handling supervisor events. Will check
// and route the event to the correct process.
func esRouterFn(ctx context.Context, p *Process) func() {
	fn := func() {

		// The inch is not used for this function, so we set up a
		// listener that logs info so the user can detect the error.
		go func() {
			for {
				select {
				case ev := <-p.InCh:
					slog.Error("an even was received on this actors inch, which is not in use, dropping event", "at actor", p.Event, "from src node", ev.SrcNode, "event nr", ev.Nr)
				case <-ctx.Done():
				}
			}
		}()

		for {
			select {
			case ev := <-p.SupervisorEventCh:
				func() {
					// If there is a next event defined, we make a copy of all the fields  of the current event,
					// and put that as the previousEvent on the next event. We can use this information later
					// if need to check something in the previous event.
					if ev.NextEvent != nil {
						// Keep the information about the current event, so we are able to check for things
						// like ackTimeout and what node to reply back to if ack should be given.
						ev.NextEvent.PreviousEvent = CopyEventFields(&ev)
					}

					// Check if process is registred and valid.
					p.supervisorProcesses.mu.Lock()
					procFromProcMap, procMapValueOK := p.supervisorProcesses.procMap[ev.Name]
					p.supervisorProcesses.mu.Unlock()

					if !procMapValueOK {
						slog.Error("esRouterFn", "on", p.Config.NodeName, "found no process registered for the event type", ev.Name)
						return
					}

					select {
					case procFromProcMap.InCh <- ev:
					default:
						select {
						case procFromProcMap.InCh <- ev:
						case <-time.After(time.Second * 5):
							slog.Debug("esRouterFn", "Routing event", p.Event, "node", p.Config.NodeName, "name", ev.Name, "Inch", procFromProcMap.InCh, "error", "timed out trying to deliver the event on the inch of the er process")
						}
					}
				}()

			case <-p.Ctx.Done():
				if slog.Default().Enabled(ctx, slog.LevelDebug) {
					slog.Debug("esRouterFn", "got ctx.Done, on", p.Config.NodeName)
				}

				return
			}
		}
	}

	return fn
}

// ------------------------------------------------------------------------------
// Events and event functions, Process handling
// ------------------------------------------------------------------------------

// ESProcesses handles information about the currently running processes in the local Actress system.
const ESProcesses EventName = "ESProcesses"

// InstructionESProcessesAdd will instruct to get all information about all processes.
const InstructionESProcessesAdd Instruction = "InstructionESProcessesAdd"
const InstructionESProcessesDelete Instruction = "InstructionESProcessesDelete"
const InstructionESProcessesGetAll Instruction = "InstructionESProcessesGetAll"

type esProcessesMapDataIn struct {
	Name EventName
}

type ESProcessesMap map[EventName]string

// ETFunc for handling information about the currently running processes in the local Actress system.
func esProcessesFn() ETFunc {
	ETfn := func(ctx context.Context, p *Process) func() {
		fn := func() {

			// The map of all the running processes.
			processMap := make(ESProcessesMap)

			for {
				p.SignalReady()

				select {
				case ev := <-p.InCh:

					switch ev.Instruction {
					// Add The received data about a process to the map.
					case InstructionESProcessesAdd:
						md := esProcessesMapDataIn{}

						err := cbor.Unmarshal(ev.Data, &md)
						if err != nil {
							slog.Error("esProcessesFn", "failed to unmarshal esProcesses map in data", err)
						}

						processMap[md.Name] = string(md.Name)

						if slog.Default().Enabled(ctx, slog.LevelDebug) {
							slog.Debug("esProcessesFn", "on", p.Config.NodeName, "processesMap", processMap)
						}

						// Nothing to output are produced so we just add for the .NextEvent if defined.
						if ev.NextEvent != nil {
							p.AddEvent(*ev.NextEvent)
						}

					case InstructionESProcessesDelete:
						md := esProcessesMapDataIn{}

						err := cbor.Unmarshal(ev.Data, &md)
						if err != nil {
							slog.Error("esProcessesFn", "failed to unmarshal esProcesses map in data", err)
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
							slog.Error("esProcessesFn", "failed to marshal esProcesses for push all", err)
						}

						nEv := ev.NextEvent
						nEv.Data = b

						if ev.NextEvent != nil {
							p.AddEvent(*nEv)
						}

					default:
						slog.Error("esProcessesFn", "not a defined instruction", ev.Instruction)
					}

				case <-p.Ctx.Done():
					if slog.Default().Enabled(ctx, slog.LevelDebug) {
						slog.Debug("esProcessesFn", "got ctx.Done, on", p.Config.NodeName)
					}

					return
				}
			}
		}

		return fn
	}

	return ETfn
}
