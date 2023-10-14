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
	pycryptoData "github.com/aws/aws-sdk-go/service/paymentcryptographydata"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
	"net/http"
)

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// PaymentCryptoData struct encapsulates the AWS PaymentCryptography Data access functionality
type PaymentCryptoData struct {
	// define the AWS region that PaymentCryptography is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// define PaymentCryptography Data key or key alias
	KeyArn string

	// store aws session object
	sess *session.Session

	// store PaymentCryptography Data client object
	pyDataClient *pycryptoData.PaymentCryptographyData

	_parentSegment *xray.XRayParentSegment
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the PaymentCryptography Data service
func (k *PaymentCryptoData) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			k._parentSegment = parentSegment[0]
		}

		seg := xray.NewSegment("PaymentCryptoData-Connect", k._parentSegment)
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KDS-AWS-Region", k.AwsRegion)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = k.connectInternal()

		if err == nil {
			awsxray.AWS(k.pyDataClient.Client)
		}

		return err
	} else {
		return k.connectInternal()
	}
}

// Connect will establish a connection to the PaymentCryptography Data service
func (k *PaymentCryptoData) connectInternal() error {
	// clean up prior session reference
	k.sess = nil

	if !k.AwsRegion.Valid() || k.AwsRegion == awsregion.UNKNOWN {
		return errors.New("Connect To PaymentCryptoData Failed: (AWS Session Error) " + "Region is Required")
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
		return errors.New("Connect to PaymentCryptoData Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection and keep session object in struct
	if sess, err := session.NewSession(
		&aws.Config{
			Region:     aws.String(k.AwsRegion.Key()),
			HTTPClient: httpCli,
		}); err != nil {
		// aws session error
		return errors.New("Connect To PaymentCryptoData Failed: (AWS Session Error) " + err.Error())
	} else {
		// aws session obtained
		k.sess = sess

		// create cached objects for shared use
		k.pyDataClient = pycryptoData.New(k.sess)

		if k.pyDataClient == nil {
			return errors.New("Connect To PaymentCryptoData Client Failed: (New PaymentCryptography Client Connection) " + "Connection Object Nil")
		}

		// session stored to struct
		return nil
	}
}

// Disconnect will disjoin from aws session by clearing it
func (k *PaymentCryptoData) Disconnect() {
	k.pyDataClient = nil
	k.sess = nil
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (k *PaymentCryptoData) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	k._parentSegment = parentSegment
}

func awsNilString(input string) *string {
	if input == "" {
		return nil
	}
	return aws.String(input)
}

// ----------------------------------------------------------------------------------------------------------------
// PaymentCryptography Data encrypt/decrypt via RSA functions
// ----------------------------------------------------------------------------------------------------------------

func (k *PaymentCryptoData) encrypt(plainText string, encryptionAttributes *pycryptoData.EncryptionDecryptionAttributes) (cipherText string, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("PaymentCryptoData-encrypt", k._parentSegment)
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
	if k.pyDataClient == nil {
		err = errors.New("encrypt with PaymentCryptoData key Failed: " + "PaymentCryptoData Client is Required")
		return "", err
	}

	if len(k.KeyArn) <= 0 {
		err = errors.New("encrypt with PaymentCryptoData key Failed: " + "PaymentCryptoData KeyArn is Required")
		return "", err
	}

	// encrypt asymmetric using kms cmk
	var encryptedOutput *pycryptoData.EncryptDataOutput
	var e error

	encryptDataInput := &pycryptoData.EncryptDataInput{
		EncryptionAttributes: encryptionAttributes,
		KeyIdentifier:        aws.String(k.KeyArn),
		PlainText:            aws.String(plainText),
	}

	if segCtx == nil {
		encryptedOutput, e = k.pyDataClient.EncryptData(encryptDataInput)
	} else {
		encryptedOutput, e = k.pyDataClient.EncryptDataWithContext(segCtx, encryptDataInput)
	}

	if e != nil {
		err = errors.New("encrypt with PaymentCryptoData key Failed: " + e.Error())
		return "", err
	}
	if encryptedOutput != nil {
		cipherText = aws.StringValue(encryptedOutput.CipherText)
	}
	return cipherText, nil
}

func (k *PaymentCryptoData) decrypt(cipherText string, decryptionAttributes *pycryptoData.EncryptionDecryptionAttributes) (plainText string, err error) {

	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("PaymentCryptoData-decrypt", k._parentSegment)
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
	if k.pyDataClient == nil {
		err = errors.New("decrypt with PaymentCryptoData Key Failed: " + "PaymentCryptoData Client is Required")
		return "", err
	}

	if len(k.KeyArn) <= 0 {
		err = errors.New("decrypt with PaymentCryptoData Key Failed: " + "PaymentCryptoData Key Name is Required")
		return "", err
	}

	if len(cipherText) <= 0 {
		err = errors.New("decrypt with PaymentCryptoData Key Failed: " + "Cipher Text is Required")
		return "", err
	}

	//prepare input
	var decryptedOutput *pycryptoData.DecryptDataOutput
	var e error

	decryptInput := &pycryptoData.DecryptDataInput{
		CipherText:           aws.String(cipherText),
		DecryptionAttributes: decryptionAttributes,
		KeyIdentifier:        aws.String(k.KeyArn),
	}

	if segCtx == nil {
		decryptedOutput, e = k.pyDataClient.DecryptData(decryptInput)
	} else {
		decryptedOutput, e = k.pyDataClient.DecryptDataWithContext(segCtx, decryptInput)
	}

	if e != nil {
		err = errors.New("decrypt with PaymentCryptoData Key Failed: " + e.Error())
		return "", err
	}

	if decryptedOutput != nil {
		plainText = aws.StringValue(decryptedOutput.PlainText)
	}

	return plainText, nil
}

func (k *PaymentCryptoData) encryptRSA(plainText string, paddingType string) (cipherText string, err error) {
	encryptionAttributes := &pycryptoData.EncryptionDecryptionAttributes{
		Asymmetric: &pycryptoData.AsymmetricEncryptionAttributes{
			PaddingType: awsNilString(paddingType),
		},
	}
	return k.encrypt(plainText, encryptionAttributes)
}

func (k *PaymentCryptoData) decryptRSA(cipherText string, paddingType string) (plainText string, err error) {
	decryptionAttributes := &pycryptoData.EncryptionDecryptionAttributes{
		Asymmetric: &pycryptoData.AsymmetricEncryptionAttributes{
			PaddingType: awsNilString(paddingType),
		},
	}
	return k.decrypt(cipherText, decryptionAttributes)
}

// EncryptViaRSAPKCS1 the padding scheme is PKCS1, the plainText & cipherText is both hex encoded string
func (k *PaymentCryptoData) EncryptViaRSAPKCS1(plainText string) (cipherText string, err error) {
	return k.encryptRSA(plainText, pycryptoData.PaddingTypePkcs1)
}

// EncryptViaRSANone the padding scheme is None, the plainText & cipherText is both hex encoded string
func (k *PaymentCryptoData) EncryptViaRSANone(plainText string) (cipherText string, err error) {
	return k.encryptRSA(plainText, "")
}

// EncryptViaRSAOEAPSHA512 the padding scheme is OEAP SHA512, the plainText & cipherText is both hex encoded string
func (k *PaymentCryptoData) EncryptViaRSAOEAPSHA512(plainText string) (cipherText string, err error) {
	return k.encryptRSA(plainText, pycryptoData.PaddingTypeOaepSha512)
}

// EncryptViaRSAOEAPSHA256 the padding scheme is OEAP SHA256, the plainText & cipherText is both hex encoded string
func (k *PaymentCryptoData) EncryptViaRSAOEAPSHA256(plainText string) (cipherText string, err error) {
	return k.encryptRSA(plainText, pycryptoData.PaddingTypeOaepSha256)
}

// EncryptViaRSAOEAPSHA128 the padding scheme is OEAP SHA1, the plainText & cipherText is both hex encoded string
func (k *PaymentCryptoData) EncryptViaRSAOEAPSHA128(plainText string) (cipherText string, err error) {
	return k.encryptRSA(plainText, pycryptoData.PaddingTypeOaepSha1)
}

// DecryptViaRSAPKCS1 the padding scheme is PKCS1, the plainText & cipherText is both hex encoded string
func (k *PaymentCryptoData) DecryptViaRSAPKCS1(cipherText string) (plainText string, err error) {
	return k.decryptRSA(cipherText, pycryptoData.PaddingTypePkcs1)
}

// DecryptViaRSANone the padding scheme is None, the plainText & cipherText is both hex encoded string
func (k *PaymentCryptoData) DecryptViaRSANone(cipherText string) (plainText string, err error) {
	return k.decryptRSA(cipherText, "")
}

// DecryptViaRSAOEAPSHA512 the padding scheme is OEAP SHA512, the plainText & cipherText is both hex encoded string
func (k *PaymentCryptoData) DecryptViaRSAOEAPSHA512(cipherText string) (plainText string, err error) {
	return k.decryptRSA(cipherText, pycryptoData.PaddingTypeOaepSha512)
}

// DecryptViaRSAOEAPSHA256 the padding scheme is OEAP SHA256, the plainText & cipherText is both hex encoded string
func (k *PaymentCryptoData) DecryptViaRSAOEAPSHA256(cipherText string) (plainText string, err error) {
	return k.decryptRSA(cipherText, pycryptoData.PaddingTypeOaepSha256)
}

// DecryptViaRSAOEAPSHA128 the padding scheme is OEAP SHA1, the plainText & cipherText is both hex encoded string
func (k *PaymentCryptoData) DecryptViaRSAOEAPSHA128(cipherText string) (plainText string, err error) {
	return k.decryptRSA(cipherText, pycryptoData.PaddingTypeOaepSha1)
}

// ----------------------------------------------------------------------------------------------------------------
// PaymentCryptography Data encrypt/decrypt via AES functions
// ----------------------------------------------------------------------------------------------------------------

func (k *PaymentCryptoData) encryptAES(plainText string, iv, mode string) (cipherText string, err error) {
	encryptionAttributes := &pycryptoData.EncryptionDecryptionAttributes{
		Symmetric: &pycryptoData.SymmetricEncryptionAttributes{
			InitializationVector: awsNilString(iv),
			Mode:                 aws.String(mode),
			PaddingType:          nil,
		},
	}
	return k.encrypt(plainText, encryptionAttributes)
}

func (k *PaymentCryptoData) decryptAES(cipherText string, iv, mode string) (plainText string, err error) {
	decryptionAttributes := &pycryptoData.EncryptionDecryptionAttributes{
		Symmetric: &pycryptoData.SymmetricEncryptionAttributes{
			InitializationVector: awsNilString(iv),
			Mode:                 aws.String(mode),
			PaddingType:          nil,
		},
	}
	return k.decrypt(cipherText, decryptionAttributes)
}

func (k *PaymentCryptoData) EncryptViaAesECB(plainText string) (cipherText string, err error) {
	return k.encryptAES(plainText, "", pycryptoData.EncryptionModeEcb)
}
func (k *PaymentCryptoData) EncryptViaAesCBC(plainText, iv string) (cipherText string, err error) {
	return k.encryptAES(plainText, iv, pycryptoData.EncryptionModeCbc)
}
func (k *PaymentCryptoData) EncryptViaAesCFB(plainText, iv string) (cipherText string, err error) {
	return k.encryptAES(plainText, iv, pycryptoData.EncryptionModeCfb)
}
func (k *PaymentCryptoData) EncryptViaAesOFB(plainText, iv string) (cipherText string, err error) {
	return k.encryptAES(plainText, iv, pycryptoData.EncryptionModeOfb)
}

func (k *PaymentCryptoData) DecryptViaAesECB(cipherText string) (plainText string, err error) {
	return k.decryptAES(cipherText, "", pycryptoData.EncryptionModeEcb)
}
func (k *PaymentCryptoData) DecryptViaAesCBC(cipherText, iv string) (plainText string, err error) {
	return k.decryptAES(cipherText, iv, pycryptoData.EncryptionModeCbc)
}
func (k *PaymentCryptoData) DecryptViaAesCFB(cipherText, iv string) (plainText string, err error) {
	return k.decryptAES(cipherText, iv, pycryptoData.EncryptionModeCfb)
}
func (k *PaymentCryptoData) DecryptViaAesOFB(cipherText, iv string) (plainText string, err error) {
	return k.decryptAES(cipherText, iv, pycryptoData.EncryptionModeOfb)
}
