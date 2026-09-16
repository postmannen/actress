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
	"unsafe"
)

// ERRouter for error events.
const ERRouter EventName = "ERRouter"

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
