package crypto

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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	util "github.com/aldelo/common"
	"io/ioutil"
)

type TlsConfig struct {}

// GetServerTlsConfig returns *tls.config configured for server TLS or mTLS based on parameters
//
// serverCertPemPath = (required) path and file name to the server cert pem (unencrypted version)
// serverKeyPemPath = (required) path and file name to the server key pem (unencrypted version)
// clientCaCertPath = (optional) one or more client ca cert path and file name, in case tls.config is for mTLS
func (t *TlsConfig) GetServerTlsConfig(serverCertPemPath string,
									   serverKeyPemPath string,
									   clientCaCertPemPath []string) (*tls.Config, error) {
	if util.LenTrim(serverCertPemPath) == 0 || util.LenTrim(serverKeyPemPath) == 0 {
		return nil, fmt.Errorf("Server TLS Config Requires Server Certificate and Key Pem Path")
	}

	// create server cert
	serverCert, err := tls.LoadX509KeyPair(serverCertPemPath, serverKeyPemPath)

	if err != nil {
		return nil, fmt.Errorf("Load X509 Key Pair Failed: %s", err.Error())
	}

	// if client ca cert pem defined, prep for mTLS
	certPool := x509.NewCertPool()
	certPoolCount := 0

	if len(clientCaCertPemPath) > 0 {
		for _, v := range clientCaCertPemPath {
			if util.LenTrim(v) > 0 {
				if clientCa, e := ioutil.ReadFile(v); e != nil {
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
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
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

	certPool := x509.NewCertPool()

	for _, v := range serverCaCertPemPath {
		if util.LenTrim(v) > 0 {
			if serverCa, e := ioutil.ReadFile(v); e != nil {
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
		RootCAs: certPool,
	}

	// for mTls set client cert
	if util.LenTrim(clientCertPemPath) > 0 && util.LenTrim(clientKeyPemPath) > 0 {
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