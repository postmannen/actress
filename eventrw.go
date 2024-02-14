package actress

import (
	"fmt"
	"io"
	"log"
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

// Write the data into Event.Data, and put the event into the EventCh to be processed.
func (m *EventRW) Write(b []byte) (int, error) {

	fmt.Printf(" *2 DEBUG WRITE: ev nr: %v, info: %v , eventType: %v, PREPARING for Writing to erw, len(b): %v, %v\n", m.Ev.Nr, m.Info, m.Ev.
		EventType, len(b), string(b))

	if len(b) > 0 {
		ev := m.Ev
		ev.Data = b
		// Todo: Make this one generic to cath both standard, custom, and dynamic event?
		m.P.AddDynEvent(*ev)
		fmt.Printf(" *2 DEBUG WRITE: ev nr: %v, info: %v , eventType: %v, DONE Writing to erw, len(b): %v, %v\n", m.Ev.Nr, m.Info, m.Ev.
			EventType, len(b), string(b))
	}

	log.Printf("%v, len: %v when writing, not adding an event\n", m.Info, len(b))
	return len(b), nil
}

// Read the data into b.
func (m *EventRW) Read(b []byte) (int, error) {
	if m.Pos >= len(m.Ev.Data) {
		fmt.Printf(" *1 DEBUG READ: ev nr: %v, info: %v, eventType: %v, EOF reading from erw, len(m.ev.Data): %v, %v\n", m.Ev.Nr, m.Info, m.Ev.EventType, len(m.Ev.Data), string(m.Ev.Data))
		return 0, io.EOF
	}

	fmt.Printf(" *1 DEBUG READ: ev nr: %v, info: %v, eventType: %v, PREPARING reading from erw, len(m.ev.Data): %v, %v\n", m.Ev.Nr, m.Info, m.Ev.EventType, len(m.Ev.Data), string(m.Ev.Data))

	n := copy(b, m.Ev.Data[m.Pos:])

	fmt.Printf(" *1 DEBUG READ: ev nr: %v, info: %v, eventType: %v, DONE reading from erw, len(m.ev.Data): %v, %v\n", m.Ev.Nr, m.Info, m.Ev.EventType, len(m.Ev.Data), string(m.Ev.Data))

	m.Pos += n
	return n, nil
}
