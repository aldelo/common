package rest

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
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aldelo/common/tlsconfig"
	"google.golang.org/protobuf/proto"
)

// HTTP client configuration constants
const (
	defaultHTTPClientTimeout = 30  // Default HTTP client timeout in seconds
	maxHTTPClientTimeout     = 300 // Maximum HTTP client timeout in seconds (5 minutes)
	httpSuccessStatusMin     = 200 // Minimum HTTP success status code
	httpErrorStatusMin       = 300 // Minimum HTTP error status code
	httpInternalErrorStatus  = 500 // HTTP internal server error status code
)

var mu sync.RWMutex

// maxResponseBytes defines the maximum response body size to prevent memory exhaustion
// from malicious or oversized responses (10MB limit)
const maxResponseBytes = 10 << 20 // 10MB

// server ca pems stores list of self-signed CAs for client tls config
var serverCaPems []string

// client tls config stores the current client tls root CA config object
var clientTlsConfig *tls.Config

// client timeout seconds for http client requests
var clientTimeoutSeconds int

func SetClientTimeoutSeconds(timeoutSeconds int) {
	mu.Lock()
	defer mu.Unlock()

	clientTimeoutSeconds = timeoutSeconds
}

// AppendServerCAPemFiles adds self-signed server ca pems to local cache,
// and then recreates the clientTlsConfig object based on the new list of CAs
func AppendServerCAPemFiles(caPemFilePath ...string) error {
	if len(caPemFilePath) > 0 {
		mu.Lock()
		defer mu.Unlock()

		serverCaPems = append(serverCaPems, caPemFilePath...)
		return newClientTlsCAsConfig()
	} else {
		return nil
	}
}

// ResetServerCAPemFiles first clears serverCaPems cache,
// then adds self-signed server ca pems to local cache,
// then recreates the clientTlsConfig object based on the new list of CAs
func ResetServerCAPemFiles(caPemFilePath ...string) error {
	mu.Lock()
	defer mu.Unlock()

	serverCaPems = []string{}
	clientTlsConfig = nil

	if len(caPemFilePath) > 0 {
		serverCaPems = append(serverCaPems, caPemFilePath...)
		return newClientTlsCAsConfig()
	} else {
		return nil
	}
}

// newClientTlsCAsConfig creates new ClientTlsConfig object for the serverCaPems in cache
func newClientTlsCAsConfig() error {
	if len(serverCaPems) == 0 {
		clientTlsConfig = nil
		return nil
	}

	t := &tlsconfig.TlsConfig{}

	if c, e := t.GetClientTlsConfig(serverCaPems, "", ""); e != nil {
		return e
	} else {
		clientTlsConfig = c
		return nil
	}
}

// newHttpClient creates an HTTP client with proper transport configuration.
// Note: For high-throughput scenarios, consider using a shared http.Client at the application level.
func newHttpClient(timeout int, tlsCfg *tls.Config) *http.Client {
	// Configure transport with connection pooling limits to prevent socket exhaustion
	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	if tlsCfg != nil {
		tr.TLSClientConfig = tlsCfg
	}

	return &http.Client{
		Transport: tr,
		Timeout:   time.Duration(timeout) * time.Second,
	}
}

// HeaderKeyValue is struct used for containing http header element key value pair
type HeaderKeyValue struct {
	Key   string
	Value string
}

// GET sends url get request to host and retrieve the body response in string
func GET(url string, headers []*HeaderKeyValue) (statusCode int, body string, err error) {
	mu.RLock()
	timeout := clientTimeoutSeconds
	tlsCfg := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout, tlsCfg)

	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("GET", url, nil); err != nil {
		return 0, "", errors.New("Create New Http GET Request Failed: " + err.Error())
	}

	// add headers to request if any
	if len(headers) > 0 {
		for _, v := range headers {
			req.Header.Add(v.Key, v.Value)
		}
	}

	// execute http request and assign response
	var resp *http.Response

	if resp, err = client.Do(req); err != nil {
		return httpInternalErrorStatus, "", errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Get Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", err
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {
		return statusCode, "", errors.New("[" + strconv.Itoa(statusCode) + " - Get Resp] " + string(respBytes))
	}

	// success
	return statusCode, string(respBytes), nil
}

// POST sends url post request to host and retrieve the body response in string
//
// Default Header = Content-Type: application/x-www-form-urlencoded
//
// JSON Content-Type Header:
//
//	Content-Type: application/json
func POST(url string, headers []*HeaderKeyValue, requestBody string) (statusCode int, responseBody string, err error) {
	mu.RLock()
	timeout := clientTimeoutSeconds
	tlsCfg := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout, tlsCfg)

	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("POST", url, bytes.NewBuffer([]byte(requestBody))); err != nil {
		return 0, "", errors.New("Create New Http Post Request Failed: " + err.Error())
	}

	// add headers to request if any
	contentTypeConfigured := false

	if len(headers) > 0 {
		for _, v := range headers {
			req.Header.Add(v.Key, v.Value)

			if strings.ToUpper(v.Key) == "CONTENT-TYPE" {
				contentTypeConfigured = true
			}
		}
	}

	if !contentTypeConfigured {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	}

	// execute http request and assign response
	var resp *http.Response

	if resp, err = client.Do(req); err != nil {
		return httpInternalErrorStatus, "", errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Post Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", err
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {
		return statusCode, "", errors.New("[" + strconv.Itoa(statusCode) + " - Post Resp] " + string(respBytes))
	}

	return statusCode, string(respBytes), nil
}

// PUT sends url put request to host and retrieve the body response in string
//
// Default Header = Content-Type: application/x-www-form-urlencoded
//
// JSON Content-Type Header:
//
//	Content-Type: application/json
func PUT(url string, headers []*HeaderKeyValue, requestBody string) (statusCode int, responseBody string, err error) {
	mu.RLock()
	timeout := clientTimeoutSeconds
	tlsCfg := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout, tlsCfg)

	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("PUT", url, bytes.NewBuffer([]byte(requestBody))); err != nil {
		return 0, "", errors.New("Create New Http Put Request Failed: " + err.Error())
	}

	// add headers to request if any
	contentTypeConfigured := false

	if len(headers) > 0 {
		for _, v := range headers {
			req.Header.Add(v.Key, v.Value)

			if strings.ToUpper(v.Key) == "CONTENT-TYPE" {
				contentTypeConfigured = true
			}
		}
	}

	if !contentTypeConfigured {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	}

	// execute http request and assign response
	var resp *http.Response

	if resp, err = client.Do(req); err != nil {
		return httpInternalErrorStatus, "", errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Put Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", err
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {
		return statusCode, "", errors.New("[" + strconv.Itoa(statusCode) + " - Put Resp] " + string(respBytes))
	}

	return statusCode, string(respBytes), nil
}

// DELETE sends url delete request to host and performs delete action (no body expected)
//
// Default Header = Content-Type: application/x-www-form-urlencoded
//
// JSON Content-Type Header:
//
//	Content-Type: application/json
func DELETE(url string, headers []*HeaderKeyValue) (statusCode int, body string, err error) {
	mu.RLock()
	timeout := clientTimeoutSeconds
	tlsCfg := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout, tlsCfg)
	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("DELETE", url, nil); err != nil {
		return 0, "", errors.New("Create New Http Delete Request Failed: " + err.Error())
	}

	// add headers to request if any
	if len(headers) > 0 {
		for _, v := range headers {
			req.Header.Add(v.Key, v.Value)
		}
	}

	// execute http request and assign response
	var resp *http.Response

	if resp, err = client.Do(req); err != nil {
		return httpInternalErrorStatus, "", errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Delete Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", err
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {
		return statusCode, "", errors.New("[" + strconv.Itoa(statusCode) + " - Delete Resp] " + string(respBytes))
	}

	// success
	return statusCode, string(respBytes), nil
}

// GETProtoBuf sends url get request to host, and retrieves response via protobuf object as an output pointer parameter
//
// default header if not specified:
//
//	Content-Type: application/x-protobuf
func GETProtoBuf(url string, headers []*HeaderKeyValue, outResponseProtoBufObjectPtr proto.Message) (statusCode int, err error) {
	mu.RLock()
	timeout := clientTimeoutSeconds
	tlsCfg := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout, tlsCfg)
	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("GET", url, nil); err != nil {
		outResponseProtoBufObjectPtr = nil
		return 0, errors.New("Create New Http GET ProtoBuf Request Failed: " + err.Error())
	}

	// add headers to request if any
	contentTypeConfigured := false

	if len(headers) > 0 {
		for _, v := range headers {
			req.Header.Add(v.Key, v.Value)

			if strings.ToUpper(v.Key) == "CONTENT-TYPE" {
				contentTypeConfigured = true
			}
		}
	}

	if !contentTypeConfigured {
		req.Header.Add("Content-Type", "application/x-protobuf")
	}

	// execute http request and assign response
	var resp *http.Response

	if resp, err = client.Do(req); err != nil {
		outResponseProtoBufObjectPtr = nil
		return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Get ProtoBuf Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		outResponseProtoBufObjectPtr = nil
		return statusCode, err
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {
		outResponseProtoBufObjectPtr = nil
		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Get ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal bytes to protobuf object
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {
			outResponseProtoBufObjectPtr = nil
			return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Get ProtoBuf Error] Unmarshal ProtoBuf Response Failed: " + err.Error())
		}
	}

	// success if outResponseProtoBufObjectPtr is not nil
	if outResponseProtoBufObjectPtr != nil {
		return statusCode, nil
	} else {
		return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Get ProtoBuf Error] Expected ProtoBuf Response Object Nil")
	}
}

// POSTProtoBuf sends url post request to host, with body content in protobuf pointer object,
// and retrieves response in protobuf object as output pointer parameter
//
// default header if not specified:
//
//	Content-Type: application/x-protobuf
func POSTProtoBuf(url string, headers []*HeaderKeyValue, requestProtoBufObjectPtr proto.Message, outResponseProtoBufObjectPtr proto.Message) (statusCode int, err error) {
	mu.RLock()
	timeout := clientTimeoutSeconds
	tlsCfg := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout, tlsCfg)
	// marshal proto message to bytes
	if requestProtoBufObjectPtr == nil {
		outResponseProtoBufObjectPtr = nil
		return 0, errors.New("Request ProtoBuf Object is Nil")
	}

	reqBytes, err2 := proto.Marshal(requestProtoBufObjectPtr)

	if err2 != nil {
		outResponseProtoBufObjectPtr = nil
		return 0, errors.New("Request ProtoBuf Object Marshaling Failed: " + err2.Error())
	}

	// create http request from client
	req, err3 := http.NewRequest("POST", url, bytes.NewReader(reqBytes))

	if err3 != nil {
		outResponseProtoBufObjectPtr = nil
		return 0, errors.New("Create New Http Post ProtoBuf Request Failed: " + err3.Error())
	}

	// add headers to request if any
	contentTypeConfigured := false

	if len(headers) > 0 {
		for _, v := range headers {
			req.Header.Add(v.Key, v.Value)

			if strings.ToUpper(v.Key) == "CONTENT-TYPE" {
				contentTypeConfigured = true
			}
		}
	}

	if !contentTypeConfigured {
		req.Header.Add("Content-Type", "application/x-protobuf")
	}

	// execute http request and assign response
	var resp *http.Response

	if resp, err = client.Do(req); err != nil {
		outResponseProtoBufObjectPtr = nil
		return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Post ProtoBuf Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		outResponseProtoBufObjectPtr = nil
		return statusCode, err
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {
		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Post ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal response bytes into protobuf object message
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {
			outResponseProtoBufObjectPtr = nil
			return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Post ProtoBuf Error] Unmarshal ProtoBuf Response Failed: " + err.Error())
		}
	}

	// success if outResponseProtoBufObjectPtr is not nil
	if outResponseProtoBufObjectPtr != nil {
		return statusCode, nil
	} else {
		return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Post ProtoBuf Error] Expected ProtoBuf Response Object Nil")
	}
}

// PUTProtoBuf sends url put request to host, with body content in protobuf pointer object,
// and retrieves response in protobuf object as output pointer parameter
//
// default header if not specified:
//
//	Content-Type: application/x-protobuf
func PUTProtoBuf(url string, headers []*HeaderKeyValue, requestProtoBufObjectPtr proto.Message, outResponseProtoBufObjectPtr proto.Message) (statusCode int, err error) {
	mu.RLock()
	timeout := clientTimeoutSeconds
	tlsCfg := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout, tlsCfg)
	// marshal proto message to bytes
	if requestProtoBufObjectPtr == nil {
		outResponseProtoBufObjectPtr = nil
		return 0, errors.New("Request ProtoBuf Object is Nil")
	}

	reqBytes, err2 := proto.Marshal(requestProtoBufObjectPtr)

	if err2 != nil {
		outResponseProtoBufObjectPtr = nil
		return 0, errors.New("Request ProtoBuf Object Marshaling Failed: " + err2.Error())
	}

	// create http request from client
	req, err3 := http.NewRequest("PUT", url, bytes.NewReader(reqBytes))

	if err3 != nil {
		outResponseProtoBufObjectPtr = nil
		return 0, errors.New("Create New Http PUT ProtoBuf Request Failed: " + err3.Error())
	}

	// add headers to request if any
	contentTypeConfigured := false

	if len(headers) > 0 {
		for _, v := range headers {
			req.Header.Add(v.Key, v.Value)

			if strings.ToUpper(v.Key) == "CONTENT-TYPE" {
				contentTypeConfigured = true
			}
		}
	}

	if !contentTypeConfigured {
		req.Header.Add("Content-Type", "application/x-protobuf")
	}

	// execute http request and assign response
	var resp *http.Response

	if resp, err = client.Do(req); err != nil {
		outResponseProtoBufObjectPtr = nil
		return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Put ProtoBuf Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		outResponseProtoBufObjectPtr = nil
		return statusCode, err
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {
		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Put ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal response bytes into protobuf object message
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {
			outResponseProtoBufObjectPtr = nil
			return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Put ProtoBuf Error] Unmarshal ProtoBuf Response Failed: " + err.Error())
		}
	}

	// success if outResponseProtoBufObjectPtr is not nil
	if outResponseProtoBufObjectPtr != nil {
		return statusCode, nil
	} else {
		return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Put ProtoBuf Error] Expected ProtoBuf Response Object Nil")
	}
}

// DELETEProtoBuf sends url delete request to host, and retrieves response via protobuf object as an output pointer parameter
//
// default header if not specified:
//
//	Content-Type: application/x-protobuf
func DELETEProtoBuf(url string, headers []*HeaderKeyValue, outResponseProtoBufObjectPtr proto.Message) (statusCode int, err error) {
	mu.RLock()
	timeout := clientTimeoutSeconds
	tlsCfg := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout, tlsCfg)
	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("DELETE", url, nil); err != nil {
		outResponseProtoBufObjectPtr = nil
		return 0, errors.New("Create New Http Delete ProtoBuf Request Failed: " + err.Error())
	}

	// add headers to request if any
	contentTypeConfigured := false

	if len(headers) > 0 {
		for _, v := range headers {
			req.Header.Add(v.Key, v.Value)

			if strings.ToUpper(v.Key) == "CONTENT-TYPE" {
				contentTypeConfigured = true
			}
		}
	}

	if !contentTypeConfigured {
		req.Header.Add("Content-Type", "application/x-protobuf")
	}

	// execute http request and assign response
	var resp *http.Response

	if resp, err = client.Do(req); err != nil {
		outResponseProtoBufObjectPtr = nil
		return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Delete ProtoBuf Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		outResponseProtoBufObjectPtr = nil
		return statusCode, err
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {
		outResponseProtoBufObjectPtr = nil
		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Delete ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal bytes to protobuf object
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {
			outResponseProtoBufObjectPtr = nil
			return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Delete ProtoBuf Error] Unmarshal ProtoBuf Response Failed: " + err.Error())
		}
	}

	// success if outResponseProtoBufObjectPtr is not nil
	if outResponseProtoBufObjectPtr != nil {
		return statusCode, nil
	} else {
		return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Delete ProtoBuf Error] Expected ProtoBuf Response Object Nil")
	}
}
