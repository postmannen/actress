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
const ETNatsClient ac.EventType = "ETNatsClient"
const ETNatsPub ac.EventType = "ETNatsPub"
const ETNatsSub ac.EventType = "ETNatsSub"

const NatsSubject string = "mysubject"

// Start both a nats subscriber and a nats publisher
func etNatsClientFunc(ctx context.Context, p *ac.Process) func() {
	fn := func() {
		// Connect to the nats server
		conn, err := nats.Connect("localhost:4222",
			nats.MaxReconnects(-1),
		)
		if err != nil {
			p.AddEvent(ac.Event{EventType: ac.ERFatal,
				EventKind: ac.EventKindError,
				Err:       fmt.Errorf("error: nats.Connect failed, %vn", err)})

		}
		defer conn.Close()

		subFn := func(ctx context.Context, p *ac.Process) func() {
			fn := func() {
				// Subscribe for new incomming messages
				go func() {
					sub, err := conn.QueueSubscribe(NatsSubject, NatsSubject, func(msg *nats.Msg) {
						p.AddEvent(ac.Event{
							EventType: ac.ETPrint,
							EventKind: ac.EventKindStatic,
							Data:      []byte(fmt.Sprintf("* nats: got message on subject %v, payload: %v", msg.Subject, string(msg.Data))),
						})
					})
					if err != nil {
						p.AddEvent(ac.Event{
							EventType: ac.ERFatal,
							EventKind: ac.EventKindError,
							Err:       fmt.Errorf("error: failed to nats.QueueSubscribe: %v", err)})
					}
					defer sub.Unsubscribe()
					<-ctx.Done()

				}()

			}

			return fn
		}

		pubFn := func(ctx context.Context, p *ac.Process) func() {
			fn := func() {
				// Publish messages. Subject is put in Event.cmd[0], and the payload is in Event.Data
				for {
					select {
					case ev := <-p.InCh:
						go func() {
							//fmt.Printf("********* GOT EVENT: %v\n", ev)
							err := conn.Publish(ev.Cmd[0], ev.Data)
							if err != nil {
								p.AddEvent(ac.Event{EventType: ac.ERFatal,
									EventKind: ac.EventKindError,
									Err:       fmt.Errorf("error: failed to nats.Publish: %v", err)})
							}
						}()
					case <-ctx.Done():
						return
					}
				}
			}
			return fn
		}

		ac.NewProcess(ctx, p, ETNatsSub, subFn).Act()
		ac.NewProcess(ctx, p, ETNatsPub, pubFn).Act()

		<-ctx.Done()
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
		EventKind: ac.EventKindStatic,
		Cmd:       []string{"/bin/bash", "-c", "docker run --rm -p 4222:4222 -i nats:latest"},
		NextEvent: &ac.Event{EventType: ac.ETPrint}})

	// Wait a second so the nats-server is started before we connect the client.
	time.Sleep(time.Second * 2)

	// Create a nats client process.
	err = ac.NewProcess(ctx, rootAct, ETNatsClient, etNatsClientFunc).Act()
	if err != nil {
		log.Fatal(err)
	}

	// Loop and send Events to the new Nats Client process.
	var nr int
	for {
		rootAct.AddEvent(ac.Event{
			EventType: ETNatsPub,
			EventKind: ac.EventKindStatic,
			Cmd:       []string{NatsSubject},
			Data:      []byte(fmt.Sprintf("some data that was put in here: %v", nr)),
		})
		nr++
		time.Sleep(time.Second * 2)
	}
}
