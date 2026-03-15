# Actress

A Concurrent Actor framework written in Go.

**NB: This is still in the idea phase, so concepts are being tested out and things might/will change rapidly. 
<u>Expect breaking changes between commits</u>**.

## Overview

Create custom processes where what the processes do are either your own piece of code, or it can be a command called from the Operating system. The processes can communicate by sending events to pass the result from one processes to the next for further processing, or by chaining together process as workflows to create a series of Events that together will provide some end result.

### Processes

A process are like a module capable of performing a specific tasks. The nature of the process is determined by an Name and a Function attached to each process. A process have an InCh for receiving events, and an AddEvent function for sending Events. The processes can themselves spawn new processes. Processes can also send Event messages to other processes.

A process can hold state within the Process Function.

### Events

To initiate and trigger the execution of the process's function, we send events. Each process has its own unique event name. Events serve as the communication within the system. They can carry data, either with the result of something a process did to pass it on to the next process for further processing, instructions for what a process should do, or both. An event can contain a chain of events to create workflows of what do do and in what order by using the NextEvent feature (see examples for usage).

```Go
type Event struct {
	Nr int
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

Event Functions holds the logic for what a process shall do when an event is received, and what to do with the data the event carries. The Event functions are callback functions that are executed when a process is created.

The programmer can decide if the Process Function should depend on the input from the input channel of the process, or just continously do some work on it's own. For an event function to be triggered to work on events it should hold a **for** loop that listens on the Process InCh for new Events.

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
    cfg, _ := actress.NewConfig("debug")
    rootAct := actress.NewRootProcess(ctx, nil, cfg, nil)

    // Start the root process/actor.
    err := rootAct.Act()
    if err != nil {
        log.Fatal(err)
    }

    // Create a test channel where we receive the end result.
    testCh := make(chan string)

    // Define two event's for two processes.
    const ETTest1 actress.Name = "ETTest1"
    const ETTest2 actress.Name = "ETTest2"

    // Define the first ETFunc function that will be attached to the ETTest1 Name process.
    test1Func := func(ctx context.Context, p *actress.Process) func() {
        fn := func() {
            for {
                select {
                case ev := <-p.InCh:
                    upper := strings.ToUpper(string(ev.Data))
                    // Pass on the processing to the next process, and use the NextEvent we have specified in main
                    // for the Name, and add the result of ToUpper to the data field.
                    p.AddEvent(actress.Event{Name: ev.NextEvent.Name,
                        
                        Data:      []byte(upper)})
                case <-ctx.Done():
                    return
                }
            }
        }
        return fn
    }

    // Define the second ETFunc function that will be attached to the ETTest2 Name process.
    test2Func := func(ctx context.Context, p *actress.Process) func() {
        fn := func() {
            for {
                select {
                case result := <-p.InCh:
                    dots := string(result.Data) + "..."                 
                    // Send the result on the testCh so we are able to to receive it in main().
                    testCh <- string(dots)

                    // Also create an informational error message.
                    p.AddEvent(actress.Event{Name: actress.ERDebug,
                        
                        Err:       fmt.Errorf("info: done with the acting")})

                case <-ctx.Done():
                    return
                }
            }
        }
        return fn
    }

    // Register the event names and event function as processes,
    // and start them with the Act() method.
    actress.NewProcess(ctx, rootAct, ETTest1,  test1Func).Act()
    actress.NewProcess(ctx, rootAct, ETTest2,  test2Func).Act()

    // Pass in an event destined for an ETTest1 Name process, and also specify
    // the next event to be used when passing the result on from ETTest1 to the next
    // process which here is ETTest2.
    rootAct.AddEvent(actress.Event{Name: ETTest1,
        
        Data:      []byte("test"),
        NextEvent: &actress.Event{Name: ETTest2,
            
    },
    )

    // Wait and receive the result from the ETTest2 process.
    fmt.Printf("The result: %v\n", <-testCh)

    time.Sleep(time.Second * 2)
}
```

## Remote delivery

If the DstNode field of an event is set, the event can be sent to the remote node using the ETRemote process if an ETRemote process has been started, and a etRemoteFunc has been defined for it. If no value is set in the DstNode field, the event will be processed locally.

How this works is that when the routing logic notices that the DstNode field is set, it will create a new event of type ETRemote, and put the original even in the NextEvent field of the new ETRemote event, and the event is added to the queue with the AddEvent method of the Actress. Tip, check the [NextEvent](#nextevent) section for more information about the NextEvent field.

The ETRemote process will then receive the event, and we can take the original event that we find in the NextEvent field, use that, and send it to the remote node using the for example DstNode field as the topic.

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

And then what the general MQTT actreess for the receiving side would look like.

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

## Details

Short intro about the Events.

The events for all processes, both static, dynamic, error, and supervisor uses the same event type and structure.
The even type is identified by the firs 2 letters of the event.Name:

- ET, static events
- ED, dynamic event
- ER, error events
- EC, custom events
- ES, supervisor events

The reason for splitting them up are for **separation** and use of **mutex'es** , for example if the event routing logic hangs on static events, it will not affect the other event kinds, so we are able to for example send errors if any of the other routers are having trouble or have massive load.

A router Actress/Process is defined for each of the event types.

### Where to use an actor process of a specific kind ?

**Static processes**, should be used for processes/actors defined at startup.
**Dynamic processes**, Can be used both for startup and runtime defined actors, but prefer static at startup unless you have a really good reason to not do it :).
**Error processes** For error logging and handling.
**Supervisor processes** For control logic and information about the whole Actress system.

## NextEvent

NextEvent makes it possible to define an event as a chain of Events. An example could be that we want to get the content of a web page, and print the result to the screen. We could do that in the following way.

```go
p.AddEvent(Event{Name: Name("ETBleeping"), NextEvent: &Event{Name: ETPrint}})
```

## Dynamic Processes

The purpose of dynamic processes is to have short lived processes that can be quickly started, and removed again when it's job is done. The only difference between a Static process and a Dynamic process are that the dynamic processes have a mutex in the DynamicProcesses map so that we can delete the processes when they are no longer needed at runtime withhout causing a datarace.

A typical example could be that there is a processes that needs to communicate in some other way with another process that cant be done with the current process's event channel. We can then spawn a dynamic process to take care of that. Check out the test and files in the examples directory. A process can spawn as many dynamic processes as it needs.
