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
