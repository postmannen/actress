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

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/postmannen/actress"
)

const ETEventProcessor actress.EventName = "ETEventProcessor"
const ETEventCreator actress.EventName = "ETEventCreator"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	cfg, _ := actress.NewConfig("debug")
	rootAct := actress.NewRootProcess(ctx, nil, cfg, nil)

	// etHttpGetFn will reveive the event, do some processing with it.
	// An EventRW will then be create from the Event, and it's Read and
	// Write methods are used for sending of the event.
	etEventProcessorFn := func(ctx context.Context, p *actress.Process) func() {
		fn := func() {
			for {
				select {
				case ev := <-p.InCh:
					ev.NextEvent.Data = append(ev.Data, []byte(" processesed the data")...)
					// Use the next event that where defined in the event for the reply.
					evR := NewEventRW(p, ev.NextEvent)
					_, err := io.Copy(evR, evR)
					if err != nil {
						log.Fatalf("error: failed to io.Copy evR<-evR: %v\n", err)
					}

					fmt.Printf(" *** etHttpGetFn: Got event, processed it, and sent it back again: %v\n", string(ev.Data))
				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	// etEventCreatorFn create an event with some content An EventRW will
	// then be create from the Event, and it's Read and it's Write methods
	// are used for sending of the event.
	// The same RW methods are also used for the received result.
	etEventCreatorFn := func(ctx context.Context, p *actress.Process) func() {

		fn := func() {
			// Reveive the result of the processed event from the other processes.
			go func() {
				for {
					select {
					case ev := <-p.InCh:
						buf := &bytes.Buffer{}
						evRW := NewEventRW(p, &ev)
						_, err := io.Copy(buf, evRW)
						if err != nil {
							log.Fatalf("error: failed to io.Copy buf<-erRW: %v\n", err)
						}

						fmt.Printf(" * etProxyListenerFn: received length: %v, and contains: \"%v\"\n\n", buf.Len(), buf.String())
					case <-ctx.Done():
						return
					}
				}
			}()

			// Create some events, create an EventRW of the Event, and use it's
			// Write method with io.Copy to send it of to the other process.
			counter := 0
			for {
				// Create a an Event with just EventType set, and the NextEvent
				// set so the reply event will be sent back to us. The Data field
				// will be set and handled by the Write method when called.
				erw := NewEventRW(p, &actress.Event{Name: ETEventProcessor,
					NextEvent: &actress.Event{Name: ETEventCreator}})

				buf := bytes.NewBuffer([]byte{})
				str := fmt.Sprintf("some data for event nr: %v", counter)
				buf.Write([]byte(str))

				_, err := io.Copy(erw, buf)
				if err != nil {
					log.Fatalf("error: failed to io.Copy erw<-buf: %v\n", err)
				}

				fmt.Printf(" etProxyListenerFn: sending brand new event with nr %v, which contains: \"%v\"\n", counter, str)

				counter++
				time.Sleep(time.Second * 2)
			}

			// - main

			// ---
		}

		return fn
	}

	// Start all the registered actors.
	err := rootAct.Act()
	if err != nil {
		log.Fatal(err)
	}

	// Register the event type and attach a function to it.
	actress.NewProcess(ctx, rootAct, ETEventProcessor, etEventProcessorFn).Act()
	actress.NewProcess(ctx, rootAct, ETEventCreator, etEventCreatorFn).Act()

	<-ctx.Done()
}

type eventRW struct {
	p  *actress.Process
	ev *actress.Event
}

func NewEventRW(p *actress.Process, ev *actress.Event) *eventRW {
	m := eventRW{
		p:  p,
		ev: ev,
	}
	return &m
}

// Write the data into Event.Data, and put the event into the EventCh to be processed.
func (m *eventRW) Write(b []byte) (int, error) {
	ev := m.ev
	ev.Data = b
	m.p.AddEvent(*ev)
	return len(b), nil
}

// Read the data into b.
func (m *eventRW) Read(b []byte) (int, error) {
	copy(b, m.ev.Data)
	return len(m.ev.Data), io.EOF
}
