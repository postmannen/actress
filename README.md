# Actress

A Concurrent Actor framework written in Go.

**NB: This is still in the idea phase, so concepts are being tested out and things might/will change rapidly**.

**Expect breaking changes between commits**.

## Actress Overview

Actor systems model a program as a collection of independent processes (actors), each with its own private state that no other actor can touch. In general, Actors don't share memory, they communicate exclusively by sending messages to one another, which they handle asynchronously, one at a time. Because each actor owns its state and processes messages sequentially, there are no need for mutexes, there will be no data races, and no lock-ordering bugs to reason about. This makes actor systems a natural fit for concurrent workloads on a single machine and for distributed systems.
 
### So how is this done ?

- Define custom **Processes** (actors) that do some task. A Process can be thought of as a normal function that listens for events, which does it's task when an event is received.
- Multiple Processes can be chained together to form a complete flow of work.
- Processes communicate by sending **Events** (messages) to one another.
- Multiple processes can run on the same instance, called a **Node**.
- Multiple Nodes can run on the same machine.
- Nodes can also be spread over a network of multiple machines, and Actress provides functionality for automatically forwarding events between Nodes.

### Processes

A process are like a module that can perform a specific task. A process is defined by an Event Name and an Event Function attached to each process. A process have an InCh for receiving events, and an AddEvent function for sending Events to other processes.
The processes can themselves spawn new processes.

### Sharing state

Normally you want to avoid sharing state, and state are shared by passing the state from one processes to the next as an event message.

But, if sharing state is needed, a process running in the same Node instance can share state directly between the Process Functions if needed. This can be done by giving it a state variable or struct type that holds the state as an input argument to the event function.

Example:

```go
// The 'c *Client' are defined with the Event Functions for both processes,
// so both processes have access to the same shared memory area.

func etHello(c *Client) actress.ETFunc {
.......
}

func etGreet(c *Client) actress.ETFunc {
.......
}
```

### Events

To communicate with other processes, we send events. Each process has its own unique event name. Events are the way to communicate between the processes. They can carry data with the result of something a process did that is passed on to the next process for further processing. Think of it as the same concept as piping output in linux, where you can pipe the output of one command as the input of the next command.
An event can also contain a chain of several events to create workflows or pipes of what do do and in what order by using the **NextEvent** feature (see examples for usage).

The structure of an Event:

```Go
type Event struct {
    Nr int
    // EventType is the type of the event, Static, Dynamic, and so on
    EventType EventType `json:"eventType" yaml:"eventType" cbor:"eventType"`
    // Name is a unique name to identify the type of the event.
    Name EventName `json:"name" yaml:"name" cbor:"name"`
    // Cmd is usually used for giving instructions or parameters for
    // what an event shall do.
    Cmd []string `json:"cmd" yaml:"cmd" cbor:"cmd"`
    // Instruction got the underlying type of string. This field can
    // be used to give for example an instruction of a single word.
    // For example in switch statements at the receiving actor, or other.
    Instruction Instruction
    // Data usually carries the data from one process to the next. Example
    // could be a file read on process1 is put in the Data field, and
    // passed on to process2 to be unmarshaled.
    Data []byte `json:"data" yaml:"data" cbor:"data"`
    // Data to be transfered internally. Example is to send config directly via
    // the channel between internal actors.
    InternalCh chan chan []byte `json:"-" yaml:"-" cbor:"-"`
    // Err is used for defining the error message when the event is used
    // as an error event.
    Err error `json:"error" yaml:"error" cbor:"error"`
    // NextEvent defines a series of events to be executed like a workflow.
    // The receiving process should check this field for what kind of event
    // to create as the next step in the workflow.
    NextEvent *Event `json:"nextEvent" yaml:"nextEvent" cbor:"nextEvent"`
    // PreviousEvent allows for keeping information about the previous event if needed.
    PreviousEvent *Event `json:"previousEvent" yaml:"previousEvent" cbor:"previousEvent"`
    // Dst node.
    DstNode Node `json:"dstNode" yaml:"dstNode" cbor:"dstNode"`
    // Src node.
    SrcNode Node `json:"srcNode" yaml:"srcNode" cbor:"srcNode"`
}
```

### Event Functions (ETFunc)

Event Functions holds the logic for what a process shall do when an event is received, and then what to do with the data the event carries. The Event functions are functions that are attached to a process when the process is created.

The programmer can decide if the Process Function should depend on the input from the **in channel** of the process, or just continously do some work on it's own. For an event function to be triggered to work on events it should hold a **for** loop that listens on the Process's `InCh` for new Events.

### Examples

Check out the test files for examples for how to define an Event and it's Process function, or for more complete examples check out the [examples](examples/) folder.

### Quick start

```Go
package main

import (
        "context"
        "fmt"
        "log/slog"
        "os"
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
                slog.Error("main", "act on root failed", err)
                os.Exit(1)
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
```

## Remote delivery

If the DstNode field of an event is set, the event can be sent to the remote node set using the ETRemote process, if that process has been started with an etRemoteFunc, where etRemoteFunc defines how to do remote delivery. If no value is set in the DstNode field, the event will be processed locally.

How this works is that when the routing logic notices that the DstNode field is set, it will create a new event of type ETRemote, put the original event in the NextEvent field of the new ETRemote event, and then the event is added to the queue with the AddEvent method of the Actress. Tip, check the [NextEvent](#nextevent) section for more information about the NextEvent field.

The local ETRemote process will then receive the event, and take the original event that we find in the NextEvent field, use that, and send it to the remote node using the for example DstNode field as the Subject if NATS is used.

The actress.ETRemote event type is already defined in the actress package, but no etRemoteFunc is defined for it. It is up to the programmer to define an etRemoteFunc and start the ETRemote process.

### A high level overview of how registering and starting an ETRemote process works

```go
etRemoteFunc := func(ctx context.Context, p *actress.Process) func() {
        fn := func() {
            for {
                select {
                case ev := <-p.InCh:
                    // The event received here came here since an event was processed,
                    // and a value was set in the DstNode field of the event.
                    // Also, when an event is received here, the type of event is ETRemote,
                    // and the NextEvent field holds the original event that was
                    // processed when it was decided to send it to a remote node.
                    //
                    // We can now take the NextEvent and choose to do what we want with the event.
                    //
                    // The DstNode field holds the name of the remote node. We can then use
                    // that as the topic if we want send the event to a remote node over MQTT.

                    // ..write some code here that will marshal the event to example JSON,
                    //  and send it via MQTT, and use the value defined
                    // in the DstNode field as the topic.
                    //
                    // NB: If for example MQTT is chosen as the communication protocol, we will
                    // also need to define an MQTT receiver Actress/Process that will be able
                    // to receive the event on the remote node.
                    
                case <-ctx.Done():
                    return
                }
            }
        }
        return fn
    }

// Register the event name and event function as a process,
// and start it with the Act() method.
actress.NewProcess(ctx, rootAct, actress.ETRemote, etRemoteFunc).Act()
```

And then what the general MQTT or NATS actreess for the receiving side could look like.

```go

// Define the event name for the MQTT receiver process.
const ETMQTTReceiver actress.Name = "ETMQTTReceiver"

etMQTTReceiverFunc := func(ctx context.Context, p *actress.Process) func() {
        fn := func() {
            go func() {
                // The outline of how an MQTT receiver could be implemented.
                // 1. Connect to MQTT broker.
                // 2. Subscribe to topic.
                // 3. Receive message.
                // 4. Unmarshal message, to get the actress.Event.
                // 5. Use the AddEvent() method to add the event to the queue
                //    of messages to be handled
            }()
            <-ctx.Done():
            return
        }
        return fn
    }

// Register the event name and event function as a process,
// and start it with the Act() method.
actress.NewProcess(ctx, rootAct, ETMQTTReceiver, etMQTTReceiverFunc).Act()
```

An example using two root processes and ETRemote via a channel for communication between them can be found in the test files as **TestETRemoteTwoProcesses**.

## Details

Short intro about the Events.

The events for all processes, both static, dynamic, error, and supervisor uses the same event type and structure.
The event type is identified by the first 2 letters of the Event.Name:

- ET, static events
- ED, dynamic event
- ER, error events
- EC, custom events
- ES, supervisor events

The reason for splitting them up are for **separation**. For example if the event routing logic hangs on static events, it will not affect the other event kinds, so we are able to for example send errors if any of the other routers are having trouble or have massive load.

A router Actress/Process is defined for each of the event types.

### Where and how to use an actor process of a specific type ?

**Static processes**, normally used for processes/actors defined at startup.
**Dynamic processes**, Can be used both for startup and runtime defined actors, but prefer static at startup unless you have a really good reason to not do it :).
**Error processes** For error logging and handling.
**Supervisor processes** For control logic and information about the whole Actress system.

## NextEvent

NextEvent makes it possible to define an event as a chain of Events, similar to piping command output in linux. An example could be that we want to get the content of a web page, and print the result to the screen. We could do that in the following way.

```go
p.AddEvent(Event{Name: Name("ETBleeping"), NextEvent: &Event{Name: ETPrint}})
```

## Dynamic Processes

The purpose of dynamic processes is to have short lived processes that can be quickly started, and removed again when it's job is done. Dynamic processes are not manually given a name, but automatically assigned an UUID to identify it.

A typical example could be that there is a processes that needs to communicate in some other way with another process that can't be done with the current process's event channel. We can then spawn a dynamic process to take care of that. Check out the test files in the examples directory. A process can spawn as many dynamic processes as it needs.
