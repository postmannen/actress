import { Event, EventType, ETRouter, ETPrint, ETDone, ETExit, ERLog, ERDebug, ERFatal } from './event.js';

// Router process function for routing events to correct processes
export function routerProcessFn(event, process) {
    // This is handled by the main process event queue processing
    // The router functionality is built into the Process class
    return Promise.resolve();
}

// Print process function - logs events to console
export function printProcessFn(event, process) {
    if (event.data !== null && event.data !== undefined) {
        console.log('Print:', event.data);
    }
    if (event.cmd && event.cmd.length > 0) {
        console.log('Print cmd:', event.cmd.join(' '));
    }
    
    // If there's a next event, execute it
    if (event.nextEvent) {
        process.addEvent(event.nextEvent);
    }
    
    return Promise.resolve();
}

// Done process function - signals completion
export function doneProcessFn(event, process) {
    console.log('Process done:', process.eventType.name);
    
    // If there's a next event, execute it
    if (event.nextEvent) {
        process.addEvent(event.nextEvent);
    }
    
    return Promise.resolve();
}

// Exit process function - stops the entire system
export function exitProcessFn(event, process) {
    console.log('Exit requested');
    
    // Cancel all processes
    process.processes.forEach(proc => proc.cancel());
    process.dynProcesses.forEach(proc => proc.cancel());
    process.errProcesses.forEach(proc => proc.cancel());
    
    return Promise.resolve();
}

// Test channel process function - forwards events to test queue
export function testChProcessFn(event, process) {
    process.addTestEvent(event);
    return Promise.resolve();
}

// Error router process function
export function errorRouterProcessFn(event, process) {
    // Route error to appropriate error handler
    const errorHandler = process.errProcesses.get(event.eventType.name);
    if (errorHandler) {
        errorHandler.inQueue.push(event);
        if (errorHandler.isRunning) {
            errorHandler.processInQueue();
        }
    } else {
        // Default error handling
        console.error('Unhandled error:', event.error);
    }
    
    return Promise.resolve();
}

// Log error process function
export function logErrorProcessFn(event, process) {
    if (event.error) {
        console.log('Log:', event.error.message || event.error);
    }
    return Promise.resolve();
}

// Debug error process function
export function debugErrorProcessFn(event, process) {
    if (event.error) {
        console.debug('Debug:', event.error.message || event.error);
    }
    return Promise.resolve();
}

// Fatal error process function
export function fatalErrorProcessFn(event, process) {
    if (event.error) {
        console.error('Fatal:', event.error.message || event.error);
    }
    
    // Stop the system on fatal errors
    process.addEvent(new Event(ETExit));
    
    return Promise.resolve();
}

// HTTP GET process function - performs HTTP requests
export function httpGetProcessFn(event, process) {
    return new Promise(async (resolve, reject) => {
        try {
            let url = '';
            
            if (event.cmd && event.cmd.length > 0) {
                url = event.cmd[0];
            } else if (event.data && typeof event.data === 'string') {
                url = event.data;
            }
            
            if (!url) {
                throw new Error('No URL provided for HTTP GET');
            }
            
            const response = await fetch(url);
            const data = await response.text();
            
            // If there's a next event, pass the data along
            if (event.nextEvent) {
                event.nextEvent.data = data;
                process.addEvent(event.nextEvent);
            }
            
            resolve();
        } catch (error) {
            process.addError(new Event(ERLog, { error }));
            reject(error);
        }
    });
}

// Timer process function - creates delayed events
export function timerProcessFn(event, process) {
    return new Promise((resolve) => {
        let delay = 1000; // default 1 second
        
        if (event.cmd && event.cmd.length > 0) {
            delay = parseInt(event.cmd[0]) || delay;
        }
        
        setTimeout(() => {
            if (event.nextEvent) {
                process.addEvent(event.nextEvent);
            }
            resolve();
        }, delay);
    });
}

/**
 * Custom command wrapper - creates a process function that executes custom logic
 */
export function wrapperCustomCmd(customFunction) {
    return function(event, process) {
        return new Promise(async (resolve, reject) => {
            try {
                const result = await customFunction(event, process);
                
                // If there's a next event and we have result data, pass it along
                if (event.nextEvent && result !== undefined) {
                    event.nextEvent.data = result;
                    process.addEvent(event.nextEvent);
                }
                
                resolve(result);
            } catch (error) {
                process.addError(new Event(ERLog, { error }));
                reject(error);
            }
        });
    };
} 