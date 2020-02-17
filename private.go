package webHandler

import (
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"time"
)

// The structure used to transmit messages from the websocket
// to the handleWebsocketReceive function
type cmdStruct struct {
	msg []byte
	h   SocketHandler
	ws  *websocket.Conn
}

/*
 * Creates the http handler function for establishing websocket connections
 * @param sh {SocketHandler} the handler that will be assigned to websockets using this HandlerFunc
 * @param timeout {time.Duration} the timeout for receiving incoming messages on the websocket and
 *        passing them on the the control loop
 * @return {http.HandlerFunc}
 */
func (wh *WebHandler) makeConnectHandler(sh SocketHandler, timeout time.Duration) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		wh.handleConnection(w, r, sh, timeout)
	}
	return fn
}

/*
 * Start a loop to control the system by performing regular updates
 * and receiving messages from the websockets
 * @param sc {SystemCommander}
 * @param sl {[]SocketHandler} the list of system handlers
 * @param rate {time.Duration} the update frequency for the control loop
 */
func (wh *WebHandler) handleWebsocketReceive(sc SystemCommander, sl []SocketHandler, rate time.Duration) {

	defer func() {
		sc.Stop()
		//Alert the shutdown function that this goroutine is done
		close(wh.finishedExit)
	}()

	if rate > 0 {
		wh.runUpdate(sc, sl, rate)
	} else {
		wh.run(sc, sl)
	}
}

/*
 * Receive and handle messages from the websocket with (with no regular updates)
 * @param sc {SystemCommander}
 * @param sl {[]SocketHandler} the list of system handlers
 * @param uf {time.Duration} the update frequency for the control loop
 */
func (wh *WebHandler) run(sc SystemCommander, sl []SocketHandler) {
loop:
	for {
		select {
		case m, ok := <-wh.webReceive:
			if ok {
				tr := Transmitter{wt: wh.webTransmit, maxMode: Socket, ws: m.ws, sh: m.h}
				sc.Message(m.msg, m.h, tr)
				m.h.Message(m.msg, tr)
			}
		//Quit the loop
		case <-wh.exitReceive:
			break loop
		}
	}
}

/*
 * Receive and handle messages from the websocket with regular updates
 * to the websocket handlers
 * @param sc {SystemCommander}
 * @param sl {[]SocketHandler} the list of system handlers
 * @param uf {time.Duration} the update frequency for the control loop
 */
func (wh *WebHandler) runUpdate(sc SystemCommander, sl []SocketHandler, rate time.Duration) {
	//A ticker for taking status readings
	var ticker *time.Ticker
	ticker = time.NewTicker(rate)

	defer ticker.Stop()

loop:
	for {
		select {
		case m, ok := <-wh.webReceive:
			if ok {
				tr := Transmitter{wt: wh.webTransmit, maxMode: Socket, ws: m.ws, sh: m.h}
				sc.Message(m.msg, m.h, tr)
				m.h.Message(m.msg, tr)
			}
		//Handle updates to the system that should occur at a regular interval
		case <-ticker.C:
			tr := Transmitter{wt: wh.webTransmit, maxMode: Broadcast}
			sc.Update(tr)
			for _, sh := range sl {
				tr = Transmitter{wt: wh.webTransmit, maxMode: Handle, sh: sh}
				sh.Update(tr)
			}
		//Quit the loop
		case <-wh.exitReceive:
			break loop
		}
	}
}

/*
 * Run a loop to transmit status updates to the appropriate websocket connections
 */
func (wh *WebHandler) handleWebsocketSend() {

loop:
	for {
		select {
		//Get any messages that must be transmitted back
		//through the websocket
		case m := <-wh.webTransmit:

			//A lock to avoid any conflicts with closing connections
			wh.clientLock.Lock()
			//Transmit to different sub-sets of the websockets depending on the
			// code
			switch m.mode {
			case Socket:
				//Check if the websocket is still active
				if _, ok := wh.clients[m.ws]; ok {
					if err := m.ws.WriteMessage(websocket.TextMessage, m.msg); err != nil {
						log.Println(err)
					}
				}
			case Handle:
				//Transmit the message to all clients
				for ws, handle := range wh.clients {
					if handle == m.sh {
						if err := ws.WriteMessage(websocket.TextMessage, m.msg); err != nil {
							log.Println(err)
						}
					}
				}
			case Broadcast:
				//Transmit the message to all clients
				for ws, _ := range wh.clients {
					if err := ws.WriteMessage(websocket.TextMessage, m.msg); err != nil {
						log.Println(err)
					}
				}
			}
			wh.clientLock.Unlock()
		//Quit the loop
		case <-wh.exitTransmit:
			break loop
		}
	}
}

/*
 * Static function for handling incomming connections to the motor controllers
 * @param w (http.ResponseWriter)
 * @param r (*http.Request)
 * @param handle (SocketHandler) an interface to handle the system
 * @param timeout {time.Duration} the timeout for receiving incoming messages on the websocket and
 *        passing them on the the control loop
 */
func (wh *WebHandler) handleConnection(w http.ResponseWriter, r *http.Request, handle SocketHandler, timeout time.Duration) {

	//bypassing the error
	r.Header.Del("Origin")

	wh.clientLock.Lock()

	// In case the shutdown method has been called,
	// return immediately
	select {
	case <-wh.doneConn:
		wh.clientLock.Unlock()
		return
	default:
	}

	wh.wsWG.Add(1)

	// Upgrade initial GET request to a websocket
	ws, err := wh.upgrader.Upgrade(w, r, nil)
	if err != nil {
		wh.clientLock.Unlock()
		log.Print(err)
		return
	}

	wh.clients[ws] = handle
	wh.clientLock.Unlock()

	defer func() {
		wh.clientLock.Lock()
		delete(wh.clients, ws)
		wh.clientLock.Unlock()
		// Make sure we close the  connection when the function returns
		ws.Close()
		wh.wsWG.Done()
	}()

loop:
	for {
		_, message, err := ws.ReadMessage()

		//If there is an error, delete the websocket connection
		if err == nil {
			if timeout > 0 {
				select {
				case wh.webReceive <- &cmdStruct{msg: message, h: handle, ws: ws}:
				//
				case <-time.After(timeout):
				case <-wh.exitReceive:
					break loop
				}
			} else {
				select {
				case wh.webReceive <- &cmdStruct{msg: message, h: handle, ws: ws}:
				case <-wh.exitReceive:
					break loop
				}
			}
		} else {
			log.Printf("error: %v", err)
			break
		}
	}
	log.Println("Closing Connection")
}
