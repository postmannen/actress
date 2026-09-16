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
	"log/slog"
)

// ------------------------------------------------------------------------------
// Events and event functions.
// ------------------------------------------------------------------------------

// ECRouter for custom events.
const ECRouter EventName = "ECRouter"

// ECGeneralDelivery is primarily used for testing to check that the ECRouter
// properly routes events, and that custom processes start up correctly.
const ECGeneralDelivery EventName = "ECGeneralDelivery"

// ecGeneralDeliveryFn is primarily used for testing to check that the ECRouter
// properly routes events, and that custom processes start up correctly.
func ecGeneralDeliveryFn(ctx context.Context, p *Process) func() {
	fn := func() {
		p.SignalReady()

		for {
			select {
			case ev := <-p.InCh:
				// Primarily used for tests. Will just forward the event data to defined NextEvent.
				if ev.NextEvent != nil {
					nextEv := ev.NextEvent
					nextEv.Data = ev.Data
					p.AddEvent(*nextEv)
				}

			case <-p.Ctx.Done():
				slog.Info("ecGeneralDeliveryFn", "got ctx.Done, on", p.Config.NodeName)
				return
			}
		}
	}

	return fn

}
