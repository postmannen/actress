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
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestEventProcs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCh := make(chan string)

	const ETTest EventName = "ETTest"

	tFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)
				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}
	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	NewProcess(ctx, rootp, ETTest, KindStatic, tFunc).Act()

	rootp.AddEvent(Event{Name: ETTest,
		Kind: KindStatic,
		Data: []byte("test")})
	if r := <-testCh; r != "test" {
		t.Fatalf("ETTest failed\n")
	}
}

func TestDynamicProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCh := make(chan string)

	const EDTest EventName = "EDTest"

	tFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)
				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}
	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	NewProcess(ctx, rootp, EDTest, KindDynamic, tFunc).Act()

	rootp.AddEvent(Event{Name: EDTest,
		Kind: KindDynamic,
		Data: []byte("test")})
	if r := <-testCh; r != "test" {
		t.Fatalf("EDTest failed\n")
	}
}

func TestDynamicProcess2(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const EDTest EventName = "EDTest"
	// Test channel for receiving the final result.
	testCh := make(chan string)

	// tFunc is the function to be used with Name EDTest.
	// When receiving an EDTest event, we start up a dynamic
	// process. The Name to use for the new inner dynamic
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
					NewProcess(ctx, p, EventName(dyn2EVType), KindDynamic,
						func(ctx context.Context, p *Process) func() {
							return func() {
								select {
								case <-p.InCh:
									// Send an event to the dyn1EVType process.
									p.AddEvent(Event{Name: EventName(dyn1EVType),
										Kind: KindDynamic,
										Data: []byte("from dyn2")})

									// We are now done with the dyn2EVType process so we delete it.
									p.DynamicProcesses.Delete(EventName(dyn2EVType))

									p.AddEvent(Event{Name: ERLog,
										Kind:        KindError,
										Instruction: InstructionInfo,
										Err:         fmt.Errorf("successfully deleted process: %v", dyn2EVType)})

								case <-p.Ctx.Done():
									return
								}
							}
						}).Act()

				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	NewProcess(ctx, rootp, EDTest, KindDynamic, tFunc).Act()

	// Create UUID's to be used for Name's for the dynamic processes.
	// We put them in the .Cmd field of EDTest so the receiver also know
	// about them.
	dyn1EVType := NewUUID()
	dyn2EVType := NewUUID()

	NewProcess(ctx, rootp, EventName(dyn1EVType), KindDynamic,
		func(ctx context.Context, p *Process) func() {
			return func() {
				p.AddEvent(Event{Name: EventName(dyn2EVType),
					Kind: KindDynamic})

				select {
				case ev := <-p.InCh:
					testCh <- string(ev.Data)
				case <-p.Ctx.Done():
					return
				}
			}
		}).Act()

	rootp.AddEvent(Event{Name: EDTest,
		Kind: KindDynamic,
		Cmd:  []string{"", dyn1EVType, dyn2EVType}, Data: []byte("test")})

	if r := <-testCh; r != "from dyn2" {
		t.Fatalf("EDTest failed\n")
	} else {
		t.Log("\n\U0001F602 SUCCESS")
	}
}

func TestDynamicProcessReaderWriter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const EDTest EventName = "EDTest"
	// Test channel for receiving the final result.
	testCh := make(chan string)

	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	// tFunc is the function to be used with Name EDTest.
	// When receiving an EDTest event, we start up a dynamic
	// process. The Name to use for the new inner dynamic
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
					NewProcess(ctx, p, EventName(dyn2EVType), KindDynamic,
						func(ctx context.Context, p *Process) func() {
							return func() {
								select {
								case <-p.InCh:
									// Send an event to the dyn1EVType process.
									tmpEv := Event{Name: EventName(dyn1EVType), Kind: KindDynamic}

									erw := NewEventRW(p, &tmpEv, "in dyn2EVType reader writer")
									erw.Write([]byte("from dyn2"))

									p.AddEvent(tmpEv)

									// We are now done with the dyn2EVType process so we delete it.
									p.DynamicProcesses.Delete(EventName(dyn2EVType))

									p.AddEvent(Event{Name: ERLog,
										Kind:        KindError,
										Instruction: InstructionInfo,
										Err:         fmt.Errorf("successfully deleted process: %v", dyn2EVType)})

								case <-p.Ctx.Done():
									return
								}
							}
						}).Act()

				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, rootp, EDTest, KindDynamic, edTestFn).Act()

	// Create UUID's to be used for Name's for the dynamic processes.
	// We put them in the .Cmd field of EDTest so the receiver also know
	// about them.
	dyn1EVType := NewUUID()
	dyn2EVType := NewUUID()

	NewProcess(ctx, rootp, EventName(dyn1EVType), KindDynamic,
		func(ctx context.Context, p *Process) func() {
			return func() {
				p.AddEvent(Event{Name: EventName(dyn2EVType), Kind: KindDynamic})

				select {
				case ev := <-p.InCh:
					erw := NewEventRW(p, &ev, "dyn1EVType Reader/Writer")
					b, err := io.ReadAll(erw)
					if err != nil {
						t.Fatalf("dyn1EVType ReadAll failed: %v\n", err)
					}

					testCh <- string(b)
				case <-p.Ctx.Done():
					return
				}
			}
		}).Act()

	rootp.AddEvent(Event{Name: EDTest, Kind: KindDynamic, Cmd: []string{"", dyn1EVType, dyn2EVType}, Data: []byte("test")})

	if r := <-testCh; r != "from dyn2" {
		t.Fatalf("EDTest failed\n")
	} else {
		t.Log("\n\U0001F602 SUCCESS")
	}
}

func TestNextEventProcs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)

	testCh := make(chan string)
	const ETTest EventName = "ETTest"

	testFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)
				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, rootp, ETTest, KindStatic, testFunc).Act()

	const ETNextEvent EventName = "ETNextEvent"

	nextEventFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case ev := <-p.InCh:
					// Pass the data from the current event into the next event.
					nextEvent := *ev.NextEvent
					nextEvent.Data = ev.Data
					p.StaticEventCh <- nextEvent
				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, rootp, ETNextEvent, KindStatic, nextEventFunc).Act()
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	rootp.AddEvent(Event{
		Name:      ETNextEvent,
		Kind:      KindStatic,
		Data:      []byte("test"),
		NextEvent: &Event{Name: ETTest}})
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

	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	rootp.AddEvent(Event{Name: ETPidGetAll,
		Kind: KindStatic,
		NextEvent: &Event{Name: ETTestCh,
			Kind: KindStatic,
		}})

	ev := <-rootp.TestCh
	mapFromEv := make(PidVsProcMap)

	err = cbor.Unmarshal(ev.Data, &mapFromEv)
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

	//t.Fatalf("got event: %v, Data: %v\n", ev.Name, string(ev.Data))
}

// -------------------------------------------------------------
// Benchmarks
// -------------------------------------------------------------

func BenchmarkSingleProcess(b *testing.B) {
	//log.SetOutput(io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCh := make(chan string)

	const ETTest EventName = "ETTest"

	tFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)
				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)
	NewProcess(ctx, rootp, ETTest, KindStatic, tFunc).Act()
	err := rootp.Act()
	if err != nil {
		b.Fatal(err)
	}

	for n := 0; n < b.N; n++ {
		rootp.AddEvent(Event{Name: ETTest, Kind: KindStatic, Data: []byte("test")})
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

	const ETTest EventName = "ETTest"

	tFunc := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)
					p.ErrorEventCh <- Event{Name: ERTest, Err: fmt.Errorf("some error:%v", result)}
				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)
	err := rootp.Act()
	if err != nil {
		b.Fatal(err)
	}
	NewProcess(ctx, rootp, ETTest, KindStatic, tFunc).Act()

	for n := 0; n < b.N; n++ {
		rootp.AddEvent(Event{Name: ETTest,
			Kind: KindStatic,
			Data: []byte("test")})
		rootp.ErrorEventCh <- Event{Name: ERTest,
			Kind: KindError,
			Err:  fmt.Errorf("some error:%v", "apekatt")}
		if r := <-testCh; r != "test" {
			b.Fatalf("ETTest failed\n")
		}
	}
}

func BenchmarkTwoProcesses(b *testing.B) {
	//log.SetOutput(io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)

	testCh := make(chan string)

	const ETTest1 EventName = "ETTest1"
	const ETTest2 EventName = "ETTest2"

	tFunc1 := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					p.StaticEventCh <- Event{Name: ETTest2, Data: result.Data}

				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, rootp, ETTest1, KindStatic, tFunc1).Act()

	tFunc2 := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)

				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, rootp, ETTest2, KindStatic, tFunc2).Act()

	err := rootp.Act()
	if err != nil {
		b.Fatal(err)
	}

	for n := 0; n < b.N; n++ {
		rootp.AddEvent(Event{Name: ETTest1,
			Kind: KindStatic,
			Data: []byte("test")})
		if r := <-testCh; r != "test" {
			b.Fatalf("ETTest failed\n")
		}
	}
}

func BenchmarkThreeProcesses(b *testing.B) {
	//log.SetOutput(io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)

	testCh := make(chan string)

	const ETTest1 EventName = "ETTest1"
	const ETTest2 EventName = "ETTest2"
	const ETTest3 EventName = "ETTest3"

	tFunc1 := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					p.StaticEventCh <- Event{Name: ETTest2, Data: result.Data}
				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, rootp, ETTest1, KindStatic, tFunc1).Act()

	tFunc2 := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					p.StaticEventCh <- Event{Name: ETTest3, Data: result.Data}
				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, rootp, ETTest2, KindStatic, tFunc2).Act()

	tFunc3 := func(ctx context.Context, p *Process) func() {
		fn := func() {
			for {
				select {
				case result := <-p.InCh:
					testCh <- string(result.Data)

				case <-p.Ctx.Done():
					return
				}
			}
		}

		return fn
	}

	NewProcess(ctx, rootp, ETTest3, KindStatic, tFunc3).Act()

	err := rootp.Act()
	if err != nil {
		b.Fatal(err)
	}

	for n := 0; n < b.N; n++ {
		rootp.AddEvent(Event{Name: ETTest1,
			Kind: KindStatic,
			Data: []byte("test")})
		if r := <-testCh; r != "test" {
			b.Fatalf("ETTest failed\n")
		}
	}
}
