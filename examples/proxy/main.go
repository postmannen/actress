package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
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

					// Send event back that we have connected.
					fmt.Printf("***** ev nr: %v, Connected to : %v\n", ev.Nr, rHost)
					p.AddEvent(actress.Event{EventType: ev.NextEvent.EventType})

					actress.NewDynProcess(ctx, *p, actress.EventType(dynED2), func(ctx context.Context, p *actress.Process) func() {
						return func() {

							// Start Go routine for reading the event, writing the data
							// to the tcp connection
							go func() {
								ev := <-p.InCh
								erw := actress.NewEventRW(p, &ev, "in etHttpGetFn dynamic function ")

								fmt.Printf("*******************3\n")
								n, err := io.Copy(destConn, erw)
								if err != nil {
									log.Fatalf("error: failed io.Copy(destConn, erw): %v\n", err)
								}
								log.Printf(" ** etHttpGetFn, io.Copy(destConn, erw), copied bytes, n: %v\n", n)
								fmt.Printf("*******************33\n")

								// Cast the connetion to TCPConn so we can close just the write part.
								dconn := destConn.(*net.TCPConn)
								dconn.CloseWrite()
							}()

							// Start a Go routine for reading the TCP connection, and writing
							// it to the event to be sent back.
							go func() {

								erw := actress.NewEventRW(p, &actress.Event{EventType: actress.EventType(dynED1)}, "in etHttpGetFn dynamic function ")

								fmt.Printf("*******************4\n")
								n, err := io.Copy(erw, destConn)
								if err != nil {
									log.Fatalf("error: failed io.Copy(erw, destConn): %v\n", err)
								}
								log.Printf(" ** etHttpGetFn, io.Copy(erw, destConn), copied bytes, n: %v\n", n)
								fmt.Printf("*******************44\n")

								// Cast the connetion to TCPConn so we can close just the read part.
								dconn := destConn.(*net.TCPConn)
								dconn.CloseRead()
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

	etProxyListenerFn := func(ctx context.Context, p *actress.Process) func() {

		fn := func() {
			handleTunneling := func(w http.ResponseWriter, r *http.Request) {
				// dynamic process name for this (listener) side.
				dynED1 := actress.NewUUID()
				// dynamic process name for the other (httpGet) side.
				dynED2 := actress.NewUUID()

				// Send an even to the httpGet actor to prepare the
				// connection to the destination web page, and also
				// to start up the dynamic actor within ETHttpGet to
				// handle the reading and writing for that connection.
				p.AddEvent(actress.Event{Nr: eventNR, EventType: ETHttpGet, Cmd: []string{r.Host, dynED1, dynED2}, NextEvent: &actress.Event{Nr: eventNR + 1, EventType: ETProxyListener}})
				eventNR += 2

				hijacker, ok := w.(http.Hijacker)
				if !ok {
					http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
					return
				}
				clientConn, clientBuffr, err := hijacker.Hijack()
				if err != nil {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
				}

				w.WriteHeader(http.StatusOK)

				connectedEV := <-p.InCh
				fmt.Printf(" --- got connected ev: %v\n", connectedEV)

				actress.NewDynProcess(ctx, *p, actress.EventType(dynED1), func(ctx context.Context, p *actress.Process) func() {
					return func() {
						go func() {

							// TODO:
							// The problem seems to be that when reading from the clientBuffr the
							// number of bytes read are 0.

							// Creating a new Event Reader/Writer. We specify the Event to send when
							// the Write method is called.
							erw := actress.NewEventRW(p, &actress.Event{ // The first event to send to httpGet
								Nr: eventNR, EventType: actress.EventType(dynED2)}, "in etProxyListenerFn dynamic function")

							eventNR++

							fmt.Printf("*******************1\n")
							n, err := io.Copy(erw, clientBuffr)
							if err != nil {
								log.Fatalf("error: failed io.Copy(erw, clientBuffr): %v\n", err)
							}
							log.Printf(" *** etProxyListenerFn, io.Copy(erw, clientBuffr), copied bytes, n: %v\n", n)
							fmt.Printf("*******************11\n")

							//clientConn.Close()
						}()
						go func() {
							//n, err := io.Copy(clientBuffr, erw)

							ev := <-p.InCh

							erw := actress.NewEventRW(p, &ev, "in etProxyListenerFn dynamic function")
							fmt.Printf("*******************2\n")
							n, err := io.Copy(clientBuffr, erw)
							if err != nil {
								log.Fatalf("error: failed io.Copy(clientBuffr, erw): %v\n", err)
							}
							fmt.Printf("*******************22\n")
							log.Printf(" *** etProxyListenerFn, io.Copy(clientBuffr, erw), copied bytes, n: %v\n", n)

							clientConn.Close()
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
