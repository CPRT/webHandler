package webHandler

import(
	"net/http"
	"github.com/gorilla/websocket"
	"time"
)

/*
 * @param sc {SystemCommander} The overall commander for the system. Used to handle general control of the system.
 * @param hm {map[string] SocketHandler} Handles each individual system that could be accessed by a separate websocket. 
 * @returns {*WebHandler} the overall handler for the system. Can be used to access the map of functions for controlling the websockets.
 * @returns {error} any errors that occurred
 */
func InitWebHandler(sc SystemCommander, hm map[string]SocketHandler) (*WebHandler, error) {
	var wh *WebHandler = &WebHandler {
		exitReceive  : make(chan struct{}),
		exitTransmit : make(chan struct{}),
		finishedExit : make(chan struct{}),
		doneConn     : make(chan struct{}),
		webTransmit  : make(chan Transmitter, 10),
		webReceive   : make(chan *cmdStruct, 5),
		clients      : make(map[*websocket.Conn] SocketHandler),
		wFuncs       : make(WebFuncs),
	}
		
	// Create a map of transmitters to allow for transmitting 
	// messages along the websockets for each SocketHandler
	var tm map[string]Transmitter = make(map[string]Transmitter)
	for s, h := range hm {
		tm[s] = Transmitter{maxMode: Handle, sh:h, wt:wh.webTransmit}
	}
		
	if err := sc.Start(tm); err != nil {
		return nil, err
	}
	// Create the handler funcs for each SocketHandler and the list of socket handlers
	// Provide the timeout supplied by the SystemCommander
	var sl []SocketHandler = []SocketHandler{}
	var timeout time.Duration = sc.MessageTimeout()
	for s, h := range hm {
		sl = append(sl, h)
		wh.wFuncs[s] = wh.makeConnectHandler(h, timeout)
	}
	
	//go routines for synchronizing receiving and transmitting messages
	// from the websocket
	go wh.handleWebsocketSend()	
	go wh.handleWebsocketReceive(sc,sl, sc.UpdateFrequency())
	
	return wh, nil
}

/*
 * Retrieve a list names corresponding to all webHandler functions
 * @return {[]string}
 */ 
func (wh *WebHandler) GetNames() []string {
	var keys []string = make([]string, len(wh.wFuncs))
	var i uint64 = 0
	for k := range wh.wFuncs {
		keys[i] = k
		i++
	}
	return keys
}

/*
 * Retrieve the http handler function corresponding to the given string
 * This string should match the id string of one of the handlers provided to 
 * InitWebHandler
 * @param s {string} the string corresponding to the handler as assigned in InitWebHandler
 * @return {http.HandlerFunc} the function for use with HandleFunc from the http package
 */
func (wh *WebHandler) GetWebFunc(s string) http.HandlerFunc {
	return wh.wFuncs[s]
}

/* 
 * Shutdown the websocket connections and close the system
 * Shutdown may only be called once. It will panic if called multiple times.
 */
func (wh *WebHandler) Shutdown() {

	/*Closing must be done in order, starting with stopping all new connections, 
	 * then closing all connections,
	 * then closing the message receiver go routine, 
	 * then closing the driver port(s),
   	 * then closing the message transmitter go routine.
     */
	wh.clientLock.Lock()
	close(wh.doneConn)
	for k := range wh.clients {
		k.Close()
	}
	// This clears all websocket from the map
	wh.clients = make(map[*websocket.Conn] SocketHandler)
	wh.clientLock.Unlock()
	
	//Wait until all HandleConnection functions have completed
	wh.wsWG.Wait()

	//Try to close the channel by waiting for all remaining
	//roboclaw operations to complete. Give a timeout of 5 seconds
	//to ensure that the program exits.
	close(wh.exitReceive)

	//Wait for the goroutine for handling commands to quit,
	<-wh.finishedExit
	close(wh.exitTransmit)
}
