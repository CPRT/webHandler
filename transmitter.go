package webHandler

import (
	"github.com/gorilla/websocket"
)

// This structure is provided to the users of the system
// to send messages back to the websockets
type Transmitter struct {
	ws *websocket.Conn
	sh SocketHandler
	wt chan Transmitter
	msg []byte
	mode uint8
	maxMode uint8
}

/*
 * Modes available to the Transmitter to instruct which 
 * websockets the transmitter should send messages on
 */
const (
	//The order here is essential for proper function of the system
	// since the check is done by checking the checking the magnitude of the value
	// I didn't use iota, even though it would work, since it helps clarify that the 
	// order is important
	Broadcast = 0
	Handle    = 1
	Socket    = 2
)

/*
 * Retrieve the socket handler (if any) associated with this transmitter
 * @return {SocketHandler}
 */
func (t Transmitter) GetHandler() SocketHandler {
	return t.sh
}

/*
 * Retrieve a list of modess available on this transmitter
 * @return {[]uint8} the list of available codes
 */
func (t Transmitter) GetModes() []uint8 {
	if t.maxMode == Socket {
		return []uint8{Broadcast, Handle, Socket}
	} else if t.maxMode == Handle {
		return []uint8{Broadcast, Handle}
	} else /* t.maxMode == Broadcast */ {
		return []uint8{Broadcast}	
	}
}

/*
 * @param data {[]byte}
 * @param mode {uint8} the mode to use for the Transmitter. 
 * 			Options are:
 *				Broadcast: Send on all websockets
 *				Handle:   Send only on the websockets connected with the handler attached to this struct (if any)
 *				Socket:    Send only on the websocket connected with this struct (if any)
 * @returns {bool} whether the transmission occurred. This can fail if an invalid command mode is sent
 */
func (t Transmitter) Send(data []byte, mode uint8) bool {
	if t.maxMode < mode {return false}
	if t.wt == nil {return false}
	var trCopy Transmitter = t
	trCopy.msg = data
	trCopy.mode = mode
	t.wt<- trCopy
	return true
}