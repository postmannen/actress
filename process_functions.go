package actress

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/pkg/profile"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type pFunc func(context.Context, *Process) func()

// -----------------------------------------------------------------------------
// Startup processes function
// -----------------------------------------------------------------------------

// Process function used to startup all the other needed processes.
// This process will only be assigned to the root process in the
// newProcess function.
func procProcessesStartFunc(ctx context.Context, pRoot *Process) func() {
	if !pRoot.isRoot {
		log.Fatalf("error: the process must be root to run the processes startup function, current process\n")
	}

	fn := func() {
		// temporary map just for starting up the standard processes
		// to avoid having a mutex to protect map in the process.processes
		// type while we're starting up the processes.
		tmpPMap := make(map[EventType]*Process)

		for k, v := range pRoot.Processes.pFuncMap {
			p := NewProcess(ctx, *pRoot, k, v)
			pRoot.Processes.inChMap[k] = p.InCh
			tmpPMap[k] = p
		}

		for _, v := range tmpPMap {
			err := v.Act()
			if err != nil {
				log.Printf("%v\n", err)
				os.Exit(1)
			}
		}
	}

	return fn
}

// -----------------------------------------------------------------------------
// Builtin standard functions
// -----------------------------------------------------------------------------

// Process function for routing and handling events.
func procRouterFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case e := <-p.EventCh:

				//go func() {
				p.Processes.inChMap[e.EventType] <- e
				//}()

			case <-ctx.Done():
				p.AddError(Event{
					EventType: ERLog,
					Err:       fmt.Errorf("info: got ctx.Done"),
				})

				return
			}
		}
	}

	return fn
}

// Process function for handling CTRL+C pressed.
func procOsSignalFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		// Wait for ctrl+c to stop the server.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)

		// Block and wait for CTRL+C
		sig := <-sigCh
		log.Printf("Got terminate signal, terminating all processes, %v\n", sig)
		os.Exit(0)
	}

	return fn
}

func procProfilingFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		//defer profile.Start(profile.BlockProfile).Stop()
		//defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
		//defer profile.Start(profile.TraceProfile, profile.ProfilePath(".")).Stop()
		defer profile.Start(profile.MemProfile, profile.MemProfileRate(1)).Stop()
		//defer profile.Start(profile.MemProfileHeap).Stop()
		//defer profile.Start(profile.MemProfileAllocs).Stop()

		go http.ListenAndServe("localhost:6060", nil)

		reg := prometheus.NewRegistry()
		reg.MustRegister(collectors.NewGoCollector())
		procTotal := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ctrl_processes_total",
			Help: "The current number of total running processes",
		})
		reg.MustRegister(procTotal)

		http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	}

	return fn
}

func procDoneFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			d := <-p.InCh

			go func() {
				fmt.Printf("info: got event ETDone: %v\n", string(d.Data))
				p.AddError(Event{
					EventType: ERLog,
					Err:       fmt.Errorf("info: got etDone"),
				})
			}()
		}
	}

	return fn
}

func procPrintFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case d := <-p.InCh:

				go func() {
					fmt.Printf("info: got event ETPrint: %v\n", string(d.Data))
					p.AddEvent(Event{EventType: ETDone, Data: []byte("finished printing the event")})
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

func procExitFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case d := <-p.InCh:

				go func() {
					fmt.Printf("info: got event ETExit: %v\n", string(d.Data))
					os.Exit(0)
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

// -----------------------------------------------------------------------------
// Error handling functions
// -----------------------------------------------------------------------------

// Process function for routing and handling events.
func procErrorRouterFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case e := <-p.ErrorCh:

				go func() {
					p.Processes.inChMap[e.EventType] <- e
				}()

			case <-ctx.Done():
				// NB: Bevare of this one getting stuck if for example the error
				// handling is down. Maybe add a timeout if blocking to long,
				// and then send elsewhere if it becomes a problem.
				p.AddError(Event{
					EventType: ERLog,
					Err:       fmt.Errorf("info: got ctx.Done"),
				})
			}
		}
	}

	return fn
}

func procErrorLogFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case er := <-p.InCh:

				go func() {
					log.Printf("error for logging received: %v\n", er.Err)
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

func procDebugLogFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case er := <-p.InCh:

				go func() {
					log.Printf("error for debug logging received: %v\n", er.Err)
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}

func procFatalLogFunc(ctx context.Context, p *Process) func() {
	fn := func() {
		for {
			select {
			case er := <-p.InCh:

				go func() {
					log.Fatalf("error for fatal logging received: %v\n", er.Err)
				}()
			case <-ctx.Done():
				return
			}
		}
	}

	return fn
}
