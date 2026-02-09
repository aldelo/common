package aws

/*
 * Copyright 2020-2025 Aldelo, LP
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
	"errors"
	"net"
	"net/http"
	"time"

	helper "github.com/aldelo/common"
	"golang.org/x/net/http2"
)

// HttpClientSettings based on aws documentation
type HttpClientSettings struct {
	// Dialer.KeepAlive: negative value disables keep-alive; 0 enables keep-alive
	// default = keep-alive enabled
	DialContextKeepAlive *time.Duration

	// Dialer.Timeout: maximum amount of time a dial will wait for a connection to be created
	// default = 10 seconds
	DialContextConnectTimeout *time.Duration

	// Transport.ExpectContinueTimeout: maximum amount of time to wait for a server's first response headers after fully writing the request headers,
	// 		if the response has an "Expect: 100-continue" header,
	//		this time does not include the time to send the request header,
	//		the http client sends its payload after this timeout is exhausted
	// default = 1 second; set to 0 for no timeout and send request payload without waiting
	ExpectContinueTimeout *time.Duration

	// Client.Timeout: overall timeout for http client including connection, request, response
	// default = 60 seconds
	OverallTimeout *time.Duration

	// -------

	// Transport.MaxIdleConns: maximum number of idle (keep-alive) connections across all hosts,
	// 		use case: increasing this value when many connections in a short period from same clients
	// default = 0 means no limit
	MaxIdleConns *int

	// Transport.MaxIdleConnsPerHost: maximum number of idle (keep-alive) connections to keep per-host,
	//		use case: increasing this value when many connections in a short period from same clients
	// default = 0 means 2 idle connections per host
	// suggest setting to 250 or higher for aws sdk usage
	// use 500 as default here
	MaxIdleConnsPerHost *int

	// Transport.MaxConnsPerHost: limits the total number of connections per host, including connections in the dialing, active, and idle states
	// default = 0 means no limit
	MaxConnsPerHost *int

	// Transport.IdleConnTimeout: maximum amount of time to keep an idle network connection alive between http requests
	// default = 0 for no limit
	// use 300 seconds (5 minutes) as default here
	IdleConnTimeout *time.Duration

	DisableKeepAlive   *bool
	DisableCompression *bool

	// Transport.TLSHandshakeTimeout: maximum amount of time waiting for a TLS handshake to be completed
	// default = 10 seconds; 0 means no timeout
	TlsHandshakeTimeout *time.Duration

	// Transport.ResponseHeaderTimeout: maximum amount of time to wait for a client to read the response header,
	//		if the client isn't able to read the response's header within this duration, the request fails with a timeout error,
	//		warning: when using long-running lambda functions, as the operation does not return any response headers until the lambda has finished or timeout
	// default = no timeout, waits forever
	// use 30 seconds as default here
	ResponseHeaderTimeout *time.Duration
}

// AwsHttp2Client struct defines container for HttpClientSettings
type AwsHttp2Client struct {
	Options *HttpClientSettings
}

func (h2 *AwsHttp2Client) setDefaults() {
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}

	if h2.Options.DialContextKeepAlive == nil {
		h2.Options.DialContextKeepAlive = helper.DurationPtr(0)
	}

	if h2.Options.DialContextConnectTimeout == nil {
		h2.Options.DialContextConnectTimeout = helper.DurationPtr(10 * time.Second)
	}

	if h2.Options.ExpectContinueTimeout == nil {
		h2.Options.ExpectContinueTimeout = helper.DurationPtr(1 * time.Second)
	}

	if h2.Options.OverallTimeout == nil {
		h2.Options.OverallTimeout = helper.DurationPtr(60 * time.Second)
	}

	// --------

	if h2.Options.MaxIdleConns == nil {
		h2.Options.MaxIdleConns = helper.IntPtr(0)
	}

	if h2.Options.MaxIdleConnsPerHost == nil {
		h2.Options.MaxIdleConnsPerHost = helper.IntPtr(500)
	}

	if h2.Options.MaxConnsPerHost == nil {
		h2.Options.MaxConnsPerHost = helper.IntPtr(0)
	}

	if h2.Options.IdleConnTimeout == nil {
		h2.Options.IdleConnTimeout = helper.DurationPtr(300 * time.Second)
	}

	if h2.Options.DisableKeepAlive == nil {
		h2.Options.DisableKeepAlive = helper.BoolPtr(false)
	}

	if h2.Options.DisableCompression == nil {
		h2.Options.DisableCompression = helper.BoolPtr(false)
	}

	if h2.Options.TlsHandshakeTimeout == nil {
		h2.Options.TlsHandshakeTimeout = helper.DurationPtr(10 * time.Second)
	}

	if h2.Options.ResponseHeaderTimeout == nil {
		h2.Options.ResponseHeaderTimeout = helper.DurationPtr(30 * time.Second)
	}
}

// SetDialContextKeepAlive sets Dialer.KeepAlive: negative value disables keep-alive; 0 enables keep-alive
// default = keep-alive enabled = 0
func (h2 *AwsHttp2Client) SetDialContextKeepAlive(v time.Duration) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.DialContextKeepAlive = &v
}

// SetDialContextConnectTimeout sets Dialer.Timeout: maximum amount of time a dial will wait for a connection to be created
// default = 10 seconds
func (h2 *AwsHttp2Client) SetDialContextConnectTimeout(v time.Duration) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.DialContextConnectTimeout = &v
}

// SetExpectContinueTimeout sets Transport.ExpectContinueTimeout: maximum amount of time to wait for a server's first response headers after fully writing the request headers,
//
//	if the response has an "Expect: 100-continue" header,
//	this time does not include the time to send the request header,
//	the http client sends its payload after this timeout is exhausted
//
// default = 1 second; set to 0 for no timeout and send request payload without waiting
func (h2 *AwsHttp2Client) SetExpectContinueTimeout(v time.Duration) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.ExpectContinueTimeout = &v
}

// SetOverallTimeout sets Client.Timeout: overall timeout for http client including connection, request, response
// default = 60 seconds
func (h2 *AwsHttp2Client) SetOverallTimeout(v time.Duration) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.OverallTimeout = &v
}

// SetMaxIdleConns sets Transport.MaxIdleConns: maximum number of idle (keep-alive) connections across all hosts,
//
//	use case: increasing this value when many connections in a short period from same clients
//
// default = 0 means no limit
func (h2 *AwsHttp2Client) SetMaxIdleConns(v int) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.MaxIdleConns = &v
}

// SetMaxIdleConnsPerHost sets Transport.MaxIdleConnsPerHost: maximum number of idle (keep-alive) connections to keep per-host,
//
//	use case: increasing this value when many connections in a short period from same clients
//
// default = 0 means 2 idle connections per host
// suggest setting to 250 or higher for aws sdk usage
// use 500 as default here
func (h2 *AwsHttp2Client) SetMaxIdleConnsPerHost(v int) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.MaxIdleConnsPerHost = &v
}

// SetMaxConnsPerHost sets Transport.MaxConnsPerHost: limits the total number of connections per host, including connections in the dialing, active, and idle states
// default = 0 means no limit
func (h2 *AwsHttp2Client) SetMaxConnsPerHost(v int) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.MaxConnsPerHost = &v
}

// SetIdleConnTimeout sets Transport.IdleConnTimeout: maximum amount of time to keep an idle network connection alive between http requests
// default = 0 for no limit
func (h2 *AwsHttp2Client) SetIdleConnTimeout(v time.Duration) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.IdleConnTimeout = &v
}

// SetDisableKeepAlives sets Transport.DisableKeepAlives: if true, prevents re-use of TCP connections between different HTTP requests
// default = false
func (h2 *AwsHttp2Client) SetDisableKeepAlives(v bool) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.DisableKeepAlive = &v
}

// SetDisableCompressions sets Transport.DisableCompression: if true, prevents the Transport from requesting compression with an "Accept-Encoding: gzip" request header when the Request contains no existing Accept-Encoding value.
// default = false
func (h2 *AwsHttp2Client) SetDisableCompressions(v bool) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.DisableCompression = &v
}

// SetTlsHandshakeTimeout sets Transport.TLSHandshakeTimeout: maximum amount of time waiting for a TLS handshake to be completed
// default = 10 seconds; 0 means no timeout
func (h2 *AwsHttp2Client) SetTlsHandshakeTimeout(v time.Duration) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.TlsHandshakeTimeout = &v
}

// SetResponseHeaderTimeout sets Transport.ResponseHeaderTimeout: maximum amount of time to wait for a client to read the response header,
//
//	if the client isn't able to read the response's header within this duration, the request fails with a timeout error,
//	warning: when using long-running lambda functions, as the operation does not return any response headers until the lambda has finished or timeout
//
// default = no timeout, waits forever
// use 30 seconds as default here
func (h2 *AwsHttp2Client) SetResponseHeaderTimeout(v time.Duration) {
	if h2 == nil {
		return
	}
	if h2.Options == nil {
		h2.Options = new(HttpClientSettings)
	}
	h2.Options.ResponseHeaderTimeout = &v
}

// NewHttp2Client returns custom http2 client for aws connection
func (h2 *AwsHttp2Client) NewHttp2Client() (*http.Client, error) {
	if h2 == nil {
		return nil, errors.New("AwsHttp2Client receiver is nil")
	}
	h2.setDefaults()

	var client http.Client

	tr := &http.Transport{
		MaxIdleConns:          *h2.Options.MaxIdleConns,
		MaxIdleConnsPerHost:   *h2.Options.MaxIdleConnsPerHost,
		MaxConnsPerHost:       *h2.Options.MaxConnsPerHost,
		IdleConnTimeout:       *h2.Options.IdleConnTimeout,
		DisableKeepAlives:     *h2.Options.DisableKeepAlive,
		DisableCompression:    *h2.Options.DisableCompression,
		TLSHandshakeTimeout:   *h2.Options.TlsHandshakeTimeout,
		ResponseHeaderTimeout: *h2.Options.ResponseHeaderTimeout,

		DialContext: (&net.Dialer{
			KeepAlive: *h2.Options.DialContextKeepAlive,
			Timeout:   *h2.Options.DialContextConnectTimeout,
		}).DialContext,

		ExpectContinueTimeout: *h2.Options.ExpectContinueTimeout,
		Proxy:                 http.ProxyFromEnvironment,
	}

	// client makes HTTP/2 requests
	err := http2.ConfigureTransport(tr)

	if err != nil {
		return &client, err
	}

	return &http.Client{
		Transport: tr,
		Timeout:   *h2.Options.OverallTimeout,
	}, nil
}
