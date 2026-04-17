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
	"fmt"
	"io"
	"log"
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

// serverCaPems stores list of self-signed CAs for client TLS config.
// PROCESS-GLOBAL: shared by all consumers in the same process.
var serverCaPems []string

// clientTlsConfig stores the current client TLS root CA config object.
// PROCESS-GLOBAL: shared by all consumers in the same process.
// Always access via cloneClientTlsConfig() to get an isolated copy.
var clientTlsConfig *tls.Config

// clientTimeoutSeconds is the HTTP client request timeout.
// PROCESS-GLOBAL: shared by all consumers in the same process.
var clientTimeoutSeconds int

// sharedTransport is the process-wide *http.Transport reused by every rest.* call.
// Sharing one Transport is what makes TCP keep-alives and TLS session resumption
// work across calls — constructing a Transport per request (plus CloseIdleConnections)
// forces a fresh TCP+TLS handshake every time, which is the core cost P1-PERF-1 targets.
// PROCESS-GLOBAL: set to nil when clientTlsConfig changes so the next call rebuilds it.
// MUST be accessed under mu (Lock to mutate, RLock to read).
var sharedTransport *http.Transport

// cloneClientTlsConfig returns a deep copy of the current clientTlsConfig,
// or nil if no TLS config is set. Must be called under mu.RLock (or mu.Lock).
// The clone prevents cross-consumer interference when the http.Transport
// mutates TLS session state during handshakes.
func cloneClientTlsConfig() *tls.Config {
	if clientTlsConfig == nil {
		return nil
	}
	return clientTlsConfig.Clone()
}

// SetClientTimeoutSeconds sets the HTTP client timeout for all subsequent requests.
// PROCESS-GLOBAL: this affects every consumer of the rest package in the same process.
// Concurrent calls are safe (protected by mutex), but callers should be aware that
// changing the timeout mid-flight affects all new requests, not just the caller's.
func SetClientTimeoutSeconds(timeoutSeconds int) {
	mu.Lock()
	defer mu.Unlock()

	clientTimeoutSeconds = timeoutSeconds
}

// AppendServerCAPemFiles adds self-signed server CA PEM files to the process-global cache,
// and then recreates the clientTlsConfig object based on the new list of CAs.
// PROCESS-GLOBAL: this affects every consumer of the rest package in the same process.
func AppendServerCAPemFiles(caPemFilePath ...string) error {
	if len(caPemFilePath) > 0 {
		mu.Lock()
		defer mu.Unlock()

		serverCaPems = append(serverCaPems, caPemFilePath...)
		sharedTransport = nil // TLS config changed — force transport rebuild on next call
		return newClientTlsCAsConfig()
	} else {
		return nil
	}
}

// ResetServerCAPemFiles first clears the process-global serverCaPems cache,
// then adds self-signed server CA PEM files to the cache,
// then recreates the clientTlsConfig object based on the new list of CAs.
// PROCESS-GLOBAL: this affects every consumer of the rest package in the same process.
func ResetServerCAPemFiles(caPemFilePath ...string) error {
	mu.Lock()
	defer mu.Unlock()

	serverCaPems = []string{}
	clientTlsConfig = nil
	sharedTransport = nil // TLS config changed — force transport rebuild on next call

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

// getSharedTransport returns the process-wide *http.Transport, building it lazily
// on first use or after clientTlsConfig has been replaced (sharedTransport set to nil).
// One Transport is shared across every call so TCP keep-alives and TLS session tickets
// persist between requests. The Transport's TLSClientConfig holds a private clone of
// clientTlsConfig — subsequent http.Transport handshake mutations stay isolated from
// the process-global source config.
func getSharedTransport() *http.Transport {
	mu.RLock()
	tr := sharedTransport
	mu.RUnlock()
	if tr != nil {
		return tr
	}

	mu.Lock()
	defer mu.Unlock()
	// Double-check under write lock — another goroutine may have built it already.
	if sharedTransport != nil {
		return sharedTransport
	}
	tlsCfg := cloneClientTlsConfig()
	tr = &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	if tlsCfg != nil {
		tr.TLSClientConfig = tlsCfg
	}
	sharedTransport = tr
	return tr
}

// newHttpClient returns an *http.Client wrapping the shared process-wide Transport
// with a per-call Timeout. The Client struct itself is cheap (~80 bytes); all the
// heavy state — idle-connection pool, DNS cache, TLS session cache — lives on the
// Transport and is reused across calls. Callers MUST NOT call CloseIdleConnections
// on the returned Client: doing so would wipe the shared pool for every other caller.
func newHttpClient(timeout int) *http.Client {
	return &http.Client{
		Transport: getSharedTransport(),
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
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout)

	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("GET", url, nil); err != nil {
		return 0, "", fmt.Errorf("Create New Http GET Request Failed: %w", err)
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
		return httpInternalErrorStatus, "", fmt.Errorf("[%d - Http Get Error] %w", httpInternalErrorStatus, err)
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if closeErr := resp.Body.Close(); closeErr != nil {
		// Body close errors are typically benign (already-drained connection)
		// but worth logging for observability in case of resource leaks.
		log.Printf("rest.GET: resp.Body.Close error: %v", closeErr)
	}

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", fmt.Errorf("reading response body: %w", err)
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
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout)

	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("POST", url, bytes.NewBuffer([]byte(requestBody))); err != nil {
		return 0, "", fmt.Errorf("Create New Http Post Request Failed: %w", err)
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
		return httpInternalErrorStatus, "", fmt.Errorf("[%d - Http Post Error] %w", httpInternalErrorStatus, err)
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if closeErr := resp.Body.Close(); closeErr != nil {
		log.Printf("rest.POST: resp.Body.Close error: %v", closeErr)
	}

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", fmt.Errorf("reading response body: %w", err)
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
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout)

	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("PUT", url, bytes.NewBuffer([]byte(requestBody))); err != nil {
		return 0, "", fmt.Errorf("Create New Http Put Request Failed: %w", err)
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
		return httpInternalErrorStatus, "", fmt.Errorf("[%d - Http Put Error] %w", httpInternalErrorStatus, err)
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if closeErr := resp.Body.Close(); closeErr != nil {
		log.Printf("rest.PUT: resp.Body.Close error: %v", closeErr)
	}

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", fmt.Errorf("reading response body: %w", err)
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
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout)
	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("DELETE", url, nil); err != nil {
		return 0, "", fmt.Errorf("Create New Http Delete Request Failed: %w", err)
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
		return httpInternalErrorStatus, "", fmt.Errorf("[%d - Http Delete Error] %w", httpInternalErrorStatus, err)
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if closeErr := resp.Body.Close(); closeErr != nil {
		log.Printf("rest.DELETE: resp.Body.Close error: %v", closeErr)
	}

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", fmt.Errorf("reading response body: %w", err)
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
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout)
	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("GET", url, nil); err != nil {

		return 0, fmt.Errorf("Create New Http GET ProtoBuf Request Failed: %w", err)
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

		return httpInternalErrorStatus, fmt.Errorf("[%d - Http Get ProtoBuf Error] %w", httpInternalErrorStatus, err)
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if closeErr := resp.Body.Close(); closeErr != nil {
		log.Printf("rest.GETProtoBuf: resp.Body.Close error: %v", closeErr)
	}

	if err != nil {
		// when read error, even if 200, still return error

		return statusCode, fmt.Errorf("reading response body: %w", err)
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {

		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Get ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal bytes to protobuf object
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {

			return httpInternalErrorStatus, fmt.Errorf("[%d - Http Get ProtoBuf Error] Unmarshal ProtoBuf Response Failed: %w", httpInternalErrorStatus, err)
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
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout)
	// marshal proto message to bytes
	if requestProtoBufObjectPtr == nil {

		return 0, errors.New("Request ProtoBuf Object is Nil")
	}

	reqBytes, err2 := proto.Marshal(requestProtoBufObjectPtr)

	if err2 != nil {

		return 0, fmt.Errorf("Request ProtoBuf Object Marshaling Failed: %w", err2)
	}

	// create http request from client
	req, err3 := http.NewRequest("POST", url, bytes.NewReader(reqBytes))

	if err3 != nil {

		return 0, fmt.Errorf("Create New Http Post ProtoBuf Request Failed: %w", err3)
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

		return httpInternalErrorStatus, fmt.Errorf("[%d - Http Post ProtoBuf Error] %w", httpInternalErrorStatus, err)
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if closeErr := resp.Body.Close(); closeErr != nil {
		log.Printf("rest.POSTProtoBuf: resp.Body.Close error: %v", closeErr)
	}

	if err != nil {
		// when read error, even if 200, still return error

		return statusCode, fmt.Errorf("reading response body: %w", err)
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {
		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Post ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal response bytes into protobuf object message
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {

			return httpInternalErrorStatus, fmt.Errorf("[%d - Http Post ProtoBuf Error] Unmarshal ProtoBuf Response Failed: %w", httpInternalErrorStatus, err)
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
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout)
	// marshal proto message to bytes
	if requestProtoBufObjectPtr == nil {

		return 0, errors.New("Request ProtoBuf Object is Nil")
	}

	reqBytes, err2 := proto.Marshal(requestProtoBufObjectPtr)

	if err2 != nil {

		return 0, fmt.Errorf("Request ProtoBuf Object Marshaling Failed: %w", err2)
	}

	// create http request from client
	req, err3 := http.NewRequest("PUT", url, bytes.NewReader(reqBytes))

	if err3 != nil {

		return 0, fmt.Errorf("Create New Http PUT ProtoBuf Request Failed: %w", err3)
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

		return httpInternalErrorStatus, fmt.Errorf("[%d - Http Put ProtoBuf Error] %w", httpInternalErrorStatus, err)
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if closeErr := resp.Body.Close(); closeErr != nil {
		log.Printf("rest.PUTProtoBuf: resp.Body.Close error: %v", closeErr)
	}

	if err != nil {
		// when read error, even if 200, still return error

		return statusCode, fmt.Errorf("reading response body: %w", err)
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {
		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Put ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal response bytes into protobuf object message
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {

			return httpInternalErrorStatus, fmt.Errorf("[%d - Http Put ProtoBuf Error] Unmarshal ProtoBuf Response Failed: %w", httpInternalErrorStatus, err)
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
	mu.RUnlock()

	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout // default timeout 30 seconds
	} else if timeout > maxHTTPClientTimeout {
		timeout = maxHTTPClientTimeout // max timeout 5 minutes
	}

	// create http client
	client := newHttpClient(timeout)
	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("DELETE", url, nil); err != nil {

		return 0, fmt.Errorf("Create New Http Delete ProtoBuf Request Failed: %w", err)
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

		return httpInternalErrorStatus, fmt.Errorf("[%d - Http Delete ProtoBuf Error] %w", httpInternalErrorStatus, err)
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if closeErr := resp.Body.Close(); closeErr != nil {
		log.Printf("rest.DELETEProtoBuf: resp.Body.Close error: %v", closeErr)
	}

	if err != nil {
		// when read error, even if 200, still return error

		return statusCode, fmt.Errorf("reading response body: %w", err)
	}

	if statusCode < httpSuccessStatusMin || statusCode >= httpErrorStatusMin {

		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Delete ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal bytes to protobuf object
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {

			return httpInternalErrorStatus, fmt.Errorf("[%d - Http Delete ProtoBuf Error] Unmarshal ProtoBuf Response Failed: %w", httpInternalErrorStatus, err)
		}
	}

	// success if outResponseProtoBufObjectPtr is not nil
	if outResponseProtoBufObjectPtr != nil {
		return statusCode, nil
	} else {
		return httpInternalErrorStatus, errors.New("[" + strconv.Itoa(httpInternalErrorStatus) + " - Http Delete ProtoBuf Error] Expected ProtoBuf Response Object Nil")
	}
}
