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
	"time"

	"github.com/google/uuid"
)

// NewUUID will create and return a new UUID prefix with "ED-".
func NewUUID() string {
	u := fmt.Sprintf("ED-%v", uuid.New())
	return u
}

// ------------------------------------------------------------------------------
// Events and event functions.
// ------------------------------------------------------------------------------

// EDRouter for normal events.
const EDRouter EventName = "EDRouter"

// Process function for routing and handling events. Will check
// and route the event to the correct process.
func edRouterFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer func() {
			if slog.Default().Enabled(ctx, slog.LevelDebug) {
				slog.Debug("edRouterFn", "stopping process with defered stop", "")
			}
			p.Stop()
		}()

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
			case ev := <-p.DynamicEventCh:
				// If there is a next event defined, we make a copy of all the fields  of the current event,
				// and put that as the previousEvent on the next event. We can use this information later
				// if need to check something in the previous event.
				if ev.NextEvent != nil {
					// Keep the information about the current event, so we are able to check for things
					// like ackTimeout and what node to reply back to if ack should be given.
					ev.NextEvent.PreviousEvent = CopyEventFields(&ev)
				}

				// Dynamic processes can take a little longer to start up and be
				// registered in the map. We check here if process is registred,
				// and if it is not we retry.
				// The checking is done in a go routine so the router don't block
				// here waiting, and we continue with the next event in the queue.
				p.DynamicProcesses.mu.Lock()
				dynP, ok := p.DynamicProcesses.procMap[ev.Name]
				p.DynamicProcesses.mu.Unlock()
				if ok {
					// Process was registered. Deliver the event to the process InCh.
					if slog.Default().Enabled(ctx, slog.LevelDebug) {
						slog.Debug("edRouterFn", "on", p.Config.NodeName, "Routing event", p.Event, "node", p.Config.NodeName, "name", ev.Name)
					}

					select {
					case dynP.InCh <- ev:
					case <-ctx.Done():
						return
					}

					// Done with the event, loop back, and continue with the next event.
					continue
				}

				// The process was not registered. Wait a bit and check if it is just
				// taking some time to start.
				go func(ev Event) {
					// Try to 3 times to deliver the message.
					for i := 0; i < 3; i++ {
						slog.Error("edRouterFn", "on", p.Config.NodeName, "found no process registered for the event type", ev.Name, "ev.DstNode", ev.DstNode)
						time.Sleep(time.Second * 1)

						p.DynamicProcesses.mu.Lock()
						dynP, ok := p.DynamicProcesses.procMap[ev.Name]
						p.DynamicProcesses.mu.Unlock()

						if !ok {
							// process not found yet, loop again
							continue
						}

						// Process is now registred, so we can safely put
						//the event on the InCh of the process.
						select {
						case dynP.InCh <- ev:
						case <-ctx.Done():
						}

						return
					}
				}(ev)

			case <-p.Ctx.Done():
				if slog.Default().Enabled(ctx, slog.LevelDebug) {
					slog.Debug("edRouterFn", "got ctx.Done, on", p.Config.NodeName)
				}

				return
			}
		}
	}

	return fn
}

// EDSync is used to syncronize events.
// The EDSyncFn that is to be used with this event type takes a
// signal channel, we can then use this event type to signal that
// another event is done before we continue by setting this event
// type as the NextEvent.
const EDSync EventName = "EDSync"

// EDSyncFn is the function that will be used to syncronize events.
// It takes a channel that will be used to send a signal on when the
// EDSync event is executed.
//
// In general EDSync is used to syncronize one-off events, so delete
// the process after it is done with it's sync job.
func EDSyncFn(syncCh chan struct{}) ETFunc {
	fn := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				p.SignalReady()

				select {
				case <-p.InCh:
					syncCh <- struct{}{}
				case <-p.Ctx.Done():
					return
				}
			}
		}
		return fn
	}
	return fn
}
