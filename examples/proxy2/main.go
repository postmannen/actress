package main

import (
	"bytes"
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

				clientToDestinationBuf := actress.NewBuffer()
				destinationToClientBuf := &bytes.Buffer{}

				doCh1 := make(chan struct{}, 1)

				actress.NewDynProcess(ctx, *p, "ET1", func(context.Context, *actress.Process) func() {
					return func() {
						// clientToDestinationBuf <- clientConn
						go func() {
							defer func() {
								fmt.Println("CLOSING: clientConn")
								clientConn.Close()
							}()

							for {
								b := make([]byte, 1024*32)
								ccn, cce := clientConn.Read(b)
								if ccn > 0 {
									cdn, cde := clientToDestinationBuf.Write(b[:ccn])
									log.Printf("clientToDestinationBuf.Write bytes: %v\n", cdn)
									if cde != nil {
										log.Printf("error: clientToDestinationBuf.Write : %v\n", cde)
										break
									}
									doCh1 <- struct{}{}
								}
								if cce != nil {
									log.Printf("error: clientConn.Read: %v\n", err)
									break
								}
							}

						}()

						// destConn <- clientToDestinationBuf
						go func() {
							for {
								<-doCh1
								b := make([]byte, 1024*32)
								n, err := clientToDestinationBuf.Read(b)
								log.Printf("clientToDestinationBuf.Read), n: %v, err: %v\n", n, err)

								n, err = destConn.Write(b[:n])
								log.Printf("destConn.Read), n: %v, err: %v\n", n, err)
							}
						}()
					}
				}).Act()

				// ---------------------------------------------------------------

				doCh2 := make(chan struct{}, 1)

				actress.NewDynProcess(ctx, *p, "ET2", func(context.Context, *actress.Process) func() {
					return func() {
						// clientToDestinationBuf <- destinationConn
						go func() {
							defer func() {
								fmt.Println("CLOSING: destConn")
								destConn.Close()
							}()

							for {
								b := make([]byte, 1024*32)
								ccn, cce := destConn.Read(b)
								if ccn > 0 {
									cdn, cde := destinationToClientBuf.Write(b[:ccn])
									log.Printf("destinationToClientBuf.Write bytes: %v\n", cdn)
									if cde != nil {
										log.Printf("error: destinationToClientBuf.Write : %v\n", cde)
										break
									}
									doCh2 <- struct{}{}
								}
								if cce != nil {
									log.Printf("error: destConn.Read: %v\n", err)
									break
								}
							}

						}()

						// clientConn <- destinationToClientBuf
						go func() {
							for {
								<-doCh2
								b := make([]byte, 1024*32)
								n, err := destinationToClientBuf.Read(b)
								log.Printf("destinationToClientBuf.Read), n: %v, err: %v\n", n, err)

								n, err = clientConn.Write(b[:n])
								log.Printf("clientConn.Read), n: %v, err: %v\n", n, err)
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
