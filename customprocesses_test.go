package actress

import (
	"context"
	"testing"
	"time"
)

func TestECRouter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCh := make(chan string)

	t.Logf("--------------------------------------------------------\n")

	cfg, _ := NewConfig()
	rootp := NewRootProcess(ctx, nil, cfg, nil)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	NewProcess(ctx, rootp, ETTest, EventKindStatic, etTestfn(testCh)).Act()
	NewProcess(ctx, rootp, ECGeneralDelivery, EventKindCustom, ecGeneralDeliveryFn).Act()

	time.Sleep(time.Second * 2)

	testStr := "some custom data"

	rootp.AddEvent(Event{
		EventType: ECGeneralDelivery,
		EventKind: EventKindCustom,
		Data:      []byte(testStr),
		NextEvent: &Event{EventType: ETTest, EventKind: EventKindStatic},
	})

	select {
	case s := <-testCh:
		if s != testStr {
			t.Fatalf("string were not equal\n")
		}
	case <-ctx.Done():
	}

}
