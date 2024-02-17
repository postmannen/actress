package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	_ "net/http/pprof"

	"github.com/postmannen/actress"
)

const ETHttpGet actress.EventType = "ETHttpGet"
const ETProxyListener actress.EventType = "ETProxyListener"

func etHttpGetFn(ctx context.Context, p *actress.Process) func() {
	fn := func() {
		for {
			select {
			case evGetInit := <-p.InCh:

				rHost := evGetInit.Cmd[0]
				listenerET := evGetInit.Cmd[1]
				thisET := evGetInit.Cmd[2]

				actress.NewDynProcess(ctx, *p, actress.EventType(thisET), func(ctx context.Context, p *actress.Process) func() {
					return func() {

						// clientToDestinationBuf <- destinationConn
						go func() {

							destConn, err := net.DialTimeout("tcp", rHost, 10*time.Second)
							if err != nil {
								log.Fatalf("error: DialTimeout failed: %v\n", err)
								return
							}

							defer func() {
								fmt.Printf("CANCELED DESTINATION, %v\n", thisET)
								p.Cancel()
								defer p.DynProcesses.Delete(actress.EventType(thisET))
							}()

							// destConn <- event
							go func() {
								for {
									select {
									case ev := <-p.InCh:
										n, err := destConn.Write(ev.Data)
										log.Printf("destConn.Write), n: %v, err: %v,%v\n", n, err, thisET)
									case <-ctx.Done():
										fmt.Printf("CANCELED GO ROUTINE FOR destConn <- event\n")
										return
									}
								}
							}()

							for {
								b := make([]byte, 1024*32)
								fmt.Printf("BEFORE READING destConn, %v\n", thisET)
								ccn, cce := destConn.Read(b)
								fmt.Printf("AFTER READING destConn, n: %v, %v\n", ccn, thisET)
								if ccn > 0 {
									p.AddDynEvent(actress.Event{EventType: actress.EventType(listenerET), Data: b[:ccn]})
								}
								if cce != nil {
									log.Printf("error: destConn.Read: %v, %v\n", err, thisET)
									break
								}
							}

						}()

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
				EventType: ETHttpGet,
				Cmd:       cmd,
			})
			fmt.Printf("AddEvent to start up remote http get'er: %v\n", cmd)

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

			actress.NewDynProcess(ctx, *p, actress.EventType(listenerET), func(ctx context.Context, p *actress.Process) func() {
				return func() {
					// Event ET2 <- clientConn
					go func() {
						defer fmt.Printf("CANCELED LISTENER %v\n", listenerET)

						defer func() {
							fmt.Printf("CLOSING: clientConn, %v", listenerET)
							clientConn.Close()
							//fmt.Println("CLOSING: destConn")
							//destConn.Close()
							p.Cancel()
							defer p.DynProcesses.Delete(actress.EventType(listenerET))
						}()

						for {
							b := make([]byte, 1024*32)
							fmt.Printf("BEFORE READING clientConn, %v\n", listenerET)
							ccn, cce := clientConn.Read(b)
							fmt.Printf("AFTER READING clientConn, n: %v, %v\n", ccn, listenerET)
							if ccn > 0 {
								fmt.Printf("AFTER READING clientConn, n: %v, but BEFORE sending event, %v\n", ccn, listenerET)
								p.AddDynEvent(actress.Event{EventType: actress.EventType(geterET), Data: b[:ccn]})
								fmt.Printf("AFTER READING clientConn, n: %v, and AFTER sending event, %v\n", ccn, listenerET)
							}
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
								fmt.Printf("CANCELED GO ROUTINE FOR clientConn <- event\n")
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
				fmt.Printf("handler: handleTunneling, method: %v\n", r.Method)

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
	rootAct := actress.NewRootProcess(ctx)

	// Start all the registered actors.
	err := rootAct.Act()
	if err != nil {
		log.Fatal(err)
	}

	// Register the event type and attach a function to it.
	actress.NewProcess(ctx, *rootAct, ETHttpGet, etHttpGetFn).Act()
	actress.NewProcess(ctx, *rootAct, ETProxyListener, etProxyListenerFn).Act()

	//rootAct.AddEvent(actress.Event{EventType: ETHttpGet, Cmd: []string{"http://vg.no"}})
	// Receive and print the result.

	<-ctx.Done()
}
