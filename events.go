package actress

// The strucure of an event.
type Event struct {
	// EventType eventType `json:"eventType" yaml:"eventType"`
	EventType EventType `json:"eventType" yaml:"eventType"`
	Cmd       []string  `json:"cmd" yaml:"cmd"`
	Data      []byte    `json:"data" yaml:"data"`
	Err       error     `json:"error" yaml:"error"`
	NextEvent *Event    `json:"event" yaml:"event"`
}

type EventType string

// Event types
const (
	ETRoot     EventType = "ETRoot"
	ETRouter   EventType = "ETRouter"
	ETExit     EventType = "ETExit"
	ETOsSignal EventType = "ETOsSignal"

	ETProfiling EventType = "ETprofiling"

	ETPrint EventType = "ETPrint"
	ETDone  EventType = "ETDone"

	ERRouter EventType = "ERRouter"
	ERLog    EventType = "ERLog"
	ERDebug  EventType = "ERDebug"
	ERFatal  EventType = "ERFatal"
)
