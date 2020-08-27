package rest

/*
 * Copyright 2020 Aldelo, LP
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
	util "github.com/aldelo/common"
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"
	"google.golang.org/protobuf/proto"
)

//
// HeaderKeyValue is struct used for containing http header element key value pair
//
type HeaderKeyValue struct {
	Key string
	Value string
}

//
// GET sends url get request to host and retrieve the body response in string
//
func GET(url string, headers []*HeaderKeyValue) (statusCode int, body string, err error) {
	// create http client
	client := &http.Client{}

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
		return 404, "", errors.New("[404 - Http Get Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil && statusCode == 400 {
		return statusCode, "", err
	}

	if statusCode != 200 {
		return statusCode, "", errors.New("[" + util.Itoa(statusCode) + " - Get Resp] " + string(respBytes))
	}

	// success
	return statusCode, string(respBytes), nil
}

//
// POST sends url post request to host with simple form data and retrieve the body response in string
//
func POST(url string, headers []*HeaderKeyValue, requestBody string) (statusCode int, responseBody string, err error) {
	// create http client
	client := &http.Client{}

	// create http request from client
	var req *http.Request

	if req, err = http.NewRequest("POST", url, bytes.NewBuffer([]byte(requestBody))); err != nil {
		return 0, "", errors.New("Create New Http Post Request Failed: " + err.Error())
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
		return 404, "", errors.New("[404 - Http Post Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil && statusCode == 400 {
		return statusCode, "", err
	}

	if statusCode != 200 {
		return statusCode, "", errors.New("[" + util.Itoa(statusCode) + " - Post Resp] " + string(respBytes))
	}

	return statusCode, string(respBytes), nil
}

//
// GETProtoBuf sends url get request to host, and retrieves response via protobuf object as an output pointer parameter
//
// Note:
//		outResponseProtoBufObjectPtr: if err == nil, then this output parameter is guaranteed to be Not Nil
//
func GETProtoBuf(url string, headers []*HeaderKeyValue, outResponseProtoBufObjectPtr proto.Message) (statusCode int, err error) {
	// create http client
	client := &http.Client{}

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
		return 404, errors.New("[404 - Http Get ProtoBuf Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil && statusCode == 400 {
		outResponseProtoBufObjectPtr = nil
		return statusCode, err
	}

	if statusCode != 200 {
		outResponseProtoBufObjectPtr = nil
		return statusCode, errors.New("[" + util.Itoa(statusCode) + " - Get ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + util.Itoa(len(respBytes)))
	}

	// unmarshal bytes to protobuf object
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {
			outResponseProtoBufObjectPtr = nil
			return 404, errors.New("[404 - Http Get ProtoBuf Error] Unmarshal ProtoBuf Response Failed: " + err.Error())
		}
	}

	// success if outResponseProtoBufObjectPtr is not nil
	if outResponseProtoBufObjectPtr != nil {
		return statusCode, nil
	} else {
		return 404, errors.New("[404 - Http Get ProtoBuf Error] Expected ProtoBuf Response Object Nil")
	}
}

//
// POSTProtoBuf sends url post request to host, with body content in protobuf pointer object,
// and retrieves response in protobuf object as output pointer parameter
//
// Note:
//		outResponseProtoBufObjectPtr: if err == nil, then this output parameter is guaranteed to be Not Nil
//
func POSTProtoBuf(url string, headers []*HeaderKeyValue, requestProtoBufObjectPtr proto.Message, outResponseProtoBufObjectPtr proto.Message) (statusCode int, err error) {
	// create http client
	client := &http.Client{}

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
		return 404, errors.New("[404 - Http Post ProtoBuf Error] " + err.Error())
	}

	// evaluate response
	statusCode = resp.StatusCode

	var respBytes []byte

	respBytes, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	resp.Close = true

	// clean up stale connections
	client.CloseIdleConnections()

	if err != nil && statusCode == 400 {
		outResponseProtoBufObjectPtr = nil
		return statusCode, err
	}

	if statusCode != 200 {
		return statusCode, errors.New("[" + util.Itoa(statusCode) + " - Post ProtoBuf Not 200] Response ProtoBuf Bytes Length = " + util.Itoa(len(respBytes)))
	}

	// unmarshal response bytes into protobuf object message
	if outResponseProtoBufObjectPtr != nil {
		if err = proto.Unmarshal(respBytes, outResponseProtoBufObjectPtr); err != nil {
			outResponseProtoBufObjectPtr = nil
			return 404, errors.New("[404 - Http Post ProtoBuf Error] Unmarshal ProtoBuf Response Failed: " + err.Error())
		}
	}

	// success if outResponseProtoBufObjectPtr is not nil
	if outResponseProtoBufObjectPtr != nil {
		return statusCode, nil
	} else {
		return 404, errors.New("[404 - Http Post ProtoBuf Error] Expected ProtoBuf Response Object Nil")
	}
}
