// A Rust port of examples/2actresses/main.go.
//
// The Go-style names (test1Func, rootAct, ETTest1, ...) are kept verbatim so
// the example reads like the original, so the Rust naming lints are silenced.
#![allow(non_snake_case, non_upper_case_globals)]
//
// Two processes are chained together: ETTest1 uppercases the incoming data and
// passes the result on to ETTest2 (named via the NextEvent), which appends
// "..." and delivers the result on the builtin TestCh that main reads from.

use std::sync::Arc;
use std::time::Duration;

use actress::*;

// Define the first function that will be attached to the ETTest1 event type
// process.
fn test1Func(ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            crossbeam_channel::select! {
                recv(p.InCh.rx) -> msg => {
                    let ev = match msg { Ok(e) => e, Err(_) => return };
                    let upper = String::from_utf8_lossy(&ev.Data).to_uppercase();

                    // Pass the processing on to the next process, using the
                    // NextEvent we specified in main for the event type, and
                    // add the result of the uppercasing to the Data field.
                    let next_name = ev.NextEvent.as_ref().unwrap().Name.clone();
                    p.AddEvent(Event {
                        Name: next_name,
                        Data: upper.into_bytes(),
                        ..Default::default()
                    });
                }
                recv(ctx.done()) -> _ => return,
            }
        }
    })
}

// Define the second function that will be attached to the ETTest2 event type
// process.
fn test2Func(ctx: Context, p: Arc<Process>) -> ProcFn {
    Box::new(move || {
        loop {
            crossbeam_channel::select! {
                recv(p.InCh.rx) -> msg => {
                    let result = match msg { Ok(e) => e, Err(_) => return };
                    let dots = format!("{}...", String::from_utf8_lossy(&result.Data));

                    // All actresses have a TestCh, which can be used for passing
                    // data via the routers. It is primarily intended for use in
                    // tests. Since the main code is aware of the actress using
                    // this function, we put a value on the channel here and read
                    // it in main.
                    let _ = p.TestCh.tx.send(Event {
                        Data: dots.into_bytes(),
                        ..Default::default()
                    });

                    // Also create an informational error message.
                    p.AddEvent(Event {
                        Name: ERLog,
                        Instruction: InstructionDebug,
                        Err: Some("info: done with the acting".to_string()),
                        ..Default::default()
                    });
                }
                recv(ctx.done()) -> _ => return,
            }
        }
    })
}

fn main() {
    let ctx = Context::background();

    // Create a new root process.
    let cfg = NewConfig("info");
    let rootAct = NewRootProcess(&ctx, None, cfg);

    // Define two event types for two processes.
    const ETTest1: EventName = EventName::new("ETTest1");
    const ETTest2: EventName = EventName::new("ETTest2");

    // Register the event types and event functions to processes.
    let ac1 = NewProcess(&rootAct.Ctx, &rootAct, ETTest1, Box::new(test1Func));
    let ac2 = NewProcess(&rootAct.Ctx, &rootAct, ETTest2, Box::new(test2Func));
    ac1.Act().unwrap();
    ac2.Act().unwrap();

    // Start the root process.
    rootAct.Act().unwrap();

    // Pass in an event destined for an ETTest1 event type process, and also
    // specify the next event to use when passing the result on from ETTest1 to
    // the next process, which here is ETTest2.
    rootAct.AddEvent(Event {
        Name: ETTest1,
        Data: b"test".to_vec(),
        NextEvent: Some(Box::new(Event {
            Name: ETTest2,
            ..Default::default()
        })),
        ..Default::default()
    });

    // Wait and receive the result from the ETTest2 process.
    let ev = ac2.TestCh.rx.recv().unwrap();
    println!("The result: {}", String::from_utf8_lossy(&ev.Data));

    std::thread::sleep(Duration::from_secs(2));
}
