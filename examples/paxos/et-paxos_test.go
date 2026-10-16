package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/postmannen/actress"
)

// verifyETRemoteTransport verifies that an event sent from one node really is
// delivered to the ETPaxosAcceptor... it uses the ETTest actor which
// echoes ev.Data back on testCh when it processes the event.
//
// NATS core without ack (AckTimeout < 1) silently drops messages
// published to a subject that has no subscribers yet. The node actors
// connect to the NATS server asynchronously in the background, so the
// first paxos round would otherwise race the subscribers coming up and
// randomly lose the Promise replies on their way back to root0.
//
// The probe is re-sent until it comes back, which proves that both
// publisher and subscriber legs between the two nodes are up.
func verifyETRemoteTransport(t *testing.T, testCh chan string, fromNode *actress.Process, toNode actress.Node) {
	t.Helper()

	marker := fmt.Sprintf("transport-probe:%v->%v", fromNode.Config.NodeName, toNode)
	deadline := time.Now().Add(15 * time.Second)

	for attempt := 1; ; attempt++ {
		if time.Now().After(deadline) {
			t.Fatalf("probe %v never reached %v, the transport is not up", marker, toNode)
		}

		fromNode.AddEvent(actress.Event{
			Name:    actress.ETTest,
			DstNode: toNode,
			Data:    []byte(marker),
		})

		err := func() error {
			for {
				select {
				case got := <-testCh:
					if got == marker {
						if attempt > 1 {
							t.Logf("probe %v reached %v on attempt %d", marker, toNode, attempt)
						}
						return nil
					}
					// A stale copy of an earlier probe, ignore and keep waiting.
				case <-time.After(time.Millisecond * 50):
					// Not delivered yet, send the probe again.
					return fmt.Errorf("retry")
				}
			}
		}()
		if err == nil {
			return
		}
	}
}

func TestPaxosInitial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
	}()

	// A test channel that is given to all the root actors, that allow
	// us to receive messages directly from the root processes via a
	// channel, and not as an event via the event routers.
	testCh := make(chan string, 16)

	nrOfRoots := 10

	// Start up the number of roots specified. The number 0->nrOfRoots-1 will be appended to the node name.
	// roots := newRootsValdDoAct(ctx, "root", "none", testCh, nrOfRoots)
	roots := make(map[string]*actress.Process)
	remoteCh := make(chan actress.Event)

	for i := 0; i < nrOfRoots; i++ {
		rootName := actress.Node(fmt.Sprintf("root%v", i))

		c := actress.Config{
			LogLevel: "info",
			NodeName: rootName,
		}
		p := actress.NewRootProcess(ctx, nil, &c)
		roots[string(rootName)] = p

		// Start up the essential standard actors needed.
		actress.NewProcess(ctx, p, actress.ETChannelRead, actress.ETChannelReadFn(remoteCh)).Act()
		actress.NewProcess(ctx, p, actress.ETChannelWrite, actress.ETChannelWriteFn(remoteCh)).Act()
		actress.NewProcess(ctx, p, actress.ETRemote, actress.ETRemoteChannelReadWriteFn()).Act()
		actress.NewProcess(ctx, p, actress.ETTest, actress.ETTestfn(testCh)).Act()

		p.Act()
	}

	// Get the first root, and use this to send the event to all the other roots.
	root0 := roots["root0"]

	paxosNodes := []string{}

	for _, v := range roots {
		paxosNodes = append(paxosNodes, string(v.Config.NodeName))
	}

	// Start up the paxos actors needed for each root node.
	for _, v := range roots {
		actress.NewProcess(ctx, v, ETPaxos, ETPaxosFn(paxosNodes)).Act()
		actress.NewProcess(ctx, v, ETPaxosAcceptor, ETPaxosAcceptorFn()).Act()
		actress.NewProcess(ctx, v, ETPaxosLearner, ETPaxosLearnerFn(paxosNodes)).Act()
		fmt.Printf("--- Started paxos on: %v\n", v.Config.NodeName)
	}

	// Prove the transport for every node in both directions before
	// starting the paxos round, so no paxos event is published into the
	// startup window where the destination subject has no subscriber yet.
	for name, r := range roots {
		verifyETRemoteTransport(t, testCh, root0, actress.Node(name))
		verifyETRemoteTransport(t, testCh, r, "root0")
	}

	paxosInitial := PaxosInitial{
		Data:  []byte("some value to reach concencus on.........."),
		Nodes: paxosNodes,
	}

	b, err := json.Marshal(paxosInitial)
	if err != nil {
		t.Fatalf("ETPaxosFn: marshaling paxosInitial failed: %v\n", err)
	}

	root0.AddEvent(actress.Event{
		Name:        ETPaxos,
		Instruction: InstructionPaxosNewValue,
		Data:        b,
	})

	// TODO: Add a proper check here, that we have received all the promises.
	time.Sleep(time.Second * 3)
}
