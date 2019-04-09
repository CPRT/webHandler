// A package for managing a system of websocket connections
// Created by Michael Dysart
package webHandler

import(
	"sync"
	"net/http"
	"github.com/gorilla/websocket"
	"time"
)

type WebFuncs map[string] http.HandlerFunc

/* The SocketHandler interface is used to handle incoming messages from a 
 * given websocket type. It is transmitted along with the incoming message
 * to the system commander, where it is processed. The designer of the system
 * can decide whether the message should be processes by the system commander
 * (in which case the Message) or if the SocketHandler should handle the message,
 * in which case the Message method should be capable of handling the message.
 * Similarly, the Update method need not perform any operations, but is useful
 * if regular system checks are desired.
 */
type SocketHandler interface{
	/*
 	 * Update is called during every iteration of the control loop
	 * @param tr {Transmitter} a Transmitter for sending message back to the websocket.
	 * 		This Transmitter allows Broadcast and Handle codes
	 */
	Update(tr Transmitter)
	/*
  	 * Message is called each time a message is received on the associated websocket
	 * @param m {[]byte} the incoming message from the websocket
	 * @param tr {Transmitter} a Transmitter for sending messages back to the websocket
	 *        This Transmitter allows Broadcast, Handle, and Socket codes
	 */
	Message(m []byte, tr Transmitter)
}

/* 
 * The SystemCommander interface is a broader manager for the entire system.
 * It allows for broader control of the entire system, while the SocketHandler
 * allows precision responses to all of the different websocket types connected
 * to this system.
 */
type SystemCommander interface {
	/* 
	 * Method to be called during system boot
	 * @param tm {map[string]Transmitter} a map of transmitters for each handler
	 *        in a map with the original string for the handler as a key.
	 *        The original handler can be retrieved from the transmitter if desired.
	 *        The Transmitters in the map allow the Broadcast and Handle codes
	 * @param returns {error} return a non nil error to cancel initialization of the webHandler
	 */
	Start(tm map[string]Transmitter) error
	/*
  	 * Method to be called during system shutdown
	 */
	Stop()
	/* 
	 * The frequency that the updater is called.
	 * Return a Duration less than or equal to zero to turn off the updater loop.
	 * This value is accessed by the webHandler immediately after the Start method 
	 * is called, at which point it is locked in place and cannot be altered.
	 * @return {time.Duration}
	 */
	UpdateFrequency() time.Duration
	/*
	 * A timeout for passing incoming message to the control loop. Messages that reach the timeout will be dropped.
	 * Return a Duration less than or equal to zero to set no timeout 
	 * (i.e. no messages will be dropped).
	 * This value is accessed by the webHandler immediately after the Start method 
	 * is called,, at which point it is locked in place and cannot be altered.
	 * @return {time.Duration}
	 */
	MessageTimeout() time.Duration
	/*
     * Update is called during every iteration of the control loop
	 * @param tr {Transmitter} a transmitter for sending messages back to the desired websockets
	 *        This Transmitter only allows the Broadcast code
	 */
	Update(tr Transmitter)
	/*
 	 * Message is called every time a message is received on any websocket associated with the system
	 * @param tr {Transmitter} a transmitter for sending messages back to the desired websockets
	 *        This Transmitter allows the Broadcast, Handle, and Socket codes
	 */
	Message(m []byte,sh SocketHandler, tr Transmitter)
}

/*
 * The websocket handler.
 * This initializes the SystemCommander and SocketHandler 
 * to run in a control loop for sending and receiving messages,
 * and also initializes the websocket http handlers for 
 * establishing websocket connections
 */
type WebHandler struct {
	//Channels for terminating the receiver and transmitter go routines
	exitReceive chan struct{}
	exitTransmit chan struct{}
	finishedExit chan struct{}
	doneConn chan struct{}
	
	//The upgrader for upgrading websockets
	upgrader websocket.Upgrader
	//A waitGroup for synching closing all of the connections
	wsWG sync.WaitGroup
	//A mutex for preventing concurrent accesses to the client map
	clientLock sync.Mutex
	
	//The receiver and transmitter channels
	// These have queues to help maintain order in message transmission
	webReceive chan  *cmdStruct
	webTransmit chan Transmitter  
	
	//The map for all websocket connections. The byte value indicates the type of data transmitted over this connection.
	clients map[*websocket.Conn] SocketHandler
	
	//The map of the functions available for separate websockets
	wFuncs WebFuncs
}
