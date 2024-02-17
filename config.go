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
	"log"
	"os"
	"strconv"
)

type Config struct {
	Profiling        string
	CustomEvents     bool
	Metrics          bool
	CustomEventsPath string
}

// New config will check flags and env variables set, and prepare
// and return the resulting *config.
func NewConfig() *Config {
	// The config with default values set.
	c := Config{
		Profiling:        "none",
		CustomEvents:     false,
		Metrics:          false,
		CustomEventsPath: "customevents",
	}

	c.Profiling = CheckEnv("PROFILING", c.Profiling).(string)
	c.CustomEvents = CheckEnv("CUSTOMEVENTS", c.CustomEvents).(bool)
	c.Metrics = CheckEnv("METRICS", c.Metrics).(bool)
	c.CustomEventsPath = CheckEnv("CUSTOMEVENTSPATH", c.CustomEventsPath).(string)

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
