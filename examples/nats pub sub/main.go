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
const NatsSubject string = "mysubject"

// Start both a nats subscriber and a nats publisher
func etNatsClientFunc(ctx context.Context, p *ac.Process) func() {
	fn := func() {
		// Connect to the nats server
		conn, err := nats.Connect("localhost:4222",
			nats.MaxReconnects(-1),
		)
		if err != nil {
			p.AddErr(ac.Event{EventType: ac.ERFatal, Err: fmt.Errorf("error: nats.Connect failed, %vn", err)})

		}
		defer conn.Close()

		// Subscribe for new incomming messages
		go func() {
			sub, err := conn.QueueSubscribe(NatsSubject, NatsSubject, func(msg *nats.Msg) {
				p.AddStd(ac.Event{
					EventType: ac.ETPrint,
					Data:      []byte(fmt.Sprintf("* nats: got message on subject %v, payload: %v", msg.Subject, string(msg.Data))),
				})
			})
			if err != nil {
				p.AddErr(ac.Event{Err: fmt.Errorf("error: failed to nats.QueueSubscribe: %v", err)})
			}
			defer sub.Unsubscribe()
			<-ctx.Done()

		}()

		// Publish messages. Subject is put in Event.cmd[0], and the payload is in Event.Data
		for {
			select {
			case ev := <-p.InCh:
				go func() {
					//fmt.Printf("********* GOT EVENT: %v\n", ev)
					err := conn.Publish(ev.Cmd[0], ev.Data)
					if err != nil {
						p.AddErr(ac.Event{EventType: ac.ERFatal, Err: fmt.Errorf("error: failed to nats.Publish: %v", err)})
					}
				}()
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
	rootAct := ac.NewRootProcess(ctx)
	err := rootAct.Act()
	if err != nil {
		log.Fatal(err)
	}

	// Start up a Nats-server using the ETOsCmd EventType.
	rootAct.AddStd(ac.Event{
		EventType: ac.ETOsCmd,
		Cmd:       []string{"/bin/bash", "-c", "docker run --rm -p 4222:4222 -i nats:latest"},
		NextEvent: &ac.Event{EventType: ac.ETPrint}})

	// Wait a second so the nats-server is started before we connect the client.
	time.Sleep(time.Second * 2)

	// Create a nats client process.
	err = ac.NewProcess(ctx, *rootAct, ETNatsClient, etNatsClientFunc).Act()
	if err != nil {
		log.Fatal(err)
	}

	// Loop and send Events to the new Nats Client process.
	var nr int
	for {
		rootAct.AddStd(ac.Event{
			EventType: ETNatsClient,
			Cmd:       []string{NatsSubject},
			Data:      []byte(fmt.Sprintf("some data that was put in here: %v", nr)),
		})
		nr++
		time.Sleep(time.Second * 2)
	}
}
