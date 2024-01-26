package actress

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"

	"github.com/pkg/profile"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Event defines an event. It holds:
//   - The EventType, which specifies the process are meant for.
//   - The Cmd, are meant to but not limited to be a way to give
//     instructions for what a process should do. The receiving
//     process are responsible for parsing the string slice into
//     something useful.
//   - The Data field are ment to carry the result from the work
//     done by a process, to the next process.
//   - Both Cmd and Data can be used interchangeably if it makes
//     more sense for a given scenario. No strict rules for this
//     exist. Just make sure to document the use of the given
//     EventType, so the structure of how to use the fields exist.
//   - Err, are used by the error event type (ER).
//   - NextEvent are used when we want to define a chain of events
//     to be executed. The processes must make use of the field
//     for this to work. Check out the examples folder for a simple
//     example for how it could be implemented.
type Event struct {
	// EventType eventType `json:"eventType" yaml:"eventType"`
	EventType EventType `json:"eventType" yaml:"eventType"`
	Cmd       []string  `json:"cmd" yaml:"cmd"`
	Data      []byte    `json:"data" yaml:"data"`
	Err       error     `json:"error" yaml:"error"`
	NextEvent *Event    `json:"event" yaml:"event"`
}

type EventType string

// Event types
const (
	// For the main Root process.
	ETRoot EventType = "ETRoot"
	// Router for normal events.
	ETRouter EventType = "ETRouter"
	// Will exit and kill all processes.
	ETExit EventType = "ETExit"
	// Press ctrl+c to exit.
	ETOsSignal EventType = "ETOsSignal"
	// Profiling.
	ETProfiling EventType = "ETprofiling"
	// Print the content of the .Data field of the event to
	// stdout.
	ETPrint EventType = "ETPrint"
	// Done don't currently do anything.
	ETDone EventType = "ETDone"
	// Handling pids within the system.
	// The structure of the ev.Cmd is a slice of string:
	// []string{"action","pid","process name"}
	ETPid EventType = "ETPid"
	// Will forward the incomming event to the Process.TestCh.
	ETTestCh EventType = "ETTestCh"
	// Get all the current processes running. Will return a
	// json encoded PidVsProcMap.
	ETPidGetAll EventType = "ETPidGetAll"

	// Router for error events.
	ERRouter EventType = "ERRouter"
	// Log errors.
	ERLog EventType = "ERLog"
	// Log debug errors.
	ERDebug EventType = "ERDebug"
	// Log and exit system.
	ERFatal EventType = "ERFatal"
)

type pFunc func(context.Context, *Process) func()

// -----------------------------------------------------------------------------
// Startup processes function
// -----------------------------------------------------------------------------

// Process function used to startup all the other needed processes.
// This process will only be assigned to the root process in the
// newProcess function.

// -----------------------------------------------------------------------------
// Builtin standard functions
// -----------------------------------------------------------------------------

// Process function for routing and handling events. Will check
// and route the event to the correct process.
func etRouterFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case e := <-p.EventCh:
				p.Processes.inChMap[e.EventType] <- e

			case <-ctx.Done():
				p.AddError(Event{
					EventType: ERLog,
					Err:       fmt.Errorf("info: got ctx.Done"),
				})

				return
			}
		}
	}

	return fn
}

// Process function for handling CTRL+C pressed.
func etOsSignalFunc(ctx context.Context, p *Process) func() {
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

// Will forward the event to the Process.TestCh.
func etTestChFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case e := <-p.InCh:
				p.TestCh <- e

			case <-ctx.Done():
				p.AddError(Event{
					EventType: ERLog,
					Err:       fmt.Errorf("info: got ctx.Done"),
				})

				return
			}
		}
	}

	return fn
}

// Get all the pids and processes, encode it into json.
func etPidGetAllFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case e := <-p.InCh:
				pMap := p.pids.toProc.copyOfMap()
				b, err := json.Marshal(pMap)
				if err != nil {
					log.Fatalf("error: failed to marshal pid to proc map: %v\n", err)
				}
				p.AddEvent(Event{EventType: e.NextEvent.EventType, Data: b})

			case <-ctx.Done():
				p.AddError(Event{
					EventType: ERLog,
					Err:       fmt.Errorf("info: got ctx.Done"),
				})

				return
			}
		}
	}

	return fn
}

func etProfilingFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		//defer profile.Start(profile.BlockProfile).Stop()
		//defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
		//defer profile.Start(profile.TraceProfile, profile.ProfilePath(".")).Stop()
		defer profile.Start(profile.MemProfile, profile.MemProfileRate(1)).Stop()
		//defer profile.Start(profile.MemProfileHeap).Stop()
		//defer profile.Start(profile.MemProfileAllocs).Stop()

		go http.ListenAndServe("localhost:6060", nil)

		reg := prometheus.NewRegistry()
		reg.MustRegister(collectors.NewGoCollector())
		procTotal := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ctrl_processes_total",
			Help: "The current number of total running processes",
		})
		reg.MustRegister(procTotal)

		http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	}

	return fn
}

// TODO: Check if there is still a good need for this.
func etDoneFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			d := <-p.InCh

			go func() {
				fmt.Printf("info: got event ETDone: %v\n", string(d.Data))
				p.AddError(Event{
					EventType: ERLog,
					Err:       fmt.Errorf("info: got etDone"),
				})
			}()
		}
	}

	return fn
}

// Print the content of the .Data field of the event to stdout.
func etPrintFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case d := <-p.InCh:

				go func() {
					fmt.Printf("%v\n", string(d.Data))
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

// Will exit and kill all processes.
func etExitFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case d := <-p.InCh:

				go func() {
					fmt.Printf("info: got event ETExit: %v\n", string(d.Data))
					os.Exit(0)
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

// -----------------------------------------------------------------------------
// Error handling functions
// -----------------------------------------------------------------------------

// Process function for routing and handling events.
func erRouterFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case e := <-p.ErrorCh:

				go func() {
					p.Processes.inChMap[e.EventType] <- e
				}()

			case <-ctx.Done():
				// NB: Bevare of this one getting stuck if for example the error
				// handling is down. Maybe add a timeout if blocking to long,
				// and then send elsewhere if it becomes a problem.
				p.AddError(Event{
					EventType: ERLog,
					Err:       fmt.Errorf("info: got ctx.Done"),
				})
			}
		}
	}

	return fn
}

func erLogFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case er := <-p.InCh:

				go func() {
					log.Printf("error for logging received: %v\n", er.Err)
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

func erDebugFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case er := <-p.InCh:

				go func() {
					log.Printf("error for debug logging received: %v\n", er.Err)
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

func erFatalFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case er := <-p.InCh:

				go func() {
					log.Fatalf("error for fatal logging received: %v\n", er.Err)
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

// -----------------------------------------------------------------------------
// Supervisor functions
// -----------------------------------------------------------------------------

type pidAction string

const pidGet pidAction = "pidGet"
const pidGetAll pidAction = "pidGetAll"

// Handle pids.
// The structure of the ev.Cmd is a slice of string:
// []string{"action","pid","process name"}
func etPidFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case ev := <-p.InCh:
				action := pidAction(ev.Cmd[0])
				pid, err := strconv.Atoi(ev.Cmd[1])
				if err != nil {
					log.Fatalf("failed to convert pid from string to int: %v\n", err)
				}
				procName := ev.Cmd[2]

				// Check the type of action we got.
				switch action {
				case pidGet:
					p.AddEvent(Event{EventType: ev.NextEvent.EventType, Data: []byte(fmt.Sprintf("pid: %v, process name: %v", pid, procName))})

				case pidGetAll:
					pidProcMap := p.pids.toProc.copyOfMap()
					for pid, procName := range *pidProcMap {

						p.AddEvent(Event{EventType: ev.NextEvent.EventType, Data: []byte(fmt.Sprintf("pid: %v, process name: %v", pid, procName))})
					}
				}

			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}
