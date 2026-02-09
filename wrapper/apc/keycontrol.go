package apc

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
	"sync"
	"time"

	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	pycrypto "github.com/aws/aws-sdk-go/service/paymentcryptography"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
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
	mu             sync.RWMutex
}

// ================================================================================================================
// internal helpers
// ================================================================================================================

func (k *PaymentCryptography) getClient() *pycrypto.PaymentCryptography {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.pycClient
}

func (k *PaymentCryptography) getParentSegment() *xray.XRayParentSegment {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k._parentSegment
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
			k.mu.Lock()
			k._parentSegment = parentSegment[0]
			k.mu.Unlock()
		}

		seg := xray.NewSegment("PaymentCryptography-Connect", k.getParentSegment())
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KDS-AWS-Region", k.AwsRegion)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = k.connectInternal()

		if err == nil {
			if client := k.getClient(); client != nil {
				awsxray.AWS(client.Client)
			}
		}

		return err
	} else {
		return k.connectInternal()
	}
}

// Connect will establish a connection to the PaymentCryptography service
func (k *PaymentCryptography) connectInternal() error {
	k.mu.RLock()
	region := k.AwsRegion
	httpOpts := k.HttpOptions
	k.mu.RUnlock()

	// clean up prior session reference
	k.mu.Lock()
	k.sess = nil
	k.pycClient = nil
	k.mu.Unlock()

	if !region.Valid() || region == awsregion.UNKNOWN {
		return errors.New("Connect To PaymentCryptography Failed: (AWS Session Error) " + "Region is Required")
	}

	// create custom http2 client if needed
	if httpOpts == nil {
		httpOpts = new(awshttp2.HttpClientSettings)
	}

	h2 := &awshttp2.AwsHttp2Client{
		Options: httpOpts,
	}

	httpCli, httpErr := h2.NewHttp2Client()
	if httpErr != nil {
		return errors.New("Connect to PaymentCryptography Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection and keep session object in struct
	sess, err := session.NewSession(
		&aws.Config{
			Region:     aws.String(region.Key()),
			HTTPClient: httpCli,
		})
	if err != nil {
		// aws session error
		return errors.New("Connect To PaymentCryptography Failed: (AWS Session Error) " + err.Error())
	}

	client := pycrypto.New(sess)
	if client == nil {
		return errors.New("Connect To PaymentCryptography Client Failed: (New PaymentCryptography Client Connection) " + "Connection Object Nil")
	}

	k.mu.Lock()
	k.HttpOptions = httpOpts
	k.sess = sess
	k.pycClient = client
	k.mu.Unlock()

	return nil
}

// Disconnect will disjoin from aws session by clearing it
func (k *PaymentCryptography) Disconnect() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.pycClient = nil
	k.sess = nil
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (k *PaymentCryptography) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	k.mu.Lock()
	defer k.mu.Unlock()
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

	seg := xray.NewSegmentNullable("PaymentCryptography-generateRSAKey", k.getParentSegment())
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	client := k.getClient()
	if client == nil {
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
		dataKeyOutput, e = client.CreateKey(&dataKeyInput)
	} else {
		dataKeyOutput, e = client.CreateKeyWithContext(segCtx, &dataKeyInput)
	}

	if e != nil {
		err = errors.New("generateRSAKey with PaymentCryptography Failed: (Gen Data Key) " + e.Error())
		return "", err
	}

	if dataKeyOutput == nil {
		err = errors.New("generateRSAKey with PaymentCryptography Failed: CreateKeyOutput is nil")
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

	seg := xray.NewSegmentNullable("PaymentCryptography-GenerateAES256Key", k.getParentSegment())
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	client := k.getClient()
	if client == nil {
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
		dataKeyOutput, e = client.CreateKey(&dataKeyInput)
	} else {
		dataKeyOutput, e = client.CreateKeyWithContext(segCtx, &dataKeyInput)
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

	seg := xray.NewSegmentNullable("PaymentCryptography-GetRSAPublicKey", k.getParentSegment())
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	client := k.getClient()
	if client == nil {
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
		publicKeyOutput, e = client.GetPublicKeyCertificate(pbKeyInput)
	} else {
		publicKeyOutput, e = client.GetPublicKeyCertificateWithContext(segCtx, pbKeyInput)
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

	seg := xray.NewSegmentNullable("PaymentCryptography-SetKeyAlias", k.getParentSegment())
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	client := k.getClient()
	if client == nil {
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
		aliasKeyOutput, e = client.CreateAlias(&aliasInput)
	} else {
		aliasKeyOutput, e = client.CreateAliasWithContext(segCtx, &aliasInput)
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

func (k *PaymentCryptography) ImportRootCAPublicKey(publicKey string) (keyArn string, err error) {

	var segCtx context.Context

	seg := xray.NewSegmentNullable("PaymentCryptography-ImportRootCAPublicKey", k.getParentSegment())
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	client := k.getClient()
	if client == nil {
		err = errors.New("ImportRootCAPublicKey with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return "", err
	}

	if publicKey == "" {
		err = errors.New("ImportRootCAPublicKey with PaymentCryptography Failed: (publicKey is empty) ")
		return
	}

	imInput := &pycrypto.ImportKeyInput{
		Enabled:                aws.Bool(true),
		KeyCheckValueAlgorithm: nil,
		KeyMaterial: &pycrypto.ImportKeyMaterial{
			RootCertificatePublicKey: &pycrypto.RootCertificatePublicKey{
				KeyAttributes: &pycrypto.KeyAttributes{
					KeyAlgorithm: aws.String(pycrypto.KeyAlgorithmRsa2048),
					KeyClass:     aws.String(pycrypto.KeyClassPublicKey),
					KeyModesOfUse: &pycrypto.KeyModesOfUse{
						Verify: aws.Bool(true),
					},
					KeyUsage: aws.String(pycrypto.KeyUsageTr31S0AsymmetricKeyForDigitalSignature),
				},
				PublicKeyCertificate: aws.String(publicKey),
			},
		},
		Tags: nil,
	}
	var imOutput *pycrypto.ImportKeyOutput
	var e error
	if segCtx == nil {
		imOutput, e = client.ImportKey(imInput)
	} else {
		imOutput, e = client.ImportKeyWithContext(segCtx, imInput)
	}

	if e != nil {
		return "", e
	}
	if imOutput != nil {
		if imOutput.Key != nil {
			keyArn = aws.StringValue(imOutput.Key.KeyArn)
		}
	}

	return
}

func (k *PaymentCryptography) ImportIntermediateCAPublicKey(capkArn, publicKey string) (keyArn string, err error) {

	var segCtx context.Context

	seg := xray.NewSegmentNullable("PaymentCryptography-ImportIntermediateCAPublicKey", k.getParentSegment())
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	client := k.getClient()
	if client == nil {
		err = errors.New("ImportIntermediateCAPublicKey with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return "", err
	}

	if capkArn == "" {
		err = errors.New("ImportIntermediateCAPublicKey with PaymentCryptography Failed: (capkArn is empty) ")
		return "", err
	}

	if publicKey == "" {
		err = errors.New("ImportIntermediateCAPublicKey with PaymentCryptography Failed: (publicKey is empty) ")
		return
	}

	imInput := &pycrypto.ImportKeyInput{
		Enabled:                aws.Bool(true),
		KeyCheckValueAlgorithm: nil,
		KeyMaterial: &pycrypto.ImportKeyMaterial{
			TrustedCertificatePublicKey: &pycrypto.TrustedCertificatePublicKey{
				CertificateAuthorityPublicKeyIdentifier: aws.String(capkArn),
				KeyAttributes: &pycrypto.KeyAttributes{
					KeyAlgorithm: aws.String(pycrypto.KeyAlgorithmRsa2048),
					KeyClass:     aws.String(pycrypto.KeyClassPublicKey),
					KeyModesOfUse: &pycrypto.KeyModesOfUse{
						Verify: aws.Bool(true),
					},
					KeyUsage: aws.String(pycrypto.KeyUsageTr31S0AsymmetricKeyForDigitalSignature),
				},
				PublicKeyCertificate: aws.String(publicKey),
			},
		},
		Tags: nil,
	}
	var imOutput *pycrypto.ImportKeyOutput
	var e error
	if segCtx == nil {
		imOutput, e = client.ImportKey(imInput)
	} else {
		imOutput, e = client.ImportKeyWithContext(segCtx, imInput)
	}

	if e != nil {
		return "", e
	}
	if imOutput != nil {
		if imOutput.Key != nil {
			keyArn = aws.StringValue(imOutput.Key.KeyArn)
		}
	}

	return
}

func (k *PaymentCryptography) ImportKEKey(capkArn, imToken, signCA, keyBlock string, nonce *string) (keyArn string, err error) {
	var segCtx context.Context

	seg := xray.NewSegmentNullable("PaymentCryptography-ImportKEKey", k.getParentSegment())
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	client := k.getClient()
	if client == nil {
		err = errors.New("ImportKEKey with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return "", err
	}
	if capkArn == "" || imToken == "" || signCA == "" || keyBlock == "" {
		err = errors.New("ImportKEKey with PaymentCryptography Failed: (one of capkArn, imToken, signCA, keyBlock is empty) ")
		return
	}

	imInput := &pycrypto.ImportKeyInput{
		Enabled:                aws.Bool(true),
		KeyCheckValueAlgorithm: nil,
		KeyMaterial: &pycrypto.ImportKeyMaterial{
			RootCertificatePublicKey: nil,
			Tr31KeyBlock:             nil,
			Tr34KeyBlock: &pycrypto.ImportTr34KeyBlock{
				CertificateAuthorityPublicKeyIdentifier: aws.String(capkArn), //import root ca
				ImportToken:                             aws.String(imToken),
				KeyBlockFormat:                          aws.String(pycrypto.Tr34KeyBlockFormatX9Tr342012),
				RandomNonce:                             nonce,
				SigningKeyCertificate:                   aws.String(signCA),   //public key ca for sign
				WrappedKeyBlock:                         aws.String(keyBlock), //TR-34.2012 non-CMS two pass format
			},
			TrustedCertificatePublicKey: nil,
		},
		Tags: nil,
	}

	var imOutput *pycrypto.ImportKeyOutput
	var e error
	if segCtx == nil {
		imOutput, e = client.ImportKey(imInput)
	} else {
		imOutput, e = client.ImportKeyWithContext(segCtx, imInput)
	}

	if e != nil {
		return "", e
	}

	if imOutput != nil {
		if imOutput.Key != nil {
			keyArn = aws.StringValue(imOutput.Key.KeyArn)
		}
	}

	return
}

func (k *PaymentCryptography) ImportTR31Key(keyBlock, warpKeyArn string) (keyArn string, err error) {

	var segCtx context.Context

	seg := xray.NewSegmentNullable("PaymentCryptography-ImportTR31Key", k.getParentSegment())
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	client := k.getClient()
	if client == nil {
		err = errors.New("ImportTR31Key with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return "", err
	}
	if keyBlock == "" {
		err = errors.New("ImportTR31Key with PaymentCryptography Failed: (keyBlock is empty) ")
		return
	}
	if warpKeyArn == "" {
		err = errors.New("ImportTR31Key with PaymentCryptography Failed: (warpKeyArn is empty) ")
		return
	}

	imInput := &pycrypto.ImportKeyInput{
		Enabled:                aws.Bool(true),
		KeyCheckValueAlgorithm: nil,
		KeyMaterial: &pycrypto.ImportKeyMaterial{
			RootCertificatePublicKey: nil,
			Tr31KeyBlock: &pycrypto.ImportTr31KeyBlock{
				WrappedKeyBlock:       aws.String(keyBlock),
				WrappingKeyIdentifier: aws.String(warpKeyArn),
			},
		},
		Tags: nil,
	}

	var imOutput *pycrypto.ImportKeyOutput
	var e error
	if segCtx == nil {
		imOutput, e = client.ImportKey(imInput)
	} else {
		imOutput, e = client.ImportKeyWithContext(segCtx, imInput)
	}
	if e != nil {
		return "", e
	}

	if imOutput != nil {
		if imOutput.Key != nil {
			keyArn = aws.StringValue(imOutput.Key.KeyArn)
		}
	}

	return
}

func (k *PaymentCryptography) GetParamsForImportKEKey() (cert, certChain, token string, parametersValidUntilTimestamp *time.Time, err error) {

	var segCtx context.Context

	seg := xray.NewSegmentNullable("PaymentCryptography-GetParamsForImportKEKey", k.getParentSegment())
	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	client := k.getClient()
	if client == nil {
		err = errors.New("GetParamsForImportKEKey with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return "", "", "", nil, err
	}

	pmsInput := &pycrypto.GetParametersForImportInput{
		KeyMaterialType:      aws.String(pycrypto.KeyMaterialTypeTr34KeyBlock),
		WrappingKeyAlgorithm: aws.String(pycrypto.KeyAlgorithmRsa2048),
	}

	var pmsOutput *pycrypto.GetParametersForImportOutput
	var e error
	if segCtx == nil {
		pmsOutput, e = client.GetParametersForImport(pmsInput)
	} else {
		pmsOutput, e = client.GetParametersForImportWithContext(segCtx, pmsInput)
	}
	if e != nil {
		return "", "", "", nil, e
	}

	if pmsOutput != nil {
		token = aws.StringValue(pmsOutput.ImportToken)
		cert = aws.StringValue(pmsOutput.WrappingKeyCertificate)
		certChain = aws.StringValue(pmsOutput.WrappingKeyCertificateChain)
		parametersValidUntilTimestamp = pmsOutput.ParametersValidUntilTimestamp
	}
	return
}

func (k *PaymentCryptography) ListAlias() (output *pycrypto.ListAliasesOutput, err error) {
	client := k.getClient()
	if client == nil {
		err = errors.New("ListAlias with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return nil, err
	}

	// generate alias
	var aliasInput *pycrypto.ListAliasesInput
	var aliasOutput *pycrypto.ListAliasesOutput
	var e error

	aliasOutput, e = client.ListAliases(aliasInput)

	if e != nil {
		err = errors.New("ListAlias with PaymentCryptography Failed: (ListAlias) " + e.Error())
		return
	}
	return aliasOutput, nil
}

func (k *PaymentCryptography) ListAliasNextPage(nextTK string) (output *pycrypto.ListAliasesOutput, err error) {
	client := k.getClient()
	if client == nil {
		err = errors.New("ListAliasNextPage with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return nil, err
	}

	// generate alias
	var aliasInput *pycrypto.ListAliasesInput
	var aliasOutput *pycrypto.ListAliasesOutput
	var e error

	if nextTK != "" {
		aliasInput = &pycrypto.ListAliasesInput{
			NextToken: aws.String(nextTK),
		}
	}

	aliasOutput, e = client.ListAliases(aliasInput)

	if e != nil {
		err = errors.New("ListAliasNextPage with PaymentCryptography Failed: (ListAlias) " + e.Error())
		return
	}
	return aliasOutput, nil
}

func (k *PaymentCryptography) GetKey(keyArn string) (output *pycrypto.GetKeyOutput, err error) {
	client := k.getClient()
	if client == nil {
		err = errors.New("GetKey with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return nil, err
	}

	// generate alias
	var aliasInput *pycrypto.GetKeyInput
	var aliasOutput *pycrypto.GetKeyOutput
	var e error

	aliasInput = &pycrypto.GetKeyInput{
		KeyIdentifier: aws.String(keyArn),
	}

	aliasOutput, e = client.GetKey(aliasInput)

	if e != nil {
		err = errors.New("GetKey with PaymentCryptography Failed: (GetKey) " + e.Error())
		return
	}
	return aliasOutput, nil
}

func (k *PaymentCryptography) GetKeyByAlias(keyAlias string) (output *pycrypto.GetAliasOutput, err error) {
	client := k.getClient()
	if client == nil {
		err = errors.New("GetKeyByAlias with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return nil, err
	}

	// generate alias
	var aliasInput *pycrypto.GetAliasInput
	var aliasOutput *pycrypto.GetAliasOutput
	var e error

	aliasInput = &pycrypto.GetAliasInput{
		AliasName: aws.String(keyAlias),
	}

	aliasOutput, e = client.GetAlias(aliasInput)

	if e != nil {
		err = errors.New("GetKeyByAlias with PaymentCryptography Failed: (GetKeyByAlias) " + e.Error())
		return
	}
	return aliasOutput, nil
}

func (k *PaymentCryptography) ListKeys() (output *pycrypto.ListKeysOutput, err error) {
	client := k.getClient()
	if client == nil {
		err = errors.New("ListKeys with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return nil, err
	}

	// generate alias
	var aliasInput *pycrypto.ListKeysInput
	var aliasOutput *pycrypto.ListKeysOutput
	var e error

	aliasOutput, e = client.ListKeys(aliasInput)

	if e != nil {
		err = errors.New("ListKeys with PaymentCryptography Failed: (ListKeys) " + e.Error())
		return
	}
	return aliasOutput, nil
}

func (k *PaymentCryptography) StopKeyUsage(keyArn string) (err error) {
	client := k.getClient()
	if client == nil {
		err = errors.New("DisableKey with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return err
	}

	if keyArn == "" {
		err = errors.New("DisableKey with PaymentCryptography Failed: " + "keyArn is Required")
		return err
	}

	// generate alias
	var reqInput = &pycrypto.StopKeyUsageInput{
		KeyIdentifier: aws.String(keyArn),
	}
	//var respOutput *pycrypto.StopKeyUsageOutput
	var e error

	_, e = client.StopKeyUsage(reqInput)

	if e != nil {
		err = errors.New("DisableKey with PaymentCryptography Failed: (DisableKey) " + e.Error())
		return
	}
	//if respOutput != nil {
	//	fmt.Println(respOutput.String())
	//}
	return nil
}

func (k *PaymentCryptography) StartKeyUsage(keyArn string) (err error) {
	client := k.getClient()
	if client == nil {
		err = errors.New("EnableKey with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return err
	}

	if keyArn == "" {
		err = errors.New("EnableKey with PaymentCryptography Failed: " + "keyArn is Required")
		return err
	}

	// generate alias
	var reqInput = &pycrypto.StartKeyUsageInput{
		KeyIdentifier: aws.String(keyArn),
	}
	//var respOutput *pycrypto.StartKeyUsageOutput
	var e error

	_, e = client.StartKeyUsage(reqInput)

	if e != nil {
		err = errors.New("EnableKey with PaymentCryptography Failed: (StartKeyUsage) " + e.Error())
		return
	}
	//if respOutput != nil {
	//	fmt.Println(respOutput.String())
	//}
	return nil
}

func (k *PaymentCryptography) DeleteKey(keyArn string) (err error) {
	client := k.getClient()
	if client == nil {
		err = errors.New("DeleteKey with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return err
	}

	if keyArn == "" {
		err = errors.New("DeleteKey with PaymentCryptography Failed: " + "keyArn is Required")
		return err
	}

	// generate alias
	var reqInput = &pycrypto.DeleteKeyInput{
		KeyIdentifier: aws.String(keyArn),
	}
	//var respOutput *pycrypto.DeleteKeyOutput
	var e error

	_, e = client.DeleteKey(reqInput)

	if e != nil {
		err = errors.New("DeleteKey with PaymentCryptography Failed: (DeleteKey) " + e.Error())
		return
	}

	return nil
}

func (k *PaymentCryptography) DeleteAlias(aliasName string) (err error) {
	client := k.getClient()
	if client == nil {
		err = errors.New("DeleteAlias with PaymentCryptography Failed: " + "PaymentCryptography Client is Required")
		return err
	}

	if aliasName == "" {
		err = errors.New("DeleteAlias with PaymentCryptography Failed: " + "aliasName is Required")
		return err
	}

	// generate alias
	var reqInput = &pycrypto.DeleteAliasInput{
		AliasName: aws.String(aliasName),
	}

	var e error

	_, e = client.DeleteAlias(reqInput)

	if e != nil {
		err = errors.New("DeleteAlias with PaymentCryptography Failed: (DeleteAlias) " + e.Error())
		return
	}
	return nil
}
