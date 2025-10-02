import { Event, EventType, ETRoot, ERLog, ERDebug } from './event.js';
import { Config } from './config.js';

// Process defines a process/actor.
export class Process {
    constructor(eventType, processFunction, options = {}) {
        this.eventType = eventType;
        this.processFunction = processFunction;
        this.pid = options.pid || Math.floor(Math.random() * 1000000);
        this.isRoot = options.isRoot || false;
        
        // Event queues
        this.inQueue = [];
        this.eventQueue = [];
        this.errorQueue = [];
        this.testQueue = [];
        
        // Process registries
        this.processes = options.processes || new Map();
        this.dynProcesses = options.dynProcesses || new Map();
        this.errProcesses = options.errProcesses || new Map();
        
        // Configuration
        this.config = options.config || new Config();
        
        // State management
        this.isRunning = false;
        this.isCancelled = false;
        this.worker = null;
        
        // Event listeners
        this.eventListeners = new Map();
        
        // Bind methods
        this.addEvent = this.addEvent.bind(this);
        this.addError = this.addError.bind(this);
        this.addTestEvent = this.addTestEvent.bind(this);
    }

    // Add an event to be processed
    addEvent(event) {
        if (this.isCancelled) return;
        
        this.eventQueue.push(event);
        this.processEventQueue();
    }

    // Add an error event
    addError(event) {
        if (this.isCancelled) return;
        
        this.errorQueue.push(event);
        this.processErrorQueue();
    }

    // Add a test event
    addTestEvent(event) {
        if (this.isCancelled) return;
        
        this.testQueue.push(event);
    }

    // Process the event queue
    async processEventQueue() {
        while (this.eventQueue.length > 0 && !this.isCancelled) {
            const event = this.eventQueue.shift();
            
            // Find the target process
            const targetProcess = this.processes.get(event.eventType.name);
            if (targetProcess) {
                targetProcess.inQueue.push(event);
                if (targetProcess.isRunning) {
                    targetProcess.processInQueue();
                }
            } else {
                // Try to deliver the message with retry
                this.retryEventDelivery(event);
            }
        }
    }

    // Process the error queue
    async processErrorQueue() {
        while (this.errorQueue.length > 0 && !this.isCancelled) {
            const event = this.errorQueue.shift();
            
            // Find error handler
            const errorHandler = this.errProcesses.get(event.eventType.name);
            if (errorHandler) {
                errorHandler.inQueue.push(event);
                if (errorHandler.isRunning) {
                    errorHandler.processInQueue();
                }
            } else {
                // Default error handling
                console.error('Unhandled error event:', event);
            }
        }
    }

    // Retry event delivery with backoff
    async retryEventDelivery(event, maxRetries = 3) {
        for (let i = 0; i < maxRetries; i++) {
            await new Promise(resolve => setTimeout(resolve, 100 * (i + 1)));
            
            const targetProcess = this.processes.get(event.eventType.name);
            if (targetProcess) {
                targetProcess.inQueue.push(event);
                if (targetProcess.isRunning) {
                    targetProcess.processInQueue();
                }
                return;
            }
        }
        
        // Log error if delivery failed
        this.addError(new Event(ERLog, {
            error: new Error(`No process registered for event type: ${event.eventType.name}`)
        }));
    }

    // Process the input queue
    async processInQueue() {
        while (this.inQueue.length > 0 && !this.isCancelled) {
            const event = this.inQueue.shift();
            
            try {
                if (this.processFunction) {
                    await this.processFunction(event, this);
                }
            } catch (error) {
                this.addError(new Event(ERLog, {
                    error: error
                }));
            }
        }
    }

    // Start the process
    async act() {
        if (this.isRunning) return;
        
        this.isRunning = true;
        this.isCancelled = false;
        
        // Start processing queues
        this.processInQueue();
        this.processEventQueue();
        this.processErrorQueue();
        
        return this;
    }

    // Stop the process
    cancel() {
        this.isCancelled = true;
        this.isRunning = false;
        
        if (this.worker) {
            this.worker.terminate();
            this.worker = null;
        }
    }

    // Register this process in the parent's process map
    register(parentProcess) {
        if (parentProcess && parentProcess.processes) {
            parentProcess.processes.set(this.eventType.name, this);
        }
    }

    // Create a dynamic process
    createDynamicProcess(eventType, processFunction) {
        const dynProcess = new Process(eventType, processFunction, {
            pid: this.pid + Math.floor(Math.random() * 1000),
            processes: this.processes,
            dynProcesses: this.dynProcesses,
            errProcesses: this.errProcesses,
            config: this.config
        });

        this.dynProcesses.set(eventType.name, dynProcess);
        return dynProcess;
    }

    // Remove a dynamic process
    removeDynamicProcess(eventType) {
        const dynProcess = this.dynProcesses.get(eventType.name);
        if (dynProcess) {
            dynProcess.cancel();
            this.dynProcesses.delete(eventType.name);
        }
    }

    // Get all process information
    getProcessInfo() {
        return {
            pid: this.pid,
            eventType: this.eventType.name,
            isRunning: this.isRunning,
            isCancelled: this.isCancelled,
            inQueueLength: this.inQueue.length,
            eventQueueLength: this.eventQueue.length,
            errorQueueLength: this.errorQueue.length
        };
    }
} 