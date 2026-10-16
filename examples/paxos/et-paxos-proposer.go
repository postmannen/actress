//go:build !basic

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/postmannen/actress"
)

// func init() {
// 	etNameAndFuncs = append(etNameAndFuncs, etNameAndFunc{
// 		etName:  ETNatsServer,
// 		etFunc:  func(cfg *Config) actress.ETFunc { return etNatsServerFn(cfg) },
// 		license: "standard",
// 	})
//
// 	// registerFlagSet(func(cfg *Config) *flag.FlagSet {
// 	// 	fs := flag.NewFlagSet("etconfig", flag.ContinueOnError)
// 	// 	fs.IntVar(&cfg.ETNatsServer.Port, "etNatsServerPort", CheckEnv("ETNATS_SERVER_PORT", cfg.ETNatsServer.Port).(int), "nats server port (env ETNATS_SERVER_PORT)")
// 	// 	fs.IntVar(&cfg.ETNatsServer.WSPort, "etNatsServerWSPort", CheckEnv("ETNATS_SERVER_WSPORT", cfg.ETNatsServer.WSPort).(int), "nats server websocket port (env ETNATS_SERVER_WSPORT)")
// 	// 	fs.StringVar(&cfg.ETNatsServer.Address, "etNatsServerAddress", CheckEnv("ETNATS_SERVER_ADDRESS", cfg.ETNatsServer.Address).(string), "nats server address (env ETNATS_SERVER_ADDRESS)")
// 	// 	return fs
// 	// })
// }

// Phase 1.
//
// (a) Proposer:
// A proposer selects a proposal number 'n' and sends a 'prepare'
// request with the number 'n' to a majority of acceptors.
// The Proposer will then wait for responses.
//
// (b) Acceptor:
// If an acceptor recieve a 'prepare' request with number 'n' greater than
// that of any prepare request to which it has already responded,
// then it responds to the request with a 'promise', and with that it promiseses
// not to accept any more proposals numbered less than 'n' and with the
// highest-numbered proposal, if any, that it has accepted.
// If it have already accepted a round earlier, it will include the round
// and it's value in the promise.
// ' if n > highest_prepare_request_promised, then we send out a promise'
//
// proposer -> "prepare_request{proposal number}" -> acceptor
// acceptor -> "promise_requst{highest_number_accepted, highest_value_accepted}" -> proposer
//
// -------------------------------------------------------------------------
//
// Phase 2.
//
// (a) Proposer:
//
// If the proposer receives a response to it's 'prepare' request,
// numbered n, from a *majority* of acceptors, then it sends an accept
// request to each of those acceptors for a proposal numbered 'n', with a
// value 'v', where v is the value of the highest-numbered proposal among
// the responses, or is any value if the responses reported no proposals.
//
// If the Promises received contains an earlier accepted message, it will
// take the value from the highest numbered accepted proposal from the
// Promises received, and put it in instead of it's own value, but keeping
// it's current round number, and send that in the Accept message to the
// acceptor.
//
// (b) Acceptor
//
// If an acceptor receives an 'accept' request for a proposal numbered
// 'n', it accepts the Accept message unless it has already responded to a prepare
// request having a number greater than 'n'. Said with other words, it
// will only send out an Accepted message if n is greater or equal to it's current
// highest_promised number.
// n >= highest_promised number, will result in send Accepted message to
// the proposer.
//
// (a) Proposer
//
// The proposer will then gather all the accepted messages, and when it
// have reached a majority of accepted messages, it will set the chosen
// value to true, and inform the user/client.
//
// About learners.
//
// NB: For how to inform the learners.
// 1. The Proposer can wait for a majority of accepted messages, and then
// inform the learner.
// 2. The Acceptor, when it sends out it's accepted message, can also send
// a message to the learner about the round and value, and the learner will
// wait for a quorum on that before it sets it as learned.
//
// With 2, if a learner has received at least one accepted message for a
// proposal, but does not reach a quorum within a timeout, it is a strong
// indication that messages were lost. The learner should then ask a
// proposer to run a new round with a higher proposal number. Phase 1 of
// that round will surface the already chosen value, if any, and phase 2
// will cause the acceptors to send accepted messages again, so the
// learner gets a new chance to reach a quorum.

// proposer -> "accept_request{n, value_v}" -> acceptor
// acceptor -> "accepted{TODO?}"
//
// -------------------------------------------------------------------------
//
// Phase 3.
// To learn that a value have been chosen, a learner must find out that a
// proposal has been accepted by the majority of acceptors. The algorithm
// is to have each acceptor, whenever it accepts a proposal, respond to all
// learners, sending them the proposal.
//
// acceptor -> "n, value" -> learner
//
// Comment: This should probably be sent out by the proposers when recevied accepted
// from a majority, and not from the acceptors, since the don't know if majority is
// is reached ?

// ----------------------------------------------------------------------------------------
// Paxos
// ----------------------------------------------------------------------------------------

// 1a.
// The proposer gets a value from the client it wants to reach concencus on.
// - (With basic paxos: The proposer pick's a new previous_ballot+1, creates a prepare message,
// and sends that over to the acceptor.)
// - (With Multi Paxos: it reuses the same ballot that that have been reached a promise
//	 on in an earlier phase1.)
//
// 1b.
// The acceptor checks if it is ok to promise the prepare message by:
// - checking >= highest_promised_ballot
// 	 - if ok, the highest_promised_ballot gets updated to the new promised.
// 	   By this it promises that it will never accept any future proposals
// 	   numbered less than B.
//     lower than previously primised or accepted.
//     if nok, the message is discared, and we could send a nack back.
// The proposer will then collect the promises until it reaches quorum.

// If an Acceptor received a prepare, but it have already accepted a message,
// the acceptor will then send a promise back including the accepted ballot and value.
// The promised received from the different acceptors can all possibly contain
// different old accepted values. The proposer will then wait until it have enough
// promises to reach quorum, then it will pick the already accepted value with the
// highest accepted ballot number, and use that in a new accept message.
// The new accept message will use our current ballot number that we sent out
// and received promises on, but the value used will be the already accepted one
// from the promise messages with the highest accepted ballot number.
//
// 2a.
// The proposer sends Accept(ballot, value) to the acctor, where ballot is as
// mentioned above the current (own) ballot number, and:
// - it's own value if no preious accepted values were received in the promise messages.
// - the highest previously accepted value in the accepted messages we reached quorom on.
//
// 2b.
// The acceptor checks if the ballot number B in the accept message is equal to or higher
// than any previously promised numbers by the acceptor:
// B>=highest_promised_ballot
// The acceptor will then update it's accepted V and B to the ones it just received,
// and send an Accepted(B,V) back to the proposer and learners.
// Both the proposer and the learners will wait for quorum of accept messages before
// they register the value as accepted. When quorum on accepted messages is reached,
// the proposer will inform the client/user that the value was chosen.
//
// NB: It is also possible to let the proposer send out the messages to the learners when
// the proposer knows that a message is accepted. But then the proposer becomes a single
// point of failure, and the learners might never learn the value if the proposers dies.
// It all becomes a trade off between more (less) network traffic, and correctness.
//
// NOTE: in multi paxos a new proposer will be selected if the main one dies.
// How timeouts work:
// All nodes (inactive-Proposers/acceptors/learners) that are not the leader expect to
// get heartbeats from the leader node. This can f.ex. be empty accept messages.
// If the other nodes notices that it's been to long since they heard from the leader:
// - It will appoint itself as the new leader
// - It will then raise the global ballot+1 used with multipaxos
// - It will then do a complet phase 1 with the new ballot (previous ballot+1)
// - Since that promise will contain a previous accepted value, the previous accepted
//	 value will be accepted again, and the learners will will learn the value, even
//	 though the previous proposer crashed before the learners had learned anything
// NB: It can be smart to have different timeouts on all, to lessen the chance of two
// nodes trying to be leader at the same time.

type PaxosInitial struct {
	Data  []byte
	Nodes []string
}

const InstructionPaxosNewValue actress.Instruction = "InstructionPaxosNewValue"

const ETPaxos actress.EventName = "ETPaxos"

func ETPaxosFn(paxosNodes []string) actress.ETFunc {
	fn := func(ctx context.Context, p *actress.Process) func() {
		fn := func() {
			p.SignalReady()

			for {
				select {
				case ev := <-p.InCh:

					switch ev.Instruction {
					// We got new value to be reach concensus on.
					//
					// - The initial event with the value to reach concensus on,
					//	 should be given to the proposer with:
					//	 - Name : ETPaxosProposer
					//	 - Instruction : InstructionPaxosAddValue
					// 	 - The value should be in the Data field of the event.
					case InstructionPaxosNewValue:

						// Start up a dynamic actor for the value to be handled
						// by paxos.

						uuid := actress.NewUUID()
						dynP := actress.NewProcess(ctx, p, actress.EventName(uuid), ETPaxosProposerFn())
						dynP.Act()

						p.AddEvent(actress.Event{
							Name:        actress.EventName(uuid),
							Instruction: InstructionPaxosAddValue,
							Data:        ev.Data,
						})

					}

				case <-ctx.Done():
				}
			}
		}

		return fn
	}

	return fn
}

// ----------------------------------------------------------------------------------------
// Paxos proposer
// ----------------------------------------------------------------------------------------

// paxosProposer holds the state needed for concencus to be
// reached.
type paxosProposer struct {
	// Highest number that a prepare have been started with.
	lastTriedRoundNr uint
	receivedPromises uint
	// value to propose. This might change during rounds if an
	// acceptor tells about a value with a higher promised
	// round nr.
	value []byte
	// PreviousAccepted will hold the highest round and value we've
	// got'n during any promise rounds, if any.
	previousAccepted RoundAndValue
	// receivedAccepted is used to count the number of received
	// accepted messages, so we know when quorum is reached.
	receivedAccepted uint
	// acceptedValue will hold the value accepted, after we've reached
	// quorum and received a majority of accepted messages in phase2.
	acceptedValue []byte
	// knownNodes are all the nodes we've learned of.
	knownNodes []string
	// quorumReached is set to true when quorum is reached.
	quorumReached bool
}

func newPaxosProposer() *paxosProposer {
	p := paxosProposer{
		// TODO: According to spec this should be persisted disk.
		lastTriedRoundNr: 0,
		value:            []byte{},
	}

	return &p
}

type PrepareData struct {
	RoundNr     uint
	NodeName    string
	ProcessName string
	// Nodes is used to inform the acceptors of what nodes is out there.
	// NB: This should be replaced with something like a gossip protocol
	// who learns about what nodes is out there by itself, as a service.
	Nodes []string
}

const InstructionPaxosAddValue actress.Instruction = "InstructionPaxosAddValue"
const InstructionPaxosPrepare actress.Instruction = "InstructionPaxosPrepare"
const InstructionPaxosPromise actress.Instruction = "InstructionPaxosPromise"
const InstructionPaxosPromiseNACK actress.Instruction = "InstructionPaxosPromiseNACK"

const InstructionPaxosAccept actress.Instruction = "InstructionPaxosAccept"
const InstructionPaxosAccepted actress.Instruction = "InstructionPaxosAccepted"

const InstructionPaxosLearn actress.Instruction = "InstructionPaxosLearn"

func ETPaxosProposerFn() actress.ETFunc {
	fn := func(ctx context.Context, p *actress.Process) func() {
		fn := func() {

			proposer := newPaxosProposer()

			p.SignalReady()

			// promisesReceived is used to know what nodes to sent an accept
			// message to when quorum is reached, and this map will contain
			// the nodes that promised before quorum was reached, so we also
			// can send out accept messages to those.
			promisesNotHandled := []actress.Node{}
			var quorumProposerMinLimit int

			for {
				select {
				case ev := <-p.InCh:
					switch ev.Instruction {
					// We got new value to be reach concensus on.
					//
					// - The initial event with the value to reach concensus on,
					//	 should be given to the proposer with:
					//	 - Name : ETPaxosProposer
					//	 - Instruction : InstructionPaxosAddValue
					// 	 - The value should be in the Data field of the event.
					case InstructionPaxosAddValue:
						paxosInitial := PaxosInitial{}
						err := json.Unmarshal(ev.Data, &paxosInitial)
						if err != nil {
							slog.Error("ETPaxosProposerFn", "unmarshaling paxosInitial failed", err)
							continue
						}

						// Since the initial paxos data is unmarshaled inside a switch statement,
						// and we need to know what proposer nodes to use in other switch statements,
						// we assign a variable with the value of the proposer nodes outside the for
						// loop.
						proposer.knownNodes = paxosInitial.Nodes

						// Set the variable defined outside the for loop, so it can be used in other
						// case's of the switch statement.
						quorumProposerMinLimit = (len(proposer.knownNodes) / 2) + 1

						proposer.lastTriedRoundNr++
						proposer.value = paxosInitial.Data

						promiseData := PrepareData{
							RoundNr:     proposer.lastTriedRoundNr,
							NodeName:    string(p.Config.NodeName),
							ProcessName: string(p.Event),
							Nodes:       proposer.knownNodes,
						}
						b, err := json.Marshal(promiseData)
						if err != nil {
							slog.Error("ETPaxosProposerFn", "marshaling PrepareData failed", err)
							continue
						}

						// Send a prepare to all the known acceptor nodes
						for _, proposerNode := range proposer.knownNodes {
							fmt.Printf("ETPaxosProposerFn, on %v, got InstructionPaxosAddValue, sending InstructionPaxosPrepare to: %v\n", p.Config.NodeName, proposerNode)

							prepareEV := actress.Event{
								Name:        ETPaxosAcceptor,
								Data:        b,
								Instruction: InstructionPaxosPrepare,
								DstNode:     actress.Node(proposerNode),
								SrcNode:     p.Config.NodeName,
							}
							p.AddEvent(prepareEV)
						}

					case InstructionPaxosPromise:
						fmt.Printf("ETPaxosProposerFn, on %v, got InstructionPaxosPromise from: %v\n", p.Config.NodeName, ev.SrcNode)

						promiseData := PromiseData{}
						err := json.Unmarshal(ev.Data, &promiseData)
						if err != nil {
							slog.Error("ETPaxosAcceptorFn", "unmarshaling NackData failed", err)
							continue
						}

						// If this is f.ex. an old promise hanging around
						if promiseData.RoundNR < proposer.lastTriedRoundNr {
							fmt.Printf("ETPaxosProposer: got an older promise than the current roundt, and we don't want those....")
							continue
						}

						// Check if we've the promise contained any previous accepted data.
						// If it did, we check that it is higher than anything we've got until
						// now, and if it is higher we take that value.
						// If it is lower, we handle it as a promise without any previous value,
						// and continue down the code flow.
						if promiseData.PreviousAccepted.RoundNr > 0 {
							if promiseData.PreviousAccepted.RoundNr > proposer.previousAccepted.RoundNr {
								// Keep the value for printing the previous previous round nr before replacing value.
								previousPreviousAcceptedRoundNr := proposer.previousAccepted.RoundNr

								proposer.previousAccepted.RoundNr = promiseData.PreviousAccepted.RoundNr
								// We replace the current value with what was found in the promise previous accepted.
								proposer.value = promiseData.PreviousAccepted.Value
								fmt.Printf("ETPaxosProposerFn: promise contained a previous value with new highest round nr. previous highest round: %v, with new round: %v\n", previousPreviousAcceptedRoundNr, proposer.previousAccepted.RoundNr)
							}
						}

						// If the received round nr is higher than what we got, it means we should
						// skip the current round.
						// Setting the round what we received, and also
						// setting the received counter to 0.
						if proposer.lastTriedRoundNr < promiseData.RoundNR {
							// Promises belong the the rounds we sent out prepare for,
							// if a higher round is seen in the promises, we abondon
							// the current round, and setting the received promised to 0.
							proposer.receivedPromises = 0
							// We also want to initiate a new round, and the new round
							// should have round nr slightly higher than any round seen
							// before, so we also set that.
							proposer.lastTriedRoundNr = promiseData.RoundNR

							// Trigger new round
							go func() {
								p.AddEvent(actress.Event{
									Name:        p.Event,
									Instruction: InstructionPaxosAddValue,
									Data:        proposer.value,
								})
							}()

							continue
						}

						// Add node name of the src we received promise from to the slice.
						promisesNotHandled = append(promisesNotHandled, ev.SrcNode)

						proposer.receivedPromises++
						slog.Debug("ETPaxosProposerFn", "received number of promises", proposer.receivedPromises)
						// fmt.Printf("ETPaxosProposerFn: on %v, received number of promises: %v\n", p.Config.NodeName, proposer.receivedPromises)

						// Is quorum reached ?
						if proposer.receivedPromises >= uint(quorumProposerMinLimit) {
							// fmt.Printf("ETPaxosProposerFn: on %v, enough promises for quorum, got: %v, need: %v\n", p.Config.NodeName, proposer.receivedPromises, quorumProposerMinLimit)

							// Quorum is reached, check if we have any nodes that we've received promise
							// from before quorum was reached, and also send accept messages to those.
							//
							// Since all promises are put on the promisesNotHandled slice earlier, we can
							// just iterate this slice to see what to do. This will work the first time
							// when a quorum is reached, but also for the single events we get after quorum
							// is reached, since they also will put on the slice, so we handle all here.
							if len(promisesNotHandled) > 0 {
								roundAndValue := RoundAndValue{
									RoundNr: proposer.lastTriedRoundNr,
									Value:   proposer.value,
								}
								b, err := json.Marshal(roundAndValue)
								if err != nil {
									slog.Error("ETPaxosProposerFn", " preparing accept message, marshaling RoundAndValue failed", err)
								}

								for _, dstNode := range promisesNotHandled {
									ev := actress.Event{
										Name:        ETPaxosAcceptor,
										Data:        b,
										Instruction: InstructionPaxosAccept,
										DstNode:     dstNode,
										SrcNode:     p.Config.NodeName,
									}

									// NEXT: Make the code on the acceptor to receive
									// instruction InstructionPaxosAccept events.

									p.AddEvent(ev)
								}

								// Reset the node slice since all messages are now sent.
								promisesNotHandled = []actress.Node{}
							}

							continue
						}

						// fmt.Printf("ETPaxosProposerFn: on %v, DONT HAVE enough promises for quorum, got: %v, need: %v\n", p.Config.NodeName, proposer.receivedPromises, quorumProposerMinLimit)
						continue

					case InstructionPaxosPromiseNACK:

						// TODO/Enhancement:
						// If any PROMISE or NACK reports a round greater than the current round, a
						// higher round is already in progress and we have already lost this round.
						// Stop taking part in this round, ignore any further replies for it, and
						// start a new round with a higher number.

						nackData := NackData{}
						err := json.Unmarshal(ev.Data, &nackData)
						if err != nil {
							slog.Error("ETPaxosFn", "unmarshaling NackData failed", err)
							continue
						}

						// NB: Check and verify that it is correct to set it to the
						// highest promised from the acceptor.
						// Before we eventualy set what we got with the NACK from the
						// acceptor, we check if what we got is less, and if it is less,
						// we set it to the higher seen value from the acceptor.
						if proposer.lastTriedRoundNr < nackData.HighestPromisedOnAcceptor {
							proposer.lastTriedRoundNr = nackData.HighestPromisedOnAcceptor
						}

						fmt.Printf("ETPaxosProposerFn, on %v, received NACK from: %v\n", p.Config.NodeName, ev.SrcNode)
						continue

					case InstructionPaxosAccepted:
						roundAndValue := RoundAndValue{}
						err := json.Unmarshal(ev.Data, &roundAndValue)
						if err != nil {
							slog.Error("ETPaxosProposerFn", "unmarshaling accept RoundAndValue failed", err)
						}

						proposer.receivedAccepted++

						// If the quorum is already reached, it means that the value was stored in
						// in a previous round, and we just update the counter above, and skip the
						// rest since the value was set at the time quorum was reached.
						if proposer.quorumReached {
							continue
						}

						if proposer.receivedAccepted >= uint(quorumProposerMinLimit) {
							proposer.quorumReached = true

							// fmt.Printf("ETPaxosProposerFn: on %v, enough promises for quorum of accepted, got: %v, need: %v\n", p.Config.NodeName, proposer.receivedAccepted, quorumProposerMinLimit)
							proposer.acceptedValue = roundAndValue.Value
							fmt.Printf("ETPaxosProposerFn: set the learned value on: %v, to: %s\n", p.Config.NodeName, proposer.acceptedValue[:10])
							fmt.Printf("ETPaxosProposerFn: the value is now accepted on: %v, to: %s, quorum: %v, after receiving accepted messages: %v\n", p.Config.NodeName, proposer.acceptedValue[:10], proposer.quorumReached, proposer.receivedAccepted)
						}

						fmt.Printf("ETPaxosProposerFn: on :%v RECEIVED ACCEPTED for round: %v, from: %v\n", p.Config.NodeName, roundAndValue.RoundNr, ev.SrcNode)
					}

				case <-ctx.Done():
				}
			}
		}

		return fn
	}

	return fn
}
