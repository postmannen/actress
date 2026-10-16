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

type paxosLearner struct {
	learnedRoundAndValue RoundAndValue
	knownNodes           []string
	receivedAccepted     uint
	quorumReached        bool
}

func newPaxosLearner() *paxosLearner {
	p := paxosLearner{}

	return &p
}

const ETPaxosLearner actress.EventName = "ETPaxosLearner"

func ETPaxosLearnerFn(learnerNodes []string) actress.ETFunc {
	fn := func(ctx context.Context, p *actress.Process) func() {
		fn := func() {
			p.SignalReady()

			learner := newPaxosLearner()
			learner.knownNodes = learnerNodes

			quorumMinLimit := (len(learner.knownNodes) / 2) + 1

			for {
				select {
				case ev := <-p.InCh:

					switch ev.Instruction {
					case InstructionPaxosLearn:
						roundAndValue := RoundAndValue{}
						err := json.Unmarshal(ev.Data, &roundAndValue)
						if err != nil {
							slog.Error("ETPaxosProposerFn", "unmarshaling accept RoundAndValue failed", err)
							continue
						}

						learner.receivedAccepted++

						// If the quorum is already reached, it means that the value was stored in
						// in a previous round, and we just update the counter above, and skip the
						// rest since the value was set at the time quorum was reached.
						if learner.quorumReached {
							continue
						}

						if learner.receivedAccepted >= uint(quorumMinLimit) {
							learner.quorumReached = true
							// fmt.Printf("ETPaxosProposerFn: on %v, enough promises for quorum of accepted, got: %v, need: %v\n", p.Config.NodeName, proposer.receivedAccepted, quorumProposerMinLimit)
							learner.learnedRoundAndValue.Value = roundAndValue.Value
							learner.learnedRoundAndValue.RoundNr = roundAndValue.RoundNr

							// fmt.Printf("ETPaxosLearnerFn: on: %v, the quorumMinLimit is: %v\n", p.Config.NodeName, quorumMinLimit)
							fmt.Printf("ETPaxosLearnerFn: set the learned value on: %v, to: %s, quorum: %v, after receiving learn messages: %v\n", p.Config.NodeName, learner.learnedRoundAndValue.Value[:10], learner.quorumReached, learner.receivedAccepted)
						}

						// fmt.Printf("ETPaxosLearnerFn: on :%v RECEIVED LEARN for round: %v, from: %v\n", p.Config.NodeName, roundAndValue.RoundNr, ev.SrcNode)
					}

				case <-ctx.Done():
				}
			}
		}

		return fn
	}

	return fn
}
