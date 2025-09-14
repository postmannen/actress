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
	"context"
	"fmt"
	"log"
	"sync"
)

// Add a new Event and it's process to the processes map.
func (p *Process) addToProcessesMap(et EventType, proc *Process, eventKind EventKind) {
	// Check if a process for the same event is defined, and if so we
	// cancel the current process before we replace it with a new one.
	switch eventKind {
	case EventKindStatic:
		if _, ok := p.StaticProcesses.procMap[et]; ok {
			p.StaticProcesses.procMap[et].Cancel()
		}
		p.StaticProcesses.procMap[et] = proc
	case EventKindDynamic:

		p.DynamicProcesses.mu.Lock()
		defer p.DynamicProcesses.mu.Unlock()
		if _, ok := p.DynamicProcesses.procMap[et]; ok {
			p.DynamicProcesses.procMap[et].Cancel()
		}
		p.DynamicProcesses.procMap[et] = proc
	case EventKindCustom:

		p.CustomProcesses.mu.Lock()
		defer p.CustomProcesses.mu.Unlock()
		if _, ok := p.CustomProcesses.procMap[et]; ok {
			p.CustomProcesses.procMap[et].Cancel()
		}
		p.CustomProcesses.procMap[et] = proc
	case EventKindError:
		// Check if a process for the same event is defined, and if so we
		// cancel the current process before we replace it with a new one.
		if _, ok := p.ErrorProcesses.procMap[et]; ok {
			p.ErrorProcesses.procMap[et].Cancel()
		}
		p.ErrorProcesses.procMap[et] = proc
	case EventKindSupervisor:
		// Check if a process for the same event is defined, and if so we
		// cancel the current process before we replace it with a new one.
		if _, ok := p.supervisorProcesses.procMap[et]; ok {
			p.supervisorProcesses.procMap[et].Cancel()
		}
		p.supervisorProcesses.procMap[et] = proc

	}
}

// // Delete an Event and it's process from the processes map.
// func (p *processes) delete(et EventType, proc *Process) {
// 	p.mu.Lock()
// 	defer p.mu.Unlock()
// 	p.procMap[et].cancel()
// 	delete(p.procMap, et)
// }

type pidnr int
type PidVsProcMap map[pidnr]*Process

// Holds information about the relation between a pid and it's process.
type pidToProc struct {
	mu sync.Mutex
	mp PidVsProcMap
}

// Add a pid and process to the map.
func (p *pidToProc) add(pid pidnr, proc *Process) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mp[pid] = proc
}

// Get the *Process based on the pid.
func (p *pidToProc) getProc(pid pidnr) *Process {
	p.mu.Lock()
	defer p.mu.Unlock()
	proc, ok := p.mp[pid]
	if !ok {
		return nil
	}
	return proc
}

// Return a copy of the pid vs *Processes map.
func (p *pidToProc) copyOfMap() *PidVsProcMap {
	m := make(PidVsProcMap)

	p.mu.Lock()
	defer p.mu.Unlock()

	for pid, proc := range p.mp {
		m[pid] = proc
	}

	return &m
}

func newPidToProc() *pidToProc {
	p := pidToProc{
		mp: make(PidVsProcMap),
	}
	return &p
}

type pids struct {
	mu     sync.Mutex
	nr     pidnr
	toProc pidToProc
}

func newPids() *pids {
	p := pids{
		nr:     0,
		toProc: *newPidToProc(),
	}

	return &p
}

func (p *pids) next() pidnr {
	p.mu.Lock()
	nr := p.nr
	p.nr++
	p.mu.Unlock()

	return nr
}

// Process defines a process.
type Process struct {
	// Process function.
	fn func()
	// Channel to receive events into the process function.
	InCh chan Event `json:"-"`
	// Channel to send events to be picked up by other processes.
	StaticEventCh chan Event `json:"-"`
	// Channel to send error events.
	ErrorEventCh chan Event `json:"-"`
	// Channel for getting the result in tests.
	TestCh chan Event `json:"-"`
	// Channel to use for routing events for dynamic processes.
	DynamicEventCh chan Event `json:"-"`
	// Channel to use for routing events for custom processes.
	CustomEventCh chan Event `json:"-"`
	// Channel to use for routing supervisor events
	SupervisorEventCh chan Event `json:"-"`
	// The event type for the process.
	Event EventType
	// The event kind of the process
	Kind EventKind
	// Maps for various staticProcess information.
	StaticProcesses *staticProcesses
	// Map of dynamic processes
	DynamicProcesses *dynamicProcesses
	// Map of custom processes
	CustomProcesses *customProcesses
	// Maps for various errProcess information
	ErrorProcesses *errorProcesses
	// Map of supervisor processes
	supervisorProcesses *supervisorProcesses
	// Is this the root process.
	isRoot bool
	// Holding all configuration settings.
	Config *Config
	// process ID struct
	pids *pids
	// PID of the process
	PID pidnr
	// Cancel func
	Cancel context.CancelFunc `json:"-"`
}

// NewRootProcess will prepare and return the root process
// which holds all the core elements needed, like the main
// channels for events and errors, and varouis registers or
// maps holding information about the system. Later created
// processes will reference these elements when they are
// created.
// The root process will also start up all the essential other
// processes needed, like the event router, and various standard
// error handling processes.
func NewRootProcess(ctx context.Context, fn ETFunc, conf *Config, etRemoteFunc ETFunc) *Process {
	ctx, cancel := context.WithCancel(ctx)

	p := Process{
		fn:                  nil,
		InCh:                make(chan Event),
		StaticEventCh:       make(chan Event),
		ErrorEventCh:        make(chan Event),
		TestCh:              make(chan Event),
		DynamicEventCh:      make(chan Event),
		CustomEventCh:       make(chan Event),
		SupervisorEventCh:   make(chan Event),
		Event:               ETRoot,
		StaticProcesses:     newStaticProcesses(),
		DynamicProcesses:    newDynamicProcesses(),
		CustomProcesses:     newCustomProcesses(),
		ErrorProcesses:      newErrorProcesses(),
		supervisorProcesses: newsuperVisorProcesses(),
		isRoot:              true,
		Config:              conf,
		pids:                newPids(),
		Cancel:              cancel,
	}

	p.PID = p.pids.nr

	if fn != nil {
		p.fn = fn(ctx, &p)
	}

	// Register and start all the standard child processes
	// that should spawn off the root process

	// Starting error handling processes.
	NewProcess(ctx, &p, ERLog, EventKindError, erLogFn).Act()
	NewProcess(ctx, &p, ERDebug, EventKindError, erDebugFn).Act()
	NewProcess(ctx, &p, ERFatal, EventKindError, erFatalFn).Act()
	NewProcess(ctx, &p, ERTest, EventKindError, erTestFn).Act()
	NewProcess(ctx, &p, ERNone, EventKindError, erNoneFn).Act()
	NewProcess(ctx, &p, ETPrint, EventKindStatic, etPrintFn).Act()

	NewProcess(ctx, &p, ETRouter, EventKindStatic, etRouterFn).Act()
	NewProcess(ctx, &p, ERRouter, EventKindError, erRouterFn).Act()
	NewProcess(ctx, &p, EDRouter, EventKindDynamic, edRouterFn).Act()
	NewProcess(ctx, &p, ECRouter, EventKindCustom, ecRouterFn).Act()
	NewProcess(ctx, &p, ESRouter, EventKindSupervisor, esRouterFn).Act()

	NewProcess(ctx, &p, ETProfiling, EventKindStatic, etProfilingFn).Act()

	if p.Config.CustomEvents {
		NewProcess(ctx, &p, ETProcessFromData, EventKindStatic, etProcessFromDataFn).Act()
		NewProcess(ctx, &p, ETWatchEventFile, EventKindStatic, wrapperETWatchEventFileFn(p.Config.CustomEventsPath, ".json")).Act()
	}

	// Starting the remainding processes.
	NewProcess(ctx, &p, ETOsSignal, EventKindStatic, etOsSignalFn).Act()
	NewProcess(ctx, &p, ETTestCh, EventKindStatic, etTestChFn).Act()
	NewProcess(ctx, &p, ETPid, EventKindStatic, etPidFn).Act()
	NewProcess(ctx, &p, ETReadFile, EventKindStatic, ETReadFileFn).Act()
	NewProcess(ctx, &p, ETOsCmd, EventKindStatic, etOsCmdFn).Act()

	NewProcess(ctx, &p, ETDone, EventKindStatic, etDoneFn).Act()
	NewProcess(ctx, &p, ETExit, EventKindStatic, etExitFn).Act()
	NewProcess(ctx, &p, ETPidGetAll, EventKindStatic, etPidGetAllFn).Act()

	NewProcess(ctx, &p, EDRouter, EventKindDynamic, edRouterFn).Act()

	// If there are no ETRemote function given as an argument, we just add
	// a function to be used for creating a dummy ETRemote process. It clears
	// the value of Event.DstNode, adding it back with AddEvent, so it will
	// be delivered locally only.
	if etRemoteFunc == nil {
		fn := func(ctx context.Context, p *Process) func() {
			fn2 := func() {
				for {
					select {
					case ev := <-p.InCh:
						// The original event are stored in the NextEvent.
						// Remove the DstNode so it don't get forwarded again,
						// and add the original event again.
						ev.NextEvent.DstNode = ""
						p.AddEvent(*ev.NextEvent)
					case <-ctx.Done():
						return
					}
				}
			}
			return fn2
		}
		NewProcess(ctx, &p, ETRemote, EventKindStatic, fn).Act()

	} else {
		NewProcess(ctx, &p, ETRemote, EventKindStatic, etRemoteFunc).Act()
	}

	return &p
}

// NewProcess will prepare and return a *Process. It will copy
// channels and map structures from the root process.
func NewProcess(ctx context.Context, parentP *Process, event EventType, kind EventKind, fn ETFunc) *Process {
	ctx, cancel := context.WithCancel(ctx)
	p := Process{
		fn:                  nil,
		InCh:                make(chan Event),
		StaticEventCh:       parentP.StaticEventCh,
		ErrorEventCh:        parentP.ErrorEventCh,
		TestCh:              parentP.TestCh,
		DynamicEventCh:      parentP.DynamicEventCh,
		CustomEventCh:       parentP.CustomEventCh,
		SupervisorEventCh:   parentP.SupervisorEventCh,
		Event:               event,
		Kind:                kind,
		StaticProcesses:     parentP.StaticProcesses,
		DynamicProcesses:    parentP.DynamicProcesses,
		CustomProcesses:     parentP.CustomProcesses,
		ErrorProcesses:      parentP.ErrorProcesses,
		supervisorProcesses: parentP.supervisorProcesses,
		isRoot:              false,
		Config:              parentP.Config,
		pids:                parentP.pids,
		PID:                 parentP.pids.next(),
		Cancel:              cancel,
	}

	p.addToProcessesMap(event, &p, kind)

	if fn != nil {
		p.fn = fn(ctx, &p)
	}
	return &p
}

// AddEvent will deliver the event to the correct router based
// on the specified EventKind of the Event.
// If the EventKind are missing the event will be handled as a static
// event.
// If the event is to be delivered to a remote node, AddEvent will also
// take care of that and ship the event off to the ETRemote process.
func (p *Process) AddEvent(event Event) {
	// Check if the Event is to be sent to a remote node by checking
	// if the DstNode of the event and the local NodeName are equal.
	//
	// If the Event is to be sent to a remote node we wrap it in an
	// ETRemote event, and forward it to the ETRemote actor here.
	if event.DstNode != p.Config.NodeName && event.DstNode != "" {
		log.Printf("******DEBUG******: event.DstNode: %v, p.NodeName: %v\n", event.DstNode, p.Config.NodeName)

		remoteEv := Event{
			EventType: ETRemote,
			EventKind: EventKindStatic,
			NextEvent: &event,
		}

		p.addEventStatic(remoteEv)
		return
	}

	// The event is
	switch event.EventKind {
	case EventKindSupervisor:
		p.addEventSuperVisor(event)
	case EventKindStatic:
		p.addEventStatic(event)
	case EventKindError:
		p.addEventError(event)
	case EventKindDynamic:
		p.addEventDynamic(event)
	case EventKindCustom:
		p.addEventCustom(event)
	default:
		panic(fmt.Sprintf("unknown EventKind: %v", event.EventKind))
	}
}

// Will add an event to be handled by the processes.
func (p *Process) addEventSuperVisor(event Event) {
	p.SupervisorEventCh <- event
}

// Will add an event to be handled by the processes.
func (p *Process) addEventStatic(event Event) {
	p.StaticEventCh <- event
}

// Will add an event to be handled by the processes.
func (p *Process) addEventDynamic(event Event) {
	p.DynamicEventCh <- event
}

// Will add an event to be handled by the processes.
func (p *Process) addEventCustom(event Event) {
	p.CustomEventCh <- event
}

// Will add an error to be handled by the error processes.
func (p *Process) addEventError(event Event) {
	p.ErrorEventCh <- event
}

// Will start the ETFunc attached to the process.
//
// If no ETFunc is defined for the process will just return
// after calling this function. The process can still be used
// and we can communicate with it via it's channels.
func (p *Process) Act() error {
	log.Printf("on node %v: Starting %v actor for EventType: %v\n", p.Config.NodeName, p.Kind, p.Event)
	if p.fn == nil {
		//go p.fn()
		return nil
	}

	p.pids.toProc.add(p.PID, p)

	go p.fn()

	return nil
}
