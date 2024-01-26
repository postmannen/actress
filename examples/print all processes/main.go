package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/postmannen/actress"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	rootAct := actress.NewRootProcess(ctx)
	rootAct.Act()

	// Start all the registered actors.
	err := rootAct.Act()
	if err != nil {
		log.Fatal(err)
	}

	rootAct.AddEvent(actress.Event{EventType: actress.ETPidGetAll,
		NextEvent: &actress.Event{
			EventType: actress.ETTestCh},
	},
	)

	ev := <-rootAct.TestCh
	tmpProc := make(actress.PidVsProcMap)
	err = json.Unmarshal(ev.Data, &tmpProc)

	for k, v := range tmpProc {
		fmt.Printf("pid: %v, process: %v\n", k, v)
	}
	cancel()
}
