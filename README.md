# Actress

A Concurrent Actor framework written in Go.

## Processes, Events and process Functions

A process are like a module capable of performing a specific tasks. The nature of the process is determined by the code attached to each process. This code is encapsulated within a process function block which is registered to the process.

To initiate a task and trigger the execution of the process's function, we send events. Each process has its own unique event name. Events serve as communication channels within the system. They can carry data, either as a result of something the previous process did, instructions for what a process should do, or both.

Check out the test files for examples for how to define an Event and it's Process function, or for more complete examples check out the [examples](examples/) folder.
