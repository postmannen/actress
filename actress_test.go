package actress

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
)

func TestEventProcs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCh := make(chan string)

	const ETTest EventType = "ETTest"

	tFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)
				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	rootp := NewRootProcess(ctx)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	NewProcess(ctx, *rootp, ETTest, tFunc).Act()

	rootp.AddEvent(Event{EventType: ETTest, Data: []byte("test")})
	if r := <-testCh; r != "test" {
		t.Fatalf("ETTest failed\n")
	}
}

func TestDynamicProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCh := make(chan string)

	const EDTest EventType = "EDTest"

	tFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)
				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	rootp := NewRootProcess(ctx)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	NewDynProcess(ctx, *rootp, EDTest, tFunc).Act()

	rootp.AddDynEvent(Event{EventType: EDTest, Data: []byte("test")})
	if r := <-testCh; r != "test" {
		t.Fatalf("EDTest failed\n")
	}
}

func TestDynamicProcess2(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const EDTest EventType = "EDTest"
	// Test channel for receiving the final result.
	testCh := make(chan string)

	// tFunc is the function to be used with EventType EDTest.
	// When receiving an EDTest event, we start up a dynamic
	// process. The EventType to use for the new inner dynamic
	// process can be found in the Cmd[2] field of the event
	// to EDTest. Cmd[1] holds the other dynamic process to
	// send Event to.
	tFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case ev := <-p.InCh:
					dyn1EVType := ev.Cmd[1]
					dyn2EVType := ev.Cmd[2]
					t.Logf("\ndyn1EVType: %v\ndyn2EVType: %v\n", dyn1EVType, dyn2EVType)

					// Define and start the process for dyn2EVType.
					NewDynProcess(ctx, *p, EventType(dyn2EVType),
						func(ctx context.Context, p *Process) func() {
							return func() {
								select {
								case <-p.InCh:
									// Send an event to the dyn1EVType process.
									p.AddDynEvent(Event{EventType: EventType(dyn1EVType), Data: []byte("from dyn2")})

									// We are now done with the dyn2EVType process so we delete it.
									p.DynProcesses.Delete(EventType(dyn2EVType))
									t.Logf("\nsuccessfully deleted process: %v\n", dyn2EVType)
								case <-ctx.Done():
									return
								}
							}
						}).Act()

				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	rootp := NewRootProcess(ctx)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	NewDynProcess(ctx, *rootp, EDTest, tFunc).Act()

	// Create UUID's to be used for EventType's for the dynamic processes.
	// We put them in the .Cmd field of EDTest so the receiver also know
	// about them.
	dyn1EVType := NewUUID()
	dyn2EVType := NewUUID()

	NewDynProcess(ctx, *rootp, EventType(dyn1EVType),
		func(ctx context.Context, p *Process) func() {
			return func() {
				p.AddDynEvent(Event{EventType: EventType(dyn2EVType)})

				select {
				case ev := <-p.InCh:
					testCh <- string(ev.Data)
				case <-ctx.Done():
					return
				}
			}
		}).Act()

	rootp.AddDynEvent(Event{EventType: EDTest, Cmd: []string{"", dyn1EVType, dyn2EVType}, Data: []byte("test")})

	if r := <-testCh; r != "from dyn2" {
		t.Fatalf("EDTest failed\n")
	} else {
		t.Log("\n\U0001F602 SUCCESS")
	}
}

func TestDynamicProcessReaderWriter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const EDTest EventType = "EDTest"
	// Test channel for receiving the final result.
	testCh := make(chan string)

	rootp := NewRootProcess(ctx)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	// tFunc is the function to be used with EventType EDTest.
	// When receiving an EDTest event, we start up a dynamic
	// process. The EventType to use for the new inner dynamic
	// process can be found in the Cmd[2] field of the event
	// to EDTest. Cmd[1] holds the other dynamic process to
	// send Event to.
	edTestFn := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case ev := <-p.InCh:
					dyn1EVType := ev.Cmd[1]
					dyn2EVType := ev.Cmd[2]
					t.Logf("\ndyn1EVType: %v\ndyn2EVType: %v\n", dyn1EVType, dyn2EVType)

					// Define and start the process for dyn2EVType.
					NewDynProcess(ctx, *p, EventType(dyn2EVType),
						func(ctx context.Context, p *Process) func() {
							return func() {
								select {
								case <-p.InCh:
									// Send an event to the dyn1EVType process.
									tmpEv := Event{EventType: EventType(dyn1EVType)}

									erw := NewEventRW(p, &tmpEv, "in dyn2EVType reader writer")
									erw.Write([]byte("from dyn2"))

									p.AddDynEvent(tmpEv)

									// We are now done with the dyn2EVType process so we delete it.
									p.DynProcesses.Delete(EventType(dyn2EVType))
									t.Logf("\nsuccessfully deleted process: %v\n", dyn2EVType)
								case <-ctx.Done():
									return
								}
							}
						}).Act()

				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewDynProcess(ctx, *rootp, EDTest, edTestFn).Act()

	// Create UUID's to be used for EventType's for the dynamic processes.
	// We put them in the .Cmd field of EDTest so the receiver also know
	// about them.
	dyn1EVType := NewUUID()
	dyn2EVType := NewUUID()

	NewDynProcess(ctx, *rootp, EventType(dyn1EVType),
		func(ctx context.Context, p *Process) func() {
			return func() {
				p.AddDynEvent(Event{EventType: EventType(dyn2EVType)})

				select {
				case ev := <-p.InCh:
					erw := NewEventRW(p, &ev, "dyn1EVType Reader/Writer")
					b, err := io.ReadAll(erw)
					if err != nil {
						t.Fatalf("dyn1EVType ReadAll failed: %v\n", err)
					}

					testCh <- string(b)
				case <-ctx.Done():
					return
				}
			}
		}).Act()

	rootp.AddDynEvent(Event{EventType: EDTest, Cmd: []string{"", dyn1EVType, dyn2EVType}, Data: []byte("test")})

	if r := <-testCh; r != "from dyn2" {
		t.Fatalf("EDTest failed\n")
	} else {
		t.Log("\n\U0001F602 SUCCESS")
	}
}

func TestNextEventProcs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootp := NewRootProcess(ctx)

	testCh := make(chan string)
	const ETTest EventType = "ETTest"

	testFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)
				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, *rootp, ETTest, testFunc).Act()

	const ETNextEvent EventType = "ETNextEvent"

	nextEventFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case ev := <-p.InCh:
					// Pass the data from the current event into the next event.
					nextEvent := *ev.NextEvent
					nextEvent.Data = ev.Data
					p.EventCh <- nextEvent
				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, *rootp, ETNextEvent, nextEventFunc).Act()
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	rootp.AddEvent(Event{
		EventType: ETNextEvent,
		Data:      []byte("test"),
		NextEvent: &Event{EventType: ETTest}})
	if r := <-testCh; r != "test" {
		t.Fatalf("ETTest failed\n")
	}
}

// func TestPidToProcess(t *testing.T) {
// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()
//
// 	rootp := NewRootProcess(ctx)
// 	err := rootp.Act()
// 	if err != nil {
// 		t.Fatal(err)
// 	}
//
// 	// Since ETRouter is the first process to be started we can
// 	// check that the first value in the map is an ETRouter.
// 	if p := rootp.pids.toProc.getProc(0); p.Event != ETRouter {
// 		t.Fatalf("error: process nr 0 was not ETRouter\n")
// 	}
// }

func TestPidToProcMap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootp := NewRootProcess(ctx)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	rootp.AddEvent(Event{EventType: ETPidGetAll, NextEvent: &Event{EventType: ETTestCh}})

	ev := <-rootp.TestCh
	mapFromEv := make(PidVsProcMap)

	err = json.Unmarshal(ev.Data, &mapFromEv)
	if err != nil {
		t.Fatal(err)
	}

	// Compare the map we got with the actual map.
	mapFromActual := rootp.pids.toProc.copyOfMap()

	// Check that the length of the two maps are equal
	if len(*mapFromActual) != len(mapFromEv) {
		t.Fatalf("length of maps are not equal, evMap: %v, actualMap: %v\n", len(*mapFromActual), len(mapFromEv))
	}

	// Check all elements.
	for k := range *mapFromActual {
		if _, ok := mapFromEv[k]; !ok {
			t.Fatalf("missing map value: %v\n", k)
		}
	}

	//t.Fatalf("got event: %v, Data: %v\n", ev.EventType, string(ev.Data))
}

// -------------------------------------------------------------
// Benchmarks
// -------------------------------------------------------------

func BenchmarkSingleProcess(b *testing.B) {
	//log.SetOutput(io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCh := make(chan string)

	const ETTest EventType = "ETTest"

	tFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)
				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	rootp := NewRootProcess(ctx)
	NewProcess(ctx, *rootp, ETTest, tFunc).Act()
	err := rootp.Act()
	if err != nil {
		b.Fatal(err)
	}

	for n := 0; n < b.N; n++ {
		rootp.AddEvent(Event{EventType: ETTest, Data: []byte("test")})
		if r := <-testCh; r != "test" {
			b.Fatalf("ETTest failed\n")
		}
	}
}

func BenchmarkSingleProcessEventAndError(b *testing.B) {
	//log.SetOutput(io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCh := make(chan string)

	const ETTest EventType = "ETTest"

	tFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)
					p.ErrorCh <- Event{EventType: ERTest, Err: fmt.Errorf("some error:%v", result)}
				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	rootp := NewRootProcess(ctx)
	err := rootp.Act()
	if err != nil {
		b.Fatal(err)
	}
	NewProcess(ctx, *rootp, ETTest, tFunc).Act()

	for n := 0; n < b.N; n++ {
		rootp.AddEvent(Event{EventType: ETTest, Data: []byte("test")})
		rootp.ErrorCh <- Event{EventType: ERTest, Err: fmt.Errorf("some error:%v", "apekatt")}
		if r := <-testCh; r != "test" {
			b.Fatalf("ETTest failed\n")
		}
	}
}

func BenchmarkTwoProcesses(b *testing.B) {
	//log.SetOutput(io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootp := NewRootProcess(ctx)

	testCh := make(chan string)

	const ETTest1 EventType = "ETTest1"
	const ETTest2 EventType = "ETTest2"

	tFunc1 := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					p.EventCh <- Event{EventType: ETTest2, Data: result.Data}

				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, *rootp, ETTest1, tFunc1).Act()

	tFunc2 := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)

				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, *rootp, ETTest2, tFunc2).Act()

	err := rootp.Act()
	if err != nil {
		b.Fatal(err)
	}

	for n := 0; n < b.N; n++ {
		rootp.AddEvent(Event{EventType: ETTest1, Data: []byte("test")})
		if r := <-testCh; r != "test" {
			b.Fatalf("ETTest failed\n")
		}
	}
}

func BenchmarkThreeProcesses(b *testing.B) {
	//log.SetOutput(io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootp := NewRootProcess(ctx)

	testCh := make(chan string)

	const ETTest1 EventType = "ETTest1"
	const ETTest2 EventType = "ETTest2"
	const ETTest3 EventType = "ETTest3"

	tFunc1 := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					p.EventCh <- Event{EventType: ETTest2, Data: result.Data}
				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, *rootp, ETTest1, tFunc1).Act()

	tFunc2 := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					p.EventCh <- Event{EventType: ETTest3, Data: result.Data}
				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, *rootp, ETTest2, tFunc2).Act()

	tFunc3 := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)

				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, *rootp, ETTest3, tFunc3).Act()

	err := rootp.Act()
	if err != nil {
		b.Fatal(err)
	}

	for n := 0; n < b.N; n++ {
		rootp.AddEvent(Event{EventType: ETTest1, Data: []byte("test")})
		if r := <-testCh; r != "test" {
			b.Fatalf("ETTest failed\n")
		}
	}
}
