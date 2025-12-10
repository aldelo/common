package tlsconfig

/*
 * Copyright 2020-2023 Aldelo, LP
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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
)

type TlsConfig struct{}

// GetServerTlsConfig returns *tls.config configured for server TLS or mTLS based on parameters
//
// serverCertPemPath = (required) path and file name to the server cert pem (unencrypted version)
// serverKeyPemPath = (required) path and file name to the server key pem (unencrypted version)
// clientCaCertPath = (optional) one or more client ca cert path and file name, in case tls.config is for mTLS
func (t *TlsConfig) GetServerTlsConfig(serverCertPemPath string,
	serverKeyPemPath string,
	clientCaCertPemPath []string) (*tls.Config, error) {
	if len(strings.TrimSpace(serverCertPemPath)) == 0 || len(strings.TrimSpace(serverKeyPemPath)) == 0 {
		return nil, fmt.Errorf("Server TLS Config Requires Server Certificate and Key Pem Path")
	}

	// create server cert
	serverCert, err := tls.LoadX509KeyPair(serverCertPemPath, serverKeyPemPath)

	if err != nil {
		return nil, fmt.Errorf("Load X509 Key Pair Failed: %s", err.Error())
	}

	// if client ca cert pem defined, prep for mTLS
	certPool, _ := x509.SystemCertPool()
	if certPool == nil {
		certPool = x509.NewCertPool()
	}

	certPoolCount := 0

	clientCaCertPemPath = _stringSliceExtractUnique(clientCaCertPemPath)

	if len(clientCaCertPemPath) > 0 {
		for _, v := range clientCaCertPemPath {
			if len(strings.TrimSpace(v)) > 0 {
				if clientCa, e := os.ReadFile(v); e != nil {
					return nil, fmt.Errorf("Read Client CA Pem Failed: (%s) %s", v, e.Error())
				} else {
					if !certPool.AppendCertsFromPEM(clientCa) {
						// fail to add client ca to cert pool
						return nil, fmt.Errorf("Append Client CA From Pem Failed: %s", v)
					} else {
						// client ca pem appended to ca pool
						certPoolCount++
					}
				}
			}
		}
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{
			serverCert,
		},
		MinVersion: tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{
			tls.CurveP521,
			tls.CurveP384,
			tls.CurveP256,
		},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			// Modern TLS 1.2 suites (PFS + AEAD)
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,

			// obsolete
			// tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			// tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			// tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}

	if certPoolCount > 0 {
		config.ClientAuth = tls.RequireAndVerifyClientCert
		config.ClientCAs = certPool
	} else {
		config.ClientAuth = tls.NoClientCert
	}

	return config, nil
}

// GetClientTlsConfig returns *tls.config configured for server TLS or mTLS based on parameters
//
// serverCaCertPath = (required) one or more server ca cert path and file name, required for both server TLS or mTLS
// clientCertPemPath = (optional) for mTLS setup, path and file name to the client cert pem (unencrypted version)
// clientKeyPemPath = (optional) for mTLS setup, path and file name to the client key pem (unencrypted version)
func (t *TlsConfig) GetClientTlsConfig(serverCaCertPemPath []string,
	clientCertPemPath string,
	clientKeyPemPath string) (*tls.Config, error) {
	if len(serverCaCertPemPath) == 0 {
		return nil, fmt.Errorf("Client TLS Config Requires Server CA Certificate Pem Path")
	}

	certPool, _ := x509.SystemCertPool()
	if certPool == nil {
		certPool = x509.NewCertPool()
	}

	// get unique server ca cert pem files
	serverCaCertPemPath = _stringSliceExtractUnique(serverCaCertPemPath)

	for _, v := range serverCaCertPemPath {
		if len(strings.TrimSpace(v)) > 0 {
			if serverCa, e := os.ReadFile(v); e != nil {
				return nil, fmt.Errorf("Read Server CA Pem Failed: (%s) %s", v, e.Error())
			} else {
				if !certPool.AppendCertsFromPEM(serverCa) {
					// fail to add server ca to cert pool
					return nil, fmt.Errorf("Append Server CA From Pem Failed: %s", v)
				}
			}
		}
	}

	config := &tls.Config{
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
	}

	// for mTls set client cert
	if len(strings.TrimSpace(clientCertPemPath)) > 0 && len(strings.TrimSpace(clientKeyPemPath)) > 0 {
		if clientCert, e := tls.LoadX509KeyPair(clientCertPemPath, clientKeyPemPath); e != nil {
			return nil, fmt.Errorf("Load X509 Key Pair Failed: %s", e.Error())
		} else {
			config.Certificates = []tls.Certificate{
				clientCert,
			}
		}
	}

	return config, nil
}

// ---------------------------------------------------------------------------------------------------------------------
// private functions to avoid using common namespace (which causes conflict with rest namespace - circular reference)
// ---------------------------------------------------------------------------------------------------------------------

// _stringSliceContains checks if value is contained within the strSlice
func _stringSliceContains(strSlice *[]string, value string) bool {
	if strSlice == nil {
		return false
	} else {
		for _, v := range *strSlice {
			if strings.ToLower(v) == strings.ToLower(value) {
				return true
			}
		}

		return false
	}
}

// _stringSliceExtractUnique returns unique string slice elements
func _stringSliceExtractUnique(strSlice []string) (result []string) {
	if strSlice == nil {
		return []string{}
	} else if len(strSlice) <= 1 {
		return strSlice
	} else {
		for _, v := range strSlice {
			if !_stringSliceContains(&result, v) {
				result = append(result, v)
			}
		}

		return result
	}
}
