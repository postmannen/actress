package actress

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
)

type processes struct {
	procMap map[EventName]*Process
	mu      sync.Mutex
}

// Checks if the event is defined in the processes map, and returns true if it is.
func (p *processes) IsEventDefined(ev EventName) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.procMap[ev]; !ok {
		return false
	}

	return true
}

// Delete an Event and it's process from the processes map.
func (p *processes) Delete(en EventName) {
	p.mu.Lock()
	defer p.mu.Unlock()
	proc, ok := p.procMap[en]
	if ok {
		delete(p.procMap, en)
		proc.Cancel()
		log.Printf("deleted process %v\n", en)
	}
}

// Prepare and return a new *processes structure.
func newProcesses() *processes {
	p := processes{
		procMap: make(map[EventName]*Process),
	}
	return &p
}

// -----------------------------------------------------------------------------
// Builtin standard Name's and their ETfunc's.
// -----------------------------------------------------------------------------

// ETRemote is an Name that will be used if
// an event should be delivered to a remote node.
//
// There are no ETFunc defined for ETRemote in Actress,
// so it is up to the user to write this function, and
// attach their own ETFunc when they create the process
// to handle the ETRemote Name.
//
// ETRemote are for example used in the AddEvent function,
// and will be prepended to the current event if it should
// not be handled locally.
const ETRemote EventName = "ETRemote"

// ETRouter for normal events.
const ETRouter EventName = "ETRouter"

// Process function for routing and handling events. Will check
// and route the event to the correct process.
func eventRouterFn(processes *processes, eventCh chan Event) ETFunc {
	fn2 := func(ctx context.Context, p *Process) func() {
		fn := func() {
			defer func() {
				slog.Info("etRouter", "stopped etRouter", "")
				p.Stop()
			}()

			// The inch is not used for this function, so we set up a
			// listener that logs info so the user can detect the error.
			go func() {
				for {
					select {
					case ev := <-p.InCh:
						slog.Error("an even was received on this actors inch, which is not in use, dropping event", "at actor", p.Event, "from src node", ev.SrcNode, "event nr", ev.Nr)
					case <-ctx.Done():
					}
				}
			}()

			// How long to wait for a stuck delivery from router
			const deliveryTimeout = time.Second * 3
			deliveryTimer := time.NewTimer(deliveryTimeout)

			for {
				select {
				case ev := <-eventCh:

					func(ev Event) {
						processes.mu.Lock()
						procFromProcMap, procMapValueOK := processes.procMap[ev.Name]
						processes.mu.Unlock()

						if ev.Name == ETRemote {
							if !procMapValueOK {
								slog.Error("etRouterFn", "on", p.Config.NodeName, "found no process registered for the event type ETRemote, and you need to register an ETFunc for how to handle remote connections with the EventName ", ev.Name)
								return
							}
						}

						// If there is a next event defined, we make a copy of all the fields  of the current event,
						// and put that as the previousEvent on the next event. We can use this information later
						// if need to check something in the previous event.
						if ev.NextEvent != nil {
							// Keep the information about the current event, so we are able to check for things
							// like ackTimeout and what node to reply back to if ack should be given.
							ev.NextEvent.PreviousEvent = CopyEventFields(&ev)
						}

						// Check if process is registred and valid.
						// if procMapValueOK && procFromProcMap != nil {
						if procMapValueOK {
							// If the the receiving actor is ready to receive,
							// we deliver it directly.
							// If the the receiving actor is busy, we hit the
							// default, and do the select below this one.
							select {
							case procFromProcMap.InCh <- ev:
								return

							default:
							}
						}

						deliveryTimer.Reset(deliveryTimeout)

						// Same as above, we try to deliver. If unable to deliver
						// it will wait up the time of the delivery timer is reached,
						// and then stop trying to deliver the event, and log it.
						//
						// Before we enter the select and retry to deliver, we check
						// that the process we found is not nil. If it is nil it is
						// a dynamic or custom process, and we enter the go routine
						// after this if block that will wait and check if it appears,
						// and then delivers the message.
						if procFromProcMap != nil {
							select {
							case procFromProcMap.InCh <- ev:
								deliveryTimer.Stop()
								return
							case <-ctx.Done():
								deliveryTimer.Stop()
								return
							case <-deliveryTimer.C:
								// If it is not a dynamic or custom event type we return.
								// If it is dynamic or custom it will continue with check for
								// process not registered further down below.
								if ev.EventType == Static || ev.EventType == Error || ev.EventType == Supervisor {
									slog.Error("etRouterFn", "on", p.Config.NodeName, "timeout reached when trying to deliver the event, returning and not handling event", ev.Name)
									return
								}
							}
						}

						slog.Error("etRouterFn", "on", p.Config.NodeName, "found no process registered for the event type, returning and not handling event", ev.Name)

						// The process was not registered. Wait a bit and check if it is just
						// taking some time to start.
						go func(ev Event) {
							// Try to 3 times to deliver the message.
							for i := 0; i < 3; i++ {
								slog.Error("eventRouterFn", "on", p.Config.NodeName, "found no process registered for the event type", ev.Name, "ev.DstNode", ev.DstNode)
								time.Sleep(time.Second * 1)

								processes.mu.Lock()
								procFromMap, ok := processes.procMap[ev.Name]
								processes.mu.Unlock()

								if !ok {
									// process not found yet, loop again
									continue
								}

								// Process is now registred, so we can safely put
								//the event on the InCh of the process.
								select {
								case procFromMap.InCh <- ev:
								case <-ctx.Done():
								default:
									slog.Error("eventRouterFn", "on", p.Config.NodeName, "process found registered for the event type, but channel seems blocked, discarding event", ev.Name, "ev.DstNode", ev.DstNode)
								}

								return
							}
						}(ev)

					}(ev)

				case <-p.Ctx.Done():
					if slog.Default().Enabled(ctx, slog.LevelDebug) {
						slog.Debug("etRouterFn", "got ctx.Done, on", p.Config.NodeName)
					}

					return
				}
			}
		}

		return fn
	}

	return fn2
}

// CopyEventFields copies all the descriptive meta data fields of the Event, not
// channels or Data.
func CopyEventFields(ev *Event) *Event {

	if ev == nil {
		return nil
	}

	e := Event{
		Nr:   ev.Nr,
		Name: ev.Name,

		Cmd:         ev.Cmd,
		Instruction: ev.Instruction,
		Err:         ev.Err,
		DstNode:     ev.DstNode,
		SrcNode:     ev.SrcNode,
	}

	return &e
}

// copyEventChain returns a copy of ev with its NextEvent chain
// deep-copied node by node, so each caller owns its own chain and
// routers/handlers may mutate it without racing
func copyNextEventChain(ev *Event) *Event {
	if ev == nil {
		return nil
	}

	e := *ev
	e.NextEvent = copyNextEventChain(ev.NextEvent)
	return &e
}

// ETOsSignal press ctrl+c to exit.
const ETOsSignal EventName = "ETOsSignal"

// Process function for handling CTRL+C pressed.
func etOsSignalFn(ctx context.Context, p *Process) func() {
	fn := func() {
		// The inch is not used for this function, so we set up a
		// listener that logs info so the user can detect the error.
		go func() {
			for {
				select {
				case ev := <-p.InCh:
					slog.Error("an even was received on this actors inch, which is not in use, dropping event", "at actor", p.Event, "from src node", ev.SrcNode, "event nr", ev.Nr)
				case <-ctx.Done():
				}
			}
		}()

		// Wait for ctrl+c to stop the server.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)

		// Block and wait for CTRL+C
		sig := <-sigCh
		log.Printf("Got terminate signal, terminating all processes, %v\n", sig)
		os.Exit(0)
	}

	return fn
}

// ETTest eventype are used for testing.
const ETTest EventName = "ETTest"
const InstructionCmdEOF Instruction = "InstructionCmdEOF"

// ETTestfn accepts an 'chan string' as it's input argument, and
// it will return the data field of the previous event on that
// channel. You can then listen on that channel, check the
// value delivered, and see if it contains the value you expected
// it to hold.
func ETTestfn(testCh chan string) ETFunc {
	etFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			p.SignalReady()

			for {
				select {
				case result := <-p.InCh:
					if result.Instruction == InstructionCmdEOF {
						close(testCh)
						return
					}
					testCh <- string(result.Data)

					// Check if there is a next event defined
					if result.NextEvent != nil {
						p.AddEvent(*result.NextEvent)
					}

				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	return etFunc
}

// ETTestCh will forward the incomming event to the builtin .TestCh
// of the process.
const ETTestCh EventName = "ETTestCh"

// Will forward the incomming event to the builtin .TestCh
// of the process.
func etTestChFn(ctx context.Context, p *Process) func() {
	fn := func() {
		p.SignalReady()

		for {
			select {
			case e := <-p.InCh:
				p.TestCh <- e

			case <-p.Ctx.Done():
				if slog.Default().Enabled(ctx, slog.LevelDebug) {
					slog.Debug("etTestChFn", "got ctx.Done, on", p.Config.NodeName)
				}

				return
			}
		}
	}

	return fn
}

// ETPidGetAll will get all the current processes running. Will return a
// json encoded PidVsProcMap.
const ETPidGetAll EventName = "ETPidGetAll"

// Get all the pids and processes, encode it into json.
func etPidGetAllFn(ctx context.Context, p *Process) func() {
	fn := func() {
		p.SignalReady()

		for {
			select {
			case e := <-p.InCh:
				pMap := p.pids.toProc.copyOfMap()
				b, err := cbor.Marshal(pMap)
				if err != nil {
					slog.Error("etPidGetAllFn", "failed to marshal pid to proc map", err)
					// Panic, to easier figure out where eventual errors happened now
					// during development.
					panic(err)
				}

				p.AddEvent(Event{Name: e.NextEvent.Name, Data: b})

			case <-p.Ctx.Done():
				if slog.Default().Enabled(ctx, slog.LevelDebug) {
					slog.Debug("etPidGetAllFn", "got ctx.Done, on", p.Config.NodeName)
				}

				return
			}
		}
	}

	return fn
}

// ETDone don't currently do anything.
const ETDone EventName = "ETDone"

func etDoneFn(ctx context.Context, p *Process) func() {
	fn := func() {
		p.SignalReady()

		for {
			select {
			case ev := <-p.InCh:
				slog.Info("etDoneFn", "got event ETDone", string(ev.Data))
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

// ETPrint the content of the .Data field of the event to stdout.
const ETPrint EventName = "ETPrint"

// Print the content of the .Data field of the event to stdout.
func etPrintFn(ctx context.Context, p *Process) func() {
	fn := func() {

		for {
			p.SignalReady()

			select {
			case d := <-p.InCh:

				go func() {
					fmt.Printf("%v\n", string(d.Data))
				}()
			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

// ETExit will exit and kill all processes.
const ETExit EventName = "ETExit"

// Will exit and kill all processes.
func etExitFn(ctx context.Context, p *Process) func() {
	fn := func() {
		p.SignalReady()

		for {
			select {
			case d := <-p.InCh:

				go func() {
					fmt.Printf("etExitFn: got event ETExit: %v\n", string(d.Data))
					os.Exit(0)
				}()
			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

// ETPid is handling pids within the system.
// The structure of the ev.Cmd is a slice of string:
// []string{"action","pid","process name"}
const ETPid EventName = "ETPid"

type pidAction string

const pidGet pidAction = "pidGet"
const pidGetAll pidAction = "pidGetAll"

// Handle pids.
// The structure of the ev.Cmd is a slice of string:
// []string{"action","pid","process name"}
func etPidFn(ctx context.Context, p *Process) func() {
	fn := func() {
		p.SignalReady()

		for {
			select {
			case ev := <-p.InCh:
				action := pidAction(ev.Cmd[0])
				pid, err := strconv.Atoi(ev.Cmd[1])
				if err != nil {
					slog.Error("etPidFn", "failed to convert pid from string to int", err)
					continue
				}
				procName := ev.Cmd[2]

				// Check the type of action we got.
				switch action {
				case pidGet:
					p.AddEvent(
						Event{
							Name: ev.NextEvent.Name,
							Data: fmt.Appendf([]byte{}, "pid: %v, process name: %v", pid, procName)})

				case pidGetAll:
					pidProcMap := p.pids.toProc.copyOfMap()
					for pid, proc := range *pidProcMap {
						p.AddEvent(
							Event{Name: ev.NextEvent.Name,
								Data: fmt.Appendf([]byte{}, "pid: %v, process name: %v", pid, proc.Event)})
					}
				}

			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

// ETReadFile The path path to read should be in Event.Cmd[0].
const ETReadFile EventName = "ETReadFile"

func ETReadFileFn(ctx context.Context, p *Process) func() {
	fn := func() {
		p.SignalReady()

		for {
			select {
			case ev := <-p.InCh:

				go func() {
					fh, err := os.Open(ev.Cmd[0])
					if err != nil {
						slog.Error("ETReadFileFn", "failed to open file", err)
						return
					}
					defer fh.Close()

					b, err := io.ReadAll(fh)
					if err != nil {
						slog.Error("ETReadFile", "readall failed", err)
						return
					}

					nEv := ev.NextEvent
					nEv.Data = b
					p.AddEvent(*nEv)
				}()
			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}
