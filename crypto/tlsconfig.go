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
	"io/ioutil"
	"errors"
)

type TlsConfig struct {}

// certPemFilePath = "cert-xyz.pem"
func (t *TlsConfig) GetServerTlsConfig(certPemFilePath string) (*tls.Config, error) {
	// create a CA certificate pool and add cert-xyz.pem to it
	if caCert, err := ioutil.ReadFile(certPemFilePath); err != nil {
		return nil, errors.New("Read Cert Pem Failed: " + err.Error())
	} else {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		// create tls config with CA pool and enable client certificate validation
		tlsConfig := &tls.Config{
			ClientCAs: caCertPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			MinVersion: tls.VersionTLS12,
			CurvePreferences: []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			},
		}

		return tlsConfig, nil
	}
}

func (t *TlsConfig) GetClientTlsConfig(certPemFilePath string, keyPemFilePath string) (*tls.Config, error) {
	// read key pair to create certificate
	if cert, err := tls.LoadX509KeyPair(certPemFilePath, keyPemFilePath); err != nil {
		return nil, errors.New("Load X509 KeyPair Failed: " + err.Error())
	} else {
		// create CA certificate pool and add cert-xyz.pem to it
		if caCert, err1 := ioutil.ReadFile(certPemFilePath); err1 != nil {
			return nil, errors.New("Read Cert Pem Failed: " + err1.Error())
		} else {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)

			tlsConfig := &tls.Config{
				RootCAs: caCertPool,
				Certificates: []tls.Certificate{cert},
			}

			return tlsConfig, nil
		}
	}
}