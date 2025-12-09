package tcp

/*
 * Copyright 2020-2026 Aldelo, LP
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
	"net"
	"strings"
	"sync"
	"time"

	util "github.com/aldelo/common"
)

// TCPClient defines tcp client connection struct
//
// ServerIP = tcp server ip address
// ServerPort = tcp server port
// ReceiveHandler = func handler to be triggered when data is received from tcp server
// ErrorHandler = func handler to be triggered when error is received from tcp server (while performing reader service)
// ReadBufferSize = default: 1024, defines the read buffer byte size, if > 65535, defaults to 1024
// ReaderYieldDuration = default: 25ms, defines the amount of time yielded during each reader service loop cycle, if > 1000ms, defaults to 25ms
// ReadDeadLineDuration = default: 1000ms, defines the amount of time given to read action before timeout, valid range is 250ms - 5000ms
// WriteDeadLineDuration = default: 0, duration value used to control write timeouts, this value is added to current time during write timeout set action
type TCPClient struct {
	mu sync.RWMutex

	// tcp server targets
	ServerIP   string
	ServerPort uint

	ReceiveHandler func(data []byte)
	ErrorHandler   func(err error, socketCloseFunc func())

	ReadBufferSize      uint
	ReaderYieldDuration time.Duration

	ReadDeadLineDuration  time.Duration
	WriteDeadLineDuration time.Duration

	// tcp connection
	_tcpConn       net.Conn
	_readerEnd     chan struct{}
	_readerStarted bool
}

// resolveTcpAddr takes tcp server ip and port defined within TCPClient struct,
// and resolve to net.TCPAddr target
func (c *TCPClient) resolveTcpAddr() (*net.TCPAddr, error) {
	if c == nil {
		return nil, fmt.Errorf("TCP Client Cannot Be Nil")
	}

	c.mu.RLock()
	serverIP := c.ServerIP
	serverPort := c.ServerPort
	c.mu.RUnlock()

	if util.LenTrim(serverIP) == 0 {
		return nil, fmt.Errorf("TCP Server IP is Required")
	}

	if serverPort == 0 || serverPort > 65535 {
		return nil, fmt.Errorf("TCP Server Port Must Be 1 - 65535")
	}

	return net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", serverIP, serverPort))
}

// Dial will dial tcp client to the remote tcp server host ip and port defined within the TCPClient struct,
// if Dial failed, an error is returned,
// if Dial succeeded, nil is returned
func (c *TCPClient) Dial() error {
	if c == nil {
		return fmt.Errorf("TCP Client Cannot Be Nil")
	}

	if c.ReceiveHandler == nil {
		return fmt.Errorf("TCP Client ReceiveHandler Must Be Defined")
	}

	if c.ErrorHandler == nil {
		return fmt.Errorf("TCP Client ErrorHandler Must Be Defined")
	}

	if tcpAddr, err := c.resolveTcpAddr(); err != nil {
		return err
	} else {
		c.mu.Lock()
		defer c.mu.Unlock()

		if c._tcpConn, err = net.DialTCP("tcp", nil, tcpAddr); err != nil {
			return err
		} else {
			return nil
		}
	}
}

// Close will close the tcp connection for the current TCPClient struct object
func (c *TCPClient) Close() {
	if c == nil {
		return
	}

	// stop reader first to avoid races with a closing connection
	c.StopReader()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c._tcpConn != nil {
		_ = c._tcpConn.Close()
		c._tcpConn = nil
	}
}

// Write will write data into tcp connection stream, and deliver to the TCP Server host currently connected
func (c *TCPClient) Write(data []byte) error {
	if c == nil {
		return fmt.Errorf("TCP Client Cannot Be Nil")
	}

	c.mu.RLock()
	_tcpConn := c._tcpConn
	writeDeadlineDuration := c.WriteDeadLineDuration
	c.mu.RUnlock()

	if _tcpConn == nil {
		return fmt.Errorf("TCP Server Not Yet Connected")
	}

	if len(data) == 0 {
		return fmt.Errorf("Data Required When Write to TCP Server")
	}

	if writeDeadlineDuration > 0 {
		_ = _tcpConn.SetWriteDeadline(time.Now().Add(writeDeadlineDuration))
		defer _tcpConn.SetWriteDeadline(time.Time{})
	}

	if _, err := _tcpConn.Write(data); err != nil {
		return fmt.Errorf("Write Data to TCP Server Failed: %s", err.Error())
	} else {
		return nil
	}
}

// Read will read data into tcp client from TCP Server host,
// Read will block until read deadlined timeout, then loop continues to read again
func (c *TCPClient) Read() (data []byte, timeout bool, err error) {
	if c == nil {
		return nil, false, fmt.Errorf("TCP Client Cannot Be Nil")
	}

	c.mu.RLock()
	_tcpConn := c._tcpConn
	readDeadlineDuration := c.ReadDeadLineDuration
	readBufferSize := c.ReadBufferSize
	c.mu.RUnlock()

	if _tcpConn == nil {
		return nil, false, fmt.Errorf("TCP Server Not Yet Connected")
	}

	readDeadLine := 1000 * time.Millisecond
	if readDeadlineDuration >= 250*time.Millisecond && readDeadlineDuration <= 5000*time.Millisecond {
		readDeadLine = readDeadlineDuration
	}
	_ = _tcpConn.SetReadDeadline(time.Now().Add(readDeadLine))
	defer _tcpConn.SetReadDeadline(time.Time{})

	if readBufferSize == 0 || readBufferSize > 65535 {
		readBufferSize = 1024
	}

	data = make([]byte, readBufferSize)

	if _, err = _tcpConn.Read(data); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "timeout") {
			return nil, true, fmt.Errorf("Read Data From TCP Server Timeout: %s", err)
		} else {
			return nil, false, fmt.Errorf("Read Data From TCP Server Failed: %s", err)
		}
	} else {
		return bytes.Trim(data, "\x00"), false, nil
	}
}

// StartReader will start the tcp client reader service,
// it will continuously read data from tcp server
func (c *TCPClient) StartReader() error {
	if c == nil {
		return fmt.Errorf("TCP Client Cannot Be Nil")
	}

	c.mu.RLock()
	_tcpConn := c._tcpConn
	readerYieldDuration := c.ReaderYieldDuration
	receiveHandler := c.ReceiveHandler
	errorHandler := c.ErrorHandler
	c.mu.RUnlock()

	if _tcpConn == nil {
		return fmt.Errorf("TCP Server Not Yet Connected")
	}

	c.mu.Lock()
	if c._readerStarted {
		c.mu.Unlock()
		return nil // already started
	}
	c._readerEnd = make(chan struct{}, 1) // buffered to avoid blocking StopReader
	c._readerStarted = true
	readerEnd := c._readerEnd
	c.mu.Unlock()

	yield := 25 * time.Millisecond
	if readerYieldDuration > 0 && readerYieldDuration <= 1000*time.Millisecond {
		yield = readerYieldDuration
	}

	// start reader loop and continue until Reader End
	go func() {
		defer func() {
			c.mu.Lock()
			c._readerStarted = false
			c._readerEnd = nil
			c.mu.Unlock()
		}()

		for {
			select {
			case <-readerEnd:
				// command received to end tcp reader service
				return

			default:
				// perform read action on connected tcp server
				if data, timeout, err := c.Read(); err != nil && !timeout {
					// read fail, not timeout, deliver error data to handler, stop reader
					if errorHandler != nil {
						// send error to error handler
						if strings.Contains(strings.ToLower(err.Error()), "eof") {
							errorHandler(fmt.Errorf("TCP Client Socket Closed By Remote Host"), c.Close)
						} else {
							errorHandler(err, nil)
						}
					}
					return

				} else if !timeout {
					// read successful, deliver read data to handler
					if receiveHandler != nil {
						// send received data to receive handler
						receiveHandler(data)
					} else {
						// if error handler is not defined, end reader service
						if errorHandler != nil {
							errorHandler(fmt.Errorf("TCP Client Receive Handler Undefined"), c.Close)
						}
						return
					}
				}

				// note:
				// if read timed out, the loop will continue
			}

			time.Sleep(yield)
		}
	}()

	return nil
}

// StopReader will end the tcp client reader service
func (c *TCPClient) StopReader() {
	if c == nil {
		return
	}

	c.mu.Lock()
	if !c._readerStarted || c._readerEnd == nil {
		c.mu.Unlock()
		return // not started
	}
	readerEnd := c._readerEnd

	// mark stopped; goroutine's defer will also mark stopped, but this ensures
	// we don't start another while a stop is in progress
	c._readerStarted = false
	c._readerEnd = nil
	c.mu.Unlock()

	// non-blocking stop signal so it won't deadlock if the reader already exited
	select {
	case readerEnd <- struct{}{}:
	default:
	}
}
