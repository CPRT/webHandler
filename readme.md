A Websocket Based Communication and System Manager
--------------------------------------------------

The webHandler package provides an api for initializing a control system and handling communication to and from that system via websockets.

Problem Statement
-----------------

When designing control software there are typically several goals that the designed must accomplish. The first of these is to provide a communication framework for the system. For a system involving many separate components, it can be time consuming to re-implement the communication framework for each component. Furthermore, if each component runs in a separate thread then the user must include code for ensurring atomic access to system resources, which is error prone. A further goal is to allow easy configuration of the system, allowing components to be easily swapped on startup.

The webhandle API is designed to tackle these issues. It provides a communication framework for the system as websockets that is easily scalable as new components are added.

Overview
--------

The webhandler API sets up the communication framework for user defined control software. It automatically runs a control loop to ensure safe, concurrent sending and receiving of messages to the user's system. The user does not need to worry about the websocket initialization details, and can instead focus on the design of system components.

The system involves three main parts:

	- WebHandler
	- Transmitter
	- SystemCommander
	- SocketHandler
	
The `SystemCommander` and `SocketHandler` are interfaces whose concrete types must implemented by the user for controlling
their system, while the `WebHandler` is a struct that manages the aforementioned interfaces and sets up communication
to the desired websockets.

The `Transmitter` is a structure that is provided to the user defined interfaces to allow them to send messages through the websockets to the client processes.

The `SystemCommander` is used as an overall controller of the system. All aspects of the system, such as initializing 
and closing drivers, must be performed by this interface. 

The `SocketHandler` allows for more fine tuned control of the system than the SystemCommander. Each websocket path managed
by the system has a corresponding SocketHandler, which is used to control how messages along that websocket and dealt with.

The webhandler API is designed for systems involving numerous components that cannot operate simultaneously. Only one system component may handle messages or updates at a time, preventing concurrent access or race conditions without the need for user defined semaphores or mutexes. Components can be easily added and removed when initializing the WebHandler.

WebHandler
----------

The `WebHandler` organizes all the websockets connected to the system. It can be used to retrieve the `http.HandleFunc` functions that are used to initialize a websocket that corresponds to each instance of `SocketHandler` provided when initializing the `WebHandler`. It also provides startup and shutdown methods to initialize the system via the `SystemCommander`. While 
active, the `WebHandler` runs a control loop to receive messages from the websockets and to enable regular updates
on the system (at a rate specified by the `SystemCommander`). The handling of those messages and updates 
is performed by user defined code in the `SystemCommander` and `SocketHandler`. The `WebHandler` also allows the 
`SystemCommander` and `SocketHandler` to send messages back to clients of the system through the websockets. The control loop ensures that only one handle is executing at a given time, and that the handler will run to completion before any other messages or updates are processed.

The `webHandler` is initialized with a single `SystemCommander` as well as a map containing all SocketHandlers as values.

For example, suppose you have two structs, processCommander and processHandler, which implement the `SystemCommander` 
and `SocketHandler` interfaces respectively. The system would be initialized as follows:

First, a map is created, linking each `SocketHandler` to a string key. The string key is a unique name
that can be used to retrieve the `http.HandlerFunc` used for initializing the websocket (as well, it is 
used in the Start method of `SystemCommander`, see below).
```
	var pm map[string]webHandler.SocketHandler = map[string]webHandler.SocketHandler {
		"main":&processHandler{procs: procs},
	}
	var pc *processCommander= &processCommander{}
```

Next, `InitWebHandler` is called with the `SystemCommander` and previously defined map as arguments.
```
	wh, err := webHandler.InitWebHandler(pc, pm)
```
If any error occurs while initializing the system, `wh` returns nil and the err value is non nil.

At this point, the system is ready to use. To initialize the websockets, use the method `GetWebFunc` to retrieve the 
desired `http.HandlerFunc` that can be passed to `HandleFunc` from the http package. The argument to `GetWebFunc` corresponds
to one of the keys used in the map argument to `InitWebHandler`. The `http.HandlerFunc` returned will use the `SocketHandler`
corresponding to it's key when it receives or sends messages.
```
	mux := http.NewServeMux()
	mux.HandleFunc("/main", wh.GetWebFunc("main"))
```
At this point the webHandler is running. Whenever a message is received, the `Message` method of the `SystemCommander` is 
be called, followed by the `Message` method of the `SocketHandler` associated with that websocket.

At a rate specified by the `SystemCommander`, the `Update` method of the `SystemCommander` is called 
followed by the `Update` methods of all `SocketHandlers`. The order in which individual `SocketHandlers` are updated is randomly selected and cannot be relied on.

If at any point a panic is triggered in the control loop, the `Stop` method of the `SystemCommander` will be called and the `WebHandler` will exit. To turn off the system, call the `Shutdown` method of the `WebHandler`. This will also call the `Stop` method of the `SystemCommander`, as well as cleanly shutting down all websockets.
```
	wh.Shutdown()
```

Transmitter
-----------

The `Transmitter` is used to send messages back along websocket connections associated with the webHandler.
A Transmitter is capable of sending messages on different subsets of websockets, depending on the code
used with the `Transmitter`. The `Transmitter` supports three modes:

	webHandler.Broadcast : Send messages on all websockets associated with the system.
	webHandler.Handle    : Send messages on all websockets associated with the SocketHandler attached to the Transmitter.
	webHandler.Socket    : Send messages only on the single websocket attached to the Transmitter.
	
The `WebHandler` decides which modes are allowed on a specific `Transmitter`, and the websocket and/or `SocketHandler` 
attached to the `Transmitter`. These are not configurable by the user.

Every time the `Update` or `Message` method is called, a `Transmitter` is provided as one of the arguments. Depending
on the specific function call, `Transmitter` is configured to allow different subsets of websockets to be accessed.

Every time the `Message` method is called (for both the `SystemCommander` and `SocketHandler`), the Transmitter
is configured to allow `Broadcast`, `Handle`, and `Socket` modes. If using `Handle` mode the messages will be sent to 
all websocket connections associated with the `SocketHandler` that received the original message. If using `Socket`
mode the messages will be sent only to the websocket connection that received the original message.

When the `Update` method is called for the `SystemCommander`, only `Broadcast` mode is available on the `Transmitter` argument. 
Attempting to use either of the other two modes will fail.

When the `Update` method is called for a `SocketHandler`, the `Broadcast` and `Handle` modes are available. If using 
the `Handle` mode the messages will be sent to all websocket connections associated with the SocketHandler whose update 
method was called.

`Transmitters` are also provided in the `Start` method of the `SystemCommander` (see below). 

As an example, consider the Message method of a `SystemCommander` (in this case the processCommander mentioned above). 
This will send messages using the `Transmitter's` `Send` method. 
```
func (pc *processCommander) Message(data []byte, sh webHandler.SocketHandler, tr webHandler.Transmitter) {
	tr.Send([]byte("Update"), webHandler.Broadcast) // returns true, message sent on all websockets
	tr.Send([]byte("Update"), webHandler.Handle)    // returns true, message sent on all websockets associated with sh 
	tr.Send([]byte("Update"), webHandler.Socket)    // returns true, message sent on websocket connection that received data
}
```

In the above example all modes were permitted. Consider the `Update` method of a `SocketHandler` (in this case,
the `processHandler` mentioned above).
```
func (ph *processHandler) Update(tr webHandler.Transmitter) {
	tr.Send([]byte("Update"), webHandler.Broadcast) // returns true, message sent on all websockets
	tr.Send([]byte("Update"), webHandler.Handle)    // returns true, message sent on all websockets associated with ph 
	tr.Send([]byte("Update"), webHandler.Socket)    // returns false, message not sent because there is no websocket 
							// associated with tr
}
```

The `GetModes` method can retrieve a list of modes available to the `Transmitter`, however these are set by the `WebHandler` and cannot be affected by the user.
```
	var modes []uint8 = tr.GetModes()
```

SystemCommander
---------------

The `SystemCommander` is the primary controller for the system within the `WebHandler`. It manages those portions that
all components of the system rely on. For example, it may manage a single motor controller driver that is shared 
by each `SocketHandler` involved in the system.

The `SystemCommander` must have at least for methods to comply with the `SystemCommander` interface. These are:

	- Start
	- Stop
	- UpdateFrequency
	- MessageTimeout
	- Update
	- Message
	
The `Start` method is called from the `InitWebHandler` method of `WebHandler`. It is used to perform any initialization
of the system that is required. It is provided a map where the keys are strings (corresponding to the strings 
provided in the map argument to `InitWebHandler`) and the values are Transmitter structs. Each transmitter struct 
contains the `SocketHandler` provided to the corresponding key in the argument to `InitWebHandler`, and thus can be 
used to send messages to all websockets associated with that `SocketHandler`. Immediately after the `Start` method 
has completed, the message timeout and update frequency are retrieved and locked in place. Then, the control
loop begins to handle synchronizing message reception and updates.

The `Stop` method is called if the control loop exits for any reason. This could be due to the `Shutdown` method being
called on the `WebHandler`, or if a panic is called the Message or Update methods of the `SystemCommander` or `SocketHandler`.
The `Stop` method is called after the control loop has exited, so no messages or updates can be handled after the Stop
method has begun execution.

The `UpdateFrequency` method returns the rate at which Update methods should be called in the control loop. This method 
should return a `time.Duration` of less than or equal to zero to turn off the update loop. The back-end uses `time.Ticker`
to control the update frequency, so if the synchronous control loop takes too long to execute a previous Message or 
Update method then the next update will not occur. This will prevent any of the update methods from ever bring called. 
The value for the frequency is retrieved immediately after the Start method is called, at which 
point it is locked in place and cannot be altered.

The `MessageTimeout` method returns a timeout for passing incoming messages to the control loop. 
This allows the system to drop incoming messages in case the control loop is running slow.
If an incoming message runs past the timeout before being received by the synchronous control loop (this 
will occur if either a previous `Message` or `Update` method for the `SystemCommander` or a `SocketHandler` 
takes too long to complete) then the message will be dropped. This method should return a `time.Duration` of 
less than or equal to zero to assign no timeout (no messages will be dropped). The message timeout is retrieved
immediately after the Start method is called, at which point it is locked in place and cannot be altered.

The `Update` method is called at a frequency specified by the `UpdateFrequency` method. The `Update` method for the `SystemCommander` is called immediately prior to the Update methods of the `SocketHandlers`. This `Update` method is passed a 
Transmitter that only permits `Broadcast` mode.

The `Message` method is called every time a message is received on any websocket. This method is called immediately prior
to the Message method of the `SocketHandler`. This method is passed a Transmitter that permits all modes, as well as 
the SocketHandler associated with the incoming message.

SocketHandler
-------------

The `SocketHandler` is a user structure that corresponds to a single handler for each individual websocket path.

The `SocketHandler` must have at least two methods to comply with the `SocketHandler` interface. These are:

	- Update
	- Message
	
The `Update` method is called at a frequency specified by the `UpdateFrequency` method specified by the `SystemCommander`.
The `Update` method for all `SocketHandlers` is called immediately after the Update Method for the `SystemCommander`.
The order of execution of the `SocketHandler` `Update` methods is unpredictable and should not be relied on.
The Update method takes a `Transmitter` that allows `Broadcast` and `Handler` modes. Using Handle mode with this 
`Transmitter` will send messages to all websocket connections attached to the `SocketHandler` that received the `Transmitter`.

The `Message` method is called every time a message is received on a websocket attached to the given `SocketHandler`.
This method is called immediately after the Message method for the SystemCommander. This method is passed a `Transmitter`
that permits all modes.

Use Case
--------

The webhandler API was developed because the designers at CPRT found that they were repeating similar communication designes in many systems for their rover. We can describe one such system to highlight the purpose of this system.

On the rover we had a motor system consisting of a variety of motor controllers all connected to a single Raspberry Pi through the UART interface. Together, these motor controllers were able to control several systems on the rover, including locomotion, arm, and claw. Since all of these motor controllers were controlled through a single port, the system had to be designed to ensure that only one set of motor controllers was being interacted with at a given time. Furthermore, it was desired to be able to configure which subsystems were active at a given time; sometimes the rover only had to use its wheels, but other times it only had to use its arm. Thus, it made sense to design different components for each subsystem. Having components also made it easier to configure which systems are active at a given time. These components were the individual `SocketHandlers` in the `WebHandler`.

Since each component had to use a common UART port to access the motor controllers, we developed a `SystemCommander` to initialize the motor controllers.

Since each component in our system must handle messages independently without, the `WebHandler` only calls a single `SocketHandler` `Message` method at a time, thus ensuring that only one component has access to the motor controllers at a time.

We also wanted a means to ensure that if there is a communication problem with the base station the rover would turn off all motors. For that reason, the `SystemCommander` has an `UpdateFrequency` method that returns `time.Second`. Whenever a message is received on any websocket, the `Message` method of the `SystemCommander` is called (in addition to the `Message` method of the appropriate `SocketHandler`. In our implementation the `SystemCommander` `Message` method logs the time when the `Message` was received. When the `SystemCommander` `Update` method is called (once every second), it checks the time since the last message was received, and if it is greater than one second all motors are shutdown.

