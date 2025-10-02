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
	"flag"
	"log"
	"os"
	"strconv"
)

// Config holds all the configuration settings for the actress system.
type Config struct {
	CustomEvents     bool
	Metrics          bool
	CustomEventsPath string
	NodeName         Node
	LogLevel         string
}

// New config prepare a *Config, and a *flag.FlagSet, and return the
// resulting actress *Config and *flag.FlagSet.
// The flags are checked for env variables, and if not found, the default value is used.
// The flagset needs to be parsed for the flags to be set.
//
// The logLevel is the default log level for the system, and can be provided as an input argument.
func NewConfig(logLevel string) (*Config, *flag.FlagSet) {
	// The config with default values set.
	c := Config{
		CustomEvents:     false,
		Metrics:          false,
		CustomEventsPath: "customevents",
		NodeName:         "replaceme",
		LogLevel:         logLevel,
	}

	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.BoolVar(&c.CustomEvents, "customEvents", CheckEnv("CUSTOMEVENTS", c.CustomEvents).(bool), "custom events (env CUSTOMEVENTS)")
	fs.BoolVar(&c.Metrics, "metrics", CheckEnv("METRICS", c.Metrics).(bool), "metrics (env METRICS)")
	fs.StringVar(&c.CustomEventsPath, "customEventsPath", CheckEnv("CUSTOMEVENTSPATH", c.CustomEventsPath).(string), "custom events path (env CUSTOMEVENTSPATH)")
	fs.StringVar((*string)(&c.NodeName), "nodeName", CheckEnv("NODENAME", string(c.NodeName)).(string), "nodename (env NODENAME)")
	fs.StringVar(&c.LogLevel, "logLevel", CheckEnv("LOGLEVEL", c.LogLevel).(string), "log level (env LOGLEVEL), allowed values are: debug, info, error, fatal, none")
	return &c, fs
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
