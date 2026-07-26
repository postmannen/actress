# Actress-JS

A Concurrent Actor framework for JavaScript/Browser applications, based on the idea of the Go Actress library.

## Overview

Actress-JS is an opinionated take on an actor system. Create custom processes where each process performs specific tasks, communicating through events to pass results and chain operations together as workflows.

### Key Features

- **Event-driven Architecture**: Processes communicate via events
- **Process Management**: Create, manage, and coordinate multiple concurrent processes
- **Event Chaining**: Chain events together to create workflows using NextEvent
- **Error Handling**: Built-in error handling and routing system
- **Browser Optimized**: Designed for browser environments
- **Zero Dependencies**: Pure JavaScript with no external dependencies

## Quick Start

### Basic Setup

```html
<!DOCTYPE html>
<html>
<head>
    <title>Actress-JS Example</title>
</head>
<body>
    <script type="module">
        import { Actress, EventType, Event, ETPrint } from './src/actress.js';

        // Initialize the actress system
        const actress = new Actress();
        await actress.init();

        // Define a custom event type
        const ETCustom = new EventType('ETCustom');

        // Create a process
        const customProcess = actress.addProcess(ETCustom, async (event, process) => {
            console.log('Processing:', event.data);
            
            // Chain to print process
            if (event.nextEvent) {
                event.nextEvent.data = 'Processed: ' + event.data;
                process.addEvent(event.nextEvent);
            }
        });

        await customProcess.act();

        // Send an event
        actress.sendEvent(new Event(ETCustom, {
            data: 'Hello World',
            nextEvent: new Event(ETPrint)
        }));
    </script>
</body>
</html>
```

### Two Processes Example

```javascript
import { Actress, EventType, Event, ERDebug } from './src/actress.js';

const actress = new Actress();
await actress.init();

// Define event types
const ETTest1 = new EventType('ETTest1');
const ETTest2 = new EventType('ETTest2');

// First process: convert to uppercase
const test1Process = actress.addProcess(ETTest1, async (event, process) => {
    const upper = event.data.toUpperCase();
    
    if (event.nextEvent) {
        event.nextEvent.data = upper;
        process.addEvent(event.nextEvent);
    }
});

// Second process: add dots
const test2Process = actress.addProcess(ETTest2, async (event, process) => {
    const result = event.data + '...';
    console.log('Final result:', result);
    
    // Send completion signal
    process.addError(new Event(ERDebug, {
        error: new Error('Processing complete')
    }));
});

// Start processes
await test1Process.act();
await test2Process.act();

// Send event with chain
actress.sendEvent(new Event(ETTest1, {
    data: 'hello world',
    nextEvent: new Event(ETTest2)
}));
```

## Core Concepts

### Events

Events are the communication mechanism between processes:

```javascript
import { Event, EventType } from './src/actress.js';

const event = new Event(new EventType('MyEvent'), {
    cmd: ['param1', 'param2'],      // Command parameters
    data: 'some data',              // Data payload
    nextEvent: nextEvent            // Chain to next event
});
```

### Processes

Processes are actors that handle specific event types:

```javascript
const myProcess = actress.addProcess(eventType, async (event, process) => {
    // Process the event
    const result = await doSomething(event.data);
    
    // Chain to next event if specified
    if (event.nextEvent) {
        event.nextEvent.data = result;
        process.addEvent(event.nextEvent);
    }
});
```

### Event Types

Event types are unique identifiers for routing events to processes:

```javascript
import { EventType } from './src/actress.js';

const ETCustom = new EventType('ETCustom');
const ETProcess = new EventType('ETProcess');
```

## Built-in Event Types

Actress-JS includes several built-in event types:

- **ETPrint**: Logs data to console
- **ETDone**: Signals process completion
- **ETExit**: Shuts down the system
- **ETTestCh**: Routes events to test queue
- **ERLog**: Error logging
- **ERDebug**: Debug logging
- **ERFatal**: Fatal error handling

## Built-in Process Functions

### HTTP GET Process

```javascript
import { httpGetProcessFn, EventType, Event, ETPrint } from './src/actress.js';

const ETHttpGet = new EventType('ETHttpGet');
const httpProcess = actress.addProcess(ETHttpGet, httpGetProcessFn);
await httpProcess.act();

// Fetch data and print result
actress.sendEvent(new Event(ETHttpGet, {
    cmd: ['https://api.github.com/zen'],
    nextEvent: new Event(ETPrint)
}));
```

### Timer Process

```javascript
import { timerProcessFn, EventType, Event } from './src/actress.js';

const ETTimer = new EventType('ETTimer');
const timerProcess = actress.addProcess(ETTimer, timerProcessFn);
await timerProcess.act();

// Wait 2 seconds then execute next event
actress.sendEvent(new Event(ETTimer, {
    cmd: ['2000'], // 2000ms delay
    nextEvent: new Event(ETPrint, { data: 'Timer completed!' })
}));
```

## Configuration

Configure the actress system:

```javascript
import { Actress, Config } from './src/actress.js';

const config = new Config({
    debug: true,
    enableMetrics: true,
    maxRetries: 5,
    retryDelay: 200
});

const actress = new Actress(config);
```

## Error Handling

```javascript
import { Event, ERLog, ERDebug, ERFatal } from './src/actress.js';

// Log an error
process.addError(new Event(ERLog, {
    error: new Error('Something went wrong')
}));

// Debug message
process.addError(new Event(ERDebug, {
    error: new Error('Debug info')
}));

// Fatal error (stops system)
process.addError(new Event(ERFatal, {
    error: new Error('Critical error')
}));
```

## Dynamic Processes

Create and manage dynamic processes:

```javascript
// Create dynamic process
const dynProcess = parentProcess.createDynamicProcess(
    new EventType('ETDynamic'),
    async (event, process) => {
        // Process logic
        console.log('Dynamic process:', event.data);
        
        // Remove self when done
        parentProcess.removeDynamicProcess(event.eventType);
    }
);

await dynProcess.act();
```

## System Status

Monitor the system:

```javascript
const status = actress.getStatus();
console.log('System status:', status);
// Output:
// {
//   initialized: true,
//   isRunning: true,
//   processCount: 5,
//   dynProcessCount: 0,
//   errProcessCount: 4,
//   rootProcess: { pid: 0, eventType: 'ETRoot', ... }
// }
```

## Testing

Access test events for testing:

```javascript
// Send test events
process.addTestEvent(new Event(ETTestCh, { data: 'test data' }));

// Get test events
const testEvents = actress.getTestEvents();

// Clear test events
actress.clearTestEvents();
```

## Examples

See the `examples/` directory for complete working examples:

## Browser Compatibility

Actress-JS should work in most browsers.

## License

AGPL-3.0
