package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/postmannen/actress"
)

// ----------------------------------------------------------------------------------------
// Paxos acceptor
// ----------------------------------------------------------------------------------------

type paxosAcceptor struct {
	// Highest prepare round nr that have answered with a PROMISE for
	highestPromisedNr uint
	// Highest accept round nr that was sent out an ACCEPT for
	acceptedRoundNr uint
	// The accepted value for the accepted round.
	acceptedValue []byte
	// knownNodes are all the nodes we've learned of.
	knownNodes []string
}

func newPaxosAcceptor() *paxosAcceptor {
	p := paxosAcceptor{}

	return &p
}

type RoundAndValue struct {
	RoundNr uint
	Value   []byte
}

type PromiseData struct {
	RoundNR          uint
	PreviousAccepted RoundAndValue
}

type NackData struct {
	HighestPromisedOnAcceptor uint
}

const ETPaxosAcceptor actress.EventName = "ETPaxosAcceptor"

func ETPaxosAcceptorFn() actress.ETFunc {
	fn := func(ctx context.Context, p *actress.Process) func() {
		fn := func() {
			p.SignalReady()

			acceptor := newPaxosAcceptor()
			var prepareDataProcessName string

			for {
				select {
				case ev := <-p.InCh:

					switch ev.Instruction {

					case InstructionPaxosPrepare:

						prepareData := PrepareData{}
						err := json.Unmarshal(ev.Data, &prepareData)
						if err != nil {
							slog.Error("ETPaxosAcceptorFn", "marshaling PrepareData failed", err)
							continue
						}

						// NB: As of now, it does not exist any servide that let us learn
						// about what other nodes that are out there, so for now we learn
						// about other nodes via the prepare message from the proposer.
						acceptor.knownNodes = prepareData.Nodes

						// set variable for process name outside the switch, so other case's
						// also can use the value
						prepareDataProcessName = prepareData.ProcessName

						// The different cases decide what to do with the received PREPARE,
						// and if we should send a PROMISE, or not.
						switch {
						// Less than previous promised, send NACK
						case prepareData.RoundNr < acceptor.highestPromisedNr:
							nackData := NackData{
								HighestPromisedOnAcceptor: acceptor.highestPromisedNr,
							}

							b, err := json.Marshal(nackData)
							if err != nil {
								slog.Error("ETPaxosAcceptorFn", "marshaling NackData failed", err)
								continue
							}

							promiseEV := actress.Event{
								Name:        actress.EventName(prepareData.ProcessName),
								Instruction: InstructionPaxosPromiseNACK,
								Data:        b,
								DstNode:     ev.SrcNode,
								SrcNode:     p.Config.NodeName,
							}
							p.AddEvent(promiseEV)
							fmt.Printf("ETPaxosAcceptorFn, on %v, less than highest promised: %v\n", p.Config.NodeName, ev.SrcNode)

						// Larger than previous, send new promise
						case prepareData.RoundNr > acceptor.highestPromisedNr:
							// Setting highest promised the new highest we just received.
							acceptor.highestPromisedNr = prepareData.RoundNr

							promiseData := PromiseData{
								RoundNR: prepareData.RoundNr,
								PreviousAccepted: RoundAndValue{
									acceptor.acceptedRoundNr,
									acceptor.acceptedValue,
								},
							}

							b, err := json.Marshal(promiseData)
							if err != nil {
								slog.Error("ETPaxosAcceptorFn", "marshaling PromiseData failed", err)
								continue
							}

							promiseEV := actress.Event{
								Name:        actress.EventName(prepareData.ProcessName),
								Instruction: InstructionPaxosPromise,
								Data:        b,
								DstNode:     actress.Node(ev.SrcNode),
								SrcNode:     p.Config.NodeName,
							}
							p.AddEvent(promiseEV)

							fmt.Printf("ETPaxosAcceptorFn, on %v, larger than, sending Promise to: %v\n", p.Config.NodeName, ev.SrcNode)

						// Equal to last promised, send last promised again
						case prepareData.RoundNr == acceptor.highestPromisedNr:
							// TODO: According to the paxos spec, we should also add the proposer
							// ID to the round nr, to be able sort if more nodes are using the
							// same number.
							promiseData := PromiseData{
								RoundNR: prepareData.RoundNr,
								PreviousAccepted: RoundAndValue{
									acceptor.acceptedRoundNr,
									acceptor.acceptedValue,
								},
							}

							b, err := json.Marshal(promiseData)
							if err != nil {
								slog.Error("ETPaxosAcceptorFn", "marshaling PromiseData failed", err)
								continue
							}

							promiseEV := actress.Event{
								Name:        actress.EventName(prepareData.ProcessName),
								Instruction: InstructionPaxosPromise,
								Data:        b,
								DstNode:     ev.SrcNode,
								SrcNode:     p.Config.NodeName,
							}
							p.AddEvent(promiseEV)

							fmt.Printf("ETPaxosAcceptorFn, on %v, equal round, sending Promise to: %v\n", p.Config.NodeName, ev.SrcNode)

						}

					case InstructionPaxosAccept:

						roundAndValue := RoundAndValue{}
						err := json.Unmarshal(ev.Data, &roundAndValue)
						if err != nil {
							slog.Error("ETPaxosAcceptorFn", "unmarshaling accept RoundAndValue failed", err)
						}

						if roundAndValue.RoundNr >= acceptor.highestPromisedNr {
							for _, v := range acceptor.knownNodes {
								acceptor.acceptedRoundNr = roundAndValue.RoundNr
								acceptor.acceptedValue = roundAndValue.Value

								// Send Accepted message back to the proposer
								acceptedEV := actress.Event{
									Name:        actress.EventName(prepareDataProcessName),
									Instruction: InstructionPaxosAccepted,
									// NB: It seems unnecessary to send back both the round and the value
									// to the proposer as the accepted data, since it already have this
									// information already. But sending it with for now incase it makes
									// value with debugging etc.
									Data:    ev.Data,
									DstNode: actress.Node(v),
									SrcNode: p.Config.NodeName,
								}
								p.AddEvent(acceptedEV)

								// Send learn message back to the learner
								learnedEV := actress.Event{
									Name:        ETPaxosLearner,
									Instruction: InstructionPaxosLearn,
									// NB: It seems unnecessary to send back both the round and the value
									// to the proposer as the accepted data, since it already have this
									// information already. But sending it with for now incase it makes
									// value with debugging etc.
									Data:    ev.Data,
									DstNode: actress.Node(v),
									SrcNode: p.Config.NodeName,
								}
								p.AddEvent(learnedEV)
							}

						}

						// fmt.Printf("-----------GOT ACCEPT ON ACCEPTOR, from: %v-------------, round: %v, value: %s\n", ev.SrcNode, roundAndValue.RoundNr, roundAndValue.Value)
					}

				case <-ctx.Done():
				}
			}
		}

		return fn
	}

	return fn
}
