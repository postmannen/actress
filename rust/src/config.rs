// Actress Copyright (C) 2024  Bjørn Tore Svinningen
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

use crate::events::Node;
use std::env;

// Config holds all the configuration settings for the actress system.
pub struct Config {
    pub CustomEvents: bool,
    pub Metrics: bool,
    pub CustomEventsPath: String,
    pub NodeName: Node,
    pub LogLevel: String,
}

// NewConfig prepares a Config with default values, then applies environment
// variable overrides. The Go version returns both a *Config and a *flag.FlagSet
// so the caller can parse command line flags; here we apply the environment
// overrides directly (which is what the flag defaults do via CheckEnv) and
// return the Config. The environment always wins over the defaults.
//
// The logLevel is the default log level for the system, and can be provided as
// an input argument.
pub fn NewConfig(logLevel: &str) -> Config {
    let mut c = Config {
        CustomEvents: false,
        Metrics: false,
        CustomEventsPath: "customevents".to_string(),
        NodeName: Node::new("replaceme"),
        LogLevel: logLevel.to_string(),
    };

    if let Ok(v) = env::var("CUSTOMEVENTS") {
        c.CustomEvents = v == "1" || v == "true";
    }
    if let Ok(v) = env::var("METRICS") {
        c.Metrics = v == "1" || v == "true";
    }
    if let Ok(v) = env::var("CUSTOMEVENTSPATH") {
        c.CustomEventsPath = v;
    }
    if let Ok(v) = env::var("NODENAME") {
        c.NodeName = Node::from_string(v);
    }
    if let Ok(v) = env::var("LOGLEVEL") {
        c.LogLevel = v;
    }

    c
}
