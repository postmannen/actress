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
	EventType EventType `json:"eventType" yaml:"eventType" cbor:"eventType"`
	// EventKind is a more general way to describe the event that can
	// be used to destinguish if it is static, error or dynamic event.
	EventKind EventKind `json:"eventKind" yaml:"eventKind" cbor:"eventKind"`
	// Cmd is usually used for giving instructions or parameters for
	// what an event shall do.
	Cmd []string `json:"cmd" yaml:"cmd" cbor:"cmd"`
	// Instruction got the underlying type of string. This field can
	// be used to give for example an instruction of a single word.
	// For example in switch statements at the receiving actor, or other.
	Instruction Instruction
	// Args are similar to Cmd, but the parameters can be stored in
	// a hashmap as key/value items.
	Args map[string]string `json:"args" yaml:"args" cbor:"args"`
	// Data usually carries the data from one process to the next. Example
	// could be a file read on process1 is put in the Data field, and
	// passed on to process2 to be unmarshaled.
	Data []byte `json:"data" yaml:"data" cbor:"data"`
	// Data to be transfered internally. Example is to send config directly via
	// the channel between internal actors.
	InternalCh chan chan []byte `json:"-" yaml:"-" cbor:"-"`
	// Err is used for defining the error message when the event is used
	// as an error event.
	Err error `json:"error" yaml:"error" cbor:"error"`
	// NextEvent defines a series of events to be executed like a workflow.
	// The receiving process should check this field for what kind of event
	// to create as the next step in the workflow.
	NextEvent *Event `json:"nextEvent" yaml:"nextEvent" cbor:"nextEvent"`
	// PreviousEvent allows for keeping information about the previous event if needed.
	PreviousEvent *Event `json:"previousEvent" yaml:"previousEvent" cbor:"previousEvent"`
	// Dst node.
	DstNode Node `json:"dst" yaml:"dst" cbor:"dst"`
	// Src node.
	SrcNode Node `json:"src" yaml:"src" cbor:"src"`
}

type Instruction string

type EventKind string

const EventKindStatic EventKind = "EventKindStatic"
const EventKindError EventKind = "EventKindError"

// NewDynProcess will prepare and return a *Process. It will copy
// channels and map structures from the root process.
// The purpose of dynamic processes is to have short lived processes
// that can be quickly started, and removed again when it's job is done.
// The only difference between a process and a dynamic process are that
// the dynamic processes have a mutex in processes map DynamicProcesses so
// we also can delete the processes when they are no longer needed.
const EventKindDynamic EventKind = "EventKindDynamic"
const EventKindCustom EventKind = "EventKindCustom"
const EventKindSupervisor EventKind = "EventKindSupervisor"

type Node string

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

// The main Root process. By default the root process don't have an ETFunc
// registered with it to handle the ETRoot eventtype, but one can be created
// with the normal ETFunc function signature, and defined when creating a
// new root process.
const ETRoot EventType = "ETRoot"

// Function type describing the signature of a function that is to be used
// when creating a new process.
type ETFunc func(context.Context, *Process) func()
