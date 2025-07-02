// Event defines an event. It holds:
// - The EventType, which specifies the process are meant for.
// - The Cmd, are meant to but not limited to be a way to give
//   instructions for what a process should do.
// - The Data field are meant to carry the result from the work
//   done by a process, to the next process.
// - Both Cmd and Data can be used interchangeably if it makes
//   more sense for a given scenario.
// - Err, are used by the error event type.
// - NextEvent are used when we want to define a chain of events
//   to be executed.

export class Event {
    constructor(eventType, options = {}) {
        this.nr = 0;
        this.eventType = eventType;
        this.cmd = options.cmd || [];
        this.data = options.data || null;
        this.error = options.error || null;
        this.nextEvent = options.nextEvent || null;
    }
}

// EventType is a unique name used to identify events. It is used both for
// creating processes and also for routing messages to the correct process.
export class EventType {
    constructor(name) {
        this.name = name;
    }

    toString() {
        return this.name;
    }

    equals(other) {
        return this.name === other.name;
    }
}

// Built-in Event Types
export const ETRoot = new EventType('ETRoot');
export const ETRouter = new EventType('ETRouter');
export const ETTestCh = new EventType('ETTestCh');
export const ETPrint = new EventType('ETPrint');
export const ETDone = new EventType('ETDone');
export const ETExit = new EventType('ETExit');

// Error Event Types
export const ERRouter = new EventType('ERRouter');
export const ERLog = new EventType('ERLog');
export const ERDebug = new EventType('ERDebug');
export const ERFatal = new EventType('ERFatal');
export const ERTest = new EventType('ERTest');

// Factory function to create new events with optional parameters
export function newEvent(eventType, options = {}) {
    return new Event(eventType, options);
}

// Option functions for creating events
export const EventOptions = {
    withCmd: (cmd) => ({ cmd }),
    withData: (data) => ({ data }),
    withError: (error) => ({ error }),
    withNext: (nextEvent) => ({ nextEvent })
}; 