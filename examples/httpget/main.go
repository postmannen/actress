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

package main

import (
	"context"
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
	rootAct := actress.NewRootProcess(ctx, nil)

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

	// Start all the registered actors.
	err := rootAct.Act()
	if err != nil {
		log.Fatal(err)
	}

	// Register the event type and attach a function to it.
	actress.NewProcess(ctx, *rootAct, ETWriteToFile, WriteToFileFunc).Act()
	actress.NewProcess(ctx, *rootAct, ETHttpGet, httpGetFunc).Act()

	rootAct.AddEvent(actress.Event{EventType: ETHttpGet, Cmd: []string{"http://vg.no"}})
	// Receive and print the result.

	time.Sleep(time.Second * 2)
	cancel()
}
