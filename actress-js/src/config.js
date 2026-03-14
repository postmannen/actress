// Configuration class for actress-js
export class Config {
    constructor(options = {}) {
        // Default configuration
        this.debug = options.debug !== undefined ? options.debug : false;
        this.enableMetrics = options.enableMetrics !== undefined ? options.enableMetrics : false;
        this.maxRetries = options.maxRetries !== undefined ? options.maxRetries : 3;
        this.retryDelay = options.retryDelay !== undefined ? options.retryDelay : 100;
        this.enableWorkers = options.enableWorkers !== undefined ? options.enableWorkers : false;
        this.workerScript = options.workerScript || null;
        
        // Load from environment/localStorage if in browser
        this.loadFromEnvironment();
    }

    // Load configuration from browser environment
    loadFromEnvironment() {
        if (typeof window !== 'undefined' && window.localStorage) {
            // Load from localStorage
            const storedConfig = localStorage.getItem('actress-js-config');
            if (storedConfig) {
                try {
                    const parsed = JSON.parse(storedConfig);
                    Object.assign(this, parsed);
                } catch (e) {
                    console.warn('Failed to parse stored actress-js config:', e);
                }
            }
        }
        
        // Check for global configuration
        if (typeof window !== 'undefined' && window.ACTRESS_CONFIG) {
            Object.assign(this, window.ACTRESS_CONFIG);
        }
    }

    // Save configuration to localStorage
    save() {
        if (typeof window !== 'undefined' && window.localStorage) {
            localStorage.setItem('actress-js-config', JSON.stringify(this));
        }
    }

    // Update configuration
    update(options) {
        Object.assign(this, options);
        this.save();
    }

    // Get configuration as plain object
    toObject() {
        return {
            debug: this.debug,
            enableMetrics: this.enableMetrics,
            maxRetries: this.maxRetries,
            retryDelay: this.retryDelay,
            enableWorkers: this.enableWorkers,
            workerScript: this.workerScript
        };
    }
} 