package actress

import (
	"context"
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

func TestEventSliceProcs(t *testing.T) {
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

func BenchmarkSingleProcess(b *testing.B) {
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

func BenchmarkTwoProcesses(b *testing.B) {
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
