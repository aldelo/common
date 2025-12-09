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

var mu sync.RWMutex

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

// HeaderKeyValue is struct used for containing http header element key value pair
type HeaderKeyValue struct {
	Key   string
	Value string
}

// GET sends url get request to host and retrieve the body response in string
func GET(url string, headers []*HeaderKeyValue) (statusCode int, body string, err error) {
	mu.RLock()
	timeout := clientTimeoutSeconds
	tls := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = 30 // default timeout 30 seconds
	} else if timeout > 300 {
		timeout = 300 // max timeout 5 minutes
	}

	// create http client
	var client *http.Client

	if tls == nil {
		client = &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		}
	} else {
		tr := &http.Transport{
			TLSClientConfig: tls,
		}

		client = &http.Client{
			Transport: tr,
			Timeout:   time.Duration(timeout) * time.Second,
		}
	}

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
		return 500, "", errors.New("[500 - Http Get Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", err
	}

	if statusCode < 200 || statusCode >= 300 {
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
	tls := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = 30 // default timeout 30 seconds
	} else if timeout > 300 {
		timeout = 300 // max timeout 5 minutes
	}

	// create http client
	var client *http.Client

	if tls == nil {
		client = &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		}
	} else {
		tr := &http.Transport{
			TLSClientConfig: tls,
		}

		client = &http.Client{
			Transport: tr,
			Timeout:   time.Duration(timeout) * time.Second,
		}
	}

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
		return 500, "", errors.New("[500 - Http Post Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", err
	}

	if statusCode < 200 || statusCode >= 300 {
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
	tls := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = 30 // default timeout 30 seconds
	} else if timeout > 300 {
		timeout = 300 // max timeout 5 minutes
	}

	// create http client
	var client *http.Client

	if tls == nil {
		client = &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		}
	} else {
		tr := &http.Transport{
			TLSClientConfig: tls,
		}

		client = &http.Client{
			Transport: tr,
			Timeout:   time.Duration(timeout) * time.Second,
		}
	}

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
		return 500, "", errors.New("[500 - Http Put Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", err
	}

	if statusCode < 200 || statusCode >= 300 {
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
	tls := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = 30 // default timeout 30 seconds
	} else if timeout > 300 {
		timeout = 300 // max timeout 5 minutes
	}

	// create http client
	var client *http.Client

	if tls == nil {
		client = &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		}
	} else {
		tr := &http.Transport{
			TLSClientConfig: tls,
		}

		client = &http.Client{
			Transport: tr,
			Timeout:   time.Duration(timeout) * time.Second,
		}
	}

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
		return 500, "", errors.New("[500 - Http Delete Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		return statusCode, "", err
	}

	if statusCode < 200 || statusCode >= 300 {
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
	tls := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = 30 // default timeout 30 seconds
	} else if timeout > 300 {
		timeout = 300 // max timeout 5 minutes
	}

	// create http client
	var client *http.Client

	if tls == nil {
		client = &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		}
	} else {
		tr := &http.Transport{
			TLSClientConfig: tls,
		}

		client = &http.Client{
			Transport: tr,
			Timeout:   time.Duration(timeout) * time.Second,
		}
	}

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
		return 500, errors.New("[500 - Http Get ProtoBuf Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		outResponseProtoBufObjectPtr = nil
		return statusCode, err
	}

	if statusCode < 200 || statusCode >= 300 {
		outResponseProtoBufObjectPtr = nil
		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Get ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal bytes to protobuf object
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {
			outResponseProtoBufObjectPtr = nil
			return 500, errors.New("[500 - Http Get ProtoBuf Error] Unmarshal ProtoBuf Response Failed: " + err.Error())
		}
	}

	// success if outResponseProtoBufObjectPtr is not nil
	if outResponseProtoBufObjectPtr != nil {
		return statusCode, nil
	} else {
		return 500, errors.New("[500 - Http Get ProtoBuf Error] Expected ProtoBuf Response Object Nil")
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
	tls := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = 30 // default timeout 30 seconds
	} else if timeout > 300 {
		timeout = 300 // max timeout 5 minutes
	}

	// create http client
	var client *http.Client

	if tls == nil {
		client = &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		}
	} else {
		tr := &http.Transport{
			TLSClientConfig: tls,
		}

		client = &http.Client{
			Transport: tr,
			Timeout:   time.Duration(timeout) * time.Second,
		}
	}

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
		return 500, errors.New("[500 - Http Post ProtoBuf Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		outResponseProtoBufObjectPtr = nil
		return statusCode, err
	}

	if statusCode < 200 || statusCode >= 300 {
		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Post ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal response bytes into protobuf object message
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {
			outResponseProtoBufObjectPtr = nil
			return 500, errors.New("[500 - Http Post ProtoBuf Error] Unmarshal ProtoBuf Response Failed: " + err.Error())
		}
	}

	// success if outResponseProtoBufObjectPtr is not nil
	if outResponseProtoBufObjectPtr != nil {
		return statusCode, nil
	} else {
		return 500, errors.New("[500 - Http Post ProtoBuf Error] Expected ProtoBuf Response Object Nil")
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
	tls := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = 30 // default timeout 30 seconds
	} else if timeout > 300 {
		timeout = 300 // max timeout 5 minutes
	}

	// create http client
	var client *http.Client

	if tls == nil {
		client = &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		}
	} else {
		tr := &http.Transport{
			TLSClientConfig: tls,
		}

		client = &http.Client{
			Transport: tr,
			Timeout:   time.Duration(timeout) * time.Second,
		}
	}

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
		return 500, errors.New("[500 - Http Put ProtoBuf Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		outResponseProtoBufObjectPtr = nil
		return statusCode, err
	}

	if statusCode < 200 || statusCode >= 300 {
		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Put ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal response bytes into protobuf object message
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {
			outResponseProtoBufObjectPtr = nil
			return 500, errors.New("[500 - Http Put ProtoBuf Error] Unmarshal ProtoBuf Response Failed: " + err.Error())
		}
	}

	// success if outResponseProtoBufObjectPtr is not nil
	if outResponseProtoBufObjectPtr != nil {
		return statusCode, nil
	} else {
		return 500, errors.New("[500 - Http Put ProtoBuf Error] Expected ProtoBuf Response Object Nil")
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
	tls := clientTlsConfig
	mu.RUnlock()

	if timeout <= 0 {
		timeout = 30 // default timeout 30 seconds
	} else if timeout > 300 {
		timeout = 300 // max timeout 5 minutes
	}

	// create http client
	var client *http.Client

	if tls == nil {
		client = &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		}
	} else {
		tr := &http.Transport{
			TLSClientConfig: tls,
		}

		client = &http.Client{
			Transport: tr,
			Timeout:   time.Duration(timeout) * time.Second,
		}
	}

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
		return 500, errors.New("[500 - Http Delete ProtoBuf Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil {
		// when read error, even if 200, still return error
		outResponseProtoBufObjectPtr = nil
		return statusCode, err
	}

	if statusCode < 200 || statusCode >= 300 {
		outResponseProtoBufObjectPtr = nil
		return statusCode, errors.New("[" + strconv.Itoa(statusCode) + " - Delete ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + strconv.Itoa(len(respBytes)))
	}

	// unmarshal bytes to protobuf object
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {
			outResponseProtoBufObjectPtr = nil
			return 500, errors.New("[500 - Http Delete ProtoBuf Error] Unmarshal ProtoBuf Response Failed: " + err.Error())
		}
	}

	// success if outResponseProtoBufObjectPtr is not nil
	if outResponseProtoBufObjectPtr != nil {
		return statusCode, nil
	} else {
		return 500, errors.New("[500 - Http Delete ProtoBuf Error] Expected ProtoBuf Response Object Nil")
	}
}
