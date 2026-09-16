package actress

import (
	"context"
	"log/slog"

	"github.com/fxamacker/cbor/v2"
)

// ------------------------------------------------------------------------------
// Events and event functions
// ------------------------------------------------------------------------------

// ESRouter for supervisor events.
const ESRouter EventName = "ESRouter"

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
