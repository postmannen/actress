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

	"github.com/fxamacker/cbor/v2"
)

type staticProcesses struct {
	procMap map[EventName]*Process
	mu      sync.Mutex
}

// Checks if the event is defined in the processes map, and returns true if it is.
func (p *staticProcesses) IsEventDefined(ev EventName) bool {
	if _, ok := p.procMap[ev]; !ok {
		return false
	}

	return true
}

// Prepare and return a new *processes structure.
func newStaticProcesses() *staticProcesses {
	p := staticProcesses{
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

// Router for normal events.
const ETRouter EventName = "ETRouter"

// Process function for routing and handling events. Will check
// and route the event to the correct process.
func etRouterFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer func() {
			// fmt.Printf("STOPPED ETRouter!!!")
			p.Stop()
		}()

		for {
			select {
			case ev := <-p.StaticEventCh:

				if ev.Name == ETRemote {
					p.StaticProcesses.mu.Lock()
					if _, ok := p.StaticProcesses.procMap[ev.Name]; !ok {
						slog.Error("etRouterFn", "on", p.Config.NodeName, "found no process registered for the event type ETRemote, and you need to register an ETFunc for how to handle remote connections with the EventName ", ev.Name)
					}
					p.StaticProcesses.mu.Unlock()
				}
				if slog.Default().Enabled(context.TODO(), slog.LevelDebug) {
					slog.Debug("etRouterFn", "event nr", ev.Nr, "received on StaticEventCh on", CopyEventFields(&ev))
				}
				// If there is a next event defined, we make a copy of all the fields  of the current event,
				// and put that as the previousEvent on the next event. We can use this information later
				// if need to check something in the previous event.
				if ev.NextEvent != nil {
					// Keep the information about the current event, so we are able to check for things
					// like ackTimeout and what node to reply back to if ack should be given.
					ev.NextEvent.PreviousEvent = CopyEventFields(&ev)
				}

				if slog.Default().Enabled(context.TODO(), slog.LevelDebug) {
					slog.Debug("etRouterFn", "event nr", ev.Nr, "after CopyEventFields on", CopyEventFields(&ev))
				}

				// Check if process is registred and valid.
				p.StaticProcesses.mu.Lock()
				if _, ok := p.StaticProcesses.procMap[ev.Name]; !ok {
					slog.Error("etRouterFn", "on", p.Config.NodeName, "found no process registered for the event type", ev.Name)
				}
				p.StaticProcesses.mu.Unlock()

				if slog.Default().Enabled(context.TODO(), slog.LevelDebug) {
					slog.Debug("etRouterFn", "event nr", ev.Nr, "after checking if process is registred in procMap", CopyEventFields(&ev))
				}

				p.StaticProcesses.mu.Lock()
				if p.StaticProcesses.procMap[ev.Name] == nil {
					slog.Error("etRouterFn", "on", p.Config.NodeName, "found no process registered for the event type", ev.Name)
					continue
				}
				inCh := p.StaticProcesses.procMap[ev.Name].InCh
				p.StaticProcesses.mu.Unlock()

				if slog.Default().Enabled(context.TODO(), slog.LevelDebug) {
					slog.Debug("etRouterFn", "event nr", ev.Nr, "after getting the process InCh from procMap", CopyEventFields(&ev))
					slog.Debug("etRouterFn", "event nr", ev.Nr, "before putting event on process InCh", p.Event, "node", p.Config.NodeName, "name", ev.Name, "Inch", inCh)
					slog.Debug("etRouterFn", "nextEvent", CopyEventFields(ev.NextEvent))
				}
				inCh <- ev

				if slog.Default().Enabled(context.TODO(), slog.LevelDebug) {
					slog.Debug("etRouterFn", "event nr", ev.Nr, "after routing event to process InCh", CopyEventFields(&ev))
				}

			case <-p.Ctx.Done():
				slog.Debug("etRouterFn", "got ctx.Done, on", p.Config.NodeName)

				return
			}
		}
	}

	return fn
}

// Copy all the descriptive meta data fields of the Event, not
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

// Press ctrl+c to exit.
const ETOsSignal EventName = "ETOsSignal"

// Process function for handling CTRL+C pressed.
func etOsSignalFn(ctx context.Context, p *Process) func() {
	fn := func() {
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

// The ETTest eventype are used for testing.
const ETTest EventName = "ETTest"
const InstructionCmdEOF Instruction = "InstructionCmdEOF"

// etTestFn accepts an 'chan string' as it's input argument, and
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

// Will forward the incomming event to the builtin .TestCh
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
				slog.Debug("etTestChFn", "got ctx.Done, on", p.Config.NodeName)

				return
			}
		}
	}

	return fn
}

// Get all the current processes running. Will return a
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
					panic(err)
				}

				p.AddEvent(Event{Name: e.NextEvent.Name, Data: b})

			case <-p.Ctx.Done():
				slog.Debug("etPidGetAllFn", "got ctx.Done, on", p.Config.NodeName)

				return
			}
		}
	}

	return fn
}

// Done don't currently do anything.
const ETDone EventName = "ETDone"

func etDoneFn(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			p.SignalReady()

			d := <-p.InCh

			go func() {
				slog.Info("etDoneFn", "got event ETDone", string(d.Data))
				slog.Error("etDoneFn", "got etDone, on", p.Config.NodeName)
			}()
		}
	}

	return fn
}

// Print the content of the .Data field of the event to stdout.
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

// Will exit and kill all processes.
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

// Handling pids within the system.
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
					log.Fatalf("etPidFn: failed to convert pid from string to int: %v\n", err)
				}
				procName := ev.Cmd[2]

				// Check the type of action we got.
				switch action {
				case pidGet:
					p.AddEvent(Event{Name: ev.NextEvent.Name,

						Data: []byte(fmt.Sprintf("pid: %v, process name: %v", pid, procName))})

				case pidGetAll:
					pidProcMap := p.pids.toProc.copyOfMap()
					for pid, procName := range *pidProcMap {

						p.AddEvent(Event{Name: ev.NextEvent.Name,

							Data: []byte(fmt.Sprintf("pid: %v, process name: %v", pid, procName))})
					}
				}

			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

// Read file. The path path to read should be in Event.Cmd[0].
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
						log.Fatalf("etReadFileFn: failed to open file: %v\n", err)
					}
					defer fh.Close()

					b, err := io.ReadAll(fh)
					if err != nil {
						log.Fatalf("etReadFileFn: failed to open file: %v\n", err)
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
