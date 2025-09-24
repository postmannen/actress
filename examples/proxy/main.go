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
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"time"

	_ "net/http/pprof"

	"github.com/postmannen/actress"
)

const ETHttpGet actress.EventName = "ETHttpGet"
const ETProxyListener actress.EventName = "ETProxyListener"

func etHttpGetFn(ctx context.Context, p *actress.Process) func() {
	fn := func() {
		for {
			select {
			case evGetInit := <-p.InCh:

				rHost := evGetInit.Cmd[0]
				listenerET := evGetInit.Cmd[1]
				thisET := evGetInit.Cmd[2]

				actress.NewProcess(ctx, p, actress.EventName(thisET), func(ctx context.Context, p *actress.Process) func() {
					return func() {
						timeout := 5
						ctx, cancel := context.WithTimeout(ctx, time.Second*time.Duration(timeout))

						// clientToDestinationBuf <- destinationConn

						destConn, err := net.DialTimeout("tcp", rHost, time.Duration(timeout)*time.Second)
						if err != nil {
							log.Fatalf("error: DialTimeout failed: %v\n", err)
							return
						}

						defer func() {
							log.Printf("CANCELED DESTINATION, %v\n", thisET)
							cancel()
							defer p.DynamicProcesses.Delete(actress.EventName(thisET))
						}()

						// destConn <- event
						go func() {
							for {
								select {
								case ev := <-p.InCh:
									n, err := destConn.Write(ev.Data)
									if len(ev.Data) == 0 {
										destConn.Close()
									}
									log.Printf("destConn.Write), n: %v, err: %v,%v\n", n, err, thisET)
								case <-ctx.Done():
									log.Printf("CANCELED GO ROUTINE FOR destConn <- event, closing destConn\n")
									destConn.Close()
									return
								}
							}
						}()

						for {
							b := make([]byte, 1024*32)

							ccn, cce := destConn.Read(b)
							log.Printf("READ destConn, n: %v, err: %v, %v\n", ccn, err, thisET)
							if ccn > 0 {
								p.AddEvent(actress.Event{Name: actress.EventName(listenerET),
									Data: b[:ccn]})
								log.Printf("AFTER READING destConn, n: %v, and AFTER sending event, %v\n", ccn, thisET)
							}
							if cce != nil {
								log.Printf("error: destConn.Read: %v, %v\n", err, thisET)
								break
							}
						}

					}
				}).Act()

			case <-ctx.Done():
				log.Printf("CANCELED PROCESS FOR ETHttpGet\n")
				return
			}
		}
	}

	return fn
}

func etProxyListenerFn(ctx context.Context, p *actress.Process) func() {

	fn := func() {
		handleTunneling := func(w http.ResponseWriter, r *http.Request) {
			// Send event to start up the process to do HTTP GET.
			rHost := r.Host
			listenerET := actress.NewUUID()
			geterET := actress.NewUUID()

			cmd := []string{
				rHost,
				listenerET,
				geterET,
			}

			p.AddEvent(actress.Event{
				Name: ETHttpGet,
				Cmd:  cmd,
			})
			log.Printf("Added Event to start up remote http get'er: %v\n", cmd)

			w.WriteHeader(http.StatusOK)
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
				return
			}
			clientConn, _, err := hijacker.Hijack()
			if err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
			}

			actress.NewProcess(ctx, p, actress.EventName(listenerET), func(ctx context.Context, p *actress.Process) func() {
				return func() {
					// Event ET2 <- clientConn
					go func() {
						defer log.Printf("CANCELED LISTENER %v\n", listenerET)

						defer func() {
							log.Printf("CLOSING: clientConn, %v", listenerET)
							clientConn.Close()
							p.Cancel()
							defer p.DynamicProcesses.Delete(actress.EventName(listenerET))
						}()

						for {
							b := make([]byte, 1024*32)
							ccn, cce := clientConn.Read(b)
							log.Printf("AFTER READING clientConn, n: %v, %v\n", ccn, listenerET)
							//if ccn > 0 {

							p.AddEvent(actress.Event{Name: actress.EventName(geterET),
								Data: b[:ccn]})
							log.Printf("AFTER READING clientConn, n: %v, and AFTER sending event, %v\n", ccn, listenerET)

							//}
							if cce != nil {
								log.Printf("error: clientConn.Read: %v, %v\n", err, listenerET)
								break
							}
						}

					}()

					// clientConn <- event
					go func() {
						for {
							select {
							case ev := <-p.InCh:
								n, err := clientConn.Write(ev.Data)
								log.Printf("clientConn.Write), n: %v, err: %v\n", n, err)
							case <-ctx.Done():
								log.Printf("CANCELED GO ROUTINE FOR clientConn <- event\n")
								return
							}
						}
					}()

				}
			}).Act()

			// ---------------------------------------------------------------

		}

		// Start up a http listener for the client to connect to.
		server := &http.Server{
			Addr: ":8888",
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

				handleTunneling(w, r)
				log.Printf("handler: handleTunneling, method: %v\n", r.Method)

			}),
			// Disable HTTP/2.
			TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
		}

		log.Fatal(server.ListenAndServe())
		// ---
	}

	return fn
}

func main() {
	// runtime.SetCPUProfileRate(1000)
	// defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()

	go http.ListenAndServe("localhost:6060", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	cfg, _ := actress.NewConfig("debug")
	rootAct := actress.NewRootProcess(ctx, nil, cfg, nil)

	// Start all the registered actors.
	err := rootAct.Act()
	if err != nil {
		log.Fatal(err)
	}

	// Register the event type and attach a function to it.
	actress.NewProcess(ctx, rootAct, ETHttpGet, etHttpGetFn).Act()
	actress.NewProcess(ctx, rootAct, ETProxyListener, etProxyListenerFn).Act()

	//rootAct.AddEvent(actress.Event{EventType: ETHttpGet, Cmd: []string{"http://vg.no"}})
	// Receive and print the result.

	<-ctx.Done()
}
