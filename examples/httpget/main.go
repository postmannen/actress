package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/postmannen/actress"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	rootAct := actress.NewActress(ctx)

	// Create a test channel where we receive the end result.
	testCh := make(chan string)

	const ETHttpGet actress.EventType = "ETHttpGet"
	const ETWriteToFile actress.EventType = "ETWriteToFile"

	httpGetFunc := func(ctx context.Context, p *actress.Process) func() {
		fn := func() {
			for {
				select {
				case ev := <-p.InCh:
					go func() {
						resp, err := http.Get(ev.Cmd[0])
						if err != nil {
							log.Fatalf("http get failed: %v\n", err)
						}

						b, err := io.ReadAll(resp.Body)
						if err != nil {
							log.Fatalf("http body read failed: %v\n", err)
						}

						err = resp.Body.Close()
						if err != nil {
							log.Fatalf("http body close failed: %v\n", err)
						}

						p.AddEvent(actress.Event{EventType: ETWriteToFile, Data: b})
					}()

				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	WriteToFileFunc := func(ctx context.Context, p *actress.Process) func() {
		fn := func() {
			for {
				select {
				case ev := <-p.InCh:
					go func() {
						err := os.WriteFile("web.html", ev.Data, 0777)
						if err != nil {
							log.Fatalf("write file failed: %v\n", err)
						}
					}()

				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	// Register the event type and attach a function to it.
	rootAct.RegisterProcess(ETWriteToFile, WriteToFileFunc)
	rootAct.RegisterProcess(ETHttpGet, httpGetFunc)

	// Start all the registered actors.
	err := rootAct.Act()
	if err != nil {
		log.Fatal(err)
	}

	rootAct.AddEvent(actress.Event{EventType: ETWriteToFile, Cmd: []string{"http://vg.no"}})
	// Receive and print the result.
	fmt.Printf("The result: %v\n", <-testCh)

	cancel()
	time.Sleep(time.Second * 2)
}
