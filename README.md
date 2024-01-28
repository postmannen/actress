# Actress

A Concurrent Actor framework written in Go.

## Processes

A process are like a module capable of performing a specific tasks. The nature of the process is determined by an EventType and a Function attached to each process. A process have an InCh for receiving events, and an AddEvent for sending Events. The processes can themselves spawn new processes. Processes can also send Event messages to other processes.

## Events

To initiate a task and trigger the execution of the process's function, we send events. Each process has its own unique event name. Events serve as communication channels within the system. They can carry data, either as a result of something the previous process did, instructions for what a process should do, or both. An event can contain a chain of events to create workflows of what do do and in what order by using the NextEvent feature (see example).

## Event Functions

Event Functions holds the logic for what to do when an event is received, and what to do with the data it holds.

## Examples

Check out the test files for examples for how to define an Event and it's Process function, or for more complete examples check out the [examples](examples/) folder.

## Quick start

```Go
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
    rootAct := actress.NewRootProcess(ctx)
    // Create a test channel where we receive the end result.
    testCh := make(chan string)

    // Define two event typess for two processes.
    const ETTest1 actress.EventType = "ETTest1"
    const ETTest2 actress.EventType = "ETTest2"

    // Define the first function that will be attached to the ETTest1 EventType process.
    test1Func := func(ctx context.Context, p *actress.Process) func() {
        fn := func() {
            for {
                select {
                case ev := <-p.InCh:
                    upper := strings.ToUpper(string(ev.Data))
                    // Pass on the processing to the next process, and use the NextEvent we have specified in main
                    // for the EventType, and add the result of ToUpper to the data field.
                    p.AddEvent(actress.Event{EventType: ev.NextEvent.EventType, Data: []byte(upper)})
                case <-ctx.Done():
                    return
                }
            }
        }
        return fn
    }

    // Define the second function that will be attached to the ETTest2 EventType process.
    test2Func := func(ctx context.Context, p *actress.Process) func() {
        fn := func() {
            for {
                select {
                case result := <-p.InCh:
                    dots := string(result.Data) + "..."

                    // Send the result on the testCh so we are able to to receive it in main().
                    testCh <- string(dots)

                    // Also create an informational error message.
                    p.AddError(actress.Event{EventType: actress.ERDebug, Err: fmt.Errorf("info: done with the acting")})

                case <-ctx.Done():
                    return
                }
            }
        }
        return fn
    }

    // Register the event types and event function to processes.
    actress.NewProcess(ctx, *rootAct, ETTest1, test1Func).Act()
    actress.NewProcess(ctx, *rootAct, ETTest2, test2Func).Act()

    // Start all the registered processes.
    err := rootAct.Act()
    if err != nil {
        log.Fatal(err)
    }

    // Pass in an event destined for an ETTest1 EventType process, and also specify
    // the next event to be used when passing the result on from ETTest1 to the next
    // process which here is ETTest2.
    rootAct.AddEvent(actress.Event{EventType: ETTest1, Data: []byte("test"), NextEvent: &actress.Event{EventType: ETTest2}})

    // Wait and receive the result from the ETTest2 process.
    fmt.Printf("The result: %v\n", <-testCh)

    time.Sleep(time.Second * 2)
    cancel()
}
```
