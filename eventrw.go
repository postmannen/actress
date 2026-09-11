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
	"io"
	"log/slog"
)

type EventRW struct {
	P    *Process
	Ev   *Event
	Info string
	Pos  int
}

// NewEventRW will return a type that adds Read and Write methods
// to the Event type.
func NewEventRW(p *Process, ev *Event, info string) *EventRW {
	m := EventRW{
		P:    p,
		Ev:   ev,
		Info: info,
		Pos:  0,
	}
	return &m
}

// Write the data into Event.Data, and put the event into the StaticEventCh to be processed.
func (m *EventRW) Write(b []byte) (int, error) {

	if len(b) == 0 {
		return 0, nil
	}

	if len(b) > 0 {
		ev := *m.Ev
		// Create a deep copy of the byte slice
		ev.Data = append([]byte{}, b...)

		m.P.AddEvent(ev)
	}

	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		slog.Debug("EventRW.Write", "info", m.Info, "len b", len(b))
	}
	return len(b), nil
}

// Read the data into b.
func (m *EventRW) Read(b []byte) (int, error) {
	if m.Pos >= len(m.Ev.Data) {
		return 0, io.EOF
	}

	n := copy(b, m.Ev.Data[m.Pos:])

	m.Pos += n
	return n, nil
}
