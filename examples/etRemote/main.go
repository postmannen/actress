package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/postmannen/actress"
)

const readBufferSize = 8192
const addressString = "239.0.0.0:9999"

func etRemoteFn(ctx context.Context, p *actress.Process) func() {
	fn := func() {

		// Start the multicast receiver
		err := actress.NewProcess(ctx, p, ETMulticastReceive, etMulticastReceive).Act()
		if err != nil {
			log.Fatal(err)
		}
		// Start the multicast sender
		err = actress.NewProcess(ctx, p, ETMulticastSend, etMulticastSend).Act()
		if err != nil {
			log.Fatal(err)
		}

		for {
			p.SignalReady()

			select {
			// Wait for ETRemote events, that are wrapped around the actual
			// event that should be forwarded, and the actual event is to be
			// found in ev.NextEvent
			case ev := <-p.InCh:
				// Get the actual event that is wrapped in the ETRemote event.
				nextEv := ev.NextEvent
				// Set the .SrcNode so the remote node could check where the
				// event came from originally before we pass it on.
				// We now wrap the whole "original" event in an ETMulticast
				// send event, and we put the original event in the NextEvent
				// field.
				nextEv.SrcNode = p.Config.NodeName
				multicastEV := actress.Event{
					Name:      ETMulticastSend,
					NextEvent: nextEv,
				}

				p.AddEvent(multicastEV)
			case <-p.Ctx.Done():
				return
			}
		}
	}
	return fn
}

const ETMulticastSend actress.EventName = "ETMulticastSend"

func etMulticastSend(ctx context.Context, p *actress.Process) func() {
	fn := func() {
		addr, err := net.ResolveUDPAddr("udp4", addressString)
		if err != nil {
			log.Fatalf("failed to resolve udp address\n")
		}
		conn, err := net.DialUDP("udp4", nil, addr)
		if err != nil {
			log.Fatalf("dial multicast failed: %v\n", err)
		}
		defer conn.Close()

		for {
			select {
			case ev := <-p.InCh:
				evNextEvent := ev.NextEvent
				// The nextEvent received here is the original event as it was before it
				// was encapsulated in an ETRemote, and sent to here, and the DstNode field
				// is still set there. This can be useful if we wanted to use the name of
				// the DstNode when creating a mqtt topic or nats subject, but now with
				// multicast we don't really need it.
				// Either way, we should blank it out before sending the event, so we don't
				// end in a situation where the ETRemote loops sending back and forth.
				evNextEvent.DstNode = ""

				b, err := json.Marshal(evNextEvent)
				if err != nil {
					log.Fatalf("error: failed to marshal json data to send: %v\n", err)
				}

				_, err = conn.Write(b)
				if err != nil {
					log.Fatalf("error: failed to send multicast data\n")
				}

			case <-ctx.Done():
				return
			}
		}
	}
	return fn
}

const ETMulticastReceive actress.EventName = "ETMulticastReceive"

func etMulticastReceive(ctx context.Context, p *actress.Process) func() {
	fn := func() {
		addr, err := net.ResolveUDPAddr("udp4", addressString)
		if err != nil {
			log.Fatalf("failed to resolve udp address\n")
		}

		// Open up a connection
		conn, err := net.ListenMulticastUDP("udp4", nil, addr)
		if err != nil {
			log.Fatalf("error: filed to set up mutlicast listener: %v\n", err)
		}

		for {
			buffer := make([]byte, readBufferSize)
			n, src, err := conn.ReadFromUDP(buffer)
			if err != nil {
				log.Fatalf("error: failed to read udp connection: %v\n", err)
			}
			fmt.Printf("info on %v: read udp package, from: %v\n", p.Config.NodeName, src)

			ev := actress.Event{}
			err = json.Unmarshal(buffer[:n], &ev)
			if err != nil {
				log.Fatalf("error: failed to unmarshal read udp data %v\n", err)
			}

			p.AddEvent(ev)
		}
	}
	return fn
}

const ETTest1 actress.EventName = "ETTest1"

// Define the first function that will be attached to the ETTest1 EventType process.
func test1Func(ctx context.Context, p *actress.Process) func() {
	fn := func() {
		for {
			select {
			case ev := <-p.InCh:
				upper := strings.ToUpper(string(ev.Data))
				// Pass on the processing to the next process, and use the
				// NextEvent we have specified in main for the EventType,
				// and add the result of ToUpper to the data field.
				p.AddEvent(actress.Event{
					Name:    ev.NextEvent.Name,
					Data:    []byte(upper),
					DstNode: ev.NextEvent.DstNode,
				})

			case <-ctx.Done():
				return
			}
		}
	}
	return fn
}

const ETTest2 actress.EventName = "ETTest2"

// Define the second function that will be attached to the ETTest2 EventType
// process.
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
	// Quick intro:
	// - We start etTest1 only on actress system "one".
	// - We start etTest2 both on "one" and "two".
	// - From "one" we deliver and event to etTest1, which will initiate a broadcast.
	// - The broadcasted message should be received on the etTest2 function of both
	// 	 actress system "one" and "two".

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	cfg1, _ := actress.NewConfig("info")
	cfg1.NodeName = "one"
	rootAct1 := actress.NewRootProcess(ctx, nil, cfg1)

	// Register the event types and event function to processes.
	oneAc1 := actress.NewProcess(ctx, rootAct1, ETTest1, test1Func)
	oneAc1.Act()
	oneAc2 := actress.NewProcess(ctx, rootAct1, ETTest2, test2Func)
	oneAc2.Act()
	actress.NewProcess(ctx, rootAct1, actress.ETRemote, etRemoteFn).Act()

	// Start all the registered processes on actress system 2
	err := rootAct1.Act()
	if err != nil {
		log.Fatal(err)
	}

	// -----------

	// Create a new root process.
	cfg2, _ := actress.NewConfig("info")
	cfg2.NodeName = "two"
	rootAct2 := actress.NewRootProcess(ctx, nil, cfg2)

	// Register the event types and event function to processes.
	twoAc2 := actress.NewProcess(ctx, rootAct2, ETTest2, test2Func)
	twoAc2.Act()
	actress.NewProcess(ctx, rootAct2, actress.ETRemote, etRemoteFn).Act()

	// Start all the registered processes on actress system 2
	err = rootAct2.Act()
	if err != nil {
		log.Fatal(err)
	}

	// -----------------

	// Pass in an event destined for an ETTest1 EventType process, and also specify
	// the next event to be used when passing the result on from ETTest1 to the next
	// process which here is ETTest2. We want the next event to be handled on another
	// node, so we specify the DstNode of the NextEvent to some random value so it will
	// be sent via the ETRemote function.
	// Since multicast don't use a specific node's name, we can just write whatever we
	// want. The trick is that DstNode needs to contain something so the system thinks
	// it is to be delivered elsewhere and handled with the ETRemote actor.
	rootAct1.AddEvent(actress.Event{Name: ETTest1,
		Data:      []byte("test data from one"),
		NextEvent: &actress.Event{Name: ETTest2, DstNode: "doesntReallyMatterwWeremMulticasting"},
	},
	)

	// Wait and receive the result from the ETTest processes.
	ev2 := <-twoAc2.TestCh
	ev1 := <-oneAc2.TestCh
	fmt.Printf("The result: one: %s, two: %s\n", ev1.Data, ev2.Data)
}
