package actress

import (
	"context"
	"testing"
)

func TestECRouter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testCh := make(chan string)

	t.Logf("--------------------------------------------------------\n")

	cfg, _ := NewConfig("debug")
	rootp := NewRootProcess(ctx, nil, cfg, nil)
	err := rootp.Act()
	if err != nil {
		t.Fatal(err)
	}

	NewProcess(ctx, rootp, ETTest, KindStatic, etTestfn(testCh)).Act()
	NewProcess(ctx, rootp, ECGeneralDelivery, KindCustom, ecGeneralDeliveryFn).Act()

	testStr := "some custom data"

	rootp.AddEvent(Event{
		Name:      ECGeneralDelivery,
		Kind:      KindCustom,
		Data:      []byte(testStr),
		NextEvent: &Event{Name: ETTest, Kind: KindStatic},
	})

	select {
	case s := <-testCh:
		if s != testStr {
			t.Fatalf("string were not equal\n")
		}
	case <-ctx.Done():
	}

}
