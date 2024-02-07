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

const ETEventProcessor actress.EventType = "ETEventProcessor"
const ETEventCreator actress.EventType = "ETEventCreator"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	rootAct := actress.NewRootProcess(ctx)

	// etHttpGetFn will reveive the event, do some processing with it.
	// An EventRW will then be create from the Event, and it's Read and
	// Write methods are used for sending of the event.
	etEventProcessorFn := func(ctx context.Context, p *actress.Process) func() {
		fn := func() {
			for {
				select {
				case ev := <-p.InCh:
					ev.Data = append(ev.Data, []byte(" processesed the data")...)
					evR := NewEventRW(p, &ev, ETEventCreator)
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
						evRW := NewEventRW(p, &ev, ETEventProcessor)
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
				erw := NewEventRW(p, nil, ETEventProcessor)
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
	actress.NewProcess(ctx, *rootAct, ETEventProcessor, etEventProcessorFn).Act()
	actress.NewProcess(ctx, *rootAct, ETEventCreator, etEventCreatorFn).Act()

	<-ctx.Done()
}

type eventRW struct {
	p               *actress.Process
	ev              *actress.Event
	writeToEventype *actress.EventType
}

func NewEventRW(p *actress.Process, ev *actress.Event, writeToEventype actress.EventType) *eventRW {
	m := eventRW{p: p,
		ev:              ev,
		writeToEventype: &writeToEventype}
	return &m
}

func (m *eventRW) Write(b []byte) (int, error) {
	m.p.AddEvent(actress.Event{EventType: *m.writeToEventype, Data: b})
	return len(b), nil
}

func (m *eventRW) Read(b []byte) (int, error) {
	copy(b, m.ev.Data)
	return len(m.ev.Data), io.EOF
}
