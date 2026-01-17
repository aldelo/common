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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	dnsLookupTimeout       = 5 * time.Second // bounded DNS timeout
	recaptchaVerifyTimeout = 5 * time.Second // bounded ReCAPTCHA verification timeout
	maxReCaptchaBodyBytes  = 1 << 20         // cap google response body to 1mb
	maxRequestBodyBytes    = 10 << 20        // cap inbound http request body to 10mb
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
			switch a := addr.(type) {
			case *net.IPNet:
				if a == nil || a.IP == nil {
					continue
				}
				if a.IP.IsLoopback() || a.IP.IsUnspecified() || !a.IP.IsGlobalUnicast() {
					continue
				}
				if v4 := a.IP.To4(); v4 != nil {
					return v4.String()
				}
			case *net.IPAddr: // handle *net.IPAddr to cover platforms that return IPAddr
				if a == nil || a.IP == nil {
					continue
				}
				if a.IP.IsLoopback() || a.IP.IsUnspecified() || !a.IP.IsGlobalUnicast() {
					continue
				}
				if v4 := a.IP.To4(); v4 != nil {
					return v4.String()
				}
			}
		}
	}

	return ""
}

// DnsLookupIps returns list of IPs for the given host
// if host is private on aws route 53, then lookup ip will work only when within given aws vpc that host was registered with
func DnsLookupIps(host string) (ipList []net.IP) {
	ctx, cancel := context.WithTimeout(context.Background(), dnsLookupTimeout)
	defer cancel()

	target := ParseHostFromURL(host)
	if target == "" {
		log.Printf("DNS Lookup IP Failed for Host: invalid host '%s'", host)
		return []net.IP{}
	}

	// short-circuit IP literals (including zone-stripped IPv6) to avoid needless DNS and zone failures
	ipOnly := target
	if zoneIdx := strings.Index(target, "%"); zoneIdx != -1 {
		ipOnly = target[:zoneIdx]
	}
	if ip := net.ParseIP(ipOnly); ip != nil {
		ipList = append(ipList, ip)
		log.Printf("DNS Lookup IP Returned %v for Host: %s (literal)", ipList, host)
		return ipList
	}

	// bounded, context-aware DNS lookup using IPAddr for Go <=1.21 compatibility
	ipAddrs, err := net.DefaultResolver.LookupIPAddr(ctx, target)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("DNS Lookup IP Failed for Host (timeout %s): %s", dnsLookupTimeout, host)
		} else {
			log.Printf("DNS Lookup IP Failed for Host: %v, %s", err, host)
		}
		return []net.IP{}
	}

	for _, addr := range ipAddrs { // work with net.IPAddr results
		if addr.IP == nil {
			continue
		}
		ipList = append(ipList, addr.IP)
	}

	if len(ipList) == 0 {
		log.Printf("DNS Lookup IP Returned No Results for Host: %s", host)
	}

	log.Printf("DNS Lookup IP Returned %v for Host: %s", ipList, host)
	return ipList
}

// DnsLookupSrvs returns list of IP and port addresses based on host
// if host is private on aws route 53, then lookup ip will work only when within given aws vpc that host was registered with
func DnsLookupSrvs(host string) (ipList []string) {
	ctx, cancel := context.WithTimeout(context.Background(), dnsLookupTimeout)
	defer cancel()

	raw := strings.TrimSpace(host)
	if raw == "" {
		log.Printf("DNS Lookup SRV Failed for Host: invalid host '%s'", host)
		return []string{}
	}

	var (
		service string
		proto   string
		target  string
	)

	switch {
	case strings.HasPrefix(raw, "_"):
		// allow full SRV name (e.g., _ldap._tcp.example.com)
		target = raw
	case strings.Contains(raw, "://"):
		// interpret scheme as service[+proto], default proto tcp (e.g., grpc://example.com or sip+udp://example.com)
		parsed, err := url.Parse(raw)
		if err != nil {
			log.Printf("DNS Lookup SRV Failed for Host: %v, %s", err, host)
			return []string{}
		}
		schemeParts := strings.Split(parsed.Scheme, "+")
		service = schemeParts[0]
		proto = "tcp"
		if len(schemeParts) > 1 && schemeParts[1] != "" {
			proto = schemeParts[1]
		}
		target = parsed.Hostname()
	default:
		target = ParseHostFromURL(raw)
	}

	if target == "" {
		log.Printf("DNS Lookup SRV Failed for Host: invalid host '%s'", host)
		return []string{}
	}

	// Validate service/proto before calling LookupSRV to avoid deterministic resolver errors.
	if service == "" && !strings.HasPrefix(target, "_") {
		log.Printf("DNS Lookup SRV Failed for Host: service/proto missing for target '%s' (expected _service._proto.domain or scheme://host)", host)
		return []string{}
	}

	if service == "" && strings.HasPrefix(target, "_") {
		// If caller supplied full SRV name in target (leading underscore), split it.
		parts := strings.SplitN(target, ".", 3)
		if len(parts) >= 2 {
			service = strings.TrimPrefix(parts[0], "_")
			proto = strings.TrimPrefix(parts[1], "_")
			if len(parts) == 3 {
				target = parts[2]
			} else {
				target = ""
			}
		}
	}

	if service == "" || proto == "" || target == "" {
		log.Printf("DNS Lookup SRV Failed for Host: incomplete service/proto/target after parsing '%s'", host)
		return []string{}
	}

	service = strings.ToLower(service) // normalize to avoid resolver mismatches
	proto = strings.ToLower(proto)     // normalize to avoid resolver mismatches

	// bounded, context-aware SRV lookup
	_, addrs, err := net.DefaultResolver.LookupSRV(ctx, service, proto, target)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("DNS Lookup SRV Failed for Host (timeout %s): %s", dnsLookupTimeout, host)
		} else {
			log.Printf("DNS Lookup SRV Failed for Host: %v, %s", err, host)
		}
		return []string{}
	}

	deadline, hasDeadline := ctx.Deadline() // track remaining time for A/AAAA lookups
	seen := make(map[string]struct{})

	for _, v := range addrs {
		targetHost := strings.TrimSuffix(v.Target, ".")
		if targetHost == "" || targetHost == "." {
			continue
		}

		// ensure A/AAAA lookups do not run on an already-expired context
		remaining := dnsLookupTimeout
		if hasDeadline {
			remaining = time.Until(deadline)
			if remaining <= 0 {
				log.Printf("DNS Lookup SRV A/AAAA Resolve Failed (timeout %s) for Host: %s Target: %s", dnsLookupTimeout, host, targetHost)
				return ipList
			}
		}
		ipCtx, ipCancel := context.WithTimeout(context.Background(), remaining)
		ipAddrs, err := net.DefaultResolver.LookupIPAddr(ipCtx, targetHost)
		ipCancel()

		if err != nil {
			if errors.Is(ipCtx.Err(), context.DeadlineExceeded) {
				log.Printf("DNS Lookup SRV A/AAAA Resolve Failed (timeout %s) for Host: %s Target: %s", dnsLookupTimeout, host, targetHost)
			} else {
				log.Printf("DNS Lookup SRV A/AAAA Resolve Failed for Host: %s Target: %s, Error: %v", host, targetHost, err)
			}
			continue
		}

		for _, ipAddr := range ipAddrs {
			if ipAddr.IP == nil || len(ipAddr.IP) == 0 {
				continue
			}
			entry := net.JoinHostPort(ipAddr.String(), strconv.Itoa(int(v.Port)))
			if _, exists := seen[entry]; exists {
				continue
			}
			seen[entry] = struct{}{}
			ipList = append(ipList, entry)
		}
	}

	if len(ipList) == 0 {
		log.Printf("DNS Lookup SRV Returned No Results for Host: %s", host)
		return []string{}
	}

	log.Printf("DNS Lookup SRV Returned %v for Host: %s", ipList, host)
	return ipList
}

// ParseHostFromURL will parse out the host name from url
func ParseHostFromURL(u string) string {
	trimmed := strings.TrimSpace(u)
	if trimmed == "" {
		return ""
	}

	// escape IPv6 zone identifiers inside bracketed hosts, even when a scheme is present
	escapeIPv6Zone := func(raw string) string {
		start := strings.Index(raw, "[")
		end := strings.Index(raw, "]")
		if start == -1 || end == -1 || end <= start {
			return raw
		}

		hostPart := raw[start+1 : end]
		if !strings.Contains(hostPart, "%") {
			return raw
		}

		isHex := func(b byte) bool {
			return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
		}

		var b strings.Builder
		b.Grow(len(hostPart) + 4)
		for i := 0; i < len(hostPart); i++ {
			if hostPart[i] == '%' {
				if i+2 < len(hostPart) && isHex(hostPart[i+1]) && isHex(hostPart[i+2]) {
					// already percent-encoded
					b.WriteByte('%')
					continue
				}
				b.WriteString("%25")
				continue
			}
			b.WriteByte(hostPart[i])
		}

		return raw[:start+1] + b.String() + raw[end:]
	}

	// normalize scheme-less inputs, safely handling IPv6 literals with optional port/zone
	if !strings.Contains(trimmed, "://") {
		hostPort := trimmed

		// detect IPv6 (with optional zone) and separate port before bracketing
		if strings.Count(hostPort, ":") >= 2 && !strings.ContainsAny(hostPort, "[]") {
			raw := hostPort
			port := ""
			if idx := strings.LastIndex(raw, ":"); idx != -1 {
				possiblePort := raw[idx+1:]
				if possiblePort != "" {
					if _, err := strconv.Atoi(possiblePort); err == nil {
						hostOnly := raw[:idx]
						if ip := net.ParseIP(strings.Split(hostOnly, "%")[0]); ip != nil {
							hostPort = hostOnly // host portion only
							port = possiblePort
						}
					}
				}
			}

			if ip := net.ParseIP(strings.Split(hostPort, "%")[0]); ip != nil {
				hostOnlyEscaped := strings.ReplaceAll(hostPort, "%", "%25")
				if port != "" {
					hostPort = "[" + hostOnlyEscaped + "]:" + port // keep port outside brackets
				} else {
					hostPort = "[" + hostOnlyEscaped + "]" // no port
				}
			}
		}

		trimmed = "http://" + hostPort
	} else {
		// recover scheme-bearing unbracketed IPv6 literals (e.g., http://fe80::1%en0)
		split := strings.SplitN(trimmed, "://", 2)
		if len(split) == 2 {
			scheme, rest := split[0], split[1]
			authority := rest
			path := ""
			if slash := strings.Index(rest, "/"); slash != -1 {
				authority = rest[:slash]
				path = rest[slash:]
			}
			if strings.Count(authority, ":") >= 2 && !strings.ContainsAny(authority, "[]") {
				var hostOnly, port string
				if idx := strings.LastIndex(authority, ":"); idx != -1 {
					possiblePort := authority[idx+1:]
					if _, err := strconv.Atoi(possiblePort); err == nil {
						hostOnly = authority[:idx]
						port = possiblePort
					} else {
						hostOnly = authority
					}
				} else {
					hostOnly = authority
				}
				hostOnlyEscaped := strings.ReplaceAll(hostOnly, "%", "%25")
				if port != "" {
					authority = "[" + hostOnlyEscaped + "]:" + port
				} else {
					authority = "[" + hostOnlyEscaped + "]"
				}
			}
			trimmed = scheme + "://" + authority + path
		}
	}

	// also escape zones when a scheme was already present
	trimmed = escapeIPv6Zone(trimmed)

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}

	host := parsed.Hostname()
	if host == "" && parsed.Host != "" {
		// Fallback for unusual cases where Host is set but Hostname is empty.
		host = parsed.Host
	}

	// unescape zone back to original form (if any)
	if unescaped, unescapeErr := url.PathUnescape(host); unescapeErr == nil {
		host = unescaped
	}

	return host
}

// VerifyGoogleReCAPTCHAv2 will verify recaptcha v2 response data against given secret and obtain a response from google server
func VerifyGoogleReCAPTCHAv2(response string, secret string) (success bool, challengeTs time.Time, hostName string, err error) {
	if LenTrim(response) == 0 {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Response From CLient is Required")
	}

	if LenTrim(secret) == 0 {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Secret Key is Required")
	}

	// bounded context and stdlib HTTP client with form POST per Google spec
	ctx, cancel := context.WithTimeout(context.Background(), recaptchaVerifyTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", response)

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.google.com/recaptcha/api/siteverify", strings.NewReader(form.Encode()))
	if reqErr != nil {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Request Build Failed: %v", reqErr)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: recaptchaVerifyTimeout}
	httpResp, httpErr := client.Do(req)
	if httpErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Failed: timeout after %s", recaptchaVerifyTimeout)
		}
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Failed: %v", httpErr)
	}
	defer httpResp.Body.Close()

	limitedResp := io.LimitReader(httpResp.Body, maxReCaptchaBodyBytes+1)
	bodyBytes, readErr := io.ReadAll(limitedResp)
	if readErr != nil {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Response Read Failed: %v", readErr)
	}
	if int64(len(bodyBytes)) > maxReCaptchaBodyBytes {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Response Failed: body exceeds %d bytes", maxReCaptchaBodyBytes)
	}

	if httpResp.StatusCode != http.StatusOK {
		return false, time.Time{}, "", fmt.Errorf("ReCAPTCHA Service Failed: Status Code %d, Body: %s", httpResp.StatusCode, string(bodyBytes))
	}

	// strongly typed JSON parsing
	type recaptchaResponse struct {
		Success     bool     `json:"success"`
		ChallengeTS string   `json:"challenge_ts"`
		Hostname    string   `json:"hostname"`
		ErrorCodes  []string `json:"error-codes"`
	}

	var resp recaptchaResponse
	if err = json.Unmarshal(bodyBytes, &resp); err != nil {
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
			success = false
			err = fmt.Errorf("ReCAPTCHA Service Response Failed: (Parse challenge_ts Error) %s", parseErr)
			return success, challengeTs, hostName, err
		}
	}

	if !success {
		if len(resp.ErrorCodes) > 0 {
			err = fmt.Errorf("ReCAPTCHA Verify Errors: %s", strings.Join(resp.ErrorCodes, ", "))
		} else if err == nil {
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
	if req.Body == nil {
		return []byte{}, fmt.Errorf("Http Request Body Nil")
	}

	limited := io.LimitReader(req.Body, maxRequestBodyBytes+1)
	body, readErr := io.ReadAll(limited)
	closeErr := req.Body.Close()

	if readErr == nil && int64(len(body)) > maxRequestBodyBytes {
		return body, fmt.Errorf("Http Request Body exceeds limit of %d bytes", maxRequestBodyBytes)
	}

	req.Body = io.NopCloser(bytes.NewBuffer(body))

	// Prefer reporting the read error; if none, surface close error if present.
	if readErr != nil && closeErr != nil {
		return body, fmt.Errorf("read error: %v; close error: %v", readErr, closeErr)
	}
	if readErr != nil {
		return body, readErr
	}
	if closeErr != nil {
		return body, closeErr
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
	}

	keys := make([]string, 0, len(headerMap))
	for k := range headerMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buf := ""
	for _, k := range keys {
		buf += fmt.Sprintf("%s: %s\n", k, headerMap[k])
	}

	return buf
}
