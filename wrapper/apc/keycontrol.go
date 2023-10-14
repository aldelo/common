package apc

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

// =================================================================================================================
// AWS CREDENTIAL:
//		use $> aws configure (to set aws access key and secret to target machine)
//		Store AWS Access ID and Secret Key into Default Profile Using '$ aws configure' cli
//
// To Install & Setup AWS CLI on Host:
//		1) https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2-linux.html
//				On Ubuntu, if host does not have zip and unzip:
//					$> sudo apt install zip
//					$> sudo apt install unzip
//				On Ubuntu, to install AWS CLI v2:
//					$> curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
//					$> unzip awscliv2.zip
//					$> sudo ./aws/install
//		2) $> aws configure set region awsRegionName --profile default
// 		3) $> aws configure
//				follow prompts to enter Access ID and Secret Key
//
// AWS Region Name Reference:
//		us-west-2, us-east-1, ap-northeast-1, etc
//		See: https://docs.aws.amazon.com/general/latest/gr/rande.html
// =================================================================================================================

import (
	"context"
	"errors"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	pycrypto "github.com/aws/aws-sdk-go/service/paymentcryptography"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
	"net/http"
)

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// PaymentCryptography struct encapsulates the AWS PaymentCryptography access functionality
type PaymentCryptography struct {
	// define the AWS region that PaymentCryptography is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// store aws session object
	sess *session.Session

	// store PaymentCryptography client object
	pycClient *pycrypto.PaymentCryptography

	_parentSegment *xray.XRayParentSegment
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the PaymentCryptography service
func (k *PaymentCryptography) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			k._parentSegment = parentSegment[0]
		}

		seg := xray.NewSegment("PaymentCryptography-Connect", k._parentSegment)
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KDS-AWS-Region", k.AwsRegion)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = k.connectInternal()

		if err == nil {
			awsxray.AWS(k.pycClient.Client)
		}

		return err
	} else {
		return k.connectInternal()
	}
}

// Connect will establish a connection to the PaymentCryptography service
func (k *PaymentCryptography) connectInternal() error {
	// clean up prior session reference
	k.sess = nil

	if !k.AwsRegion.Valid() || k.AwsRegion == awsregion.UNKNOWN {
		return errors.New("Connect To PaymentCryptography Failed: (AWS Session Error) " + "Region is Required")
	}

	// create custom http2 client if needed
	var httpCli *http.Client
	var httpErr error

	if k.HttpOptions == nil {
		k.HttpOptions = new(awshttp2.HttpClientSettings)
	}

	// use custom http2 client
	h2 := &awshttp2.AwsHttp2Client{
		Options: k.HttpOptions,
	}

	if httpCli, httpErr = h2.NewHttp2Client(); httpErr != nil {
		return errors.New("Connect to PaymentCryptography Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection and keep session object in struct
	if sess, err := session.NewSession(
		&aws.Config{
			Region:     aws.String(k.AwsRegion.Key()),
			HTTPClient: httpCli,
		}); err != nil {
		// aws session error
		return errors.New("Connect To PaymentCryptography Failed: (AWS Session Error) " + err.Error())
	} else {
		// aws session obtained
		k.sess = sess

		// create cached objects for shared use
		k.pycClient = pycrypto.New(k.sess)

		if k.pycClient == nil {
			return errors.New("Connect To PaymentCryptography Client Failed: (New PaymentCryptography Client Connection) " + "Connection Object Nil")
		}

		// session stored to struct
		return nil
	}
}

// Disconnect will disjoin from aws session by clearing it
func (k *PaymentCryptography) Disconnect() {
	k.pycClient = nil
	k.sess = nil
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (k *PaymentCryptography) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	k._parentSegment = parentSegment
}

// ----------------------------------------------------------------------------------------------------------------
// PaymentCryptography Generate Key, support AES_256, RSA_2048, RSA_4096
// ----------------------------------------------------------------------------------------------------------------

// GenerateRSA2048 generates an RSA key of 2048 bits
func (k *PaymentCryptography) GenerateRSA2048() (keyArn string, err error) {
	return k.generateRSAKey(pycrypto.KeyAlgorithmRsa2048)
}

// GenerateRSA4096 generates an RSA key of 4096 bits
func (k *PaymentCryptography) GenerateRSA4096() (keyArn string, err error) {
	return k.generateRSAKey(pycrypto.KeyAlgorithmRsa4096)
}

// generateRSAKey KeyAlgorithm: AES_256, RSA_2048, RSA_4096
func (k *PaymentCryptography) generateRSAKey(KeyAlgorithm string) (keyArn string, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("PaymentCryptography-generateRSAKey", k._parentSegment)
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.pycClient == nil {
		err = errors.New("generateRSAKey with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return "", err
	}
	// prepare key info
	dataKeyInput := pycrypto.CreateKeyInput{
		Enabled:    aws.Bool(true),
		Exportable: aws.Bool(false),
		KeyAttributes: &pycrypto.KeyAttributes{
			KeyAlgorithm: aws.String(KeyAlgorithm),
			KeyClass:     aws.String(pycrypto.KeyClassAsymmetricKeyPair),
			KeyModesOfUse: &pycrypto.KeyModesOfUse{
				Decrypt:        aws.Bool(true),
				DeriveKey:      nil,
				Encrypt:        aws.Bool(true),
				Generate:       nil,
				NoRestrictions: nil,
				Sign:           nil,
				Unwrap:         aws.Bool(true),
				Verify:         nil,
				Wrap:           aws.Bool(true),
			},
			KeyUsage: aws.String(pycrypto.KeyUsageTr31D1AsymmetricKeyForDataEncryption),
		},
		KeyCheckValueAlgorithm: nil,
		Tags:                   nil,
	}

	// generate data key
	var dataKeyOutput *pycrypto.CreateKeyOutput
	var e error

	if segCtx == nil {
		dataKeyOutput, e = k.pycClient.CreateKey(&dataKeyInput)
	} else {
		dataKeyOutput, e = k.pycClient.CreateKeyWithContext(segCtx, &dataKeyInput)
	}

	if e != nil {
		err = errors.New("generateRSAKey with PaymentCryptography Failed: (Gen Data Key) " + e.Error())
		return "", err
	}

	if dataKeyOutput.Key != nil {
		keyArn = aws.StringValue(dataKeyOutput.Key.KeyArn)
	}

	return keyArn, nil
}

// GenerateAES256Key generates an AES key of 256 bits
func (k *PaymentCryptography) GenerateAES256Key() (keyArn string, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("PaymentCryptography-GenerateAES256Key", k._parentSegment)
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.pycClient == nil {
		err = errors.New("GenerateAES256Key with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return "", err
	}

	// prepare key info
	dataKeyInput := pycrypto.CreateKeyInput{
		Enabled:    aws.Bool(true),
		Exportable: aws.Bool(false),
		KeyAttributes: &pycrypto.KeyAttributes{
			KeyAlgorithm: aws.String(pycrypto.KeyAlgorithmAes256),
			KeyClass:     aws.String(pycrypto.KeyClassSymmetricKey),
			KeyModesOfUse: &pycrypto.KeyModesOfUse{
				Decrypt:        aws.Bool(true),
				DeriveKey:      nil,
				Encrypt:        aws.Bool(true),
				Generate:       nil,
				NoRestrictions: nil,
				Sign:           nil,
				Unwrap:         aws.Bool(true),
				Verify:         nil,
				Wrap:           aws.Bool(true),
			},
			KeyUsage: aws.String(pycrypto.KeyUsageTr31D0SymmetricDataEncryptionKey),
		},
		KeyCheckValueAlgorithm: nil,
		Tags:                   nil,
	}

	// generate data key
	var dataKeyOutput *pycrypto.CreateKeyOutput
	var e error

	if segCtx == nil {
		dataKeyOutput, e = k.pycClient.CreateKey(&dataKeyInput)
	} else {
		dataKeyOutput, e = k.pycClient.CreateKeyWithContext(segCtx, &dataKeyInput)
	}

	if e != nil {
		err = errors.New("GenerateAES256Key with PaymentCryptography Failed: (Gen Data Key) " + e.Error())
		return "", err
	}

	if dataKeyOutput.Key != nil {
		keyArn = aws.StringValue(dataKeyOutput.Key.KeyArn)
	}

	return keyArn, nil
}

// GetRSAPublicKey retrieves the RSA public key by using the keyArn, cert & certChain is a base64 string
func (k *PaymentCryptography) GetRSAPublicKey(keyArn string) (cert, certChain string, err error) {

	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("PaymentCryptography-GetRSAPublicKey", k._parentSegment)
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.pycClient == nil {
		err = errors.New("GetRSAPublicKey with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return "", "", err
	}

	if keyArn == "" {
		err = errors.New("GetRSAPublicKey with PaymentCryptography Failed: (Validate keyArn error) ")
		return "", "", err
	}

	var publicKeyOutput *pycrypto.GetPublicKeyCertificateOutput
	var e error
	pbKeyInput := &pycrypto.GetPublicKeyCertificateInput{
		KeyIdentifier: aws.String(keyArn),
	}

	if segCtx == nil {
		publicKeyOutput, e = k.pycClient.GetPublicKeyCertificate(pbKeyInput)
	} else {
		publicKeyOutput, e = k.pycClient.GetPublicKeyCertificateWithContext(segCtx, pbKeyInput)
	}

	if e != nil {
		err = errors.New("GetRSAPublicKey with PaymentCryptography Failed: (Get Data Key) " + e.Error())
		return "", "", err
	}
	return aws.StringValue(publicKeyOutput.KeyCertificate), aws.StringValue(publicKeyOutput.KeyCertificateChain), nil
}

// SetKeyAlias KeyAliasName is used as an alias for keyArn
func (k *PaymentCryptography) SetKeyAlias(keyArn, KeyAliasName string) (respAliasName string, err error) {

	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("PaymentCryptography-SetKeyAlias", k._parentSegment)
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.pycClient == nil {
		err = errors.New("SetKeyAlias with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return "", err
	}

	if keyArn == "" {
		err = errors.New("SetKeyAlias with PaymentCryptography Failed: (keyArn is empty) ")
		return
	}
	if KeyAliasName == "" {
		err = errors.New("SetKeyAlias with PaymentCryptography Failed: (KeyAliasName is empty) ")
		return
	}

	keyId := "alias/" + KeyAliasName
	aliasInput := pycrypto.CreateAliasInput{
		AliasName: aws.String(keyId),
		KeyArn:    aws.String(keyArn),
	}

	// generate alias
	var aliasKeyOutput *pycrypto.CreateAliasOutput
	var e error

	if segCtx == nil {
		aliasKeyOutput, e = k.pycClient.CreateAlias(&aliasInput)
	} else {
		aliasKeyOutput, e = k.pycClient.CreateAliasWithContext(segCtx, &aliasInput)
	}
	if e != nil {
		err = errors.New("SetKeyAlias with PaymentCryptography Failed: (CreateAlias) " + e.Error())
		return
	}
	if aliasKeyOutput != nil {
		if aliasKeyOutput.Alias != nil {
			respAliasName = aws.StringValue(aliasKeyOutput.Alias.AliasName)
		}
	}

	return respAliasName, nil
}
