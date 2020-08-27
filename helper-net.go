package helper

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
	"fmt"
	"net"
	"strings"
)

// GetNetListener triggers the specified port to listen via tcp
func GetNetListener(port uint) (net.Listener, error) {
	if l, e := net.Listen("tcp", fmt.Sprintf(":%d", port)); e != nil {
		return nil, fmt.Errorf("Listen Tcp on Port %d Failed: %v", port, e)
	} else {
		return l, nil
	}
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
