# Actress

A Concurrent Actor framework written in Go.

## Overview

### Processes

A process are like a module capable of performing a specific tasks. The nature of the process is determined by an EventType and a Function attached to each process. A process have an InCh for receiving events, and an AddEvent for sending Events. The processes can themselves spawn new processes. Processes can also send Event messages to other processes.

### Events

To initiate a task and trigger the execution of the process's function, we send events. Each process has its own unique event name. Events serve as communication channels within the system. They can carry data, either as a result of something the previous process did, instructions for what a process should do, or both. An event can contain a chain of events to create workflows of what do do and in what order by using the NextEvent feature (see examples for usage).

```Go
type Event struct {
    // EventType is a unique name to identify the type of the event.
    EventType EventType 
    // Cmd is usually used for giving instructions or parameters for
    // what an event shall do.
    Cmd       []string
    // Data usually carries the data from one process to the next. Example
    // could be a file read on process1 is put in the Data field, and
    // passed on to process2 to be unmarshaled.
    Data      []byte
    // Err is used for defining the error message when the event is used
    // as an error event.
    Err       error
    // NextEvent defines a series of events to be executed like a workflow.
    // The receiving process should check this field for what kind of event
    // to create as the next step in the workflow.    
    NextEvent *Event 
}
```

### Event Functions

Event Functions holds the logic for what to do when an event is received, and what to do with the data the event holds. The Event functions are callback functions that are executed when a process are created.

For an event function to continously work on on events it should hold a for loop that listens on the Process InCh for new Events.

### Examples

Check out the test files for examples for how to define an Event and it's Process function, or for more complete examples check out the [examples](examples/) folder.

### Quick start

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

    // Define two event types for two processes.
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
                    p.AddStd(actress.Event{EventType: ev.NextEvent.EventType, Data: []byte(upper)})
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
                    p.AddErr(actress.Event{EventType: actress.ERDebug, Err: fmt.Errorf("info: done with the acting")})

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
    rootAct.AddStd(actress.Event{EventType: ETTest1, Data: []byte("test"), NextEvent: &actress.Event{EventType: ETTest2}})

    // Wait and receive the result from the ETTest2 process.
    fmt.Printf("The result: %v\n", <-testCh)

    time.Sleep(time.Second * 2)
    cancel()
}
```

## Details

### Custom Events Processes

Custom Event Processes allows for dynimally adding new EventTypes and Processes at runtime. This feature is enabled by setting the `CUSTOMEVENTS` env variable to `true`, and also the path for where to look for configs with `CUSTOMEVENTSPATH`. The folder is continously being watched for changes, so any updates to config JSON files will be activated immediately. For adding custom event, put files in the `CUSTOMEVENTSPATH` with the extension `.json`. The files should be in the same format as an **Event**.

```json
{"Name":"ET1","Cmd":["/bin/bash","-c"]}
```

The example above will automatically create a Process that have an EventType of `ET1`. We can then send Events using `ET1` as the EventType, and what we put in Event.Cmd will be appended to the existing values that already exist in the Custom Event. Example follows.

#### Custom Event Example 1

We use the Custom Event from above, and add a new event using `ET1` like this:

```go
p.AddStd(Event{EventType: EventType("ET1"), Cmd: []string{"ls -l"}})
```

When the Event is received at the ET1 process it's Cmd is appended what was defined earlier when creating the ET1 Process. The end result of the Cmd field will be `[]string{"/bin/bash","-c","ls -l"}` which is then executed.

#### Custom Event Example 2

We can also add the whole command to be executed in the `.json` file likes this.

```json
{"Name":"ETBleeping","Cmd":["/bin/bash","-c","curl -L https://bleepingcomputer.com"]}
```

Since this Event specification is complete in itself we don't have to use the Cmd field when adding an Event to use it.

```go
p.AddStd(Event{EventType: EventType("ETBleeping")})
```

## NextEvent

NextEvent makes it possible to define an event as a chain of Events. An example could be that we want to get the content of a web page, and print the result to the screen. We could do that in the following way.

```go
p.AddStd(Event{EventType: EventType("ETBleeping"), NextEvent: &Event{EventType: ETPrint}})
```
