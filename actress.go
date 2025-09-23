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
	"time"

	"github.com/fxamacker/cbor/v2"
)

// Add a new Event and it's process to the processes map.
// Check if a process for the same event is defined, if so we
// cancel the current process before we replace it with a new one.
func (p *Process) addToProcessesMap() {
	switch p.Kind {
	case KindStatic:
		if _, ok := p.StaticProcesses.procMap[p.Event]; ok {
			p.StaticProcesses.procMap[p.Event].Cancel()
		}
		p.StaticProcesses.procMap[p.Event] = p
	case KindDynamic:
		p.DynamicProcesses.mu.Lock()
		defer p.DynamicProcesses.mu.Unlock()
		if _, ok := p.DynamicProcesses.procMap[p.Event]; ok {
			p.DynamicProcesses.procMap[p.Event].Cancel()
		}
		p.DynamicProcesses.procMap[p.Event] = p
	case KindCustom:
		p.CustomProcesses.mu.Lock()
		defer p.CustomProcesses.mu.Unlock()
		if _, ok := p.CustomProcesses.procMap[p.Event]; ok {
			p.CustomProcesses.procMap[p.Event].Cancel()
		}
		p.CustomProcesses.procMap[p.Event] = p
	case KindError:
		if _, ok := p.ErrorProcesses.procMap[p.Event]; ok {
			p.ErrorProcesses.procMap[p.Event].Cancel()
		}
		p.ErrorProcesses.procMap[p.Event] = p
	case KindSupervisor:
		p.supervisorProcesses.mu.Lock()
		defer p.supervisorProcesses.mu.Unlock()
		if _, ok := p.supervisorProcesses.procMap[p.Event]; ok {
			p.supervisorProcesses.procMap[p.Event].Cancel()
		}
		p.supervisorProcesses.procMap[p.Event] = p

	}
}

// Delete an Event and it's process from the processes map.
func (p *Process) deleteFromProcessesMap() {
	// Check if a process for the same event is defined, and if so we
	// cancel the current process before we replace it with a new one.
	switch p.Kind {
	case KindStatic:
		p.AddEvent(Event{
			Name:        ERLog,
			Kind:        KindError,
			Instruction: InstructionError,
			Err:         fmt.Errorf("not allowed to delete static process")})
	case KindDynamic:
		p.DynamicProcesses.mu.Lock()
		delete(p.DynamicProcesses.procMap, p.Event)
		p.DynamicProcesses.mu.Unlock()
	case KindCustom:
		p.CustomProcesses.mu.Lock()
		delete(p.CustomProcesses.procMap, p.Event)
		p.CustomProcesses.mu.Unlock()
	case KindError:
		p.AddEvent(Event{
			Name:        ERLog,
			Kind:        KindError,
			Instruction: InstructionError,
			Err:         fmt.Errorf("not allowed to delete error process")})
	case KindSupervisor:
		p.AddEvent(Event{
			Name:        ERLog,
			Kind:        KindError,
			Instruction: InstructionError,
			Err:         fmt.Errorf("not allowed to delete supervisor process")})

	}
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

// Delete a pid and process from the map.
func (p *pidToProc) remove(pid pidnr) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.mp, pid)
}

// // Get the *Process based on the pid.
// func (p *pidToProc) getProc(pid pidnr) *Process {
// 	p.mu.Lock()
// 	defer p.mu.Unlock()
// 	proc, ok := p.mp[pid]
// 	if !ok {
// 		return nil
// 	}
// 	return proc
// }

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

// Create and return a new *pidToProc struct.
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

// Create and return a new *pids struct.
func newPids() *pids {
	p := pids{
		nr:     0,
		toProc: *newPidToProc(),
	}

	return &p
}

// Will return the next pidnr for a new process.
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
	Event EventName
	// The event kind of the process
	Kind Kind
	// Maps for various staticProcess information.
	// NB: Added a Mutex on this structure, though it should really not be needed,
	//	since there is only reads from the static procMap. Decide later if we should
	//	remove it again.
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
	// The context of the process
	Ctx context.Context `json:"-"`
	// Cancel func for context of the process
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
		Ctx:                 ctx,
		Cancel:              cancel,
	}

	p.PID = p.pids.nr

	if fn != nil {
		p.fn = fn(ctx, &p)
	}

	// Register and start all the standard child processes of root.
	//
	// When starting Root, all the needed processes to handle the system are not
	// yet started. For example both the ESRouter and the ESProcesses, and the
	// Error processes are needed to be able to register processes in the
	// ESProcesses process.
	// To be able to register the processes with the esProcesses actor we store
	// the information about each process started in a slice, so when all
	// processes are started we can go through each element of the slice and
	// use the information to register all the processes in ESProcesses.
	pi := newRegisterProcessInfo()

	// Starting error handling processes.
	NewProcess(ctx, &p, ERLog, KindError, erLogFn).actForRoot(pi)
	NewProcess(ctx, &p, ERTest, KindError, erTestFn).actForRoot(pi)
	NewProcess(ctx, &p, ERNone, KindError, erNoneFn).actForRoot(pi)
	NewProcess(ctx, &p, ETPrint, KindStatic, etPrintFn).actForRoot(pi)

	NewProcess(ctx, &p, ETRouter, KindStatic, etRouterFn).actForRoot(pi)
	NewProcess(ctx, &p, ERRouter, KindError, erRouterFn).actForRoot(pi)
	NewProcess(ctx, &p, EDRouter, KindDynamic, edRouterFn).actForRoot(pi)
	NewProcess(ctx, &p, ECRouter, KindCustom, ecRouterFn).actForRoot(pi)
	NewProcess(ctx, &p, ESRouter, KindSupervisor, esRouterFn).actForRoot(pi)

	NewProcess(ctx, &p, ESProcesses, KindSupervisor, esProcessesFn()).actForRoot(pi)

	if p.Config.CustomEvents {
		NewProcess(ctx, &p, ETProcessFromData, KindStatic, etProcessFromDataFn).actForRoot(pi)
		NewProcess(ctx, &p, ETWatchEventFile, KindStatic, wrapperETWatchEventFileFn(p.Config.CustomEventsPath, ".json")).actForRoot(pi)
	}

	// Starting the remainding processes.
	NewProcess(ctx, &p, ETOsSignal, KindStatic, etOsSignalFn).actForRoot(pi)
	NewProcess(ctx, &p, ETTestCh, KindStatic, etTestChFn).actForRoot(pi)
	NewProcess(ctx, &p, ETPid, KindStatic, etPidFn).actForRoot(pi)
	NewProcess(ctx, &p, ETReadFile, KindStatic, ETReadFileFn).actForRoot(pi)
	NewProcess(ctx, &p, ETOsCmd, KindStatic, etOsCmdFn).actForRoot(pi)

	NewProcess(ctx, &p, ETDone, KindStatic, etDoneFn).actForRoot(pi)
	NewProcess(ctx, &p, ETExit, KindStatic, etExitFn).actForRoot(pi)
	NewProcess(ctx, &p, ETPidGetAll, KindStatic, etPidGetAllFn).actForRoot(pi)

	NewProcess(ctx, &p, EDRouter, KindDynamic, edRouterFn).actForRoot(pi)

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
					case <-p.Ctx.Done():
						return
					}
				}
			}
			return fn2
		}
		// Create a new ETRemote process using the default deliver localfunction we created above.
		NewProcess(ctx, &p, ETRemote, KindStatic, fn).actForRoot(pi)

	} else {
		// If a ETRemote function is given as an argument, we use that function
		// to create a new ETRemote process.
		NewProcess(ctx, &p, ETRemote, KindStatic, etRemoteFunc).actForRoot(pi)
	}

	RegisterProcessesInESProcesses(&p, pi)

	return &p
}

// Register all the processes in ESProcesses.
func RegisterProcessesInESProcesses(p *Process, pi *registerProcessInfo) {
	// Register all the processes in ESProcesses.
	for _, md := range *pi {
		func() {
			syncCh := make(chan struct{})
			syncP := NewProcess(p.Ctx, p, ETSync, KindDynamic, ETSyncFn(syncCh))
			syncP.Act()
			defer syncP.Stop()

			b, err := cbor.Marshal(md)
			if err != nil {
				log.Fatalf("error: NewRootProcess: failed to marshal map data: %v", err)
			}

			p.AddEvent(Event{
				Name:        ESProcesses,
				Kind:        KindSupervisor,
				Instruction: InstructionESProcessesAdd,
				Data:        b,
				NextEvent: &Event{
					Name: ETSync,
					Kind: KindDynamic},
			})

			<-syncCh
		}()
	}
}

// Used for storing information about the processes to be registered in ESProcesses
// when starting the root process.
type registerProcessInfo []esProcessesMapDataIn

// Create and return a new *registerProcessInfo struct.
func newRegisterProcessInfo() *registerProcessInfo {
	pi := registerProcessInfo{}
	return &pi
}

// NewProcess will prepare and return a *Process. It will copy
// channels and map structures from the root process.
func NewProcess(ctx context.Context, parentP *Process, event EventName, kind Kind, fn ETFunc) *Process {
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
		Ctx:                 ctx,
		Cancel:              cancel,
	}

	if fn != nil {
		p.fn = fn(ctx, &p)
	}
	return &p
}

// AddEvent will deliver the event to the correct router based
// on the specified Kind of the Event.
// If the Kind are missing the event will be handled as a static
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

		remoteEv := Event{
			Name:      ETRemote,
			Kind:      KindStatic,
			NextEvent: &event,
		}

		p.AddEvent(Event{Name: ERLog,
			Instruction: InstructionDebug,
			Kind:        KindError,
			Err:         fmt.Errorf("[AddEvent] on %v, wrapping event nr %v: %v in ETRemote, event.DstNode: %v", p.Config.NodeName, event.Nr, event.Name, event.DstNode)})

		p.addEventStatic(remoteEv)
		return
	}

	// The event is
	switch event.Kind {
	case KindStatic:
		p.addEventStatic(event)
	case KindError:
		// If the log level is not "none", we add the event to the error processes.
		// If it is "none" we just return so the event is dropped, and nothing is logged.
		// InstructionFatal is always logged, even if the log level is "none".
		if p.Config.LogLevel != "none" && event.Instruction != InstructionFatal {
			p.addEventError(event)
		}
		return
	case KindSupervisor:
		p.addEventSuperVisor(event)
	case KindDynamic:
		p.addEventDynamic(event)
	case KindCustom:
		p.addEventCustom(event)
	default:
		panic(fmt.Sprintf("unknown Kind: %v", event.Kind))
	}
}

// Will add an event to be handled by the processes.
func (p *Process) addEventSuperVisor(event Event) {
	p.AddEvent(Event{Name: ERLog,
		Instruction: InstructionDebug,
		Kind:        KindError,
		Err:         fmt.Errorf("[addEventSuperVisor][1 of 2] on %v, event : %v", p.Config.NodeName, CopyEventFields(&event))})
	p.SupervisorEventCh <- event
	p.AddEvent(Event{Name: ERLog,
		Instruction: InstructionDebug,
		Kind:        KindError,
		Err:         fmt.Errorf("[addEventSuperVisor][2 of 2] on %v, event : %v", p.Config.NodeName, CopyEventFields(&event))})

}

// Will add an event to be handled by the processes.
func (p *Process) addEventStatic(event Event) {
	p.AddEvent(Event{Name: ERLog,
		Instruction: InstructionDebug,
		Kind:        KindError,
		Err:         fmt.Errorf("[addEventStatic][1 of 2] before adding event to StaticEventCh on %v, event : %v", p.Config.NodeName, CopyEventFields(&event))})
	select {
	case p.StaticEventCh <- event:
		p.AddEvent(Event{Name: ERLog,
			Instruction: InstructionDebug,
			Kind:        KindError,
			Err:         fmt.Errorf("[addEventStatic][2 of 2] added event to StaticEventCh on %v, event : %v", p.Config.NodeName, CopyEventFields(&event))})
	case <-time.After(time.Second * 5):
		p.AddEvent(Event{Name: ERLog,
			Instruction: InstructionError,
			Kind:        KindError,
			Err:         fmt.Errorf("[addEventStatic] TIMEOUT: reason...one of the later AddEvent commands probably are not working well. Check the debug output: %+v", event)})
	}
}

// Will add an event to be handled by the processes.
func (p *Process) addEventDynamic(event Event) {
	p.AddEvent(Event{Name: ERLog,
		Instruction: InstructionDebug,
		Kind:        KindError,
		Err:         fmt.Errorf("[addEventDynamic][1 of 2] on %v, event : %v", p.Config.NodeName, CopyEventFields(&event))})
	p.DynamicEventCh <- event
	p.AddEvent(Event{Name: ERLog,
		Instruction: InstructionDebug,
		Kind:        KindError,
		Err:         fmt.Errorf("[addEventDynamic][2 of 2] on %v, event : %v", p.Config.NodeName, CopyEventFields(&event))})

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
	if p.Config.LogLevel != "none" {
		log.Printf("on node %v: Starting %v actor for Name: %v\n", p.Config.NodeName, p.Kind, p.Event)
	}

	p.pids.toProc.add(p.PID, p)

	p.addToProcessesMap()

	if p.fn != nil {
		go p.fn()
	}

	return nil
}

// actForRoot is similar to the Act function, but it is used for starting the root process
// so we can register the processes in ESProcesses after all the processes are started.
func (p *Process) actForRoot(pi *registerProcessInfo) error {

	if p.Config.LogLevel != "none" {
		log.Printf("on node %v: ROOT ACTOR: Starting %v actor for Name: %v\n", p.Config.NodeName, p.Kind, p.Event)
	}
	p.pids.toProc.add(p.PID, p)

	p.addToProcessesMap()

	if p.fn != nil {
		go p.fn()
	}

	toRegister := esProcessesMapDataIn{
		Name: p.Event,
		Kind: p.Kind,
	}

	*pi = append(*pi, toRegister)

	return nil

}

// Will Cancel the context attached to the process.
// Will delete the process from the processes map.
// Will delete the pid from the pids map.
func (p *Process) Stop() error {
	p.Cancel()
	p.deleteFromProcessesMap()
	p.pids.toProc.remove(p.PID)

	return nil
}
