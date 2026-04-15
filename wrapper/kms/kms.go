package kms

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
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	util "github.com/aldelo/common"
	"github.com/aldelo/common/crypto"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
)

// defaultKMSCallTimeout bounds the duration of any single KMS SDK call
// that the wrapper would otherwise invoke without a deadline.
//
// SP-008 P2-CMN-2 (2026-04-15): every KMS wrapper method previously
// branched on `segCtx == nil` and called the no-context SDK method
// (cli.Encrypt, cli.Decrypt, cli.Sign, ...) on the nil path. Those
// methods use context.Background() internally, which means a hung AWS
// endpoint could block the caller indefinitely — the wrapper had no
// default upper bound unless the application had xray enabled (which
// provided segCtx via seg.Ctx) or passed its own deadline. 30s was
// chosen to be long enough for any legitimate KMS operation (the
// heaviest are CreateKey/ScheduleKeyDeletion, which usually complete
// in <2s) and short enough to fail fast when AWS is unreachable.
const defaultKMSCallTimeout = 30 * time.Second

// ensureKMSCtx normalizes segCtx into a context suitable for every
// cli.*WithContext SDK call in this package. If segCtx is non-nil
// (the xray-enabled path populates it from seg.Ctx) it is returned
// as-is with a no-op cancel; if segCtx is nil we mint a fresh
// context.Background() wrapped in WithTimeout(defaultKMSCallTimeout)
// so the caller always has an upper bound. Callers MUST invoke the
// returned cancel function before returning from the enclosing method
// (defer or call explicitly after the SDK call returns). Passing the
// no-op cancel through to callers keeps the call-site shape uniform.
func ensureKMSCtx(segCtx context.Context) (context.Context, context.CancelFunc) {
	if segCtx != nil {
		return segCtx, func() {}
	}
	return context.WithTimeout(context.Background(), defaultKMSCallTimeout)
}

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// KMS struct encapsulates the AWS KMS access functionality
type KMS struct {
	// define the AWS region that KMS is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// define kms key name
	AesKmsKeyName       string
	RsaKmsKeyName       string
	SignatureKmsKeyName string

	// optional: override AWS endpoint URL for testing (e.g., LocalStack)
	CustomEndpoint string

	// store aws session object
	sess *session.Session

	// store kms client object
	kmsClient *kms.KMS

	_parentSegment *xray.XRayParentSegment

	mu sync.RWMutex
}

// internal helpers for safe access
func (k *KMS) setSessionAndClient(sess *session.Session, cli *kms.KMS) {
	k.mu.Lock()
	k.sess = sess
	k.kmsClient = cli
	k.mu.Unlock()
}

func (k *KMS) getClient() (*kms.KMS, error) {
	k.mu.RLock()
	cli := k.kmsClient
	k.mu.RUnlock()

	if cli == nil {
		return nil, errors.New("KMS Client is Required")
	}
	return cli, nil
}

func (k *KMS) getParentSegment() *xray.XRayParentSegment {
	k.mu.RLock()
	ps := k._parentSegment
	k.mu.RUnlock()
	return ps
}

func (k *KMS) getAWSRegion() awsregion.AWSRegion {
	k.mu.RLock()
	ar := k.AwsRegion
	k.mu.RUnlock()
	return ar
}

func (k *KMS) getHttpOptions() *awshttp2.HttpClientSettings {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.HttpOptions
}

func (k *KMS) getAesKeyName() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.AesKmsKeyName
}

func (k *KMS) getRsaKeyName() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.RsaKmsKeyName
}

func (k *KMS) getSigKeyName() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.SignatureKmsKeyName
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the KMS service
func (k *KMS) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if k == nil {
		return errors.New("KMS receiver is nil")
	}
	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			k.UpdateParentSegment(parentSegment[0])
		}

		seg := xray.NewSegment("KMS-Connect", k.getParentSegment())
		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KDS-AWS-Region", k.getAWSRegion())

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()

		err = k.connectInternal()

		if err == nil {
			cli, _ := k.getClient()
			if cli != nil {
				awsxray.AWS(cli.Client)
			}
		}

		return err
	} else {
		return k.connectInternal()
	}
}

// Connect will establish a connection to the KMS service
func (k *KMS) connectInternal() error {
	// clean up prior session reference
	k.setSessionAndClient(nil, nil)

	region := k.getAWSRegion()
	if !region.Valid() || region == awsregion.UNKNOWN {
		return errors.New("Connect To KMS Failed: (AWS Session Error) " + "Region is Required")
	}

	httpOpts := k.getHttpOptions()
	if httpOpts == nil {
		httpOpts = &awshttp2.HttpClientSettings{}
	}

	// use custom http2 client
	h2 := &awshttp2.AwsHttp2Client{
		Options: httpOpts,
	}

	httpCli, httpErr := h2.NewHttp2Client()
	if httpErr != nil {
		return errors.New("Connect to KMS Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection and keep session object in struct
	cfg := &aws.Config{
		Region:     aws.String(region.Key()),
		HTTPClient: httpCli,
	}

	k.mu.RLock()
	customEP := k.CustomEndpoint
	k.mu.RUnlock()

	if len(customEP) > 0 {
		cfg.Endpoint = aws.String(customEP)
	}

	sess, err := session.NewSession(cfg)
	if err != nil {
		// aws session error
		return errors.New("Connect To KMS Failed: (AWS Session Error) " + err.Error())
	}

	cli := kms.New(sess)
	if cli == nil {
		return errors.New("Connect To KMS Failed: (New KMS Client Connection) " + "Connection Object Nil")
	}

	// store session and client atomically under lock
	k.setSessionAndClient(sess, cli)
	return nil
}

// Disconnect will disjoin from aws session by clearing it
func (k *KMS) Disconnect() {
	if k == nil {
		return
	}
	k.setSessionAndClient(nil, nil)
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (k *KMS) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	if k == nil {
		return
	}
	k.mu.Lock()
	k._parentSegment = parentSegment
	k.mu.Unlock()
}

// ----------------------------------------------------------------------------------------------------------------
// kms-cmk encrypt/decrypt via aes 256 functions
// ----------------------------------------------------------------------------------------------------------------

// EncryptViaCmkAes256 will use kms cmk to encrypt plainText using aes 256 symmetric kms cmk key, and return cipherText string,
// the cipherText can only be decrypted with aes 256 symmetric kms cmk key
func (k *KMS) EncryptViaCmkAes256(plainText string) (cipherText string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	// SP-008 P3-CMN-3 (2026-04-15): hoist config reads into a single
	// RLock block at function entry. The prior form invoked
	// getParentSegment / getAesKeyName / getClient / getAesKeyName as
	// 4 independent RLock pairs per call (~40 ns overhead, 0 allocs).
	// Snapshotting once eliminates three of those RLock pairs and also
	// removes the torn-read hazard where a mid-call config update
	// between getter calls could surface stale client + new key name
	// (or vice versa). The xray defer closure now reads the captured
	// `aesKeyName` local instead of calling k.getAesKeyName() again.
	k.mu.RLock()
	cli := k.kmsClient
	aesKeyName := k.AesKmsKeyName
	parentSeg := k._parentSegment
	k.mu.RUnlock()

	seg := xray.NewSegmentNullable("KMS-EncryptViaCmkAes256", parentSeg)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-EncryptViaCmkAes256-AES-KMS-KeyName", aesKeyName)
			_ = seg.SafeAddMetadata("KMS-EncryptViaCmkAes256-PlainText-Length", len(plainText))
			_ = seg.SafeAddMetadata("KMS-EncryptViaCmkAes256-Result-CipherText-Length", len(cipherText))

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	if cli == nil {
		err = errors.New("EncryptViaCmkAes256 with KMS CMK Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(aesKeyName) <= 0 {
		err = errors.New("EncryptViaCmkAes256 with KMS CMK Failed: " + "AES KMS Key Name is Required")
		return "", err
	}

	if len(plainText) <= 0 {
		err = errors.New("EncryptViaCmkAes256 with KMS CMK Failed: " + "PlainText is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + aesKeyName

	// encrypt symmetric using kms cmk
	var encryptedOutput *kms.EncryptOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil so a hung endpoint cannot block
	// the caller indefinitely.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	encryptedOutput, e = cli.EncryptWithContext(kmsCtx,
		&kms.EncryptInput{
			EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
			KeyId:               aws.String(keyId),
			Plaintext:           []byte(plainText),
		})
	kmsCancel()

	if e != nil {
		err = errors.New("EncryptViaCmkAes256 with KMS CMK Failed: (Symmetric Encrypt) " + e.Error())
		return "", err
	}

	if encryptedOutput == nil {
		err = errors.New("EncryptViaCmkAes256 Failed: output is nil")
		return "", err
	}

	// return encrypted cipher text blob
	cipherText = util.ByteToHex(encryptedOutput.CiphertextBlob)
	return cipherText, nil
}

// ReEncryptViaCmkAes256 will re-encrypt sourceCipherText using the new targetKmsKeyName via kms, (must be targeting aes 256 key)
// the re-encrypted cipherText is then returned
func (k *KMS) ReEncryptViaCmkAes256(sourceCipherText string, targetKmsKeyName string) (targetCipherText string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-ReEncryptViaCmkAes256", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-ReEncryptViaCmkAes256-Source-AES-KMS-KeyName", k.getAesKeyName())
			_ = seg.SafeAddMetadata("KMS-ReEncryptViaCmkAes256-Target-AES-KMS-KeyName", targetKmsKeyName)
			_ = seg.SafeAddMetadata("KMS-ReEncryptViaCmkAes256-SourceCipherText-Length", len(sourceCipherText))
			_ = seg.SafeAddMetadata("KMS-ReEncryptViaCmkAes256-Result-Target-CipherText-Length", len(targetCipherText))

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: " + cliErr.Error())
		return "", err
	}

	aesKeyName := k.getAesKeyName()
	if len(aesKeyName) <= 0 {
		err = errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: " + "AES KMS Key Name is Required")
		return "", err
	}

	if len(sourceCipherText) <= 0 {
		err = errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: " + "Source CipherText is Required")
		return "", err
	}

	if len(targetKmsKeyName) <= 0 {
		err = errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: " + "Target KMS Key Name is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + aesKeyName

	// convert hex to bytes
	cipherBytes, ce := util.HexToByte(sourceCipherText)

	if ce != nil {
		err = errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: (Unmarshal Source CipherText Hex To Byte) " + ce.Error())
		return "", err
	}

	// re-encrypt symmetric kms cmk
	var reEncryptOutput *kms.ReEncryptOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	reEncryptOutput, e = cli.ReEncryptWithContext(kmsCtx,
		&kms.ReEncryptInput{
			SourceEncryptionAlgorithm:      aws.String("SYMMETRIC_DEFAULT"),
			SourceKeyId:                    aws.String(keyId),
			DestinationEncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
			DestinationKeyId:               aws.String("alias/" + targetKmsKeyName),
			CiphertextBlob:                 cipherBytes,
		})
	kmsCancel()

	if e != nil {
		err = errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: (Symmetric ReEncrypt) " + e.Error())
		return "", err
	}

	if reEncryptOutput == nil {
		err = errors.New("ReEncryptViaCmkAes256 Failed: output is nil")
		return "", err
	}

	// return encrypted cipher text blob
	targetCipherText = util.ByteToHex(reEncryptOutput.CiphertextBlob)
	return targetCipherText, nil
}

// DecryptViaCmkAes256 will use kms cmk to decrypt cipherText using symmetric aes 256 kms cmk key, and return plainText string,
// the cipherText can only be decrypted with the symmetric aes 256 kms cmk key
func (k *KMS) DecryptViaCmkAes256(cipherText string) (plainText string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}

	// SP-008 P3-CMN-3 (2026-04-15): single RLock snapshot at function entry
	// replaces 3 independent getter RLock pairs and closes the torn-read
	// hazard where a mid-call config update could surface stale client +
	// new key name (or vice versa). See EncryptViaCmkAes256 for rationale.
	k.mu.RLock()
	cli := k.kmsClient
	aesKeyName := k.AesKmsKeyName
	parentSeg := k._parentSegment
	k.mu.RUnlock()

	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-DecryptViaCmkAes256", parentSeg)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-DecryptViaCmkAes256-AES-KMS-KeyName", aesKeyName)
			_ = seg.SafeAddMetadata("KMS-DecryptViaCmkAes256-CipherText-Length", len(cipherText))
			_ = seg.SafeAddMetadata("KMS-DecryptViaCmkAes256-Result-PlainText-Length", len(plainText))

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	if cli == nil {
		err = errors.New("DecryptViaCmkAes256 with KMS CMK Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(aesKeyName) <= 0 {
		err = errors.New("DecryptViaCmkAes256 with KMS CMK Failed: " + "AES KMS Key Name is Required")
		return "", err
	}

	if len(cipherText) <= 0 {
		err = errors.New("DecryptViaCmkAes256 with KMS CMK Failed: " + "Cipher Text is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + aesKeyName
	cipherBytes, ce := util.HexToByte(cipherText)

	if ce != nil {
		err = errors.New("DecryptViaCmkAes256 with KMS CMK Failed: (Unmarshal CipherText Hex To Byte) " + ce.Error())
		return "", err
	}

	// decrypt symmetric using kms cmk
	var decryptedOutput *kms.DecryptOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	decryptedOutput, e = cli.DecryptWithContext(kmsCtx,
		&kms.DecryptInput{
			EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
			KeyId:               aws.String(keyId),
			CiphertextBlob:      cipherBytes,
		})
	kmsCancel()

	if e != nil {
		err = errors.New("DecryptViaCmkAes256 with KMS CMK Failed: (Symmetric Decrypt) " + e.Error())
		return "", err
	}

	if decryptedOutput == nil {
		err = errors.New("DecryptViaCmkAes256 Failed: output is nil")
		return "", err
	}

	// return decrypted cipher text blob
	plainText = string(decryptedOutput.Plaintext)
	return plainText, nil
}

// ----------------------------------------------------------------------------------------------------------------
// kms-cmk encrypt/decrypt via rsa 2048 public/private key functions
// ----------------------------------------------------------------------------------------------------------------

// GenerateEncryptionDecryptionKeyRsa2048 will generate a new rsa 2048 key pair using kms cmk, and return the creation output
// the key pair can only be used for rsa 2048 asymmetric encryption/decryption
// keyName = the Alias name to create
// keyPolicy = the key policy json string to apply
//
//	keyPolicy := map[string]interface{}{
//		"Id":      "key-consolepolicy-3",
//		"Version": "2012-10-17",
//		"Statement": []map[string]interface{}{
//			{
//				"Effect":    "Allow",
//				"Principal": map[string]interface{}{"AWS": "arn:aws:iam::***************:user/xxxxxxxxxxxxxxxxxxx"}, // Replace with your account ID
//				"Action":    "kms:*",
//				"Resource":  "*",
//			},
//			{
//				"Sid":       "Allow access for Key Administrators",
//				"Effect":    "Allow",
//				"Principal": map[string]interface{}{"AWS": "arn:aws:iam::***************:user/xxxxxxxxxxxxxxxxxxx"}, // Replace with your account ID
//				"Action":    "kms:*",
//				"Resource":  "*",
//			},
//			{
//				"Sid":       "Allow access for Key Administrators",
//				"Effect":    "Allow",
//				"Principal": map[string]interface{}{"AWS": "arn:aws:iam::***************:user/xxxxxxxxxxxxxxxxxxx"},
//				"Action": []string{
//					"kms:Create*",
//					"kms:Describe*",
//					"kms:Enable*",
//					"kms:List*",
//					"kms:Put*",
//					"kms:Update*",
//					"kms:Revoke*",
//					"kms:Disable*",
//					"kms:Get*",
//					"kms:Delete*",
//					"kms:TagResource",
//					"kms:UntagResource",
//					"kms:ScheduleKeyDeletion",
//					"kms:CancelKeyDeletion",
//				},
//				"Resource": "*",
//			},
//			{
//				"Sid":       "Allow use of the key",
//				"Effect":    "Allow",
//				"Principal": map[string]interface{}{"AWS": "arn:aws:iam::***************:user/xxxxxxxxxxxxxxxxxxx"},
//				"Action": []string{
//					"kms:Encrypt",
//					"kms:Decrypt",
//					"kms:ReEncrypt*",
//					"kms:DescribeKey",
//					"kms:GetPublicKey",
//				},
//				"Resource": "*",
//			},
//			{
//				"Sid":       "Allow attachment of persistent resources",
//				"Effect":    "Allow",
//				"Principal": map[string]interface{}{"AWS": "arn:aws:iam::***************:user/xxxxxxxxxxxxxxxxxxx"},
//				"Action": []string{
//					"kms:CreateGrant",
//					"kms:ListGrants",
//					"kms:RevokeGrant",
//				},
//				"Resource": "*",
//				"Condition": map[string]interface{}{
//					"Bool": map[string]interface{}{
//						"kms:GrantIsForAWSResource": "true",
//					},
//				},
//			},
//		},
//	}
func (k *KMS) GenerateEncryptionDecryptionKeyRsa2048(keyName string, keyPolicyJSON string) (encryptedOutput *kms.CreateKeyOutput, err error) {
	if k == nil {
		return nil, errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-GenerateDataKeyRsa2048", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-GenerateEncryptionDecryptionKeyRsa2048-RSA-KMS-KeyName", k.getRsaKeyName())
			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	if strings.TrimSpace(keyName) == "" {
		return nil, errors.New("GenerateEncryptionDecryptionKeyRsa2048 with KMS CMK Failed: keyName (alias) is required")
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("GenerateEncryptionDecryptionKeyRsa2048 with KMS CMK Failed: " + cliErr.Error())
		return nil, err
	}

	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	encryptedOutput, e = cli.CreateKeyWithContext(kmsCtx, &kms.CreateKeyInput{
		Description: aws.String("Common RSA 2048 Key Creation"),
		KeySpec:     aws.String(kms.KeySpecRsa2048),
		KeyUsage:    aws.String(kms.KeyUsageTypeEncryptDecrypt),
		Policy:      aws.String(string(keyPolicyJSON)),
	})
	kmsCancel()

	if e != nil {
		err = errors.New("GenerateEncryptionDecryptionKeyRsa2048 with KMS CMK Failed: (RSA Key Create Fail) " + e.Error())
		return nil, err
	}

	if encryptedOutput == nil || encryptedOutput.KeyMetadata == nil || encryptedOutput.KeyMetadata.KeyId == nil {
		err = errors.New("GenerateEncryptionDecryptionKeyRsa2048 with KMS CMK Failed: CreateKeyOutput or KeyMetadata is nil")
		return nil, err
	}

	aliasName := "alias/" + keyName // Change to your desired alias name

	aliasInput := &kms.CreateAliasInput{
		AliasName:   aws.String(aliasName),
		TargetKeyId: encryptedOutput.KeyMetadata.KeyId,
	}

	_, err = cli.CreateAlias(aliasInput)
	if err != nil {
		// Best-effort cleanup: schedule the orphaned key for deletion
		_, _ = cli.ScheduleKeyDeletion(&kms.ScheduleKeyDeletionInput{
			KeyId:               encryptedOutput.KeyMetadata.KeyId,
			PendingWindowInDays: aws.Int64(7),
		})
		return nil, err
	}

	// return creation data
	return encryptedOutput, nil
}

func (k *KMS) GenerateSignVerifyKeyRsa2048(keyName string, keyPolicy interface{}) (encryptedOutput *kms.CreateKeyOutput, err error) {
	if k == nil {
		return nil, errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-GenerateSignVerifyKeyRsa2048", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-GenerateSignVerifyKeyRsa2048-RSA-KMS-KeyName", k.getRsaKeyName())
			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	if strings.TrimSpace(keyName) == "" {
		return nil, errors.New("GenerateSignVerifyKeyRsa2048 with KMS CMK Failed: keyName (alias) is required")
	}

	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("GenerateSignVerifyKeyRsa2048 with KMS CMK Failed: " + cliErr.Error())
		return nil, err
	}

	keyPolicyJSON, err := json.Marshal(keyPolicy)
	if err != nil {
		return nil, err
	}

	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	encryptedOutput, e = cli.CreateKeyWithContext(kmsCtx, &kms.CreateKeyInput{
		Description: aws.String("Common RSA 2048 Key Creation"),
		KeySpec:     aws.String(kms.KeySpecRsa2048),
		KeyUsage:    aws.String(kms.KeyUsageTypeSignVerify),
		Policy:      aws.String(string(keyPolicyJSON)),
	})
	kmsCancel()

	if e != nil {
		err = errors.New("GenerateSignVerifyKeyRsa2048 with KMS CMK Failed: (RSA Key Create Fail) " + e.Error())
		return nil, err
	}

	if encryptedOutput == nil || encryptedOutput.KeyMetadata == nil || encryptedOutput.KeyMetadata.KeyId == nil {
		err = errors.New("GenerateSignVerifyKeyRsa2048 with KMS CMK Failed: CreateKeyOutput or KeyMetadata is nil")
		return nil, err
	}

	aliasName := "alias/" + keyName // Change to your desired alias name

	aliasInput := &kms.CreateAliasInput{
		AliasName:   aws.String(aliasName),
		TargetKeyId: encryptedOutput.KeyMetadata.KeyId,
	}

	_, err = cli.CreateAlias(aliasInput)
	if err != nil {
		// Best-effort cleanup: schedule the orphaned key for deletion
		_, _ = cli.ScheduleKeyDeletion(&kms.ScheduleKeyDeletionInput{
			KeyId:               encryptedOutput.KeyMetadata.KeyId,
			PendingWindowInDays: aws.Int64(7),
		})
		return nil, err
	}

	// return creation data
	return encryptedOutput, nil
}

func (k *KMS) KeyDeleteWithAlias(alias string, PendingWindowInDays int64) (output *kms.ScheduleKeyDeletionOutput, err error) {
	if k == nil {
		return nil, errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-KeyDeleteWithAlias", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-KeyDeleteWithAlias-RSA-KMS-KeyName", k.getRsaKeyName())
			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("KeyDeleteWithAlias with KMS CMK Failed: " + cliErr.Error())
		return nil, err
	}

	if alias == "" {
		err = errors.New("KeyDeleteWithAlias with KMS CMK Failed: " + "Alias is Required")
		return nil, err
	}

	if PendingWindowInDays < 7 || PendingWindowInDays > 30 {
		err = errors.New("KeyDeleteWithAlias with KMS CMK Failed: PendingWindowInDays must be between 7 and 30 (inclusive)")
		return nil, err
	}

	aliasName := "alias/" + alias // Change to your desired alias name

	descOut, e1 := cli.DescribeKey(&kms.DescribeKeyInput{
		KeyId: aws.String(aliasName),
	})
	if e1 != nil {
		return nil, fmt.Errorf("KeyDeleteWithAlias with KMS CMK Failed: describe key: %w", e1)
	}
	if descOut == nil || descOut.KeyMetadata == nil || descOut.KeyMetadata.KeyId == nil {
		return nil, errors.New("KeyDeleteWithAlias with KMS CMK Failed: DescribeKey output or KeyMetadata is nil")
	}
	keyId := descOut.KeyMetadata.KeyId
	if *keyId == "" {
		return nil, errors.New("KeyDeleteWithAlias with KMS CMK Failed: key id is empty after DescribeKey")
	}

	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	output, e = cli.ScheduleKeyDeletionWithContext(kmsCtx, &kms.ScheduleKeyDeletionInput{
		KeyId:               keyId,
		PendingWindowInDays: aws.Int64(PendingWindowInDays),
	})
	kmsCancel()

	if e != nil {
		err = errors.New("KeyDeleteWithAlias with KMS CMK Failed: (RSA Key Delete Fail) " + e.Error())
		return nil, err
	}
	_, e = cli.DeleteAlias(&kms.DeleteAliasInput{
		AliasName: aws.String(aliasName),
	})
	if e != nil {
		err = errors.New("KeyDeleteWithAlias with KMS CMK Failed: (RSA Key Delete Fail) " + e.Error())
		return nil, err
	}

	return output, nil
}

func (k *KMS) KeyDeleteWithArnID(arn string, PendingWindowInDays int64) (output *kms.ScheduleKeyDeletionOutput, err error) {
	if k == nil {
		return nil, errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-KeyDeleteWithArnID", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-KeyDeleteWithArnID-RSA-KMS-KeyName", k.getRsaKeyName())
			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("KeyDeleteWithArnID with KMS CMK Failed: " + cliErr.Error())
		return nil, err
	}

	if arn == "" {
		err = errors.New("KeyDeleteWithArnID with KMS CMK Failed: " + "ARN is Required")
		return nil, err
	}

	if PendingWindowInDays < 7 || PendingWindowInDays > 30 {
		err = errors.New("KeyDeleteWithArnID with KMS CMK Failed: PendingWindowInDays must be between 7 and 30 (inclusive)")
		return nil, err
	}

	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	output, e = cli.ScheduleKeyDeletionWithContext(kmsCtx, &kms.ScheduleKeyDeletionInput{
		KeyId:               aws.String(arn),
		PendingWindowInDays: aws.Int64(PendingWindowInDays),
	})
	kmsCancel()

	if e != nil {
		err = errors.New("KeyDeleteWithArnID with KMS CMK Failed: (RSA Key Delete Fail) " + e.Error())
		return nil, err
	}

	return output, nil
}

// EncryptViaCmkRsa2048 will use kms cmk to encrypt plainText with asymmetric rsa 2048 kms cmk public key, and return cipherText string,
// the cipherText can only be decrypted with the paired asymmetric rsa 2048 kms cmk private key
//
// *** To Encrypt using Public Key Outside of KMS ***
//  1. Copy Public Key from AWS KMS for the given RSA CMK
//  2. Using External RSA Public Key Crypto Encrypt Function with the given Public Key to Encrypt
func (k *KMS) EncryptViaCmkRsa2048(plainText string) (cipherText string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}

	// SP-008 P3-CMN-3 (2026-04-15): single RLock snapshot at function entry
	// replaces 3 independent getter RLock pairs and closes the torn-read
	// hazard where a mid-call config update could surface stale client +
	// new key name (or vice versa). See EncryptViaCmkAes256 for rationale.
	k.mu.RLock()
	cli := k.kmsClient
	rsaKeyName := k.RsaKmsKeyName
	parentSeg := k._parentSegment
	k.mu.RUnlock()

	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-EncryptViaCmkRsa2048", parentSeg)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-EncryptViaCmkRsa2048-RSA-KMS-KeyName", rsaKeyName)
			_ = seg.SafeAddMetadata("KMS-EncryptViaCmkRsa2048-PlainText-Length", len(plainText))
			_ = seg.SafeAddMetadata("KMS-EncryptViaCmkRsa2048-Result-CipherText-Length", len(cipherText))

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	if cli == nil {
		err = errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(rsaKeyName) <= 0 {
		err = errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "RSA KMS Key Name is Required")
		return "", err
	}

	if len(plainText) <= 0 {
		err = errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "PlainText is Required")
		return "", err
	}

	if len(plainText) > 190 {
		err = errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "PlainText Cannot Exceed 190 Bytes")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + rsaKeyName

	// encrypt asymmetric using kms cmk
	var encryptedOutput *kms.EncryptOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	encryptedOutput, e = cli.EncryptWithContext(kmsCtx,
		&kms.EncryptInput{
			EncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
			KeyId:               aws.String(keyId),
			Plaintext:           []byte(plainText),
		})
	kmsCancel()

	if e != nil {
		err = errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: (Asymmetric Encrypt) " + e.Error())
		return "", err
	}

	if encryptedOutput == nil {
		err = errors.New("EncryptViaCmkRsa2048 Failed: output is nil")
		return "", err
	}

	// return encrypted cipher text blob
	cipherText = util.ByteToHex(encryptedOutput.CiphertextBlob)
	return cipherText, nil
}

func (k *KMS) GetRSAPublicKey(alias string) (output *kms.GetPublicKeyOutput, err error) {
	if k == nil {
		return nil, errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-GetRSAPublicKey", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-GetRSAPublicKey-RSA-KMS-KeyName", k.getRsaKeyName())
			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("GetRSAPublicKey with KMS CMK Failed: " + cliErr.Error())
		return nil, err
	}

	if alias == "" {
		err = errors.New("GetRSAPublicKey with KMS CMK Failed: " + "Alias is Required")
		return nil, err
	}

	aliasName := "alias/" + alias // Change to your desired alias name

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	output, err = cli.GetPublicKeyWithContext(kmsCtx, &kms.GetPublicKeyInput{
		KeyId: aws.String(aliasName),
	})
	kmsCancel()

	if err != nil {
		return nil, fmt.Errorf("GetRSAPublicKey with KMS CMK Failed: (RSA GetPublicKey Fail) %w", err)
	}

	return output, nil
}

// ReEncryptViaCmkRsa2048 will re-encrypt sourceCipherText using the new targetKmsKeyName via kms, (must be targeting rsa 2048 key)
// the re-encrypted cipherText is then returned
func (k *KMS) ReEncryptViaCmkRsa2048(sourceCipherText string, targetKmsKeyName string) (targetCipherText string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-ReEncryptViaCmkRsa2048", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-ReEncryptViaCmkRsa2048-Source-RSA-KMS-KeyName", k.getRsaKeyName())
			_ = seg.SafeAddMetadata("KMS-ReEncryptViaCmkRsa2048-Target-RSA-KMS-KeyName", targetKmsKeyName)
			_ = seg.SafeAddMetadata("KMS-ReEncryptViaCmkRsa2048-Source-CipherText-Length", len(sourceCipherText))
			_ = seg.SafeAddMetadata("KMS-ReEncryptViaCmkRsa2048-Result-Target-CipherText-Length", len(targetCipherText))

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: " + cliErr.Error())
		return "", err
	}

	rsaKeyName := k.getRsaKeyName()
	if len(rsaKeyName) <= 0 {
		err = errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: " + "RSA KMS Key Name is Required")
		return "", err
	}

	if len(sourceCipherText) <= 0 {
		err = errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: " + "Source CipherText is Required")
		return "", err
	}

	if len(targetKmsKeyName) <= 0 {
		err = errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: " + "Target KMS Key Name is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + rsaKeyName

	// convert hex to bytes
	cipherBytes, ce := util.HexToByte(sourceCipherText)

	if ce != nil {
		err = errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: (Unmarshal Source CipherText Hex To Byte) " + ce.Error())
		return "", err
	}

	// re-encrypt asymmetric kms cmk
	var reEncryptOutput *kms.ReEncryptOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	reEncryptOutput, e = cli.ReEncryptWithContext(kmsCtx,
		&kms.ReEncryptInput{
			SourceEncryptionAlgorithm:      aws.String("RSAES_OAEP_SHA_256"),
			SourceKeyId:                    aws.String(keyId),
			DestinationEncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
			DestinationKeyId:               aws.String("alias/" + targetKmsKeyName),
			CiphertextBlob:                 cipherBytes,
		})
	kmsCancel()

	if e != nil {
		err = errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: (Asymmetric ReEncrypt) " + e.Error())
		return "", err
	}

	if reEncryptOutput == nil {
		err = errors.New("ReEncryptViaCmkRsa2048 Failed: output is nil")
		return "", err
	}

	// return encrypted cipher text blob
	targetCipherText = util.ByteToHex(reEncryptOutput.CiphertextBlob)
	return targetCipherText, nil
}

// DecryptViaCmkRsa2048 will use kms cmk to decrypt cipherText using asymmetric rsa 2048 kms cmk private key, and return plainText string,
// the cipherText can only be decrypted with the asymmetric rsa 2048 kms cmk private key
func (k *KMS) DecryptViaCmkRsa2048(cipherText string) (plainText string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}

	// SP-008 P3-CMN-3 (2026-04-15): single RLock snapshot at function entry
	// replaces 3 independent getter RLock pairs and closes the torn-read
	// hazard where a mid-call config update could surface stale client +
	// new key name (or vice versa). See EncryptViaCmkAes256 for rationale.
	k.mu.RLock()
	cli := k.kmsClient
	rsaKeyName := k.RsaKmsKeyName
	parentSeg := k._parentSegment
	k.mu.RUnlock()

	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-DecryptViaCmkRsa2048", parentSeg)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-DecryptViaCmkRsa2048-RSA-KMS-KeyName", rsaKeyName)
			_ = seg.SafeAddMetadata("KMS-DecryptViaCmkRsa2048-CipherText-Length", len(cipherText))
			_ = seg.SafeAddMetadata("KMS-DecryptViaCmkRsa2048-Result-PlainText-Length", len(plainText))

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	if cli == nil {
		err = errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(rsaKeyName) <= 0 {
		err = errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: " + "RSA KMS Key Name is Required")
		return "", err
	}

	if len(cipherText) <= 0 {
		err = errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: " + "Cipher Text is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + rsaKeyName
	cipherBytes, ce := util.HexToByte(cipherText)

	if ce != nil {
		err = errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: (Unmarshal CipherText Hex To Byte) " + ce.Error())
		return "", err
	}

	// decrypt symmetric using kms cmk
	var decryptedOutput *kms.DecryptOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	decryptedOutput, e = cli.DecryptWithContext(kmsCtx,
		&kms.DecryptInput{
			EncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
			KeyId:               aws.String(keyId),
			CiphertextBlob:      cipherBytes,
		})
	kmsCancel()

	if e != nil {
		err = errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: (Asymmetric Decrypt) " + e.Error())
		return "", err
	}

	if decryptedOutput == nil {
		err = errors.New("DecryptViaCmkRsa2048 Failed: output is nil")
		return "", err
	}

	// return decrypted cipher text blob
	plainText = string(decryptedOutput.Plaintext)
	return plainText, nil
}

// DecryptViaCmkRsa2048PKCS1 will use kms cmk to decrypt cipherText using asymmetric rsa 2048 kms cmk private key, and return plainText string,
// the cipherText can only be decrypted with the asymmetric rsa 2048 kms cmk private key
func (k *KMS) DecryptViaCmkRsa2048PKCS1(cipherText string) (plainText string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}
	return k.decryptViaCmkRsa2048Base(cipherText, kms.AlgorithmSpecRsaesPkcs1V15)
}

func (k *KMS) decryptViaCmkRsa2048Base(cipherText, encryptionAlgorithm string) (plainText string, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-decryptViaCmkRsa2048Base", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-decryptViaCmkRsa2048Base-RSA-KMS-KeyName", k.getRsaKeyName())
			_ = seg.SafeAddMetadata("KMS-decryptViaCmkRsa2048Base-CipherText-Length", len(cipherText))
			_ = seg.SafeAddMetadata("KMS-decryptViaCmkRsa2048Base-Result-PlainText-Length", len(plainText))

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("decryptViaCmkRsa2048Base with KMS CMK Failed: " + cliErr.Error())
		return "", err
	}

	rsaKeyName := k.getRsaKeyName()
	if len(rsaKeyName) <= 0 {
		err = errors.New("decryptViaCmkRsa2048Base with KMS CMK Failed: " + "RSA KMS Key Name is Required")
		return "", err
	}

	if len(cipherText) <= 0 {
		err = errors.New("decryptViaCmkRsa2048Base with KMS CMK Failed: " + "Cipher Text is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + rsaKeyName
	cipherBytes, ce := util.HexToByte(cipherText)

	if ce != nil {
		err = errors.New("decryptViaCmkRsa2048Base with KMS CMK Failed: (Unmarshal CipherText Hex To Byte) " + ce.Error())
		return "", err
	}

	// decrypt symmetric using kms cmk
	var decryptedOutput *kms.DecryptOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	decryptedOutput, e = cli.DecryptWithContext(kmsCtx,
		&kms.DecryptInput{
			EncryptionAlgorithm: aws.String(encryptionAlgorithm),
			KeyId:               aws.String(keyId),
			CiphertextBlob:      cipherBytes,
		})
	kmsCancel()

	if e != nil {
		err = errors.New("decryptViaCmkRsa2048Base with KMS CMK Failed: (Asymmetric Decrypt) " + e.Error())
		return "", err
	}

	if decryptedOutput == nil {
		err = errors.New("decryptViaCmkRsa2048Base Failed: output is nil")
		return "", err
	}

	// return decrypted cipher text blob
	plainText = string(decryptedOutput.Plaintext)
	return plainText, nil
}

// ----------------------------------------------------------------------------------------------------------------
// kms-cmk sign/verify via rsa 2048 private/public key functions
// ----------------------------------------------------------------------------------------------------------------

// SignViaCmkRsa2048 will sign dataToSign using KMS CMK RSA Sign/Verify Key (Private Key on KMS will be used to securely sign)
func (k *KMS) SignViaCmkRsa2048(dataToSign string) (signature string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-SignViaCmkRsa2048", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-SignViaCmkRsa2048-Signature-KMS-KeyName", k.getSigKeyName())
			_ = seg.SafeAddMetadata("KMS-SignViaCmkRsa2048-DataToSign-Length", len(dataToSign))
			_ = seg.SafeAddMetadata("KMS-SignViaCmkRsa2048-Result-Signature", signature)

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("SignViaCmkRsa2048 with KMS Failed: " + cliErr.Error())
		return "", err
	}

	sigKeyName := k.getSigKeyName()
	if len(sigKeyName) <= 0 {
		err = errors.New("SignViaCmkRsa2048 with KMS Failed: " + "Signature KMS Key Name is Required")
		return "", err
	}

	if len(dataToSign) <= 0 {
		err = errors.New("SignViaCmkRsa2048 with KMS Failed: " + "Data To Sign is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + sigKeyName

	// perform sign action using kms rsa sign/verify cmk
	var signOutput *kms.SignOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	signOutput, e = cli.SignWithContext(kmsCtx,
		&kms.SignInput{
			KeyId:            aws.String(keyId),
			SigningAlgorithm: aws.String("RSASSA_PKCS1_V1_5_SHA_256"),
			MessageType:      aws.String("RAW"),
			Message:          []byte(dataToSign),
		})
	kmsCancel()

	if e != nil {
		err = errors.New("SignViaCmkRsa2048 with KMS Failed: (Sign Action) " + e.Error())
		return "", err
	}

	if signOutput == nil {
		err = errors.New("SignViaCmkRsa2048 Failed: output is nil")
		return "", err
	}

	// return signature
	signature = util.ByteToHex(signOutput.Signature)
	return signature, nil
}

// VerifyViaCmkRsa2048 will verify dataToVerify with signature using KMS CMK RSA Sign/Verify Key (Public Key on KMS will be used securely to verify)
//
// signatureToVerify = prior signed signature in hex to verify against the dataToVerify parameter
//
// *** To Verify using Public Key Outside of KMS ***
//  1. Copy Public Key from AWS KMS for the given RSA CMK
//  2. Using External RSA Public Key Crypto Verify Function with the given Public Key to Verify
func (k *KMS) VerifyViaCmkRsa2048(dataToVerify string, signatureToVerify string) (signatureValid bool, err error) {
	if k == nil {
		return false, errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-VerifyViaCmkRsa2048", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-VerifyViaCmkRsa2048-Signature-KMS-KeyName", k.getSigKeyName())
			_ = seg.SafeAddMetadata("KMS-VerifyViaCmkRsa2048-DataToVerify-Length", len(dataToVerify))
			_ = seg.SafeAddMetadata("KMS-VerifyViaCmkRsa2048-Signature-To-Verify", signatureToVerify)
			_ = seg.SafeAddMetadata("KMS-VerifyViaCmkRsa2048-Result-SignatureValid", signatureValid)

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("VerifyViaCmkRsa2048 with KMS Failed: " + cliErr.Error())
		return false, err
	}

	sigKeyName := k.getSigKeyName()
	if len(sigKeyName) <= 0 {
		err = errors.New("VerifyViaCmkRsa2048 with KMS Failed: " + "Signature KMS Key Name is Required")
		return false, err
	}

	if len(dataToVerify) <= 0 {
		err = errors.New("VerifyViaCmkRsa2048 with KMS Failed: " + "Data To Verify is Required")
		return false, err
	}

	if len(signatureToVerify) <= 0 {
		err = errors.New("VerifyViaCmkRsa2048 with KMS Failed: " + "Signature To Verify is Required")
		return false, err
	}

	// prepare key info
	keyId := "alias/" + sigKeyName
	signatureBytes, ce := util.HexToByte(signatureToVerify)

	if ce != nil {
		err = errors.New("VerifyViaCmkRsa2048 with KMS Failed: (Marshal SignatureToVerify Hex To Byte) " + ce.Error())
		return false, err
	}

	// perform verify action using kms rsa sign/verify cmk
	var verifyOutput *kms.VerifyOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	verifyOutput, e = cli.VerifyWithContext(kmsCtx,
		&kms.VerifyInput{
			KeyId:            aws.String(keyId),
			SigningAlgorithm: aws.String("RSASSA_PKCS1_V1_5_SHA_256"),
			MessageType:      aws.String("RAW"),
			Message:          []byte(dataToVerify),
			Signature:        signatureBytes,
		})
	kmsCancel()

	if e != nil {
		err = errors.New("VerifyViaCmkRsa2048 with KMS Failed: (Verify Action) " + e.Error())
		return false, err
	}

	// return verify result
	if verifyOutput == nil || verifyOutput.SignatureValid == nil {
		err = errors.New("VerifyViaCmkRsa2048 with KMS Failed: VerifyOutput or SignatureValid is nil")
		return false, err
	}
	signatureValid = *verifyOutput.SignatureValid
	return signatureValid, nil
}

// ----------------------------------------------------------------------------------------------------------------
// kms data-key encrypt/decrypt aes 256 functions
// ----------------------------------------------------------------------------------------------------------------

// GenerateDataKeyAes256 will return an encrypted data key generated by kms cmk,
// this data key is encrypted, and able to decrypt only via kms cmk (therefore it is safe to store in memory or at rest)
//
// cipherKey = encrypted data key in hex (must use KMS CMK to decrypt such key)
func (k *KMS) GenerateDataKeyAes256() (cipherKey string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-GenerateDataKeyAes256", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-GenerateDataKeyAes256-AES-KMS-KeyName", k.getAesKeyName())
			_ = seg.SafeAddMetadata("KMS-GenerateDataKeyAes256-Result-CipherKey-Length", len(cipherKey))

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("GenerateDataKeyAes256 with KMS Failed: " + cliErr.Error())
		return "", err
	}

	keyName := k.getAesKeyName()
	if len(keyName) <= 0 {
		err = errors.New("GenerateDataKeyAes256 with KMS Failed: " + "AES KMS Key Name is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + keyName
	keySpec := "AES_256" // always use AES 256
	dataKeyInput := kms.GenerateDataKeyWithoutPlaintextInput{
		KeyId:   aws.String(keyId),
		KeySpec: aws.String(keySpec),
	}

	// generate data key
	var dataKeyOutput *kms.GenerateDataKeyWithoutPlaintextOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	dataKeyOutput, e = cli.GenerateDataKeyWithoutPlaintextWithContext(kmsCtx, &dataKeyInput)
	kmsCancel()

	if e != nil {
		err = errors.New("GenerateDataKeyAes256 with KMS Failed: (Gen Data Key) " + e.Error())
		return "", err
	}

	if dataKeyOutput == nil {
		err = errors.New("GenerateDataKeyAes256 Failed: output is nil")
		return "", err
	}

	// return encrypted key via cipherKey
	cipherKey = util.ByteToHex(dataKeyOutput.CiphertextBlob)
	return cipherKey, nil
}

// EncryptWithDataKeyAes256 will encrypt plainText using cipherKey that was generated via GenerateDataKeyAes256()
//
// cipherKey = encrypted data key in hex (must use KMS CMK to decrypt such key)
//
// Plaintext-handling caveat (F6 pass-3 contrarian 2026-04-14;
// corrected SP-008 lens-3 2026-04-15):
//
// This helper converts the KMS plaintext data key to a Go string via
// string(dataKeyOutput.Plaintext) to pass it to crypto.AesGcmEncrypt.
// The string() conversion copies the bytes onto the Go heap as an
// immutable string header. The subsequent dataKeyOutput.SetPlaintext(
// []byte{}) call does NOT zero-scrub the original SDK-owned backing
// slice — per aws-sdk-go/service/kms/api.go, SetPlaintext is simply
// `s.Plaintext = v`, a pointer reassignment that rebinds the struct
// field to the fresh empty slice and leaves the previous backing
// array live on the heap, unreferenced but intact, until the Go
// garbage collector reclaims it.
//
// Concretely, two copies of the plaintext data key coexist in
// process memory between the string() conversion and the next GC
// cycle:
//  1. The immutable Go-heap string copy produced by string(...).
//  2. The original AWS SDK-owned []byte that is no longer referenced
//     by dataKeyOutput after SetPlaintext but has not been
//     overwritten.
//
// Either copy is visible to heap scanners, debuggers, or a core dump
// captured during that window. Neither copy is guaranteed to be
// overwritten even after GC — the memory pages may be recycled and
// re-used without being zeroed.
//
// Treat the SetPlaintext call as best-effort reference drop only: it
// makes both copies GC-eligible, but it does NOT overwrite any bytes.
// Callers that need stronger scrubbing guarantees must use a
// []byte-based AEAD API that never converts to string and manually
// overwrites the plaintext slice before release (not currently
// exposed by this package).
func (k *KMS) EncryptWithDataKeyAes256(plainText string, cipherKey string) (cipherText string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-EncryptWithDataKeyAes256", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-EncryptWithDataKeyAes256-AES-KMS-KeyName", k.getAesKeyName())
			_ = seg.SafeAddMetadata("KMS-EncryptWithDataKeyAes256-PlainText-Length", len(plainText))
			_ = seg.SafeAddMetadata("KMS-EncryptWithDataKeyAes256-CipherKey-Length", len(cipherKey))
			_ = seg.SafeAddMetadata("KMS-EncryptWithDataKeyAes256-Result-CipherText-Length", len(cipherText))

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("EncryptWithDataKeyAes256 with KMS Failed: " + cliErr.Error())
		return "", err
	}

	keyName := k.getAesKeyName()
	if len(keyName) <= 0 {
		err = errors.New("EncryptWithDataKeyAes256 with KMS Failed: " + "AES KMS Key Name is Required")
		return "", err
	}

	if len(plainText) <= 0 {
		err = errors.New("EncryptWithDataKeyAes256 with KMS Failed: " + "PlainText is Required")
		return "", err
	}

	if len(cipherKey) <= 0 {
		err = errors.New("EncryptWithDataKeyAes256 with KMS Failed: " + "CipherKey is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + keyName
	cipherBytes, ce := util.HexToByte(cipherKey)

	if ce != nil {
		err = errors.New("EncryptWithDataKeyAes256 with KMS Failed: (Unmarshal CipherKey Hex To Byte) " + ce.Error())
		return "", err
	}

	// decrypt cipherKey using kms cmk
	var dataKeyOutput *kms.DecryptOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	dataKeyOutput, e = cli.DecryptWithContext(kmsCtx,
		&kms.DecryptInput{
			EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
			KeyId:               aws.String(keyId),
			CiphertextBlob:      cipherBytes,
		})
	kmsCancel()

	if e != nil {
		err = errors.New("EncryptWithDataKeyAes256 with KMS Failed: (Decrypt Data Key) " + e.Error())
		return "", err
	}

	if dataKeyOutput == nil {
		err = errors.New("EncryptWithDataKeyAes256 Failed: output is nil")
		return "", err
	}

	// perform encryption action using decrypted plaintext data key
	//
	// F6 caveat: string(dataKeyOutput.Plaintext) copies the data key
	// bytes onto the Go heap as an immutable string. The SetPlaintext
	// call below does NOT zero-scrub the SDK-owned backing slice — it
	// is a pointer reassignment that rebinds dataKeyOutput.Plaintext
	// to an empty slice and leaves the previous backing array live on
	// the heap, unreferenced but intact, until GC. Both the heap
	// string copy AND the former SDK slice remain readable in
	// process memory until GC reclaims them. See function godoc for
	// the full discussion.
	buf, e := crypto.AesGcmEncrypt(plainText, string(dataKeyOutput.Plaintext))

	// best-effort reference-drop: rebind the SDK-owned slice field to
	// an empty slice so both the former backing array and our heap
	// string copy become GC-eligible. This does NOT overwrite any
	// bytes — the underlying pages remain readable until GC and
	// possibly beyond (page recycle without zeroing is legal).
	dataKeyOutput.SetPlaintext([]byte{})
	dataKeyOutput = nil

	// evaluate encrypted result
	if e != nil {
		err = errors.New("EncryptWithDataKeyAes256 with KMS Failed: (Encrypt Data) " + e.Error())
		return "", err
	} else {
		cipherText = buf
	}

	// return encrypted data
	return cipherText, nil
}

// DecryptWithDataKeyAes256 will decrypt cipherText using cipherKey that was generated via GenerateDataKeyAes256()
//
// cipherKey = encrypted data key in hex (must use KMS CMK to decrypt such key)
//
// Plaintext-handling caveat (F6 pass-3 contrarian 2026-04-14;
// corrected SP-008 lens-3 2026-04-15):
//
// Same caveat as EncryptWithDataKeyAes256. The string() conversion
// of the KMS plaintext data key creates an immutable heap copy. The
// subsequent SetPlaintext([]byte{}) call is a pointer reassignment
// (per aws-sdk-go/service/kms/api.go) — it does NOT overwrite the
// original SDK-owned backing slice; it merely rebinds the
// dataKeyOutput.Plaintext field to an empty slice and leaves the
// prior backing array live on the heap, unreferenced but intact,
// until GC. Both the heap string copy and the former SDK slice
// remain readable in process memory from the string() conversion
// until GC reclaims them (and may persist on recycled pages even
// after GC). Treat scrubbing as best-effort reference-drop only.
// See EncryptWithDataKeyAes256 godoc for the full discussion.
func (k *KMS) DecryptWithDataKeyAes256(cipherText string, cipherKey string) (plainText string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-DecryptWithDataKeyAes256", k.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.SafeAddMetadata("KMS-DecryptWithDataKeyAes256-AES-KMS-KeyName", k.getAesKeyName())
			_ = seg.SafeAddMetadata("KMS-DecryptWithDataKeyAes256-CipherText-Length", len(cipherText))
			_ = seg.SafeAddMetadata("KMS-DecryptWithDataKeyAes256-CipherKey-Length", len(cipherKey))
			_ = seg.SafeAddMetadata("KMS-DecryptWithDataKeyAes256-Result-PlainText-Length", len(plainText))

			if err != nil {
				_ = seg.SafeAddError(err)
			}
		}()
	}

	// validate
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("DecryptWithDataKeyAes256 with KMS Failed: " + cliErr.Error())
		return "", err
	}

	keyName := k.getAesKeyName()
	if len(keyName) <= 0 {
		err = errors.New("DecryptWithDataKeyAes256 with KMS Failed: " + "AES KMS Key Name is Required")
		return "", err
	}

	if len(cipherText) <= 0 {
		err = errors.New("DecryptWithDataKeyAes256 with KMS Failed: " + "CipherText is Required")
		return "", err
	}

	if len(cipherKey) <= 0 {
		err = errors.New("DecryptWithDataKeyAes256 with KMS Failed: " + "CipherKey is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + keyName
	cipherBytes, ce := util.HexToByte(cipherKey)

	if ce != nil {
		err = errors.New("DecryptWithDataKeyAes256 with KMS Failed: (Unmarshal CipherKey Hex To Byte) " + ce.Error())
		return "", err
	}

	// decrypt cipherKey using kms cmk
	var dataKeyOutput *kms.DecryptOutput
	var e error

	// SP-008 P2-CMN-2: always use *WithContext — ensureKMSCtx yields a
	// 30s timeout ctx when segCtx is nil.
	kmsCtx, kmsCancel := ensureKMSCtx(segCtx)
	dataKeyOutput, e = cli.DecryptWithContext(kmsCtx,
		&kms.DecryptInput{
			EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
			KeyId:               aws.String(keyId),
			CiphertextBlob:      cipherBytes,
		})
	kmsCancel()

	if e != nil {
		err = errors.New("DecryptWithDataKeyAes256 with KMS Failed: (Decrypt Data Key) " + e.Error())
		return "", err
	}

	if dataKeyOutput == nil {
		err = errors.New("DecryptWithDataKeyAes256 Failed: output is nil")
		return "", err
	}

	// perform decryption action using decrypted plaintext data key
	//
	// F6 caveat: string(dataKeyOutput.Plaintext) copies the data key
	// bytes onto the Go heap as an immutable string. The SetPlaintext
	// call below does NOT zero-scrub the SDK-owned backing slice — it
	// is a pointer reassignment that rebinds dataKeyOutput.Plaintext
	// to an empty slice and leaves the previous backing array live on
	// the heap, unreferenced but intact, until GC. Both the heap
	// string copy AND the former SDK slice remain readable in
	// process memory until GC reclaims them. See function godoc for
	// the full discussion.
	buf, e := crypto.AesGcmDecrypt(cipherText, string(dataKeyOutput.Plaintext))

	// best-effort reference-drop: rebind the SDK-owned slice field to
	// an empty slice so both the former backing array and our heap
	// string copy become GC-eligible. This does NOT overwrite any
	// bytes — the underlying pages remain readable until GC and
	// possibly beyond (page recycle without zeroing is legal).
	dataKeyOutput.SetPlaintext([]byte{})
	dataKeyOutput = nil

	// evaluate decrypted result
	if e != nil {
		err = errors.New("DecryptWithDataKeyAes256 with KMS Failed: (Decrypt Data) " + e.Error())
		return "", err
	} else {
		plainText = buf
	}

	// return decrypted data
	return plainText, nil
}

func (k *KMS) ImportECCP256SignVerifyKey(keyAlias, keyPolicyJson string, eccPvk *ecdsa.PrivateKey) (keyArn string, err error) {
	if k == nil {
		return "", errors.New("KMS receiver is nil")
	}
	// validate inputs
	if strings.TrimSpace(keyAlias) == "" {
		return "", errors.New("ImportECCP256SignVerifyKey with KMS Failed: keyAlias is required")
	}
	if strings.TrimSpace(keyPolicyJson) == "" {
		return "", errors.New("ImportECCP256SignVerifyKey with KMS Failed: keyPolicyJson is required")
	}
	if eccPvk == nil {
		return "", errors.New("ImportECCP256SignVerifyKey with KMS Failed: eccPvk is required")
	}

	// validate client
	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("ImportECCP256SignVerifyKey with KMS Failed: " + cliErr.Error())
		return "", err
	}

	cInput := &kms.CreateKeyInput{
		BypassPolicyLockoutSafetyCheck: nil,
		CustomKeyStoreId:               nil,
		Description:                    nil,
		KeySpec:                        aws.String(kms.KeySpecEccNistP256),
		KeyUsage:                       aws.String(kms.KeyUsageTypeSignVerify),
		MultiRegion:                    aws.Bool(false),
		Origin:                         aws.String(kms.OriginTypeExternal),
		Policy:                         aws.String(keyPolicyJson),
		Tags:                           nil,
		XksKeyId:                       nil,
	}

	cOutput, e1 := cli.CreateKey(cInput)
	if e1 != nil {
		return "", e1
	}

	if cOutput == nil || cOutput.KeyMetadata == nil || cOutput.KeyMetadata.KeyId == nil {
		return "", errors.New("ImportECCP256SignVerifyKey: CreateKey returned nil output or KeyMetadata")
	}

	keyID := aws.StringValue(cOutput.KeyMetadata.KeyId)

	if keyID == "" {
		return "", errors.New("key ID is empty")
	}

	// deferred cleanup: schedule orphaned key for deletion if function returns an error
	defer func() {
		if err != nil && keyID != "" {
			_, _ = cli.ScheduleKeyDeletion(&kms.ScheduleKeyDeletionInput{
				KeyId:               aws.String(keyID),
				PendingWindowInDays: aws.Int64(7),
			})
		}
	}()

	// Step 1: Get an import token and public key from KMS
	getParams := &kms.GetParametersForImportInput{
		KeyId:             aws.String(keyID),
		WrappingAlgorithm: aws.String(kms.AlgorithmSpecRsaesOaepSha256),
		WrappingKeySpec:   aws.String(kms.KeySpecRsa2048),
	}
	resp, e2 := cli.GetParametersForImport(getParams)
	if e2 != nil {
		return "", e2
	}

	derKey, e3 := x509.MarshalPKCS8PrivateKey(eccPvk)
	if e3 != nil {
		return "", e3
	}

	// Step 2: Encrypt the key material using the provided public key
	pubKey, e4 := x509.ParsePKIXPublicKey(resp.PublicKey)
	if e4 != nil {
		return "", e4
	}

	rsaPubKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("public key is not RSA")
	}

	encryptedKeyMaterial, e5 := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaPubKey, derKey, nil)
	if e5 != nil {
		return "", e5
	}

	//Step 3: Import the encrypted key material into KMS
	importParams := &kms.ImportKeyMaterialInput{
		EncryptedKeyMaterial: encryptedKeyMaterial,
		ExpirationModel:      aws.String(kms.ExpirationModelTypeKeyMaterialDoesNotExpire),
		ImportToken:          resp.ImportToken,
		KeyId:                aws.String(keyID),
		ValidTo:              nil,
	}

	_, err = cli.ImportKeyMaterial(importParams)
	if err != nil {
		return "", err
	}

	aliasName := "alias/" + keyAlias // Change to your desired alias name

	aliasInput := &kms.CreateAliasInput{
		AliasName:   aws.String(aliasName),
		TargetKeyId: aws.String(keyID),
	}

	_, err = cli.CreateAlias(aliasInput)
	if err != nil {
		return "", err
	}

	return aws.StringValue(cOutput.KeyMetadata.Arn), nil
}

func (k *KMS) ECDH(keyArn, ephemeralPublicKeyB64 string) (sharedSecret []byte, err error) {
	if k == nil {
		return nil, errors.New("KMS receiver is nil")
	}
	if keyArn == "" {
		return nil, errors.New("ECDH with KMS Failed: keyArn is required")
	}
	if ephemeralPublicKeyB64 == "" {
		return nil, errors.New("ECDH with KMS Failed: ephemeral public key is required")
	}

	cli, cliErr := k.getClient()
	if cliErr != nil {
		err = errors.New("ECDH with KMS Failed: " + cliErr.Error())
		return nil, err
	}

	descOut, e := cli.DescribeKey(&kms.DescribeKeyInput{KeyId: aws.String(keyArn)})
	if e != nil {
		return nil, fmt.Errorf("ECDH with KMS Failed: describe key: %w", e)
	}
	if descOut == nil {
		return nil, errors.New("ECDH with KMS Failed: DescribeKey returned nil")
	}
	km := descOut.KeyMetadata
	if km == nil || km.KeySpec == nil || km.KeyUsage == nil || km.KeyState == nil {
		return nil, errors.New("ECDH with KMS Failed: key metadata is missing")
	}
	if !strings.HasPrefix(aws.StringValue(km.KeySpec), "ECC_") {
		return nil, fmt.Errorf("ECDH with KMS Failed: key spec must be ECC, got %s", aws.StringValue(km.KeySpec))
	}
	if *km.KeyUsage != kms.KeyUsageTypeKeyAgreement {
		return nil, fmt.Errorf("ECDH with KMS Failed: key usage must be KEY_AGREEMENT, got %s", aws.StringValue(km.KeyUsage))
	}
	if *km.KeyState != kms.KeyStateEnabled {
		return nil, fmt.Errorf("ECDH with KMS Failed: key state must be Enabled, got %s", aws.StringValue(km.KeyState))
	}

	//derive temp ECC public key
	eccPubBytes, e1 := base64.StdEncoding.DecodeString(ephemeralPublicKeyB64)
	if e1 != nil {
		return nil, errors.New("ECDH with KMS Failed: Invalid Base64 Public Key, " + e1.Error())
	}

	//eccPubKey, err := parseECCPublicKey(eccPubBytes)
	//if err != nil {
	//
	//}
	//
	//key, err := x509.MarshalPKCS8PrivateKey(eccPubKey)
	//if err != nil {
	//
	//}

	inputReq := &kms.DeriveSharedSecretInput{
		DryRun:                nil,
		GrantTokens:           nil,
		KeyAgreementAlgorithm: aws.String(kms.KeyAgreementAlgorithmSpecEcdh),
		KeyId:                 aws.String(keyArn),
		PublicKey:             eccPubBytes,
		Recipient:             nil,
	}

	outputResp, e2 := cli.DeriveSharedSecret(inputReq)
	if e2 != nil {
		return nil, e2
	}

	if outputResp == nil {
		return nil, errors.New("ECDH with KMS Failed: DeriveSharedSecret returned nil output")
	}

	return outputResp.SharedSecret, nil
}
