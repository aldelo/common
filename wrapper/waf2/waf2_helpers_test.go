package waf2

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
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// validateIPOrCIDR
// ---------------------------------------------------------------------------

func TestValidateIPOrCIDR(t *testing.T) {
	tests := []struct {
		name       string
		addr       string
		wantFamily string // "IPV4" or "IPV6"
		wantNorm   string // normalized CIDR string
		wantErr    bool
	}{
		// ------- IPv4: bare IP coerced to /32 -------
		{
			name:       "bare IPv4 coerced to /32",
			addr:       "1.2.3.4",
			wantFamily: "IPV4",
			wantNorm:   "1.2.3.4/32",
			wantErr:    false,
		},
		{
			name:       "bare IPv4 loopback",
			addr:       "127.0.0.1",
			wantFamily: "IPV4",
			wantNorm:   "127.0.0.1/32",
			wantErr:    false,
		},

		// ------- IPv4 CIDR -------
		{
			name:       "IPv4 /8 minimum allowed",
			addr:       "10.0.0.0/8",
			wantFamily: "IPV4",
			wantNorm:   "10.0.0.0/8",
			wantErr:    false,
		},
		{
			name:       "IPv4 /16",
			addr:       "172.16.0.0/16",
			wantFamily: "IPV4",
			wantNorm:   "172.16.0.0/16",
			wantErr:    false,
		},
		{
			name:       "IPv4 /24",
			addr:       "192.168.1.0/24",
			wantFamily: "IPV4",
			wantNorm:   "192.168.1.0/24",
			wantErr:    false,
		},
		{
			name:       "IPv4 /32 explicit",
			addr:       "10.0.0.1/32",
			wantFamily: "IPV4",
			wantNorm:   "10.0.0.1/32",
			wantErr:    false,
		},

		// ------- IPv4 CIDR normalization (host bits zeroed by net.ParseCIDR) -------
		{
			name:       "IPv4 host bits normalized",
			addr:       "192.168.1.100/24",
			wantFamily: "IPV4",
			wantNorm:   "192.168.1.0/24", // net.ParseCIDR zeros host bits
			wantErr:    false,
		},

		// ------- IPv4 out-of-range CIDR -------
		{
			name:    "IPv4 /7 below minimum",
			addr:    "10.0.0.0/7",
			wantErr: true,
		},
		{
			name:    "IPv4 /0 below minimum",
			addr:    "0.0.0.0/0",
			wantErr: true,
		},
		{
			name:    "IPv4 /33 out of range",
			addr:    "10.0.0.0/33",
			wantErr: true,
		},

		// ------- IPv6: bare IP coerced to /128 -------
		{
			name:       "bare IPv6 loopback",
			addr:       "::1",
			wantFamily: "IPV6",
			wantNorm:   "::1/128",
			wantErr:    false,
		},
		{
			name:       "bare IPv6 full address",
			addr:       "2001:db8::1",
			wantFamily: "IPV6",
			wantNorm:   "2001:db8::1/128",
			wantErr:    false,
		},

		// ------- IPv6 CIDR -------
		{
			name:       "IPv6 /32",
			addr:       "2001:db8::/32",
			wantFamily: "IPV6",
			wantNorm:   "2001:db8::/32",
			wantErr:    false,
		},
		{
			name:       "IPv6 /24 minimum allowed",
			addr:       "2001::/24",
			wantFamily: "IPV6",
			wantNorm:   "2001::/24",
			wantErr:    false,
		},
		{
			name:       "IPv6 /128 explicit",
			addr:       "::1/128",
			wantFamily: "IPV6",
			wantNorm:   "::1/128",
			wantErr:    false,
		},
		{
			name:       "IPv6 /64",
			addr:       "fe80::/64",
			wantFamily: "IPV6",
			wantNorm:   "fe80::/64",
			wantErr:    false,
		},

		// ------- IPv6 out-of-range CIDR -------
		{
			name:    "IPv6 /23 below minimum",
			addr:    "2001::/23",
			wantErr: true,
		},
		{
			name:    "IPv6 /0 below minimum",
			addr:    "::/0",
			wantErr: true,
		},

		// ------- invalid inputs -------
		{
			name:    "empty string",
			addr:    "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			addr:    "   ",
			wantErr: true,
		},
		{
			name:    "not an IP",
			addr:    "not-an-ip",
			wantErr: true,
		},
		{
			name:    "invalid octets",
			addr:    "999.999.999.999",
			wantErr: true,
		},
		{
			name:    "hostname",
			addr:    "example.com",
			wantErr: true,
		},
		{
			name:    "partial IP",
			addr:    "10.0.0",
			wantErr: true,
		},

		// ------- whitespace handling -------
		{
			name:       "leading/trailing whitespace trimmed",
			addr:       "  10.0.0.1  ",
			wantFamily: "IPV4",
			wantNorm:   "10.0.0.1/32",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			family, norm, err := validateIPOrCIDR(tt.addr)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateIPOrCIDR(%q) error = %v, wantErr %v", tt.addr, err, tt.wantErr)
				return
			}

			if tt.wantErr {
				// verify error case returns empty strings
				if family != "" || norm != "" {
					t.Errorf("validateIPOrCIDR(%q) on error: family=%q, norm=%q; want both empty", tt.addr, family, norm)
				}
				return
			}

			if family != tt.wantFamily {
				t.Errorf("validateIPOrCIDR(%q) family = %q, want %q", tt.addr, family, tt.wantFamily)
			}
			if norm != tt.wantNorm {
				t.Errorf("validateIPOrCIDR(%q) norm = %q, want %q", tt.addr, norm, tt.wantNorm)
			}
		})
	}
}

func TestValidateIPOrCIDR_EmptyError(t *testing.T) {
	_, _, err := validateIPOrCIDR("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error for empty input should mention 'required', got: %s", err.Error())
	}
}

func TestValidateIPOrCIDR_FamilyValues(t *testing.T) {
	// verify that the family string is exactly "IPV4" or "IPV6", not lowercase
	fam4, _, err := validateIPOrCIDR("10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fam4 != "IPV4" {
		t.Errorf("IPv4 family = %q, want exactly 'IPV4'", fam4)
	}

	fam6, _, err := validateIPOrCIDR("::1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fam6 != "IPV6" {
		t.Errorf("IPv6 family = %q, want exactly 'IPV6'", fam6)
	}
}
