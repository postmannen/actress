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

package actress

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

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
	Nr int
	// EventType is a unique name to identify the type of the event.
	EventType EventType `json:"eventType" yaml:"eventType"`
	// Cmd is usually used for giving instructions or parameters for
	// what an event shall do.
	Cmd []string `json:"cmd" yaml:"cmd"`
	// Data usually carries the data from one process to the next. Example
	// could be a file read on process1 is put in the Data field, and
	// passed on to process2 to be unmarshaled.
	Data []byte `json:"data" yaml:"data"`
	// Err is used for defining the error message when the event is used
	// as an error event.
	Err error `json:"error" yaml:"error"`
	// NextEvent defines a series of events to be executed like a workflow.
	// The receiving process should check this field for what kind of event
	// to create as the next step in the workflow.
	NextEvent *Event `json:"event" yaml:"event"`
}

type EventOpt func(*Event)

func NewEvent(et EventType, opts ...EventOpt) *Event {
	ev := Event{EventType: et}
	for _, opt := range opts {
		opt(&ev)
	}
	return &ev
}

func EvCmd(cmd []string) EventOpt {
	fn := func(ev *Event) {
		ev.Cmd = cmd
	}
	return fn
}

func EVData(b []byte) EventOpt {
	fn := func(ev *Event) {
		ev.Data = b
	}
	return fn
}

func EvNext(nev *Event) EventOpt {
	fn := func(ev *Event) {
		ev.NextEvent = nev
	}
	return fn
}

// EventType is a unique name used to identify events. It is used both for
// creating processes and also for routing messages to the correct process.
type EventType string

// The main Root process.
const ETRoot EventType = "ETRoot"

// Function type describing the signature of a function that is to be used
// when creating a new process.
type ETFunc func(context.Context, *Process) func()

// -----------------------------------------------------------------------------
// Builtin standard EventType's and theit ETfunc's.
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
				// Custom processes can take a little longer to start up and be
				// registered in the map. We check here if process is registred,
				// and it it is not we retry.
				if _, ok := p.Processes.procMap[e.EventType]; !ok {
					go func(ev Event) {
						// Try to 3 times to deliver the message.
						for i := 0; i < 3; i++ {
							_, ok := p.Processes.procMap[e.EventType]

							if !ok {
								p.AddError(Event{EventType: ERLog, Err: fmt.Errorf("found no process registered for the event type : %v", ev.EventType)})
								time.Sleep(time.Millisecond * 1000)
								continue
							}

							// Process is now registred, so we can safely put
							//the event on the InCh of the process.
							p.Processes.procMap[e.EventType].InCh <- e

							return
						}

					}(e)
					continue
				}

				// Process was registered. Deliver the event to the process InCh.
				p.Processes.procMap[e.EventType].InCh <- e

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
		switch p.Config.Profiling {
		case "mutex":
			runtime.SetMutexProfileFraction(1)
			defer profile.Start(profile.MutexProfile).Stop()
		case "block":
			runtime.SetBlockProfileRate(1)
			defer profile.Start(profile.BlockProfile).Stop()
		case "cpu":
			defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
		case "trace":
			defer profile.Start(profile.TraceProfile, profile.ProfilePath(".")).Stop()
		case "mem":
			defer profile.Start(profile.MemProfile, profile.MemProfileRate(1)).Stop()
		case "heap":
			defer profile.Start(profile.MemProfileHeap).Stop()
		case "alloc":
			defer profile.Start(profile.MemProfileAllocs).Stop()
		case "none":
		}

		if p.Config.Metrics || p.Config.Profiling != "" {
			go http.ListenAndServe("localhost:6060", nil)
		}

		if p.Config.Metrics {
			reg := prometheus.NewRegistry()
			reg.MustRegister(collectors.NewGoCollector())
			procTotal := prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "ctrl_processes_total",
				Help: "The current number of total running processes",
			})
			reg.MustRegister(procTotal)

			http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
			//<-ctx.Done()
		}
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

// Router for error events.
const ERRouter EventType = "ERRouter"

// Process function for routing and handling events.
func erRouterFn(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case e := <-p.ErrorCh:

				p.ErrProcesses.procMap[e.EventType].InCh <- e

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

// Log and exit system.
const ERTest EventType = "ERTest"

func erTestFn(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case er := <-p.InCh:

				go func() {
					drop := fmt.Sprintf("error for fatal logging received: %v\n", er.Err)
					_ = drop
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
				// Before we start the watcher we want to check and handle the files
				// that already are present in the directory.
				err := filepath.Walk(path,
					func(pth string, info os.FileInfo, err error) error {
						if err != nil {
							return err
						}

						// Check if the extension of the file is correct.
						ext := filepath.Ext(pth)
						if ext == extension {
							p.AddEvent(Event{EventType: ETReadFile, Cmd: []string{pth},
								NextEvent: &Event{EventType: ETCustomEvent}})
						}

						return nil
					})
				if err != nil {
					log.Fatalf("filepath Walk failed: %v\n", err)
				}

				for {
					select {
					case fsEvent, ok := <-watcher.Events:
						if !ok {
							return
						}
						//log.Println("event:", event)
						switch {
						case fsEvent.Has(fsnotify.Write) || fsEvent.Has(fsnotify.Chmod) || fsEvent.Has(fsnotify.Create):
							//fileName := filepath.Base(fsEvent.Name)
							//ext := filepath.Ext(fileName)
							//if ext == extension {
							//	log.Printf("op: %v, file : %v, extension: %v\n", fsEvent.Op, fileName, ext)
							//}
							p.AddEvent(Event{EventType: ETReadFile, Cmd: []string{fsEvent.Name},
								NextEvent: &Event{EventType: ETCustomEvent}})
						case fsEvent.Has(fsnotify.Remove):
							fileName := filepath.Base(fsEvent.Name)
							ext := filepath.Ext(fileName)
							log.Printf("remove : op: %v, file : %v, extension: %v\n", fsEvent.Op, fileName, ext)
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

					nEv := ev.NextEvent
					nEv.Data = b
					p.AddEvent(*nEv)
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
	Cmd  []string
}

// ETCustomEvent are used when reading custom user defined events
// to create processes from disk. It expects it's input in the Data
// field of the event to be the JSON serialized data of a custom Event.
// The unmarshaled custom event will then be used to prepare and start
// up a process for the new EventType.
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

					NewProcess(ctx, *p, EventType(ce.Name), WrapperCustomCmd(ce.Cmd)).Act()
					// DEBUG: Injecting an event for testing while developing.
					// p.AddEvent(Event{EventType: EventType("ET1"), Cmd: []string{"ls -l"}})
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

// Wrapper around creating an etFunc. The use of this wrapper function is to insert
// some predefined values into the cmd to be executed when a process for the eventType
// is created. When an event later is reveived, the content of the Event.Cmd field is
// appended to what is already defined there from earlier.
// An example of this is that we define the content of the command to be executed when
// the process is defined to contain []string{"/bin/bash","-c"}. When an event later
// is received and handled by this function, the contend of the .Cmd field is appended
// to the predefined fields, and will for example give a result to be executed like
// []string{"/bin/bash","-c","ls -l|grep file.txt"}.
func WrapperCustomCmd(command []string) func(ctx context.Context, p *Process) func() {
	fønk := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				ev := <-p.InCh

				go func(ev Event) {
					fmt.Printf("********* start of event: %v, cmd: %v\n", ev.EventType, ev.Cmd)
					//ctx, cancel := context.WithTimeout(ctx, time.Second*time.Duration(5))
					ctx, cancel := context.WithCancel(ctx)
					defer cancel()

					// The command is located in the first field of the string slice.
					// The rest of the arguments are in the remaining fields of the slice.
					args := command[1:]
					// Append the values of d.Cmd to the already existing values in args.
					args = append(args, ev.Cmd...)

					cmd := exec.CommandContext(ctx, command[0], args...)

					outReader, _ := cmd.StdoutPipe()
					errReader, _ := cmd.StderrPipe()
					//cmd.WaitDelay = time.Second * 5

					err := cmd.Start()
					if err != nil {
						p.AddError(Event{EventType: ERFatal, Err: fmt.Errorf("error: failed to run command, err: %v, errText: %v", err, err.Error())})
					}

					go func() {
						scanner := bufio.NewScanner(outReader)
						for scanner.Scan() {
							if ev.NextEvent == nil {
								fmt.Printf("%v\n", scanner.Text())
								continue
							}
							p.AddEvent(Event{EventType: ev.NextEvent.EventType, Data: scanner.Bytes()})
						}
					}()

					go func() {
						scanner := bufio.NewScanner(errReader)
						for scanner.Scan() {
							if ev.NextEvent == nil {
								fmt.Printf("%v\n", scanner.Text())
								continue
							}
							p.AddEvent(Event{EventType: ev.NextEvent.EventType, Data: scanner.Bytes()})
						}
					}()

					//<-ctx.Done()

					err = cmd.Wait()
					if err != nil {
						p.AddError(Event{EventType: ERFatal, Err: fmt.Errorf("error: failed to wait for command, err: %v, errText: %v", err, err.Error())})
					}

					//p.AddEvent(Event{EventType: ETPrint, Data: outText.Bytes()})
					fmt.Printf("********* End of event: %v, cmd: %v\n", ev.EventType, ev.Cmd)

				}(ev)
			}
		}

		return fn
	}

	return fønk
}

// Execute OS commands. The command to execute should be put in the first slot
// of the array at Event.Cmd[0], and all arguments should be put int the sub
// sequent slots. To make it simpler to run commands without splitting the up
// on Linux like operating systems use the -c flag with bash. Example,
// Event{EventType: etOsCmd, Cmd: ["bash","-c","ls -l|grep myfile"]}.
const ETOsCmd EventType = "etOsCmd"

func etOsCmdFn(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			ev := <-p.InCh

			go func(ev Event) {
				fmt.Printf("********* start of event: %v, cmd: %v\n", ev.EventType, ev.Cmd)
				//ctx, cancel := context.WithTimeout(ctx, time.Second*time.Duration(5))
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()

				// The command is located in the first field of the string slice.
				// The rest of the arguments are in the remaining fields of the slice.
				args := ev.Cmd[1:]
				// Append the values of d.Cmd to the already existing values in args.
				args = append(args, ev.Cmd...)

				cmd := exec.CommandContext(ctx, ev.Cmd[0], args...)

				outReader, _ := cmd.StdoutPipe()
				errReader, _ := cmd.StderrPipe()
				//cmd.WaitDelay = time.Second * 5

				err := cmd.Start()
				if err != nil {
					p.AddError(Event{EventType: ERFatal, Err: fmt.Errorf("error: failed to run command, err: %v, errText: %v", err, err.Error())})
				}

				go func() {
					scanner := bufio.NewScanner(outReader)
					for scanner.Scan() {
						if ev.NextEvent == nil {
							fmt.Printf("%v\n", scanner.Text())
							continue
						}
						p.AddEvent(Event{EventType: ev.NextEvent.EventType, Data: scanner.Bytes()})
					}
				}()

				go func() {
					scanner := bufio.NewScanner(errReader)
					for scanner.Scan() {
						if ev.NextEvent == nil {
							fmt.Printf("%v\n", scanner.Text())
							continue
						}
						p.AddEvent(Event{EventType: ev.NextEvent.EventType, Data: scanner.Bytes()})
					}
				}()

				//<-ctx.Done()

				err = cmd.Wait()
				if err != nil {
					p.AddError(Event{EventType: ERFatal, Err: fmt.Errorf("error: failed to wait for command, err: %v, errText: %v", err, err.Error())})
				}

				//p.AddEvent(Event{EventType: ETPrint, Data: outText.Bytes()})
				fmt.Printf("********* End of event: %v, cmd: %v\n", ev.EventType, ev.Cmd)

			}(ev)
		}
	}

	return fn
}
