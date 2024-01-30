package actress

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	Profiling    string
	CustomEvents bool
	Metrics      bool
}

// New config will check flags and env variables set, and prepare
// and return the resulting *config.
func NewConfig() *Config {
	// The config with default values set.
	c := Config{
		Profiling:    "none",
		CustomEvents: false,
		Metrics:      false,
	}

	c.Profiling = CheckEnv("PROFILING", c.Profiling).(string)
	c.CustomEvents = CheckEnv("CUSTOMEVENTS", c.CustomEvents).(bool)
	c.Metrics = CheckEnv("METRICS", c.Metrics).(bool)

	return &c
}

// Check if an env variable is set. If found, return the value.
// Takes the name of the env variable, and the actual variable
// containing a default value as it's input.
func CheckEnv[T any](key string, v T) any {
	val, ok := os.LookupEnv(key)
	if !ok {
		return v
	}

	switch any(v).(type) {
	case int:
		n, err := strconv.Atoi(val)
		if err != nil {
			log.Fatalf("error: failed to convert env to int: %v\n", n)
		}
		return n
	case string:
		return val
	case bool:
		if val == "1" || val == "true" {
			return true
		} else {
			return false
		}
	}

	return nil
}
