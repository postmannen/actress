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
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	ac "github.com/postmannen/actress"
)

// Subject is put in Event.cmd[0], and the payload is in Event.Data
const ETNatsPub ac.EventType = "ETNatsPub"
const ETNatsSub ac.EventType = "ETNatsSub"
const NatsSubject string = "mysubject"

// Wrapper function that will return an ETFunc as the end result that will start nats subscriber.
// By creating function that returns an ETFunc, we are able to wrap the *nats.Conn parameter
// into the function signature when we're calling NewProcess later in main() function.
func wrapperETNatsSubscriber(conn *nats.Conn) ac.ETFunc {
	fn := func(ctx context.Context, p *ac.Process) func() {

		subFn := func() {
			// Subscribe for new incomming messages
			go func() {
				sub, err := conn.QueueSubscribe(NatsSubject, NatsSubject, func(msg *nats.Msg) {
					p.AddEvent(ac.Event{
						EventType: ac.ETPrint,
						Data:      []byte(fmt.Sprintf("* nats: got message on subject %v, payload: %v", msg.Subject, string(msg.Data))),
					})
				})
				if err != nil {
					p.AddError(ac.Event{Err: fmt.Errorf("error: failed to nats.QueueSubscribe: %v", err)})
				}
				defer sub.Unsubscribe()
				<-ctx.Done()
			}()
		}

		return subFn
	}

	return fn
}

// Wrapper function that will return an ETFunc as the end result that will start nats publisher.
// By creating function that returns an ETFunc, we are able to wrap the *nats.Conn parameter
// into the function signature when we're calling NewProcess later in main() function.
func wrapperETNatsPublisher(conn *nats.Conn) ac.ETFunc {
	fn := func(ctx context.Context, p *ac.Process) func() {

		pubFn := func() {
			// Publish messages. Subject is put in Event.cmd[0], and the payload is in Event.Data
			for {
				select {
				case ev := <-p.InCh:
					go func() {
						//fmt.Printf("********* GOT EVENT: %v\n", ev)
						err := conn.Publish(ev.Cmd[0], ev.Data)
						if err != nil {
							p.AddError(ac.Event{EventType: ac.ERFatal, Err: fmt.Errorf("error: failed to nats.Publish: %v", err)})
						}
					}()
				case <-ctx.Done():
					return
				}
			}
		}
		return pubFn

	}

	return fn
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	rootAct := ac.NewRootProcess(ctx, nil)
	err := rootAct.Act()
	if err != nil {
		log.Fatal(err)
	}

	// Start up a Nats-server using the ETOsCmd EventType.
	rootAct.AddEvent(ac.Event{
		EventType: ac.ETOsCmd,
		Cmd:       []string{"/bin/bash", "-c", "docker run --rm -p 4222:4222 -i nats:latest"},
		NextEvent: &ac.Event{EventType: ac.ETPrint}})

	// Wait a second so the nats-server is started before we connect the client.
	time.Sleep(time.Second * 2)

	// Since we are using wrapper funcions for the pub sub functions we can connect
	// to the nats server here in main, and pass the conn into wrappers later when
	// we prepare the ETFunc's.
	conn, err := nats.Connect("localhost:4222",
		nats.MaxReconnects(-1),
	)
	if err != nil {
		log.Fatalf("error: nats.Connect failed, %vn", err)

	}
	defer conn.Close()

	// Create a nats sub process.
	err = ac.NewProcess(ctx, *rootAct, ETNatsSub, wrapperETNatsSubscriber(conn)).Act()
	if err != nil {
		log.Fatal(err)
	}
	// Create a nats pub process.
	err = ac.NewProcess(ctx, *rootAct, ETNatsPub, wrapperETNatsPublisher(conn)).Act()
	if err != nil {
		log.Fatal(err)
	}

	// Loop and send Events to the new Nats Client process.
	var nr int
	for {
		time.Sleep(time.Second * 2)
		rootAct.AddEvent(ac.Event{
			EventType: ETNatsPub,
			Cmd:       []string{NatsSubject},
			Data:      []byte(fmt.Sprintf("some data that was put in here: %v", nr)),
		})
		nr++
	}
}
