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
	"net"
	"strings"
	"time"
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
	// tcp server targets
	ServerIP string
	ServerPort uint

	ReceiveHandler func(data []byte)
	ErrorHandler func(err error, socketCloseFunc func())

	ReadBufferSize uint
	ReaderYieldDuration time.Duration

	ReadDeadLineDuration time.Duration
	WriteDeadLineDuration time.Duration

	// tcp connection
	_tcpConn net.Conn
	_readerEnd chan bool
	_readerStarted bool
}

// resolveTcpAddr takes tcp server ip and port defined within TCPClient struct,
// and resolve to net.TCPAddr target
func (c *TCPClient) resolveTcpAddr() (*net.TCPAddr, error) {
	if util.LenTrim(c.ServerIP) == 0 {
		return nil, fmt.Errorf("TCP Server IP is Required")
	}

	if c.ServerPort == 0 || c.ServerPort > 65535 {
		return nil, fmt.Errorf("TCP Server Port Must Be 1 - 65535")
	}

	return net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", c.ServerIP, c.ServerPort))
}

// Dial will dial tcp client to the remote tcp server host ip and port defined within the TCPClient struct,
// if Dial failed, an error is returned,
// if Dial succeeded, nil is returned
func (c *TCPClient) Dial() error {
	if c.ReceiveHandler == nil {
		return fmt.Errorf("TCP Client ReceiveHandler Must Be Defined")
	}

	if c.ErrorHandler == nil {
		return fmt.Errorf("TCP Client ErrorHandler Must Be Defined")
	}

	if tcpAddr, err := c.resolveTcpAddr(); err != nil {
		return err
	} else {
		if c._tcpConn, err = net.DialTCP("tcp", nil, tcpAddr); err != nil {
			return err
		} else {
			return nil
		}
	}
}

// Close will close the tcp connection for the current TCPClient struct object
func (c *TCPClient) Close() {
	if c._tcpConn != nil {
		_ = c._tcpConn.Close()
		c._tcpConn = nil
	}
}

// Write will write data into tcp connection stream, and deliver to the TCP Server host currently connected
func (c *TCPClient) Write(data []byte) error {
	if c._tcpConn == nil {
		return fmt.Errorf("TCP Server Not Yet Connected")
	}

	if len(data) == 0 {
		return fmt.Errorf("Data Required When Write to TCP Server")
	}

	if c.WriteDeadLineDuration > 0 {
		_ = c._tcpConn.SetWriteDeadline(time.Now().Add(c.WriteDeadLineDuration))
		defer c._tcpConn.SetWriteDeadline(time.Time{})
	}

	if _, err := c._tcpConn.Write(data); err != nil {
		return fmt.Errorf("Write Data to TCP Server Failed: %s", err.Error())
	} else {
		return nil
	}
}

// Read will read data into tcp client from TCP Server host,
// Read will block until read deadlined timeout, then loop continues to read again
func (c *TCPClient) Read() (data []byte, timeout bool, err error) {
	if c._tcpConn == nil {
		return nil, false, fmt.Errorf("TCP Server Not Yet Connected")
	}

	readDeadLine := 1000 * time.Millisecond
	if c.ReadDeadLineDuration >= 250*time.Millisecond && c.ReadDeadLineDuration <= 5000*time.Millisecond {
		readDeadLine = c.ReadDeadLineDuration
	}
	_ = c._tcpConn.SetReadDeadline(time.Now().Add(readDeadLine))
	defer c._tcpConn.SetReadDeadline(time.Time{})

	if c.ReadBufferSize == 0 || c.ReadBufferSize > 65535 {
		c.ReadBufferSize = 1024
	}

	data = make([]byte, c.ReadBufferSize)

	if _, err = c._tcpConn.Read(data); err != nil {
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
	if c._tcpConn == nil {
		return fmt.Errorf("TCP Server Not Yet Connected")
	}

	if !c._readerStarted {
		c._readerEnd = make(chan bool)
		c._readerStarted = true
	}

	yield := 25 * time.Millisecond

	if c.ReaderYieldDuration > 0 && c.ReaderYieldDuration <= 1000 * time.Millisecond {
		yield = c.ReaderYieldDuration
	}

	// start reader loop and continue until Reader End
	go func() {
		for {
			select {
			case <-c._readerEnd:
				// command received to end tcp reader service
				c._readerStarted = false
				return

			default:
				// perform read action on connected tcp server
				if data, timeout, err := c.Read(); err != nil && !timeout {
					// read fail, not timeout, deliver error data to handler, stop reader
					if c.ErrorHandler != nil {
						// send error to error handler
						if strings.Contains(strings.ToLower(err.Error()), "eof") {
							c.ErrorHandler(fmt.Errorf("TCP Client Socket Closed By Remote Host"), c.Close)
						} else {
							c.ErrorHandler(err, nil)
						}
					}

					// end reader service
					c._readerStarted = false
					return
				} else if !timeout {
					// read successful, deliver read data to handler
					if c.ReceiveHandler != nil {
						// send received data to receive handler
						c.ReceiveHandler(data)
					} else {
						// if error handler is not defined, end reader service
						if c.ErrorHandler != nil {
							c.ErrorHandler(fmt.Errorf("TCP Client Receive Handler Undefined"), c.Close)
						}

						c._readerStarted = false
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
	if c._readerStarted {
		c._readerEnd <- true
	}
}
