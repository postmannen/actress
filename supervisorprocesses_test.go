package actress

import (
	"context"
	"fmt"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestESProcesses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCh := make(chan string)

	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	NewProcess(ctx, rootp, ETTest, ETTestfn(testCh)).Act()
	// NewProcess(ctx, rootp, ESProcesses, KindSupervisor, esProcessesFn()).Act()

	md := esProcessesMapDataIn{
		Name: "TestType1",
	}

	// ---------- Add item to the esprocesses map

	b, err := cbor.Marshal(md)
	if err != nil {
		t.Fatalf("error: failed to marshal map data: %v", err)
	}

	rootp.AddEvent(Event{
		Name: ESProcesses,

		Instruction: InstructionESProcessesAdd,
		Data:        b,
		NextEvent:   &Event{Name: ETTest},
	})

	// Wait for the the reply back, which are the ETTest .NextEvent
	select {
	case <-testCh:
	case <-ctx.Done():
		return
	}

	// ---------- Get item to the esprocesses map

	rootp.AddEvent(Event{
		Name: ESProcesses,

		Instruction: InstructionESProcessesGetAll,
		NextEvent:   &Event{Name: ETTest},
	})

	// Wait for the the reply back, which are the ETTest .NextEvent
	select {
	case ev := <-testCh:
		pm := ESProcessesMap{}
		err := cbor.Unmarshal([]byte(ev), &pm)
		if err != nil {
			t.Fatalf("error: failed to unmarshal process map: %v", err)
		}

		// TODO: FIX THIS TEST WHEN WE HAVE REMOVED THE KIND TYPE.
		if pm["TestType1"] != "TestType1" {
			t.Fatalf("The received map did not contain the expected value %+v\n", pm)
		}

		fmt.Printf("\n###########################################################################\n")
		fmt.Printf("# Map contained: %v\n", pm)
		fmt.Printf("###########################################################################\n")
	case <-ctx.Done():
		return
	}

	// ---------- Delete item from the esprocesses map

	rootp.AddEvent(Event{
		Name: ESProcesses,

		Instruction: InstructionESProcessesDelete,
		Data:        b,
		NextEvent:   &Event{Name: ETTest},
	})

	// Wait for the the reply back, which are the ETTest .NextEvent
	select {
	case <-testCh:
	case <-ctx.Done():
		return
	}

}
