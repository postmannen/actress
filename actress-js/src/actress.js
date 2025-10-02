import { Process } from './process.js';
import { Event, EventType, ETRoot, ETRouter, ETPrint, ETDone, ETExit, ETTestCh, 
         ERRouter, ERLog, ERDebug, ERFatal } from './event.js';
import { Config } from './config.js';
import { routerProcessFn, printProcessFn, doneProcessFn, exitProcessFn, 
         testChProcessFn, errorRouterProcessFn, logErrorProcessFn, 
         debugErrorProcessFn, fatalErrorProcessFn } from './builtins.js';

// Create a new root process which holds all the core elements needed,
// like the main channels for events and errors, and various registers
// holding information about the system.
export function newRootProcess(processFunction = null, config = null) {
    const rootConfig = config || new Config();
    
    const rootProcess = new Process(ETRoot, processFunction, {
        pid: 0,
        isRoot: true,
        processes: new Map(),
        dynProcesses: new Map(),
        errProcesses: new Map(),
        config: rootConfig
    });

    // Set up built-in processes
    setupBuiltinProcesses(rootProcess);
    
    return rootProcess;
}

// Create a new process and register it with the parent process
export function newProcess(parentProcess, eventType, processFunction, options = {}) {
    const process = new Process(eventType, processFunction, {
        pid: parentProcess.pid + Math.floor(Math.random() * 1000),
        processes: parentProcess.processes,
        dynProcesses: parentProcess.dynProcesses,
        errProcesses: parentProcess.errProcesses,
        config: parentProcess.config,
        ...options
    });

    // Register the process
    process.register(parentProcess);
    
    return process;
}

// Set up built-in system processes
function setupBuiltinProcesses(rootProcess) {
    // Router process - handles event routing (built into Process class)
    const routerProcess = newProcess(rootProcess, ETRouter, routerProcessFn);
    
    // Print process
    const printProcess = newProcess(rootProcess, ETPrint, printProcessFn);
    
    // Done process
    const doneProcess = newProcess(rootProcess, ETDone, doneProcessFn);
    
    // Exit process
    const exitProcess = newProcess(rootProcess, ETExit, exitProcessFn);
    
    // Test channel process
    const testProcess = newProcess(rootProcess, ETTestCh, testChProcessFn);
    
    // Error processes
    const errorRouterProcess = new Process(ERRouter, errorRouterProcessFn, {
        pid: rootProcess.pid + 100,
        processes: rootProcess.processes,
        dynProcesses: rootProcess.dynProcesses,
        errProcesses: rootProcess.errProcesses,
        config: rootProcess.config
    });
    rootProcess.errProcesses.set(ERRouter.name, errorRouterProcess);
    
    const logErrorProcess = new Process(ERLog, logErrorProcessFn, {
        pid: rootProcess.pid + 101,
        processes: rootProcess.processes,
        dynProcesses: rootProcess.dynProcesses,
        errProcesses: rootProcess.errProcesses,
        config: rootProcess.config
    });
    rootProcess.errProcesses.set(ERLog.name, logErrorProcess);
    
    const debugErrorProcess = new Process(ERDebug, debugErrorProcessFn, {
        pid: rootProcess.pid + 102,
        processes: rootProcess.processes,
        dynProcesses: rootProcess.dynProcesses,
        errProcesses: rootProcess.errProcesses,
        config: rootProcess.config
    });
    rootProcess.errProcesses.set(ERDebug.name, debugErrorProcess);
    
    const fatalErrorProcess = new Process(ERFatal, fatalErrorProcessFn, {
        pid: rootProcess.pid + 103,
        processes: rootProcess.processes,
        dynProcesses: rootProcess.dynProcesses,
        errProcesses: rootProcess.errProcesses,
        config: rootProcess.config
    });
    rootProcess.errProcesses.set(ERFatal.name, fatalErrorProcess);
    
    // Start error processes
    errorRouterProcess.act();
    logErrorProcess.act();
    debugErrorProcess.act();
    fatalErrorProcess.act();
}


// Actress class - main interface for the actor system
export class Actress {
    constructor(config = null) {
        this.config = config || new Config();
        this.rootProcess = null;
        this.isRunning = false;
    }

    // Initialize the actress system
    async init(rootProcessFunction = null) {
        this.rootProcess = newRootProcess(rootProcessFunction, this.config);
        await this.rootProcess.act();
        this.isRunning = true;
        return this;
    }

    // Add a new process
    addProcess(eventType, processFunction, options = {}) {
        if (!this.rootProcess) {
            throw new Error('Actress system not initialized. Call init() first.');
        }
        
        const process = newProcess(this.rootProcess, eventType, processFunction, options);
        return process;
    }

    // Send an event to the system
    sendEvent(event) {
        if (!this.rootProcess) {
            throw new Error('Actress system not initialized. Call init() first.');
        }
        
        this.rootProcess.addEvent(event);
    }

    // Get system status
    getStatus() {
        if (!this.rootProcess) {
            return { initialized: false };
        }
        
        return {
            initialized: true,
            isRunning: this.isRunning,
            processCount: this.rootProcess.processes.size,
            dynProcessCount: this.rootProcess.dynProcesses.size,
            errProcessCount: this.rootProcess.errProcesses.size,
            rootProcess: this.rootProcess.getProcessInfo()
        };
    }

    // Shutdown the actress system
    async shutdown() {
        if (this.rootProcess) {
            this.rootProcess.addEvent(new Event(ETExit));
            this.isRunning = false;
        }
    }

    // Get test events (for testing purposes)
    getTestEvents() {
        if (!this.rootProcess) return [];
        return [...this.rootProcess.testQueue];
    }

    // Clear test events
    clearTestEvents() {
        if (this.rootProcess) {
            this.rootProcess.testQueue.length = 0;
        }
    }
}

// Export main classes and functions
export { Process, Event, EventType, Config };
export * from './event.js';
export * from './builtins.js'; 