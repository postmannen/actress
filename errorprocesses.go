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
	"log/slog"
	"os"
	"time"
	"unsafe"
)

// ERRouter for error events.
const ERRouter EventName = "ERRouter"

// Process function for routing and handling events.
func erRouterFn(ctx context.Context, p *Process) func() {
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
			case ev := <-p.ErrorEventCh:
				go func() {
					p.ErrorProcesses.mu.Lock()
					procFromProcMap, procMapValueOK := p.ErrorProcesses.procMap[ev.Name]
					p.ErrorProcesses.mu.Unlock()

					if !procMapValueOK {
						slog.Error("erRouterFn", "on", p.Config.NodeName, "found no process registered for the event type, returning and not handling event", ev.Name)
						return
					}

					// If there is a next event defined, we make a copy of all the fields  of the current event,
					// and put that as the previousEvent on the next event. We can use this information later
					// if need to check something in the previous event.
					if ev.NextEvent != nil {
						// Keep the information about the current event, so we are able to check for things
						// like ackTimeout and what node to reply back to if ack should be given.
						ev.NextEvent.PreviousEvent = CopyEventFields(&ev)
					}

					inCh := procFromProcMap.InCh

					if slog.Default().Enabled(ctx, slog.LevelDebug) {
						slog.Debug("erRouterFn", "Routing event", p.Event, "node", p.Config.NodeName, "name", ev.Name, "Inch", inCh)
					}

					select {
					case inCh <- ev:
					default:
						select {
						case inCh <- ev:
						case <-time.After(time.Second * 5):
							slog.Debug("erRouterFn", "Routing event", p.Event, "node", p.Config.NodeName, "name", ev.Name, "Inch", inCh, "error", "timed out trying to deliver the event on the inch of the er process")
						}
					}
				}()

			case <-p.Ctx.Done():
				if slog.Default().Enabled(ctx, slog.LevelDebug) {
					slog.Debug("erRouterFn", "got ctx.Done, on", p.Config.NodeName)
				}

				return
			}
		}
	}

	return fn
}

// InstructionError for error logging.
const InstructionError Instruction = "InstructionError"
const InstructionInfo Instruction = "InstructionInfo"
const InstructionDebug Instruction = "InstructionDebug"
const InstructionFatal Instruction = "InstructionFatal"

// ERLog for log errors.
const ERLog EventName = "ERLog"

// Will log errors to the console based on the Instruction field of the event.
//
// NB: The "none" is handled in the AddEvent function, to drop the event as
// early as possible, instead of sending it all the way to be dropped here.
func erLogFn(ctx context.Context, p *Process) func() {
	fn := func() {

		for {
			p.SignalReady()

			select {
			case er := <-p.InCh:

				switch er.Instruction {
				case InstructionError:
					slog.Error("erLogFn", "instructionError, msg", er.Err)
				case InstructionInfo:
					slog.Info("erLogFn", "instructionInfo, msg", er.Err)
				case InstructionDebug:
					if slog.Default().Enabled(ctx, slog.LevelDebug) {
						slog.Debug("erLogFn", "instructionDebug, msg", er.Err)
					}
				case InstructionFatal:
					slog.Error("erLogFn", "instructionFatal, msg", er.Err)
					os.Exit(1)
				default:
					slog.Error("erLogFn", "default, msg", er.Err)
				}

			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

// ERTest log and exit system.
const ERTest EventName = "ERTest"

func erTestFn(ctx context.Context, p *Process) func() {
	fn := func() {

		for {
			p.SignalReady()

			select {
			case er := <-p.InCh:

				go func() {
					drop := fmt.Sprintf("erTestFn: error for fatal logging received: %v\n", er.Err)
					_ = drop
				}()
			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

// ERNone will drop the event if it is an error event.
const ERNone EventName = "ERNone"

// Process function for dropping error events. Primarily used for testing.
func erNoneFn(ctx context.Context, p *Process) func() {
	use := func(p unsafe.Pointer) {}

	fn := func() {

		for {
			p.SignalReady()

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
