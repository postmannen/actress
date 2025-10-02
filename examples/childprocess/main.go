package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/postmannen/actress"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	cfg, _ := actress.NewConfig("debug")
	rootAct := actress.NewRootProcess(ctx, nil, cfg, nil)
	rootAct.Act()

	// Create a test channel where we receive the end result.
	testCh := make(chan string)

	const ETTest1 actress.EventName = "ETTest1"
	const ETTest2 actress.EventName = "ETTest2"

	// Define the function that will be attached to the ETTest1 EventType.
	test1Func := func(ctx context.Context, p *actress.Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:

					// Define the function that will be attached to the ETTest2 EventType.
					test2Func := func(ctx context.Context, p *actress.Process) func() {
						fn := func() {
							for {
								select {
								case result := <-p.InCh:
									dots := string(result.Data) + "..."
									testCh <- string(dots)

									// Also create an informational error message.
									p.ErrorEventCh <- actress.Event{Name: actress.ERLog,
										Instruction: actress.InstructionDebug,
										Err:         fmt.Errorf("info: done with the acting")}

								case <-ctx.Done():
									return
								}
							}
						}

						return fn
					}
					actress.NewProcess(ctx, p, ETTest2, test2Func).Act()

					upper := strings.ToUpper(string(result.Data))
					p.StaticEventCh <- actress.Event{Name: ETTest2, Data: []byte(upper)}

				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	// Register the event type and attach a function to it.
	actress.NewProcess(ctx, rootAct, ETTest1, test1Func).Act()

	// Start all the registered actors.
	err := rootAct.Act()
	if err != nil {
		log.Fatal(err)
	}

	// Add and pass in an event that will be picked up by the actor
	// registered for the ETTest1 EventType, and add "test" to the
	// data field of the event.
	rootAct.AddEvent(actress.Event{Name: ETTest1,
		Data: []byte("test")})
	// Receive and print the result.
	fmt.Printf("The result: %v\n", <-testCh)

	time.Sleep(time.Second * 2)
	cancel()
}
