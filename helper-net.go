package helper

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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aldelo/common/rest"
)

// GetNetListener triggers the specified port to listen via tcp
func GetNetListener(port uint) (net.Listener, error) {
	if l, e := net.Listen("tcp", fmt.Sprintf(":%d", port)); e != nil {
		return nil, fmt.Errorf("Listen Tcp on Port %d Failed: %v", port, e)
	} else {
		return l, nil
	}
}

// IsHttpsEndpoint returns true if url is https, false if otherwise
func IsHttpsEndpoint(url string) bool {
	return strings.HasPrefix(strings.ToLower(url), "https://")
}

// GetLocalIP returns the first non loopback ip
func GetLocalIP() string {
	// walk interfaces to ensure we only return IPv4, global-unicast addresses on UP/non-loopback interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			if ipNet.IP.IsLoopback() || ipNet.IP.IsUnspecified() || !ipNet.IP.IsGlobalUnicast() {
				continue
			}
			if v4 := ipNet.IP.To4(); v4 != nil {
				return v4.String()
			}
		}
	}

	return ""
}

// DnsLookupIps returns list of IPs for the given host
// if host is private on aws route 53, then lookup ip will work only when within given aws vpc that host was registered with
func DnsLookupIps(host string) (ipList []net.IP) {
	if ips, err := net.LookupIP(host); err != nil {
		log.Printf("DNS Lookup IP Failed for Host: %v, %s", err, host)
		return []net.IP{}
	} else {
		for _, ip := range ips {
			ipList = append(ipList, ip)
		}

		if len(ipList) == 0 {
			log.Printf("DNS Lookup IP Returned No Results for Host: %s", host)
		}

		log.Printf("DNS Lookup IP Returned %v for Host: %s", ipList, host)
		return ipList
	}
}

// DnsLookupSrvs returns list of IP and port addresses based on host
// if host is private on aws route 53, then lookup ip will work only when within given aws vpc that host was registered with
func DnsLookupSrvs(host string) (ipList []string) {
	if _, addrs, err := net.LookupSRV("", "", host); err != nil {
		log.Printf("DNS Lookup SRV Failed for Host: %v, %s", err, host)
		return []string{}
	} else {
		for _, v := range addrs {
			target := strings.TrimSuffix(v.Target, ".")
			ipList = append(ipList, fmt.Sprintf("%s:%d", target, v.Port))
		}

		if len(ipList) == 0 {
			log.Printf("DNS Lookup SRV Returned No Results for Host: %s", host)
		}

		log.Printf("DNS Lookup SRV Returned %v for Host: %s", ipList, host)
		return ipList
	}
}

// ParseHostFromURL will parse out the host name from url
func ParseHostFromURL(u string) string {
	trimmed := strings.TrimSpace(u) // normalize input early
	if trimmed == "" {              // guard empty input
		return ""
	}
	if !strings.Contains(trimmed, "://") { // allow bare hosts without scheme
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed) // robust URL parsing
	if err != nil {
		return ""
	}
	return parsed.Hostname() // use standard hostname extraction (handles ports, schemes)
}

// VerifyGoogleReCAPTCHAv2 will verify recaptcha v2 response data against given secret and obtain a response from google server
func VerifyGoogleReCAPTCHAv2(response string, secret string) (success bool, challengeTs time.Time, hostName string, err error) {
	if LenTrim(response) == 0 {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Response From CLient is Required")
	}

	if LenTrim(secret) == 0 {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Secret Key is Required")
	}

	u := fmt.Sprintf("https://www.google.com/recaptcha/api/siteverify?secret=%s&response=%s", url.PathEscape(secret), url.PathEscape(response))

	statusCode, responseBody, e := rest.POST(u, []*rest.HeaderKeyValue{}, "")
	if e != nil {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Failed: %s", e)
	}
	if statusCode != http.StatusOK {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Failed: Status Code %d, Body: %s", statusCode, responseBody)
	}

	// strongly typed JSON parsing
	type recaptchaResponse struct {
		Success     bool     `json:"success"`
		ChallengeTS string   `json:"challenge_ts"`
		Hostname    string   `json:"hostname"`
		ErrorCodes  []string `json:"error-codes"`
	}

	var resp recaptchaResponse
	if err = json.Unmarshal([]byte(responseBody), &resp); err != nil {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Response Failed: (Parse Json Response Error) %s", err)
	}

	// populate outputs from typed fields
	success = resp.Success
	hostName = resp.Hostname

	if LenTrim(resp.ChallengeTS) > 0 {
		// Google returns RFC3339 timestamps
		if ts, parseErr := time.Parse(time.RFC3339, resp.ChallengeTS); parseErr == nil {
			challengeTs = ts
		} else {
			err = fmt.Errorf("ReCAPTCHA Service Response Failed: (Parse challenge_ts Error) %s", parseErr)
			return success, challengeTs, hostName, err
		}
	}

	if !success {
		if len(resp.ErrorCodes) > 0 {
			err = fmt.Errorf("ReCAPTCHA Verify Errors: %s", strings.Join(resp.ErrorCodes, ", "))
		} else {
			err = fmt.Errorf("ReCAPTCHA Verify Errors: unknown error")
		}
	}

	return success, challengeTs, hostName, err
}

// ReadHttpRequestBody reads raw body from http request body object,
// and then sets the read body back to the request (once reading will remove the body content if not restored)
func ReadHttpRequestBody(req *http.Request) ([]byte, error) {
	if req == nil {
		return []byte{}, fmt.Errorf("Http Request Nil")
	}
	if req.Body == nil { // avoid nil body panic
		return []byte{}, fmt.Errorf("Http Request Body Nil")
	}

	body, err := io.ReadAll(req.Body)
	req.Body.Close()

	req.Body = io.NopCloser(bytes.NewBuffer(body))

	if err != nil {
		return body, err
	}

	return body, nil
}

// ReadHttpRequestHeaders parses headers into map
func ReadHttpRequestHeaders(req *http.Request) (map[string]string, error) {
	if req == nil {
		return nil, fmt.Errorf("Http Request Nil")
	}

	m := make(map[string]string)

	for k, v := range req.Header {
		buf := ""

		for _, d := range v {
			if LenTrim(buf) > 0 {
				buf += "; "
			}

			buf += d
		}

		m[k] = buf
	}

	return m, nil
}

// ParseHttpHeader parses headers into map[string]string,
// where slice of header values for a key is delimited with semi-colon (;)
func ParseHttpHeader(respHeader http.Header) (map[string]string, error) {
	if respHeader == nil {
		return nil, fmt.Errorf("Http Response Header Nil")
	}

	m := make(map[string]string)

	for k, v := range respHeader {
		buf := ""

		for _, d := range v {
			if LenTrim(buf) > 0 {
				buf += "; "
			}

			buf += d
		}

		m[k] = buf
	}

	return m, nil
}

// EncodeHttpHeaderMapToString converts header map[string]string to string representation
func EncodeHttpHeaderMapToString(headerMap map[string]string) string {
	if headerMap == nil {
		return ""
	} else {
		buf := ""

		for k, v := range headerMap {
			buf += fmt.Sprintf("%s: %s\n", k, v)
		}

		return buf
	}
}
