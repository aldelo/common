package aws

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
	helper "github.com/aldelo/common"
	"golang.org/x/net/http2"
	"net"
	"net/http"
	"time"
)

// settings based on aws documentation
type HttpClientSettings struct {
	// Dialer.KeepAlive: negative value disables keep-alive; 0 enables keep-alive
	// default = keep-alive enabled
	ConnKeepAlive *time.Duration

	// Dialer.Timeout: maximum amount of time a dial will wait for a connection to be created
	// default = 30 seconds
	Connect *time.Duration

	// Transport.ExpectContinueTimeout: maximum amount of time to wait for a server's first response headers after fully writing the request headers,
	// 		if the response has an "Expect: 100-continue" header,
	//		this time does not include the time to send the request header,
	//		the http client sends its payload after this timeout is exhausted
	// default = 1 second; set to 0 for no timeout and send request payload without waiting
	ExpectContinue *time.Duration

	// Transport.IdleConnTimeout: maximum amount of time to keep an idle network connection alive between http requests
	// default = 0 for no limit
	IdleConn *time.Duration

	// Transport.MaxIdleConns: maximum number of idle (keep-alive) connections across all hosts,
	// 		use case: increasing this value when many connections in a short period from same clients
	// default = 0 means no limit
	MaxAllIdleConns *int

	// Transport.MaxIdleConnsPerHost: maximum number of idle (keep-alive) connections to keep per-host,
	//		use case: increasing this value when many connections in a short period from same clients
	// default = 0 means 2 idle connections per host
	MaxHostIdleConns *int

	// Transport.ResponseHeaderTimeout: maximum amount of time to wait for a client to read the response header,
	//		if the client isn't able to read the response's header within this duration, the request fails with a timeout error,
	//		warning: when using long-running lambda functions, as the operation does not return any response headers until the lambda has finished or timeout
	// default = no timeout, waits forever
	ResponseHeader *time.Duration

	// Transport.TLSHandshakeTimeout: maximum amount of time waiting for a TLS handshake to be completed
	// default = 10 seconds; 0 means no timeout
	TlsHandshake *time.Duration
}

type AwsHttp2Client struct {
	Options *HttpClientSettings
}

func (h2 *AwsHttp2Client) setDefaults() {
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}

	if h2.Options.ConnKeepAlive == nil {
		h2.Options.ConnKeepAlive = helper.DurationPtr(0)
	}

	if h2.Options.Connect == nil {
		h2.Options.Connect = helper.DurationPtr(5 * time.Second)
	}

	if h2.Options.ExpectContinue == nil {
		h2.Options.ExpectContinue = helper.DurationPtr(1 * time.Second)
	}

	if h2.Options.IdleConn == nil {
		h2.Options.IdleConn = helper.DurationPtr(300 * time.Second)
	}

	if h2.Options.MaxAllIdleConns == nil {
		h2.Options.MaxAllIdleConns = helper.IntPtr(0)
	}

	if h2.Options.MaxHostIdleConns == nil {
		h2.Options.MaxHostIdleConns = helper.IntPtr(100)
	}

	if h2.Options.ResponseHeader == nil {
		h2.Options.ResponseHeader = helper.DurationPtr(5 * time.Second)
	}

	if h2.Options.TlsHandshake == nil {
		h2.Options.TlsHandshake = helper.DurationPtr(5 * time.Second)
	}
}

// Dialer.KeepAlive: negative value disables keep-alive; 0 enables keep-alive
// default = keep-alive enabled
func (h2 *AwsHttp2Client) ConnKeepAlive(v time.Duration) {
	h2.Options.ConnKeepAlive = &v
}

// Dialer.Timeout: maximum amount of time a dial will wait for a connection to be created
// default = 30 seconds
func (h2 *AwsHttp2Client) ConnectTimeout(v time.Duration) {
	h2.Options.Connect = &v
}

// Transport.ExpectContinueTimeout: maximum amount of time to wait for a server's first response headers after fully writing the request headers,
// 		if the response has an "Expect: 100-continue" header,
//		this time does not include the time to send the request header,
//		the http client sends its payload after this timeout is exhausted
// default = 1 second; set to 0 for no timeout and send request payload without waiting
func (h2 *AwsHttp2Client) ExpectContinueTimeout(v time.Duration) {
	h2.Options.ExpectContinue = &v
}

// Transport.IdleConnTimeout: maximum amount of time to keep an idle network connection alive between http requests
// default = 0 for no limit
func (h2 *AwsHttp2Client) IdleConnTimeout(v time.Duration) {
	h2.Options.IdleConn = &v
}

// Transport.MaxIdleConns: maximum number of idle (keep-alive) connections across all hosts,
// 		use case: increasing this value when many connections in a short period from same clients
// default = 0 means no limit
func (h2 *AwsHttp2Client) MaxAllIdleConns(v int) {
	h2.Options.MaxAllIdleConns = &v
}

// Transport.MaxIdleConnsPerHost: maximum number of idle (keep-alive) connections to keep per-host,
//		use case: increasing this value when many connections in a short period from same clients
// default = 0 means 2 idle connections per host
func (h2 *AwsHttp2Client) MaxHostIdelConns(v int) {
	h2.Options.MaxHostIdleConns = &v
}

// Transport.ResponseHeaderTimeout: maximum amount of time to wait for a client to read the response header,
//		if the client isn't able to read the response's header within this duration, the request fails with a timeout error,
//		warning: when using long-running lambda functions, as the operation does not return any response headers until the lambda has finished or timeout
// default = no timeout, waits forever
func (h2 *AwsHttp2Client) ResponseHeaderTimeout(v time.Duration) {
	h2.Options.ResponseHeader = &v
}

// Transport.TLSHandshakeTimeout: maximum amount of time waiting for a TLS handshake to be completed
// default = 10 seconds; 0 means no timeout
func (h2 *AwsHttp2Client) TlsHandshakeTimeout(v time.Duration) {
	h2.Options.TlsHandshake = &v
}

// NewHttp2Client returns custom http2 client for aws connection
func (h2 *AwsHttp2Client) NewHttp2Client() (*http.Client, error) {
	h2.setDefaults()

	var client http.Client

	tr := &http.Transport{
		ResponseHeaderTimeout: *h2.Options.ResponseHeader,
		Proxy:                 http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			KeepAlive: *h2.Options.ConnKeepAlive,
			Timeout:   *h2.Options.Connect,
		}).DialContext,
		MaxIdleConns:          *h2.Options.MaxAllIdleConns,
		IdleConnTimeout:       *h2.Options.IdleConn,
		TLSHandshakeTimeout:   *h2.Options.TlsHandshake,
		MaxIdleConnsPerHost:   *h2.Options.MaxHostIdleConns,
		ExpectContinueTimeout: *h2.Options.ExpectContinue,
	}

	// client makes HTTP/2 requests
	err := http2.ConfigureTransport(tr)

	if err != nil {
		return &client, err
	}

	return &http.Client{
		Transport: tr,
	}, nil
}
