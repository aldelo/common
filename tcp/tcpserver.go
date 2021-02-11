package tcp

/*
 * Copyright 2020-2021 Aldelo, LP
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import (
	"bytes"
	"fmt"
	util "github.com/aldelo/common"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// TCPServer defines a concurrent tcp server for handling inbound client requests, and sending back responses
//
// Port = this tcp server port number to listen on
// ListenerErrorHandler = func to trigger when listener caused error event, this also signifies the end of tcp server listener serving mode
// ClientReceiveHandler = func to trigger when this tcp server receives data from client, the writeToClientFunc is provided for sending data back to the connected client
// ClientErrorHandler = func to trigger when this tcp server detected tcp client error during data receive event
// ReadBufferSize = default 1024, cannot be greater than 65535, defines the byte length of read buffer
// ListenerYieldDuration = default 0, no yield, valid range is 0 - 250ms, the amount of time to yield to cpu during listener accept loop cycle
// ReaderYieldDuration = default 25ms, the amount of time to yield to cpu process during each cycle of Read loop
// ReadDeadLineDuration = default 1000ms, the amount of time to wait for read action before timeout, valid range is 250ms - 5000ms
// WriteDeadLineDuration = the amount of time to wait for write action before timeout, if 0, then no timeout
type TCPServer struct {
	Port uint

	ListenerAcceptHandler func(clientIP string)
	ListenerErrorHandler func(err error)

	ClientReceiveHandler func(clientIP string, data []byte, writeToClientFunc func(writeData []byte, clientIP string) error)
	ClientErrorHandler func(clientIP string, err error)

	ReadBufferSize uint
	ListenerYieldDuration time.Duration
	ReaderYieldDuration time.Duration

	ReadDeadlineDuration time.Duration
	WriteDeadLineDuration time.Duration

	_tcpListener net.Listener
	_serving bool
	_clients map[string]net.Conn
	_clientEnd map[string]chan bool
	_mux sync.Mutex
}

// Serve will startup TCP Server and begin accepting TCP Client connections, and initiates connection processing
func (s *TCPServer) Serve() (err error) {
	if s._serving {
		return fmt.Errorf("TCP Server Already Serving, Use Close() To Stop")
	}

	if s.Port == 0 || s.Port > 65535 {
		return fmt.Errorf("TCP Server Listening Port Must Be 1 - 65535")
	}

	if s._tcpListener, err = net.Listen("tcp4", fmt.Sprintf(":%d", s.Port)); err != nil {
		return err
	} else {
		s._mux.Lock()

		s._serving = true
		s._clients = make(map[string]net.Conn)
		s._clientEnd = make(map[string]chan bool)

		s._mux.Unlock()

		// start continuous loop to accept incoming tcp client,
		// and initiate client handler go-routine for each connection
		go func() {
			for {
				// perform listener action on connected tcp client
				if c, e := s._tcpListener.Accept(); e != nil {
					// error encountered
					if s.ListenerErrorHandler != nil {
						if strings.Contains(strings.ToLower(e.Error()), "closed") {
							s.ListenerErrorHandler(fmt.Errorf("TCP Server Closed"))
						} else {
							s.ListenerErrorHandler(e)
						}
					}

					// clean up clients
					s._mux.Lock()

					for _, v := range s._clients {
						if v != nil {
							_ = v.Close()
						}
					}

					// stop serving
					s._clients = nil
					s._serving = false

					s._mux.Unlock()

					return
				} else {
					// tcp client connected accepted
					s._mux.Lock()

					// register client connection to map
					s._clients[c.RemoteAddr().String()] = c

					s._mux.Unlock()

					// handle client connection
					go s.handleClientConnection(c)

					// notify accept event
					if s.ListenerAcceptHandler != nil {
						s.ListenerAcceptHandler(c.RemoteAddr().String())
					}
				}

				if s.ListenerYieldDuration > 0 && s.ListenerYieldDuration <= 250*time.Millisecond {
					time.Sleep(s.ListenerYieldDuration)
				}
			}
		}()

		log.Println("TCP Server Addr: ", util.GetLocalIP() + ":" + util.UintToStr(s.Port))
		return nil
	}
}

// Close will shut down TCP Server, and clean up resources allocated
func (s *TCPServer) Close() {
	if s._tcpListener != nil {
		_ = s._tcpListener.Close()
	}

	// clean up clients
	s._mux.Lock()

	for _, v := range s._clients {
		if v != nil {
			_ = v.Close()
		}
	}

	// stop serving
	s._clients = nil
	s._serving = false

	s._mux.Unlock()
}

// GetConnectedClients returns list of connected tcp clients
func (s *TCPServer) GetConnectedClients() (clientsList []string) {
	s._mux.Lock()
	defer s._mux.Unlock()

	if s._clients == nil {
		return []string{}
	}

	if len(s._clients) == 0 {
		return []string{}
	}

	for k, _ := range s._clients {
		clientsList = append(clientsList, k)
	}

	return clientsList
}

// DisconnectClient will close client connection and end the client reader service
func (s *TCPServer) DisconnectClient(clientIP string) {
	if s._clientEnd != nil {
		s._clientEnd[clientIP] = make(chan bool)
		s._clientEnd[clientIP] <- true
	}
}

// WriteToClient accepts byte slice and clientIP target, for writing data to the target client
func (s *TCPServer) WriteToClient(writeData []byte, clientIP string) error {
	if writeData == nil {
		return nil
	}

	if len(writeData) == 0 {
		return nil
	}

	if util.LenTrim(clientIP) == 0 {
		return nil
	}

	s._mux.Lock()
	defer s._mux.Unlock()

	if s._clients == nil {
		return fmt.Errorf("Write To Client Failed: No TCP Client Connections Exist")
	}

	if len(s._clients) == 0 {
		return fmt.Errorf("Write To Client Failed: TCP Client Connections Count is Zero")
	}

	// obtain client connection
	if c, ok := s._clients[clientIP]; !ok {
		return fmt.Errorf("Write To Client IP %s Failed: Client Not Found in TCP Client Connections", clientIP)
	} else {
		// write data to tcp client connection
		if s.WriteDeadLineDuration > 0 {
			_ = c.SetWriteDeadline(time.Now().Add(s.WriteDeadLineDuration))
			defer c.SetWriteDeadline(time.Time{})
		}

		if _, e := c.Write(writeData); e != nil {
			return fmt.Errorf("Write To Client IP %s Failed: %s", clientIP, e.Error())
		} else {
			// write successful
			return nil
		}
	}
}

// handleClientConnection is an internal method invoked by Serve,
// handleClientConnection stores tcp client connection into map, and begins the read loop to continuously acquire client data upon arrival
func (s *TCPServer) handleClientConnection(conn net.Conn) {
	// clean up upon exit method
	defer conn.Close()

	readBufferSize := uint(1024)
	if s.ReadBufferSize > 0 && s.ReadBufferSize < 65535 {
		readBufferSize = s.ReadBufferSize
	}

	readYield := 25 * time.Millisecond
	if s.ReaderYieldDuration > 0 && s.ReaderYieldDuration < 1000*time.Millisecond {
		readYield = s.ReaderYieldDuration
	}

	readDeadline := 1000 * time.Millisecond
	if s.ReadDeadlineDuration > 250 * time.Millisecond && s.ReadDeadlineDuration <= 5000 * time.Millisecond {
		readDeadline = s.ReadDeadlineDuration
	}

	if s._clientEnd == nil {
		s._clientEnd = make(map[string]chan bool)
	}

	for {
		select {
		case <-s._clientEnd[conn.RemoteAddr().String()]:
			// command received to end tcp client connection handler
			_ = conn.Close()

			s._mux.Lock()
			delete(s._clients, conn.RemoteAddr().String())
			s._mux.Unlock()

			return

		default:
			// read data from client continuously
			readBytes := make([]byte, readBufferSize)

			_ = conn.SetReadDeadline(time.Now().Add(readDeadline))

			if _, e := conn.Read(readBytes); e != nil {
				_ = conn.SetReadDeadline(time.Time{})

				errInfo := strings.ToLower(e.Error())
				timeout := strings.Contains(errInfo, "timeout")
				eof := strings.Contains(errInfo, "eof")

				if timeout {
					// continue reader service
					time.Sleep(readYield)
					continue
				}

				if s.ClientErrorHandler != nil {
					if !eof {
						s.ClientErrorHandler(conn.RemoteAddr().String(), e)
					} else {
						s.ClientErrorHandler(conn.RemoteAddr().String(), fmt.Errorf("TCP Client Socket Closed By Remote Host"))
					}
				}

				// remove connection from map
				_ = conn.Close()

				s._mux.Lock()
				delete(s._clients, conn.RemoteAddr().String())
				s._mux.Unlock()

				// end client handler
				return

			} else {
				_ = conn.SetReadDeadline(time.Time{})

				// read ok
				if s.ClientReceiveHandler != nil {
					// send client data received to client handler for processing
					s.ClientReceiveHandler(conn.RemoteAddr().String(), bytes.Trim(readBytes, "\x00"), s.WriteToClient)
				} else {
					// if client receive handler is not defined, end the client handler service
					_ = conn.Close()

					s._mux.Lock()
					delete(s._clients, conn.RemoteAddr().String())
					s._mux.Unlock()

					return
				}
			}
		}

		time.Sleep(readYield)
	}
}




