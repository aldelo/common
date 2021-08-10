package helper

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
	"encoding/json"
	"fmt"
	"github.com/aldelo/common/rest"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
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
	return strings.ToLower(Left(url, 8)) == "https://"
}

// GetLocalIP returns the first non loopback ip
func GetLocalIP() string {
	if addrs, err := net.InterfaceAddrs(); err != nil {
		return ""
	} else {
		for _, a := range addrs {
			if ip, ok := a.(*net.IPNet); ok && !ip.IP.IsLoopback() && !ip.IP.IsInterfaceLocalMulticast() && !ip.IP.IsLinkLocalMulticast() && !ip.IP.IsLinkLocalUnicast() && !ip.IP.IsMulticast() && !ip.IP.IsUnspecified() {
				if ip.IP.To4() != nil {
					return ip.IP.String()
				}
			}
		}

		return ""
	}
}

// DnsLookupIps returns list of IPs for the given host
// if host is private on aws route 53, then lookup ip will work only when within given aws vpc that host was registered with
func DnsLookupIps(host string) (ipList []net.IP) {
	if ips, err := net.LookupIP(host); err != nil {
		return []net.IP{}
	} else {
		for _, ip := range ips {
			ipList = append(ipList, ip)
		}
		return ipList
	}
}

// DnsLookupSrvs returns list of IP and port addresses based on host
// if host is private on aws route 53, then lookup ip will work only when within given aws vpc that host was registered with
func DnsLookupSrvs(host string) (ipList []string) {
	if _, addrs, err := net.LookupSRV("", "", host); err != nil {
		return []string{}
	} else {
		for _, v := range addrs {
			ipList = append(ipList, fmt.Sprintf("%s:%d", v.Target, v.Port))
		}

		return ipList
	}
}

// ParseHostFromURL will parse out the host name from url
func ParseHostFromURL(url string) string {
	parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(url), "https://", ""), "http://", ""), "/")

	if len(parts) >= 0 {
		return parts[0]
	} else {
		return ""
	}
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

	if statusCode, responseBody, e := rest.POST(u, []*rest.HeaderKeyValue{}, ""); e != nil {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Failed: %s", e)
	} else {
		if statusCode != 200 {
			return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Failed: Status Code %d", statusCode)
		} else {
			m := make(map[string]json.RawMessage)
			if err = json.Unmarshal([]byte(responseBody), &m); err != nil {
				return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Response Failed: (Parse Json Response Error) %s", err)
			} else {
				if m == nil {
					return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Response Failed: %s", "Json Response Map Nil")
				} else {
					// response json from google is valid
					if strings.ToLower(string(m["success"])) == "true" {
						success = true
					}

					challengeTs = ParseDateTime(string(m["challenge_ts"]))
					hostName = string(m["hostname"])

					if !success {
						errs := string(m["error-codes"])
						s := []string{}

						if err = json.Unmarshal([]byte(errs), &s); err != nil {
							err = fmt.Errorf("Parse ReCAPTCHA Verify Errors Failed: %s", err)
						} else {
							buf := ""

							for _, v := range s {
								if LenTrim(v) > 0 {
									if LenTrim(buf) > 0 {
										buf += ", "
									}

									buf += v
								}
							}

							err = fmt.Errorf("ReCAPTCHA Verify Errors: %s", buf)
						}
					}

					return success, challengeTs, hostName, err
				}
			}
		}
	}
}

// ReadHttpRequestBody reads raw body from http request body object,
// and then sets the read body back to the request (once reading will remove the body content if not restored)
func ReadHttpRequestBody(req *http.Request) ([]byte, error) {
	if req == nil {
		return []byte{}, fmt.Errorf("Http Request Nil")
	}

	if body, err := ioutil.ReadAll(req.Body); err != nil {
		return []byte{}, err
	} else {
		req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		return body, nil
	}
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

