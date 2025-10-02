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
	"encoding/json"
	"fmt"
	"log"

	"github.com/postmannen/actress"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	cfg, _ := actress.NewConfig("debug")
	rootAct := actress.NewRootProcess(ctx, nil, cfg, nil)
	rootAct.Act()

	// Start all the registered actors.
	err := rootAct.Act()
	if err != nil {
		log.Fatal(err)
	}

	rootAct.AddEvent(actress.Event{Name: actress.ETPidGetAll,
		NextEvent: &actress.Event{
			Name: actress.ETTestCh,
		},
	},
	)

	ev := <-rootAct.TestCh
	tmpProc := make(actress.PidVsProcMap)
	err = json.Unmarshal(ev.Data, &tmpProc)
	if err != nil {
		log.Fatal(err)
	}

	for k, v := range tmpProc {
		fmt.Printf("pid: %v, process: %v\n", k, v)
	}
	cancel()
}
