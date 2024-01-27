package actress

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"

	"github.com/fsnotify/fsnotify"
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
// For the main Root process.
const ETRoot EventType = "ETRoot"

type ETFunc func(context.Context, *Process) func()

// -----------------------------------------------------------------------------
// Startup processes function
// -----------------------------------------------------------------------------

// Process function used to startup all the other needed processes.
// This process will only be assigned to the root process in the
// newProcess function.

// -----------------------------------------------------------------------------
// Builtin standard functions
// -----------------------------------------------------------------------------

// Router for normal events.
const ETRouter EventType = "ETRouter"

// Process function for routing and handling events. Will check
// and route the event to the correct process.
func etRouterFn(ctx context.Context, p *Process) func() {
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

// Press ctrl+c to exit.
const ETOsSignal EventType = "ETOsSignal"

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

// Will forward the incomming event to the Process.TestCh.
const ETTestCh EventType = "ETTestCh"

// Will forward the event to the Process.TestCh.
func etTestChFn(ctx context.Context, p *Process) func() {
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

// Get all the current processes running. Will return a
// json encoded PidVsProcMap.
const ETPidGetAll EventType = "ETPidGetAll"

// Get all the pids and processes, encode it into json.
func etPidGetAllFn(ctx context.Context, p *Process) func() {
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

// Profiling.
const ETProfiling EventType = "ETprofiling"

func etProfilingFn(ctx context.Context, p *Process) func() {
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

// Done don't currently do anything.
const ETDone EventType = "ETDone"

// TODO: Check if there is still a good need for this.
func etDoneFn(ctx context.Context, p *Process) func() {
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
const ETPrint EventType = "ETPrint"

// Print the content of the .Data field of the event to stdout.
func etPrintFn(ctx context.Context, p *Process) func() {
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
const ETExit EventType = "ETExit"

// Will exit and kill all processes.
func etExitFn(ctx context.Context, p *Process) func() {
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

// Router for error events.
const ERRouter EventType = "ERRouter"

// Process function for routing and handling events.
func erRouterFn(ctx context.Context, p *Process) func() {
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

// Log errors.
const ERLog EventType = "ERLog"

func erLogFn(ctx context.Context, p *Process) func() {
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

// Log debug errors.
const ERDebug EventType = "ERDebug"

func erDebugFn(ctx context.Context, p *Process) func() {
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

// Log and exit system.
const ERFatal EventType = "ERFatal"

func erFatalFn(ctx context.Context, p *Process) func() {
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

// Handling pids within the system.
// The structure of the ev.Cmd is a slice of string:
// []string{"action","pid","process name"}
const ETPid EventType = "ETPid"

type pidAction string

const pidGet pidAction = "pidGet"
const pidGetAll pidAction = "pidGetAll"

// Handle pids.
// The structure of the ev.Cmd is a slice of string:
// []string{"action","pid","process name"}
func etPidFn(ctx context.Context, p *Process) func() {
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

const ETWatchEventFile EventType = "ETWatchEventFile"

// Watch for file changes in the given path, for files with the specified extension.
// A wrapper function have been put around this function to be able to inject the
// path and the extension parameters. The returned result from the wrapper is a normal
// ETFunc, and same as the other ETFunctions specified to be used with an EventType.
func wrapperETWatchEventFileFn(path string, extension string) ETFunc {
	fønk := func(ctx context.Context, p *Process) func() {
		fn := func() {
			// Create new watcher.
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				log.Fatal(err)
			}
			defer watcher.Close()

			// Start listening for events.
			go func() {
				for {
					select {
					case event, ok := <-watcher.Events:
						if !ok {
							return
						}
						//log.Println("event:", event)
						switch {
						case event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod) || event.Has(fsnotify.Create):
							fileName := filepath.Base(event.Name)
							ext := filepath.Ext(fileName)
							if ext == extension {
								log.Printf("op: %v, file : %v, extension: %v\n", event.Op, fileName, ext)
							}
							p.AddEvent(Event{EventType: ETReadFile, Cmd: []string{event.Name}})
						case event.Has(fsnotify.Remove):
							fileName := filepath.Base(event.Name)
							ext := filepath.Ext(fileName)
							log.Printf("remove : op: %v, file : %v, extension: %v\n", event.Op, fileName, ext)
						}
					case err, ok := <-watcher.Errors:
						if !ok {
							return
						}
						log.Println("error:", err)
					case <-ctx.Done():
						return
					}
				}
			}()

			// Add a path to watch
			err = watcher.Add(path)
			if err != nil {
				log.Fatal(err)
			}

			for {
				select {
				case <-p.InCh:

				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	return fønk
}

// Read file. The path path to read should be in Event.Cmd[0].
const ETReadFile EventType = "ETReadFile"

func ETReadFileFn(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case ev := <-p.InCh:

				go func() {
					fh, err := os.Open(ev.Cmd[0])
					if err != nil {
						log.Fatalf("failed to open file: %v\n", err)
					}
					defer fh.Close()

					b, err := io.ReadAll(fh)
					if err != nil {
						log.Fatalf("failed to open file: %v\n", err)
					}

					p.AddEvent(Event{EventType: ETCustomEvent, Data: b})
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

type customEvent struct {
	Name string
	API  []string
}

// Log and exit system.
const ETCustomEvent EventType = "ETCustomEvent"

func ETCustomEventFn(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case ev := <-p.InCh:
				go func() {
					ce := customEvent{}
					err := json.Unmarshal(ev.Data, &ce)
					if err != nil {
						log.Fatalf("failed to unmarshal custom event data: %v\n", err)
					}
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}
