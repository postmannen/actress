package actress

import (
	"bufio"
	"context"
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

	"github.com/fsnotify/fsnotify"
	"github.com/fxamacker/cbor/v2"
	"github.com/goccy/go-yaml"
	"github.com/pkg/profile"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type staticProcesses struct {
	procMap map[EventType]*Process
}

// Checks if the event is defined in the processes map, and returns true if it is.
func (p *staticProcesses) IsEventDefined(ev EventType) bool {
	if _, ok := p.procMap[ev]; !ok {
		return false
	}

	return true
}

// Prepare and return a new *processes structure.
func newStaticProcesses() *staticProcesses {
	p := staticProcesses{
		procMap: make(map[EventType]*Process),
	}
	return &p
}

// -----------------------------------------------------------------------------
// Builtin standard EventType's and their ETfunc's.
// -----------------------------------------------------------------------------

// ETRemote is an EventType that will be used if
// an event should be delivered to a remote node.
//
// There are no ETFunc defined for ETRemote in Actress,
// so it is up to the user to write this function, and
// attach their own ETFunc when they create the process
// to handle the ETRemote EventType.
//
// ETRemote are for example used in the AddEvent function,
// and will be prepended to the current event if it should
// not be handled locally.
const ETRemote EventType = "ETRemote"

// Router for normal events.
const ETRouter EventType = "ETRouter"

// Process function for routing and handling events. Will check
// and route the event to the correct process.
func etRouterFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

		eventNr := 0

		for {
			select {
			case ev := <-p.StaticEventCh:
				// If there is a next event defined, we make a copy of all the fields  of the current event,
				// and put that as the previousEvent on the next event. We can use this information later
				// if need to check something in the previous event.
				if ev.NextEvent != nil {
					// Keep the information about the current event, so we are able to check for things
					// like ackTimeout and what node to reply back to if ack should be given.
					ev.NextEvent.PreviousEvent = CopyEventFields(ev)
				}

				eventNr++
				ev.Nr = eventNr

				// Check if process is registred and valid.
				if _, ok := p.StaticProcesses.procMap[ev.EventType]; !ok {
					p.AddEvent(Event{EventType: ERLog,
						EventKind: EventKindError,
						Err:       fmt.Errorf("etRouter: on %v found no process registered for the event type : %v", p.Config.NodeName, ev.EventType)})
				}

				// Process was registered. Deliver the event to the process InCh.
				// log.Printf(" -------- DEBUG1 %v ---------p.Processes.procMap[ev.EventType] : %v\n", p.Config.NodeName, p.StaticProcesses.procMap[ev.EventType])
				// log.Printf(" -------- DEBUG2 %v---------p ev.EventType : %v\n", p.Config.NodeName, ev.EventType)
				// log.Printf(" -------- DEBUG3 %v---------p.Processes.procMap[ev.EventType].InCh : %v\n", p.Config.NodeName, p.StaticProcesses.procMap[ev.EventType].InCh)
				inCh := p.StaticProcesses.procMap[ev.EventType].InCh

				fmt.Printf("DEBUG: Routing event, %v, node: %v, eventType: %v, .Inch: %v\n", p.Event, p.Config.NodeName, ev.EventType, inCh)
				inCh <- ev

			case <-p.Ctx.Done():
				p.AddEvent(Event{
					EventType: ERLog,
					EventKind: EventKindError,
					Err:       fmt.Errorf("info: got ctx.Done"),
				})

				return
			}
		}
	}

	return fn
}

// Copy all the descriptive meta data fields of the Event, not
// channels or Data.
func CopyEventFields(ev Event) *Event {
	e := Event{
		Nr:        ev.Nr,
		EventType: ev.EventType,
		EventKind: ev.EventKind,
		Cmd:       ev.Cmd,
		Args:      ev.Args,
		Err:       ev.Err,
		DstNode:   ev.DstNode,
		SrcNode:   ev.SrcNode,
	}

	return &e
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

// The ETTest eventype are used for testing.
const ETTest EventType = "ETTest"

// etTestFn accepts an 'chan string' as it's input argument, and
// it will return the data field of the previous event on that
// channel. You can then listen on that channel, check the
// value delivered, and see if it contains the value you expected
// it to hold.
func etTestfn(testCh chan string) ETFunc {
	etFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			defer p.Stop()

			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)
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
const ETTestCh EventType = "ETTestCh"

// Will forward the incomming event to the builtin .TestCh
// of the process.
func etTestChFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

		for {
			select {
			case e := <-p.InCh:
				p.TestCh <- e

			case <-p.Ctx.Done():
				p.AddEvent(Event{
					EventType: ERLog,
					EventKind: EventKindError,
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
		defer p.Stop()

		for {
			select {
			case e := <-p.InCh:
				pMap := p.pids.toProc.copyOfMap()
				b, err := cbor.Marshal(pMap)
				if err != nil {
					p.AddEvent(Event{
						EventType: ERFatal,
						Err:       fmt.Errorf("error: failed to marshal pid to proc map: %v", err),
					})
				}
				p.AddEvent(Event{EventType: e.NextEvent.EventType, EventKind: e.NextEvent.EventKind, Data: b})

			case <-p.Ctx.Done():
				p.AddEvent(Event{
					EventType: ERLog,
					EventKind: EventKindError,
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
			log.Printf("STARTING UP WEB SERVER !!!!!!!!!!!!!!!!!!!!!!!!!!")
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
			//<-p.Ctx.Done()
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
				p.AddEvent(Event{
					EventType: ERLog,
					EventKind: EventKindError,
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
		defer p.Stop()

		for {
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
const ETExit EventType = "ETExit"

// Will exit and kill all processes.
func etExitFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

		for {
			select {
			case d := <-p.InCh:

				go func() {
					fmt.Printf("info: got event ETExit: %v\n", string(d.Data))
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
const ETPid EventType = "ETPid"

type pidAction string

const pidGet pidAction = "pidGet"
const pidGetAll pidAction = "pidGetAll"

// Handle pids.
// The structure of the ev.Cmd is a slice of string:
// []string{"action","pid","process name"}
func etPidFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

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
					p.AddEvent(Event{EventType: ev.NextEvent.EventType,
						EventKind: ev.NextEvent.EventKind,
						Data:      []byte(fmt.Sprintf("pid: %v, process name: %v", pid, procName))})

				case pidGetAll:
					pidProcMap := p.pids.toProc.copyOfMap()
					for pid, procName := range *pidProcMap {

						p.AddEvent(Event{EventType: ev.NextEvent.EventType,
							EventKind: ev.NextEvent.EventKind,
							Data:      []byte(fmt.Sprintf("pid: %v, process name: %v", pid, procName))})
					}
				}

			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

const ETWatchEventFile EventType = "ETWatchEventFile"

// Watch for file changes in the given path, for files with the specified extension.
// A wrapper function have been put around this function to be able to inject the
// path and the extension parameters.
func wrapperETWatchEventFileFn(path string, extension string) ETFunc {
	fønk := func(ctx context.Context, p *Process) func() {
		fn := func() {
			defer p.Stop()

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
							p.AddEvent(Event{EventType: ETReadFile,
								EventKind: EventKindStatic,
								Cmd:       []string{pth},
								NextEvent: &Event{EventType: ETProcessFromData,
									EventKind: EventKindStatic,
								}})
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
							p.AddEvent(Event{EventType: ETReadFile,
								EventKind: EventKindStatic,
								Cmd:       []string{fsEvent.Name},
								NextEvent: &Event{EventType: ETProcessFromData,
									EventKind: EventKindStatic,
								}})

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
					case <-p.Ctx.Done():
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

				case <-p.Ctx.Done():
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
		defer p.Stop()

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
			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

type customProcessData struct {
	Name string   `yaml:"name"`
	Cmd  []string `yaml:"cmd,omitempty"`
}

// ETProcessFromData are used when reading custom user defined events
// to create processes based on it's input data taken from the event
// on the InCh. It expects it's input in the Data field of the event
// to be the JSON serialized data of a custom Event. The unmarshaled
// custom event will then be used to prepare and start up a process
// for the new EventType.
//
// The ETProcessFromData process is of Kind Static, but the process it
// starts based on the data from the file are of Kind Dynamic.
// The structure of the command read from file are as follows.
//
//  {
//    "name": "some_name",
//    "cmd": ["command", "arg1", "arg2"]
//  }
//
// or YAML
//
//  name: some_name
//  cmd:
//    - command
//    - arg1
//    - arg2
//
// The "name", are used to name the dynamic process.
// The "cmd", are the actual command to execute.
//
// To actually trigger the process we can send a custom kind event
// with the process name, and the event will be triggered to run.
// The result will be sendt to what is defined as the NextEvent.

const ETProcessFromData EventType = "ETProcessFromData"

func etProcessFromDataFn(ctx context.Context, p *Process) func() {
	fn := func() {
		defer p.Stop()

		for {
			select {
			case ev := <-p.InCh:
				go func() {
					ce := customProcessData{}
					err := yaml.Unmarshal(ev.Data, &ce)
					if err != nil {
						p.AddEvent(Event{EventType: ERFatal, EventKind: EventKindError, Err: fmt.Errorf("info: got ctx.Done")})
					}

					// Start an EventKindCustom event.
					NewProcess(ctx, p, EventType(ce.Name), EventKindCustom, ecCustomCmdFn(ce.Cmd)).Act()
					// DEBUG: Injecting an event for testing while developing.
					// p.AddEvent(Event{EventType: EventType("ET1"), Cmd: []string{"ls -l"}})
				}()
			case <-p.Ctx.Done():
				return
			}
		}
	}

	return fn
}

// ecCustomCmdFn, used with etProcessFromDataFn to run the actual command of the custom event
// that we create from the file.
func ecCustomCmdFn(command []string) func(ctx context.Context, p *Process) func() {
	fønk := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				ev := <-p.InCh

				go func(ev Event) {
					fmt.Printf("DEBUG: ecCustomCmdFn, start of event: %v, cmd: %v\n", ev.EventType, ev.Cmd)
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
						p.AddEvent(Event{EventType: ERFatal,
							EventKind: EventKindError,
							Err:       fmt.Errorf("error: failed to run command, err: %v, errText: %v", err, err.Error())})
					}

					go func() {
						scanner := bufio.NewScanner(outReader)
						for scanner.Scan() {
							if ev.NextEvent == nil {
								fmt.Printf("%v\n", scanner.Text())
								continue
							}
							p.AddEvent(Event{EventType: ev.NextEvent.EventType,
								EventKind: ev.NextEvent.EventKind,
								Data:      scanner.Bytes()})
						}
					}()

					go func() {
						scanner := bufio.NewScanner(errReader)
						for scanner.Scan() {
							if ev.NextEvent == nil {
								fmt.Printf("%v\n", scanner.Text())
								continue
							}
							p.AddEvent(Event{EventType: ev.NextEvent.EventType,
								EventKind: ev.NextEvent.EventKind,
								Data:      scanner.Bytes()})
						}
					}()

					//<-p.Ctx.Done()

					err = cmd.Wait()
					if err != nil {
						p.AddEvent(Event{EventType: ERFatal,
							EventKind: EventKindError,
							Err:       fmt.Errorf("error: failed to wait for command, err: %v, errText: %v", err, err.Error())})
					}

					//p.AddEvent(Event{EventType: ETPrint, Data: outText.Bytes()})
					fmt.Printf("DEBUG: ecCustomCmdFn, End of event: %v, cmd: %v\n", ev.EventType, ev.Cmd)

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
				fmt.Printf("DEBUG: etOsCmdFn, start of event: %v, cmd: %v\n", ev.EventType, ev.Cmd)
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
					p.AddEvent(Event{EventType: ERFatal,
						EventKind: EventKindError,
						Err:       fmt.Errorf("error: failed to run command, err: %v, errText: %v", err, err.Error())})
				}

				go func() {
					scanner := bufio.NewScanner(outReader)
					for scanner.Scan() {
						if ev.NextEvent == nil {
							fmt.Printf("%v\n", scanner.Text())
							continue
						}
						p.AddEvent(Event{EventType: ev.NextEvent.EventType,
							EventKind: ev.NextEvent.EventKind,
							Data:      scanner.Bytes()})
					}
				}()

				go func() {
					scanner := bufio.NewScanner(errReader)
					for scanner.Scan() {
						if ev.NextEvent == nil {
							fmt.Printf("%v\n", scanner.Text())
							continue
						}
						p.AddEvent(Event{EventType: ev.NextEvent.EventType,
							EventKind: ev.NextEvent.EventKind,
							Data:      scanner.Bytes()})
					}
				}()

				//<-p.Ctx.Done()

				err = cmd.Wait()
				if err != nil {
					p.AddEvent(Event{EventType: ERFatal,
						EventKind: EventKindError,
						Err:       fmt.Errorf("error: failed to wait for command, err: %v, errText: %v", err, err.Error())})
				}

				//p.AddEvent(Event{EventType: ETPrint, Data: outText.Bytes()})
				fmt.Printf("DEBUG: etOsCmdFn, End of event: %v, cmd: %v\n", ev.EventType, ev.Cmd)

			}(ev)
		}
	}

	return fn
}
