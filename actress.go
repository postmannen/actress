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
	"log"
	"sync"
)

// processes holds information about what process functions
// who belongs to what event, and also a map of the started
// processes.
type processes struct {
	procMap map[EventType]*Process
}

// Add a new Event and it's process to the processes map.
func (p *processes) add(et EventType, proc *Process) {
	// Check if a process for the same event is defined, and if so we
	// cancel the current process before we replace it with a new one.
	if _, ok := p.procMap[et]; ok {
		p.procMap[et].Cancel()
	}
	p.procMap[et] = proc
}

// // Delete an Event and it's process from the processes map.
// func (p *processes) delete(et EventType, proc *Process) {
// 	p.mu.Lock()
// 	defer p.mu.Unlock()
// 	p.procMap[et].cancel()
// 	delete(p.procMap, et)
// }

// Checks if the event is defined in the processes map, and returns true if it is.
func (p *processes) IsEventDefined(ev EventType) bool {
	if _, ok := p.procMap[ev]; !ok {
		return false
	}

	return true
}

// Prepare and return a new *processes structure.
func newProcesses() *processes {
	p := processes{
		procMap: make(map[EventType]*Process),
	}
	return &p
}

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
	EventCh chan Event `json:"-"`
	// Channel to send error events.
	ErrorCh chan Event `json:"-"`
	// Channel for getting the result in tests.
	TestCh chan Event `json:"-"`
	// Channel to use for routing events for dynamic processes.
	DynCh chan Event `json:"-"`
	// The event type for the process.
	Event EventType
	// Maps for various process information.
	Processes *processes
	// Map of dynamic processes
	DynProcesses *dynProcesses
	// Maps for various errProcess information
	ErrProcesses *errProcesses
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
func NewRootProcess(ctx context.Context) *Process {
	ctx, cancel := context.WithCancel(ctx)
	conf := NewConfig()

	p := Process{
		fn:           nil,
		InCh:         make(chan Event, 1),
		EventCh:      make(chan Event, 1),
		ErrorCh:      make(chan Event, 1),
		TestCh:       make(chan Event, 1),
		DynCh:        make(chan Event, 1),
		Event:        ETRoot,
		Processes:    newProcesses(),
		DynProcesses: newDynProcesses(),
		ErrProcesses: newErrProcesses(),
		isRoot:       true,
		Config:       conf,
		pids:         newPids(),
		Cancel:       cancel,
	}

	p.PID = p.pids.nr

	// Register and start all the standard child processes
	// that should spawn off the root process

	NewProcess(ctx, p, ETProfiling, etProfilingFn).Act()

	if p.Config.CustomEvents {
		NewProcess(ctx, p, ETCustomEvent, ETCustomEventFn).Act()
		NewProcess(ctx, p, ETWatchEventFile, wrapperETWatchEventFileFn(p.Config.CustomEventsPath, ".json")).Act()
	}

	NewProcess(ctx, p, ETRouter, etRouterFn).Act()
	NewProcess(ctx, p, ETOsSignal, etOsSignalFn).Act()
	NewProcess(ctx, p, ETTestCh, etTestChFn).Act()
	NewProcess(ctx, p, ETPid, etPidFn).Act()
	NewProcess(ctx, p, ETReadFile, ETReadFileFn).Act()
	NewProcess(ctx, p, ETOsCmd, etOsCmdFn).Act()

	NewProcess(ctx, p, ETDone, etDoneFn).Act()
	NewProcess(ctx, p, ETPrint, etPrintFn).Act()
	NewProcess(ctx, p, ETExit, etExitFn).Act()
	NewProcess(ctx, p, ETPidGetAll, etPidGetAllFn).Act()

	NewDynProcess(ctx, p, EDRouter, edRouterFn).Act()

	NewErrProcess(ctx, p, ERRouter, erRouterFn).Act()
	NewErrProcess(ctx, p, ERLog, erLogFn).Act()
	NewErrProcess(ctx, p, ERDebug, erDebugFn).Act()
	NewErrProcess(ctx, p, ERFatal, erFatalFn).Act()
	NewErrProcess(ctx, p, ERTest, erTestFn).Act()

	return &p
}

// NewProcess will prepare and return a *Process. It will copy
// channels and map structures from the root process.
func NewProcess(ctx context.Context, parentP Process, event EventType, fn ETFunc) *Process {
	ctx, cancel := context.WithCancel(ctx)
	p := Process{
		fn:           nil,
		InCh:         make(chan Event, 1),
		EventCh:      parentP.EventCh,
		ErrorCh:      parentP.ErrorCh,
		TestCh:       parentP.TestCh,
		DynCh:        parentP.DynCh,
		Event:        event,
		Processes:    parentP.Processes,
		DynProcesses: parentP.DynProcesses,
		ErrProcesses: parentP.ErrProcesses,
		isRoot:       false,
		Config:       parentP.Config,
		pids:         parentP.pids,
		PID:          parentP.pids.next(),
		Cancel:       cancel,
	}

	p.Processes.add(event, &p)

	if fn != nil {
		p.fn = fn(ctx, &p)
	}
	return &p
}

// Will add an event to be handled by the processes.
func (p *Process) AddEvent(event Event) {
	p.EventCh <- event
}

// Will add an event to be handled by the processes.
func (p *Process) AddDynEvent(event Event) {
	p.DynCh <- event
}

// Will add an error to be handled by the error processes.
func (p *Process) AddError(event Event) {
	p.ErrorCh <- event
}

// Will start the current process.
func (p *Process) Act() error {
	log.Printf("Starting actor for EventType: %v\n", p.Event)
	if p.fn == nil {
		//go p.fn()
		return nil
	}

	p.pids.toProc.add(p.PID, p)

	go p.fn()

	return nil
}
