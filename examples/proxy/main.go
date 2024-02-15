package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"time"

	_ "net/http/pprof"

	"github.com/pkg/profile"
	"github.com/postmannen/actress"
)

func main() {
	runtime.SetCPUProfileRate(1000)
	defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()

	go http.ListenAndServe("localhost:6060", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new root process.
	rootAct := actress.NewRootProcess(ctx)

	const ETHttpGet actress.EventType = "ETHttpGet"
	const ETProxyListener actress.EventType = "ETProxyListener"

	eventNR := 0

	etHttpGetFn := func(ctx context.Context, p *actress.Process) func() {
		fn := func() {
			for {
				select {
				case ev := <-p.InCh:
					rHost := ev.Cmd[0]
					// dynamic process name for this (listener) side.
					dynED1 := ev.Cmd[1]
					// dynamic process name for the other (httpGet) side.
					dynED2 := ev.Cmd[2]

					// TCP Connect to the destination.
					fmt.Printf("***** ev nr: %v,  Preparing to Connect to : %v\n", ev.Nr, rHost)
					destConn, err := net.DialTimeout("tcp", rHost, 5*time.Second)
					if err != nil {
						log.Fatalf("* error: destConn dial, err: %v, status: %v\n", err, http.StatusServiceUnavailable)
						//return
					}

					fmt.Printf("***** ev nr: %v, Connected to : %v\n", ev.Nr, rHost)

					// Send event back that we have connected.
					// fmt.Printf("***** ev nr: %v, Connected to : %v\n", ev.Nr, rHost)
					// p.AddEvent(actress.Event{EventType: ev.NextEvent.EventType})

					actress.NewDynProcess(ctx, *p, actress.EventType(dynED2), func(ctx context.Context, p *actress.Process) func() {
						return func() {

							// erw <- destConn
							go func() {

								for {
									erw := actress.NewEventRW(p, &actress.Event{EventType: actress.EventType(dynED1)}, "in etHttpGetFn dynamic function ")

									b := make([]byte, 1024*32)
									fmt.Println("* Before destConn.Read")
									ccn, cce := destConn.Read(b)
									fmt.Println("** After destConn.Read")
									if ccn > 0 {
										cdn, cde := erw.Write(b[:ccn])
										log.Printf("erw <- destConn, erw.Write bytes: %v\n", cdn)
										if cde != nil {
											log.Printf("erw <- destConn, error: erw.Write : %v\n", cde)
											break
										}
										//doCh1 <- struct{}{}
									}
									if cce != nil {
										log.Printf("erw <- destConn, error: destConn.Read: %v\n", err)
										break
									}
								}

								// Cast the connetion to TCPConn so we can close just the read part.
								//dconn := destConn.(*net.TCPConn)
								//dconn.CloseRead()
							}()

							// destConn <- erw
							go func() {
								for {
									ev := <-p.InCh
									erw := actress.NewEventRW(p, &ev, "in etHttpGetFn dynamic function ")

									b := make([]byte, 1024*32)
									ccn, cce := erw.Read(b)
									if ccn > 0 {
										cdn, cde := destConn.Write(b[:ccn])
										log.Printf("destConn <- erw, destConn.Write bytes: %v\n", cdn)
										if cde != nil {
											log.Printf("destConn <- erw, error: destConn.Write : %v\n", cde)
											break
										}
										//doCh1 <- struct{}{}
									}
									if cce != nil {
										log.Printf("destConn <- erw, error: erw.Read: %v\n", err)
										break
									}
								}

								// Cast the connetion to TCPConn so we can close just the write part.
								//dconn := destConn.(*net.TCPConn)
								//dconn.CloseWrite()
							}()

						}
					}).Act()

				case <-ctx.Done():
					return
				}
			}
		}

		return fn
	}

	// --------------------------------------------------------------------------------------

	etProxyListenerFn := func(ctx context.Context, p *actress.Process) func() {

		fn := func() {
			handleTunneling := func(w http.ResponseWriter, r *http.Request) {
				// dynamic process name for this (listener) side.
				dynED1 := actress.NewUUID()
				// dynamic process name for the other (httpGet) side.
				dynED2 := actress.NewUUID()

				// Send an even to the httpGet actor to prepare the connection to the destination web page, and also
				// to start up the dynamic actor within ETHttpGet to handle the reading and writing for that connection.
				p.AddEvent(actress.Event{Nr: eventNR, EventType: ETHttpGet, Cmd: []string{r.Host, dynED1, dynED2}, NextEvent: &actress.Event{Nr: eventNR + 1, EventType: ETProxyListener}})
				eventNR += 2

				// Hijack the response writer
				hijacker, ok := w.(http.Hijacker)
				if !ok {
					http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
					return
				}
				clientConn, _, err := hijacker.Hijack()
				if err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
				}

				w.WriteHeader(http.StatusOK)

				//connectedEV := <-p.InCh
				//fmt.Printf(" --- got connected ev: %v\n", connectedEV)

				actress.NewDynProcess(ctx, *p, actress.EventType(dynED1), func(ctx context.Context, p *actress.Process) func() {
					return func() {

						// erw <- clientConn
						go func() {
							for {
								// Creating a new Event Reader/Writer. We specify the Event to send when
								// the Write method is called.
								erw := actress.NewEventRW(p, &actress.Event{
									Nr: eventNR, EventType: actress.EventType(dynED2)}, "in etProxyListenerFn dynamic function")

								eventNR++

								b := make([]byte, 1024*32)
								fmt.Println("- Before clientConn.Read")
								ccn, cce := clientConn.Read(b)
								fmt.Println("-- After clientConn.Read")
								if ccn > 0 {
									cdn, cde := erw.Write(b[:ccn])
									log.Printf("erw <- clientConn, erw.Write bytes: %v\n", cdn)
									if cde != nil {
										log.Printf("erw <- clientConn, error: erw.Write : %v\n", cde)
										break
									}
									//doCh1 <- struct{}{}
								}
								if cce != nil {
									log.Printf("erw <- clientConn, error: clientConn.Read: %v\n", err)
									break
								}
							}
						}()
						//clientConn.Close()

						// clientConn <- erw
						go func() {
							for {
								ev := <-p.InCh

								erw := actress.NewEventRW(p, &ev, "clientConn <- erw, in etProxyListenerFn dynamic function")

								b := make([]byte, 1024*32)
								ccn, cce := erw.Read(b)
								if ccn > 0 {
									cdn, cde := clientConn.Write(b[:ccn])
									log.Printf("clientConn <- erw, clientConn.Write bytes: %v\n", cdn)
									if cde != nil {
										log.Printf("clientConn <- erw, error: clientConn.Write : %v\n", cde)
										break
									}
									//doCh1 <- struct{}{}
								}
								if cce != nil {
									log.Printf("clientConn <- erw, error: erw.Read: %v\n", err)
									break
								}
							}

							// clientConn.Close()
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
