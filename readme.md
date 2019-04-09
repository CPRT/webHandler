A Websocket Based Communication and System Manager
--------------------------------------------------

The webHandler package provides an api for initializing a system for inter-process 
communication based on websockets.

The system automatically runs a control loop to handle sending and receiving 
messages concurrently in the system, as well as controlling startup and shutdown of the system.

The system involves three main parts:

	- WebHandler
	- Transmitter
	- SystemCommander
	- SocketHandler
	
The SystemCommander and SocketHandler are interfaces that must be implemented by the user for controlling
their system, while the WebHandler is a struct that manages the aforementioned interfaces and sets up communication
to the desired websockets.

The Transmitter is a structure that is used to enable sending outgoing messages through the websocket. It controls 
access to which websockets can be transmitted on to allow more fine tuned control over which websockets receive 
outgoing messages.

The SystemCommander is used as an overall controller of the system. All aspects of the system, such as initializing 
and closing drivers, must be performed by this interface. 

The SocketHandler allows for more fine tuned control of the system than the SystemCommander. Each websocket path managed
by the system has a corresponding SocketHandler, which is used to control how messages along that websocket and dealt with.

WebHandler
----------

The WebHandler organizes all the websockets connected to the system. It can be used to retrieve the http.HandleFunc
to initialize the websockets, as well as providing startup and shutdown methods to initialize the system. While 
on, the WebHandler runs a control loop to receive messages from the websockets and to enable regular updates
on the system (at a rate specified by the SystemCommander). The handling of those messages and updates 
is performed by user defined code in the SystemCommander and SocketHandler. The WebHandler also allows the 
SystemCommander and SocketHandler to send messages back to clients of the system through the websockets.

The webHandler is initialized with a single SystemCommander as well as a map containing all SocketHandlers as values.

For example, suppose you have two structs, processCommander and processHandler, which implement the SystemCommander 
and SocketHandler interfaces respectively. The system would be initialized as follows:

First, a map would be created, linking each SocketHandler to a string key. The string key is a unique name
that can be used to retrieve the http.HandlerFunc used for initializing the websocket (as well, it is 
used in the Start method of SystemCommander, see below).
```
	var pm map[string]webHandler.SocketHandler = map[string]webHandler.SocketHandler {
		"main":&processHandler{procs: procs},
	}
	var pc *processCommander= &processCommander{}
```

Next, InitWebHandler would be called with the SystemCommander and previously defined map as arguments.
```
	wh, err := webHandler.InitWebHandler(pc, pm)
```
If any error occurs while initializing the system, the wh return value will be nil and the err value will be non nil.

At this point, the system is ready to use. To initialize the websockets, use the method GetWebFunc to retrieve the 
desired http.HandlerFunc that can be passed to HandleFunc from the http package. The argument to GetWebFunc corresponds
to one of the keys used in the map argument to InitWebHandler. The http.HandlerFunc returned will use the SocketHandler
corresponding to it's key when it receives or sends messages.
```
	mux := http.NewServeMux()
	mux.HandleFunc("/main", wh.GetWebFunc("main"))
```
At this point the webHandler is running. Whenever a message is received, the Message method of the SystemCommander will 
be called, followed by the Message method of the SocketHandler associated with that websocket.

At a rate specified by the SystemCommander, the Update method of the SystemCommander will be called 
followed by the Update methods of all SocketHandlers. The order in which individual SocketHandlers are updated cannot be 
guaranteed and should not be relied on.

If at any point a panic is triggered in the control loop, the Stop method of the systemCommand will be called.
To turn off the system, call the Shutdown method of the webHandler. This will also call the Stop method of the SystemCommander,
as well as cleanly shutting down all websockets.
```
	wh.Shutdown()
```

Transmitter
-----------

The Transmitter is used to send messages back along websocket connections associated with the webHandler.
A Transmitter is capable of sending messages on different subsets of websockets, depending on the code
used with the Transmitter. The Transmitter allows three modes:

	webHandler.Broadcast : Send messages on all websockets associated with the system.
	webHandler.Handle    : Send messages on all websockets associated with the SocketHandler attached to the Transmitter.
	webHandler.Socket    : Send messages only on the single websocket attached to the Transmitter.
	
The webHandler decides which modes are allowed on a specific Transmitter, and the websocket and/or SocketHandler 
attached to the Transmitter. These are not configurable by the user.

Every time the Update or Message method is called, a Transmitter is provided as one of the arguments. Depending
on the specific function call, Transmitter is configured to allow different subsets of websockets to be accessed.

Every time the Message method is called (for both the SystemCommander and SocketHandler), the Transmitter
is configured to allow Broadcast, Handle, and Socket modes. If using Handle mode the messages will be sent to 
all websocket connections associated with the SocketHandler that received the original message. If using Socket
mode the messages will be sent only to the websocket connection that received the original message.

When the Update method is called for the SystemCommander, only Broadcast mode is available on the Transmitter argument. 
Attempting to use either of the other two modes will fail.

When the Update method is called for a SocketHandler, the Broadcast and Handle modes are available. If using 
the Handle mode the messages will be sent to all websocket connections associated with the SocketHandler whose update 
method was called.

Transmitters are also provided in the Start method of the SystemCommander (see below). 

As an example, consider the Message method of a systemCommander (in this case the processCommander mentioned above). 
This will send messages using the Transmitters Send method. 
```
func (pc *processCommander) Message(data []byte, sh webHandler.SocketHandler, tr webHandler.Transmitter) {
	tr.Send([]byte("Update"), webHandler.Broadcast) // returns true, message sent on all websockets
	tr.Send([]byte("Update"), webHandler.Handle)    // returns true, message sent on all websockets associated with sh 
	tr.Send([]byte("Update"), webHandler.Socket)    // returns true, message sent on websocket connection that received data
}
```

In the above example all modes were permitted. Consider the Update method of a SocketHandler (in this case,
the processHandler mentioned above).
```
func (ph *processHandler) Update(tr webHandler.Transmitter) {
	tr.Send([]byte("Update"), webHandler.Broadcast) // returns true, message sent on all websockets
	tr.Send([]byte("Update"), webHandler.Handle)    // returns true, message sent on all websockets associated with ph 
	tr.Send([]byte("Update"), webHandler.Socket)    // returns false, message not sent because there is no websocket 
													// attached to the transmitter
}
```

To determine which modes are available to a Transmitter, use the GetModes method.
```
	var modes []uint8 = tr.GetModes()
```

SystemCommander
---------------

The SystemCommander is the primary controller for the system within the WebHandler. It manages those portions that
all components of the system rely on. For example, it may manage a single motor controller driver that is shared 
by each SocketHandler involved in the system.

The SystemCommander must have at least for methods to comply with the SystemCommander interface. These are:

	- Start
	- Stop
	- UpdateFrequency
	- MessageTimeout
	- Update
	- Message
	
The Start method is called during the InitWebHandler method of WebHandler. It is used to perform any initialization
of the system that is required. It is provided a map where the keys are strings (corresponding to the strings 
provided in the map argument to InitWebHandler) and the values are Transmitter structs. Each transmitter struct 
contains the SocketHandler provided to the corresponding key in the argument to InitWebHandler, and thus can be 
used to send messages to all websockets associated with that SocketHandler. Immediately after the Start method 
has completed, the message timeout and update frequency are retrieved and locked in place. Then, the control
loop begins to handle synchronizing message reception and updates.

The Stop method is called if the control loop exits for any reason. This could be due to the Shutdown method being
called on the WebHandler, or if a panic is called the Message or Update methods of the SystemCommander or SocketHandler.
The Stop method is called after the control loop has exited, so no messages or updates can be handled after the Stop
method has begun execution.

The UpdateFrequency method returns the rate at which Update methods should be called in the control loop. This method 
should return a time.Duration of less than or equal to zero to turn off the update loop. The back-end uses time.Ticker
to control the update frequency, so if the synchronous control loop takes too long to execute a previous Message or 
Update method then the next update will not occur. This will prevent any of the update methods from ever bring called. 
The value for the frequency is retrieved immediately after the Start method is called, at which 
point it is locked in place and cannot be altered.

The MessageTimeout method returns a timeout for passing incoming messages to the control loop. 
This allows the system to drop incoming messages in case the control loop is running slow.
If an incoming message runs past the timeout before being received by the synchronous control loop (this 
will occur if either a previous Message or Update method for the SystemCommander or a SocketHandler 
takes too long to complete) then the message will be dropped. This method should return a time.Duration of 
less than or equal to zero to assign no timeout (no messages will be dropped). The message timeout is retrieved
immediately after the Start method is called, at which point it is locked in place and cannot be altered.

The Update method is called at a frequency specified by the UpdateFrequency method. The Update method for the System 
Commander is called immediately prior to the Update methods of the SocketHandlers. This Update method is passed a 
Transmitter that only permits Broadcast mode.

The Message method is called every time a message is received on any websocket. This method is called immediately prior
to the Message method of the SocketHandler. This method is passed a Transmitter that permits all modes, as well as 
the SocketHandler associated with the incoming message.

SocketHandler
-------------

The SocketHandler is a single handler for each individual websocket path.

The SocketHandler must have at least two methods to comply with the SocketHandler interface. These are:

	- Update
	- Message
	
The Update method is called at a frequency specified by the UpdateFrequency method specified by the SystemCommander.
The Update method for all SocketHandlers is called immediately after the Update Method for the SystemCommander.
The order of execution of the SocketHandler Update methods is unpredictable and should not be relied on.
The Update method takes a Transmitter that allows Broadcast and Handler modes. Using Handle mode with this 
Transmitter will send messages to all websocket connections attached to the SocketHandler that received the Transmitter.

The Message method is called every time a message is received on a websocket attached to the given SocketHandler.
This method is called immediately after the Message method for the SystemCommander. This method is passed a Transmitter
that permits all modes.