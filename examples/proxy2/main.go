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

func main() {
	// runtime.SetCPUProfileRate(1000)
	// defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()

	go http.ListenAndServe("localhost:6060", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	rootAct := actress.NewRootProcess(ctx)

	const ETHttpGet actress.EventType = "ETHttpGet"
	const ETProxyListener actress.EventType = "ETProxyListener"

	etHttpGetFn := func(ctx context.Context, p *actress.Process) func() {
		fn := func() {

		}

		return fn
	}

	// --------------------------------------------------------------------------------------

	etProxyListenerFn := func(ctx context.Context, p *actress.Process) func() {

		fn := func() {
			handleTunneling := func(w http.ResponseWriter, r *http.Request) {
				destConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
				if err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
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

				actress.NewDynProcess(ctx, *p, "ET1", func(ctx2 context.Context, p2 *actress.Process) func() {
					return func() {
						// Event ET2 <- clientConn
						go func() {
							defer func() {
								fmt.Println("CLOSING: clientConn")
								clientConn.Close()
							}()

							for {
								b := make([]byte, 1024*32)
								fmt.Println("BEFORE READING clientConn")
								ccn, cce := clientConn.Read(b)
								fmt.Printf("AFTER READING clientConn, n: %v\n", ccn)
								if ccn > 0 {
									fmt.Printf("AFTER READING clientConn, n: %v, but BEFORE sending event\n", ccn)
									p.AddDynEvent(actress.Event{EventType: "ET2", Data: b[:ccn]})
									fmt.Printf("AFTER READING clientConn, n: %v, and AFTER sending event\n", ccn)
								}
								if cce != nil {
									log.Printf("error: clientConn.Read: %v\n", err)
									break
								}
							}

						}()

						// clientConn <- event
						go func() {
							for {
								ev := <-p2.InCh

								n, err := clientConn.Write(ev.Data)
								log.Printf("clientConn.Write), n: %v, err: %v\n", n, err)
							}
						}()

					}
				}).Act()

				// ---------------------------------------------------------------

				actress.NewDynProcess(ctx, *p, "ET2", func(ctx2 context.Context, p2 *actress.Process) func() {
					return func() {
						// clientToDestinationBuf <- destinationConn
						go func() {
							defer func() {
								fmt.Println("CLOSING: destConn")
								destConn.Close()
							}()

							for {
								b := make([]byte, 1024*32)
								fmt.Println("BEFORE READING destConn")
								ccn, cce := destConn.Read(b)
								fmt.Printf("AFTER READING destConn, n: %v\n", ccn)
								if ccn > 0 {
									p.AddDynEvent(actress.Event{EventType: "ET1", Data: b[:ccn]})
								}
								if cce != nil {
									log.Printf("error: destConn.Read: %v\n", err)
									break
								}
							}

						}()

						// destConn <- event
						go func() {
							for {
								ev := <-p2.InCh

								n, err := destConn.Write(ev.Data)
								log.Printf("destConn.Write), n: %v, err: %v\n", n, err)
							}
						}()
					}
				}).Act()
			}

			// - main

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
