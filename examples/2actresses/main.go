package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/postmannen/actress"
)

// Define the first function that will be attached to the ETTest1 EventType process.
func test1Func(ctx context.Context, p *actress.Process) func() {
	fn := func() {
		for {
			select {
			case ev := <-p.InCh:
				upper := strings.ToUpper(string(ev.Data))
				// Pass on the processing to the next process, and use the NextEvent we have specified in main
				// for the EventType, and add the result of ToUpper to the data field.
				p.AddEvent(actress.Event{Name: ev.NextEvent.Name,
					Data: []byte(upper)})
			case <-ctx.Done():
				return
			}
		}
	}
	return fn
}

// Define the second function that will be attached to the ETTest2 EventType process.
func test2Func(ctx context.Context, p *actress.Process) func() {
	fn := func() {
		for {
			select {
			case result := <-p.InCh:
				dots := string(result.Data) + "..."

				// All actresses have a TestCh, which can be just used for
				// passing data via the routers. It is primarily intended
				// for use in tests.
				// Since the main code are aware of the actress using this
				// function, we can then put a value on the channel here
				// and read it in main.
				p.TestCh <- actress.Event{Data: []byte(dots)}

				// Also create an informational error message.
				p.AddEvent(actress.Event{Name: actress.ERLog,
					Instruction: actress.InstructionDebug,
					Err:         fmt.Errorf("info: done with the acting")})

			case <-ctx.Done():
				return
			}
		}
	}
	return fn
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	cfg, _ := actress.NewConfig("info")
	rootAct := actress.NewRootProcess(ctx, nil, cfg)

	// Define two event typess for two processes.
	const ETTest1 actress.EventName = "ETTest1"
	const ETTest2 actress.EventName = "ETTest2"

	// Register the event types and event function to processes.
	ac1 := actress.NewProcess(ctx, rootAct, ETTest1, test1Func)
	ac2 := actress.NewProcess(ctx, rootAct, ETTest2, test2Func)
	ac1.Act()
	ac2.Act()

	// Start all the registered processes.
	err := rootAct.Act()
	if err != nil {
		log.Fatal(err)
	}

	// Pass in an event destined for an ETTest1 EventType process, and also specify
	// the next event to be used when passing the result on from ETTest1 to the next
	// process which here is ETTest2.
	rootAct.AddEvent(actress.Event{Name: ETTest1,
		Data:      []byte("test"),
		NextEvent: &actress.Event{Name: ETTest2},
	},
	)

	// Wait and receive the result from the ETTest2 process.
	ev := <-ac2.TestCh
	fmt.Printf("The result: %s\n", ev.Data)

	time.Sleep(time.Second * 2)
}
