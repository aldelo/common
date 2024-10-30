package kms

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
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"errors"
	util "github.com/aldelo/common"
	"github.com/aldelo/common/crypto"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
	"net/http"
)

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

	// store aws session object
	sess *session.Session

	// store kms client object
	kmsClient *kms.KMS

	_parentSegment *xray.XRayParentSegment
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the KMS service
func (k *KMS) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			k._parentSegment = parentSegment[0]
		}

		seg := xray.NewSegment("KMS-Connect", k._parentSegment)
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KDS-AWS-Region", k.AwsRegion)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = k.connectInternal()

		if err == nil {
			awsxray.AWS(k.kmsClient.Client)
		}

		return err
	} else {
		return k.connectInternal()
	}
}

// Connect will establish a connection to the KMS service
func (k *KMS) connectInternal() error {
	// clean up prior session reference
	k.sess = nil

	if !k.AwsRegion.Valid() || k.AwsRegion == awsregion.UNKNOWN {
		return errors.New("Connect To KMS Failed: (AWS Session Error) " + "Region is Required")
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
		return errors.New("Connect to KMS Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection and keep session object in struct
	if sess, err := session.NewSession(
		&aws.Config{
			Region:     aws.String(k.AwsRegion.Key()),
			HTTPClient: httpCli,
		}); err != nil {
		// aws session error
		return errors.New("Connect To KMS Failed: (AWS Session Error) " + err.Error())
	} else {
		// aws session obtained
		k.sess = sess

		// create cached objects for shared use
		k.kmsClient = kms.New(k.sess)

		if k.kmsClient == nil {
			return errors.New("Connect To KMS Client Failed: (New KMS Client Connection) " + "Connection Object Nil")
		}

		// session stored to struct
		return nil
	}
}

// Disconnect will disjoin from aws session by clearing it
func (k *KMS) Disconnect() {
	k.kmsClient = nil
	k.sess = nil
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (k *KMS) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	k._parentSegment = parentSegment
}

// ----------------------------------------------------------------------------------------------------------------
// kms-cmk encrypt/decrypt via aes 256 functions
// ----------------------------------------------------------------------------------------------------------------

// EncryptViaCmkAes256 will use kms cmk to encrypt plainText using aes 256 symmetric kms cmk key, and return cipherText string,
// the cipherText can only be decrypted with aes 256 symmetric kms cmk key
func (k *KMS) EncryptViaCmkAes256(plainText string) (cipherText string, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-EncryptViaCmkAes256", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-EncryptViaCmkAes256-AES-KMS-KeyName", k.AesKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-EncryptViaCmkAes256-PlainText-Length", len(plainText))
			_ = seg.Seg.AddMetadata("KMS-EncryptViaCmkAes256-Result-CipherText-Length", len(cipherText))

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("EncryptViaCmkAes256 with KMS CMK Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(k.AesKmsKeyName) <= 0 {
		err = errors.New("EncryptViaCmkAes256 with KMS CMK Failed: " + "AES KMS Key Name is Required")
		return "", err
	}

	if len(plainText) <= 0 {
		err = errors.New("EncryptViaCmkAes256 with KMS CMK Failed: " + "PlainText is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + k.AesKmsKeyName

	// encrypt symmetric using kms cmk
	var encryptedOutput *kms.EncryptOutput
	var e error

	if segCtx == nil {
		encryptedOutput, e = k.kmsClient.Encrypt(&kms.EncryptInput{
			EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
			KeyId:               aws.String(keyId),
			Plaintext:           []byte(plainText),
		})
	} else {
		encryptedOutput, e = k.kmsClient.EncryptWithContext(segCtx,
			&kms.EncryptInput{
				EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
				KeyId:               aws.String(keyId),
				Plaintext:           []byte(plainText),
			})
	}

	if e != nil {
		err = errors.New("EncryptViaCmkAes256 with KMS CMK Failed: (Symmetric Encrypt) " + e.Error())
		return "", err
	}

	// return encrypted cipher text blob
	cipherText = util.ByteToHex(encryptedOutput.CiphertextBlob)
	return cipherText, nil
}

// ReEncryptViaCmkAes256 will re-encrypt sourceCipherText using the new targetKmsKeyName via kms, (must be targeting aes 256 key)
// the re-encrypted cipherText is then returned
func (k *KMS) ReEncryptViaCmkAes256(sourceCipherText string, targetKmsKeyName string) (targetCipherText string, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-ReEncryptViaCmkAes256", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-ReEncryptViaCmkAes256-Source-AES-KMS-KeyName", k.AesKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-ReEncryptViaCmkAes256-Target-AES-KMS-KeyName", targetKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-ReEncryptViaCmkAes256-SourceCipherText-Length", len(sourceCipherText))
			_ = seg.Seg.AddMetadata("KMS-ReEncryptViaCmkAes256-Result-Target-CipherText-Length", len(targetCipherText))

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(k.AesKmsKeyName) <= 0 {
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
	keyId := "alias/" + k.AesKmsKeyName

	// convert hex to bytes
	cipherBytes, ce := util.HexToByte(sourceCipherText)

	if ce != nil {
		err = errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: (Unmarshal Source CipherText Hex To Byte) " + ce.Error())
		return "", err
	}

	// re-encrypt symmetric kms cmk
	var reEncryptOutput *kms.ReEncryptOutput
	var e error

	if segCtx == nil {
		reEncryptOutput, e = k.kmsClient.ReEncrypt(&kms.ReEncryptInput{
			SourceEncryptionAlgorithm:      aws.String("SYMMETRIC_DEFAULT"),
			SourceKeyId:                    aws.String(keyId),
			DestinationEncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
			DestinationKeyId:               aws.String("alias/" + targetKmsKeyName),
			CiphertextBlob:                 cipherBytes,
		})
	} else {
		reEncryptOutput, e = k.kmsClient.ReEncryptWithContext(segCtx,
			&kms.ReEncryptInput{
				SourceEncryptionAlgorithm:      aws.String("SYMMETRIC_DEFAULT"),
				SourceKeyId:                    aws.String(keyId),
				DestinationEncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
				DestinationKeyId:               aws.String("alias/" + targetKmsKeyName),
				CiphertextBlob:                 cipherBytes,
			})
	}

	if e != nil {
		err = errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: (Symmetric ReEncrypt) " + e.Error())
		return "", err
	}

	// return encrypted cipher text blob
	targetCipherText = util.ByteToHex(reEncryptOutput.CiphertextBlob)
	return targetCipherText, nil
}

// DecryptViaCmkAes256 will use kms cmk to decrypt cipherText using symmetric aes 256 kms cmk key, and return plainText string,
// the cipherText can only be decrypted with the symmetric aes 256 kms cmk key
func (k *KMS) DecryptViaCmkAes256(cipherText string) (plainText string, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-DecryptViaCmkAes256", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-DecryptViaCmkAes256-AES-KMS-KeyName", k.AesKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-DecryptViaCmkAes256-CipherText-Length", len(cipherText))
			_ = seg.Seg.AddMetadata("KMS-DecryptViaCmkAes256-Result-PlainText-Length", len(plainText))

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("DecryptViaCmkAes256 with KMS CMK Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(k.AesKmsKeyName) <= 0 {
		err = errors.New("DecryptViaCmkAes256 with KMS CMK Failed: " + "AES KMS Key Name is Required")
		return "", err
	}

	if len(cipherText) <= 0 {
		err = errors.New("DecryptViaCmkAes256 with KMS CMK Failed: " + "Cipher Text is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + k.AesKmsKeyName
	cipherBytes, ce := util.HexToByte(cipherText)

	if ce != nil {
		err = errors.New("DecryptViaCmkAes256 with KMS CMK Failed: (Unmarshal CipherText Hex To Byte) " + ce.Error())
		return "", err
	}

	// decrypt symmetric using kms cmk
	var decryptedOutput *kms.DecryptOutput
	var e error

	if segCtx == nil {
		decryptedOutput, e = k.kmsClient.Decrypt(&kms.DecryptInput{
			EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
			KeyId:               aws.String(keyId),
			CiphertextBlob:      cipherBytes,
		})
	} else {
		decryptedOutput, e = k.kmsClient.DecryptWithContext(segCtx,
			&kms.DecryptInput{
				EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
				KeyId:               aws.String(keyId),
				CiphertextBlob:      cipherBytes,
			})
	}

	if e != nil {
		err = errors.New("DecryptViaCmkAes256 with KMS CMK Failed: (Symmetric Decrypt) " + e.Error())
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
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-GenerateDataKeyRsa2048", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-GenerateEncryptionDecryptionKeyRsa2048-RSA-KMS-KeyName", k.RsaKmsKeyName)
			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("GenerateEncryptionDecryptionKeyRsa2048 with KMS CMK Failed: " + "KMS Client is Required")
		return nil, err
	}

	var e error

	if segCtx == nil {
		encryptedOutput, e = k.kmsClient.CreateKey(&kms.CreateKeyInput{
			Description: aws.String("Common RSA 2048 Key Creation"),
			KeySpec:     aws.String(kms.KeySpecRsa2048),
			KeyUsage:    aws.String(kms.KeyUsageTypeEncryptDecrypt),
			Policy:      aws.String(string(keyPolicyJSON)),
		})
	} else {
		encryptedOutput, e = k.kmsClient.CreateKeyWithContext(segCtx, &kms.CreateKeyInput{
			Description: aws.String("Common RSA 2048 Key Creation"),
			KeySpec:     aws.String(kms.KeySpecRsa2048),
			KeyUsage:    aws.String(kms.KeyUsageTypeEncryptDecrypt),
			Policy:      aws.String(string(keyPolicyJSON)),
		})
	}

	if e != nil {
		err = errors.New("GenerateEncryptionDecryptionKeyRsa2048 with KMS CMK Failed: (RSA Key Create Fail) " + e.Error())
		return nil, err
	}

	aliasName := "alias/" + keyName // Change to your desired alias name

	aliasInput := &kms.CreateAliasInput{
		AliasName:   aws.String(aliasName),
		TargetKeyId: encryptedOutput.KeyMetadata.KeyId,
	}

	_, err = k.kmsClient.CreateAlias(aliasInput)
	if err != nil {
		return nil, err
	}

	// return creation data
	return encryptedOutput, nil
}

func (k *KMS) GenerateSignVerifyKeyRsa2048(keyName string, keyPolicy interface{}) (encryptedOutput *kms.CreateKeyOutput, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-GenerateSignVerifyKeyRsa2048", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-GenerateSignVerifyKeyRsa2048-RSA-KMS-KeyName", k.RsaKmsKeyName)
			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("GenerateSignVerifyKeyRsa2048 with KMS CMK Failed: " + "KMS Client is Required")
		return nil, err
	}

	keyPolicyJSON, err := json.Marshal(keyPolicy)
	if err != nil {
		return nil, err
	}

	var e error

	if segCtx == nil {
		encryptedOutput, e = k.kmsClient.CreateKey(&kms.CreateKeyInput{
			Description: aws.String("Common RSA 2048 Key Creation"),
			KeySpec:     aws.String(kms.KeySpecRsa2048),
			KeyUsage:    aws.String(kms.KeyUsageTypeSignVerify),
			Policy:      aws.String(string(keyPolicyJSON)),
		})
	} else {
		encryptedOutput, e = k.kmsClient.CreateKeyWithContext(segCtx, &kms.CreateKeyInput{
			Description: aws.String("Common RSA 2048 Key Creation"),
			KeySpec:     aws.String(kms.KeySpecRsa2048),
			KeyUsage:    aws.String(kms.KeyUsageTypeSignVerify),
			Policy:      aws.String(string(keyPolicyJSON)),
		})
	}

	if e != nil {
		err = errors.New("GenerateSignVerifyKeyRsa2048 with KMS CMK Failed: (RSA Key Create Fail) " + e.Error())
		return nil, err
	}

	aliasName := "alias/" + keyName // Change to your desired alias name

	aliasInput := &kms.CreateAliasInput{
		AliasName:   aws.String(aliasName),
		TargetKeyId: encryptedOutput.KeyMetadata.KeyId,
	}

	_, err = k.kmsClient.CreateAlias(aliasInput)
	if err != nil {
		return nil, err
	}

	// return creation data
	return encryptedOutput, nil
}

func (k *KMS) KeyDeleteWithAlias(alias string, PendingWindowInDays int64) (output *kms.ScheduleKeyDeletionOutput, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-KeyDeleteWithAlias", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-KeyDeleteWithAlias-RSA-KMS-KeyName", k.RsaKmsKeyName)
			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("KeyDeleteWithAlias with KMS CMK Failed: " + "KMS Client is Required")
		return nil, err
	}

	if alias == "" {
		err = errors.New("KeyDeleteWithAlias with KMS CMK Failed: " + "Alias is Required")
		return nil, err
	}

	if PendingWindowInDays < 7 {
		err = errors.New("KeyDeleteWithAlias with KMS CMK Failed: " + "PendingWindowInDays Must Be At Least 7 Days")
		return nil, err

	}

	aliasName := "alias/" + alias // Change to your desired alias name

	result, err := k.kmsClient.GetPublicKey(&kms.GetPublicKeyInput{
		KeyId: aws.String(aliasName),
	})

	var e error

	if segCtx == nil {
		output, e = k.kmsClient.ScheduleKeyDeletion(&kms.ScheduleKeyDeletionInput{
			KeyId:               result.KeyId,
			PendingWindowInDays: aws.Int64(PendingWindowInDays),
		})
	} else {
		output, e = k.kmsClient.ScheduleKeyDeletionWithContext(segCtx, &kms.ScheduleKeyDeletionInput{
			KeyId:               result.KeyId,
			PendingWindowInDays: aws.Int64(PendingWindowInDays),
		})
	}

	if e != nil {
		err = errors.New("KeyDeleteWithAlias with KMS CMK Failed: (RSA Key Delete Fail) " + e.Error())
		return nil, err
	}

	return output, nil
}

func (k *KMS) KeyDeleteWithArnID(arn string, PendingWindowInDays int64) (output *kms.ScheduleKeyDeletionOutput, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-KeyDeleteWithArnID", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-KeyDeleteWithArnID-RSA-KMS-KeyName", k.RsaKmsKeyName)
			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("KeyDeleteWithArnID with KMS CMK Failed: " + "KMS Client is Required")
		return nil, err
	}

	if arn == "" {
		err = errors.New("KeyDeleteWithArnID with KMS CMK Failed: " + "ARN is Required")
		return nil, err
	}

	if PendingWindowInDays < 7 {
		err = errors.New("KeyDeleteWithArnID with KMS CMK Failed: " + "PendingWindowInDays Must Be At Least 7 Days")
		return nil, err
	}

	var e error

	if segCtx == nil {
		output, e = k.kmsClient.ScheduleKeyDeletion(&kms.ScheduleKeyDeletionInput{
			KeyId:               aws.String(arn),
			PendingWindowInDays: aws.Int64(PendingWindowInDays),
		})
	} else {
		output, e = k.kmsClient.ScheduleKeyDeletionWithContext(segCtx, &kms.ScheduleKeyDeletionInput{
			KeyId:               aws.String(arn),
			PendingWindowInDays: aws.Int64(PendingWindowInDays),
		})
	}

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
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-EncryptViaCmkRsa2048", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-EncryptViaCmkRsa2048-RSA-KMS-KeyName", k.RsaKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-EncryptViaCmkRsa2048-PlainText-Length", len(plainText))
			_ = seg.Seg.AddMetadata("KMS-EncryptViaCmkRsa2048-Result-CipherText-Length", len(cipherText))

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(k.RsaKmsKeyName) <= 0 {
		err = errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "RSA KMS Key Name is Required")
		return "", err
	}

	if len(plainText) <= 0 {
		err = errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "PlainText is Required")
		return "", err
	}

	if len(plainText) > 214 {
		err = errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "PlainText Cannot Exceed 214 Bytes")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + k.RsaKmsKeyName

	// encrypt asymmetric using kms cmk
	var encryptedOutput *kms.EncryptOutput
	var e error

	if segCtx == nil {
		encryptedOutput, e = k.kmsClient.Encrypt(&kms.EncryptInput{
			EncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
			KeyId:               aws.String(keyId),
			Plaintext:           []byte(plainText),
		})
	} else {
		encryptedOutput, e = k.kmsClient.EncryptWithContext(segCtx,
			&kms.EncryptInput{
				EncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
				KeyId:               aws.String(keyId),
				Plaintext:           []byte(plainText),
			})
	}

	if e != nil {
		err = errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: (Asymmetric Encrypt) " + e.Error())
		return "", err
	}

	// return encrypted cipher text blob
	cipherText = util.ByteToHex(encryptedOutput.CiphertextBlob)
	return cipherText, nil
}

func (k *KMS) GetRSAPublicKey(alias string) (output *kms.GetPublicKeyOutput, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-GetRSAPublicKey", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-GetRSAPublicKey-RSA-KMS-KeyName", k.RsaKmsKeyName)
			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("GetRSAPublicKey with KMS CMK Failed: " + "KMS Client is Required")
		return nil, err
	}

	if alias == "" {
		err = errors.New("GetRSAPublicKey with KMS CMK Failed: " + "Alias is Required")
		return nil, err
	}

	var e error

	aliasName := "alias/" + alias // Change to your desired alias name

	if segCtx == nil {
		output, err = k.kmsClient.GetPublicKey(&kms.GetPublicKeyInput{
			KeyId: aws.String(aliasName),
		})
	} else {
		output, err = k.kmsClient.GetPublicKeyWithContext(segCtx, &kms.GetPublicKeyInput{
			KeyId: aws.String(aliasName),
		})
	}

	if e != nil {
		err = errors.New("GetRSAPublicKey with KMS CMK Failed: (RSA Key Delete Fail) " + e.Error())
		return nil, err
	}

	return output, nil
}

// ReEncryptViaCmkRsa2048 will re-encrypt sourceCipherText using the new targetKmsKeyName via kms, (must be targeting rsa 2048 key)
// the re-encrypted cipherText is then returned
func (k *KMS) ReEncryptViaCmkRsa2048(sourceCipherText string, targetKmsKeyName string) (targetCipherText string, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-ReEncryptViaCmkRsa2048", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-ReEncryptViaCmkRsa2048-Source-RSA-KMS-KeyName", k.AesKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-ReEncryptViaCmkRsa2048-Target-RSA-KMS-KeyName", targetKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-ReEncryptViaCmkRsa2048-Source-CipherText-Length", len(sourceCipherText))
			_ = seg.Seg.AddMetadata("KMS-ReEncryptViaCmkRsa2048-Result-Target-CipherText-Length", len(targetCipherText))

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(k.RsaKmsKeyName) <= 0 {
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
	keyId := "alias/" + k.RsaKmsKeyName

	// convert hex to bytes
	cipherBytes, ce := util.HexToByte(sourceCipherText)

	if ce != nil {
		err = errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: (Unmarshal Source CipherText Hex To Byte) " + ce.Error())
		return "", err
	}

	// re-encrypt asymmetric kms cmk
	var reEncryptOutput *kms.ReEncryptOutput
	var e error

	if segCtx == nil {
		reEncryptOutput, e = k.kmsClient.ReEncrypt(&kms.ReEncryptInput{
			SourceEncryptionAlgorithm:      aws.String("RSAES_OAEP_SHA_256"),
			SourceKeyId:                    aws.String(keyId),
			DestinationEncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
			DestinationKeyId:               aws.String("alias/" + targetKmsKeyName),
			CiphertextBlob:                 cipherBytes,
		})
	} else {
		reEncryptOutput, e = k.kmsClient.ReEncryptWithContext(segCtx,
			&kms.ReEncryptInput{
				SourceEncryptionAlgorithm:      aws.String("RSAES_OAEP_SHA_256"),
				SourceKeyId:                    aws.String(keyId),
				DestinationEncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
				DestinationKeyId:               aws.String("alias/" + targetKmsKeyName),
				CiphertextBlob:                 cipherBytes,
			})
	}

	if e != nil {
		err = errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: (Asymmetric ReEncrypt) " + e.Error())
		return "", err
	}

	// return encrypted cipher text blob
	targetCipherText = util.ByteToHex(reEncryptOutput.CiphertextBlob)
	return targetCipherText, nil
}

// DecryptViaCmkRsa2048 will use kms cmk to decrypt cipherText using asymmetric rsa 2048 kms cmk private key, and return plainText string,
// the cipherText can only be decrypted with the asymmetric rsa 2048 kms cmk private key
func (k *KMS) DecryptViaCmkRsa2048(cipherText string) (plainText string, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-DecryptViaCmkRsa2048", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-DecryptViaCmkRsa2048-RSA-KMS-KeyName", k.RsaKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-DecryptViaCmkRsa2048-CipherText-Length", len(cipherText))
			_ = seg.Seg.AddMetadata("KMS-DecryptViaCmkRsa2048-Result-PlainText-Length", len(plainText))

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(k.RsaKmsKeyName) <= 0 {
		err = errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: " + "RSA KMS Key Name is Required")
		return "", err
	}

	if len(cipherText) <= 0 {
		err = errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: " + "Cipher Text is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + k.RsaKmsKeyName
	cipherBytes, ce := util.HexToByte(cipherText)

	if ce != nil {
		err = errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: (Unmarshal CipherText Hex To Byte) " + ce.Error())
		return "", err
	}

	// decrypt symmetric using kms cmk
	var decryptedOutput *kms.DecryptOutput
	var e error

	if segCtx == nil {
		decryptedOutput, e = k.kmsClient.Decrypt(&kms.DecryptInput{
			EncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
			KeyId:               aws.String(keyId),
			CiphertextBlob:      cipherBytes,
		})
	} else {
		decryptedOutput, e = k.kmsClient.DecryptWithContext(segCtx,
			&kms.DecryptInput{
				EncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
				KeyId:               aws.String(keyId),
				CiphertextBlob:      cipherBytes,
			})
	}

	if e != nil {
		err = errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: (Asymmetric Decrypt) " + e.Error())
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
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-SignViaCmkRsa2048", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-SignViaCmkRsa2048-Signature-KMS-KeyName", k.SignatureKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-SignViaCmkRsa2048-DataToSign-Length", len(dataToSign))
			_ = seg.Seg.AddMetadata("KMS-SignViaCmkRsa2048-Result-Signature", signature)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("SignViaCmkRsa2048 with KMS Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(k.SignatureKmsKeyName) <= 0 {
		err = errors.New("SignViaCmkRsa2048 with KMS Failed: " + "Signature KMS Key Name is Required")
		return "", err
	}

	if len(dataToSign) <= 0 {
		err = errors.New("SignViaCmkRsa2048 with KMS Failed: " + "Data To Sign is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + k.SignatureKmsKeyName

	// perform sign action using kms rsa sign/verify cmk
	var signOutput *kms.SignOutput
	var e error

	if segCtx == nil {
		signOutput, e = k.kmsClient.Sign(&kms.SignInput{
			KeyId:            aws.String(keyId),
			SigningAlgorithm: aws.String("RSASSA_PKCS1_V1_5_SHA_256"),
			MessageType:      aws.String("RAW"),
			Message:          []byte(dataToSign),
		})
	} else {
		signOutput, e = k.kmsClient.SignWithContext(segCtx,
			&kms.SignInput{
				KeyId:            aws.String(keyId),
				SigningAlgorithm: aws.String("RSASSA_PKCS1_V1_5_SHA_256"),
				MessageType:      aws.String("RAW"),
				Message:          []byte(dataToSign),
			})
	}

	if e != nil {
		err = errors.New("SignViaCmkRsa2048 with KMS Failed: (Sign Action) " + e.Error())
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
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-VerifyViaCmkRsa2048", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-VerifyViaCmkRsa2048-Signature-KMS-KeyName", k.SignatureKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-VerifyViaCmkRsa2048-DataToVerify-Length", len(dataToVerify))
			_ = seg.Seg.AddMetadata("KMS-VerifyViaCmkRsa2048-Signature-To-Verify", signatureToVerify)
			_ = seg.Seg.AddMetadata("KMS-VerifyViaCmkRsa2048-Result-SignatureValid", signatureValid)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("VerifyViaCmkRsa2048 with KMS Failed: " + "KMS Client is Required")
		return false, err
	}

	if len(k.SignatureKmsKeyName) <= 0 {
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
	keyId := "alias/" + k.SignatureKmsKeyName
	signatureBytes, ce := util.HexToByte(signatureToVerify)

	if ce != nil {
		err = errors.New("VerifyViaCmkRsa2048 with KMS Failed: (Marshal SignatureToVerify Hex To Byte) " + ce.Error())
		return false, err
	}

	// perform verify action using kms rsa sign/verify cmk
	var verifyOutput *kms.VerifyOutput
	var e error

	if segCtx == nil {
		verifyOutput, e = k.kmsClient.Verify(&kms.VerifyInput{
			KeyId:            aws.String(keyId),
			SigningAlgorithm: aws.String("RSASSA_PKCS1_V1_5_SHA_256"),
			MessageType:      aws.String("RAW"),
			Message:          []byte(dataToVerify),
			Signature:        signatureBytes,
		})
	} else {
		verifyOutput, e = k.kmsClient.VerifyWithContext(segCtx,
			&kms.VerifyInput{
				KeyId:            aws.String(keyId),
				SigningAlgorithm: aws.String("RSASSA_PKCS1_V1_5_SHA_256"),
				MessageType:      aws.String("RAW"),
				Message:          []byte(dataToVerify),
				Signature:        signatureBytes,
			})
	}

	if e != nil {
		err = errors.New("VerifyViaCmkRsa2048 with KMS Failed: (Verify Action) " + e.Error())
		return false, err
	}

	// return verify result
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
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-GenerateDataKeyAes256", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-GenerateDataKeyAes256-AES-KMS-KeyName", k.AesKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-GenerateDataKeyAes256-Result-CipherKey-Length", len(cipherKey))

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("GenerateDataKeyAes256 with KMS Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(k.AesKmsKeyName) <= 0 {
		err = errors.New("GenerateDataKeyAes256 with KMS Failed: " + "AES KMS Key Name is Required")
		return "", err
	}

	// prepare key info
	keyId := "alias/" + k.AesKmsKeyName
	keySpec := "AES_256" // always use AES 256
	dataKeyInput := kms.GenerateDataKeyWithoutPlaintextInput{
		KeyId:   aws.String(keyId),
		KeySpec: aws.String(keySpec),
	}

	// generate data key
	var dataKeyOutput *kms.GenerateDataKeyWithoutPlaintextOutput
	var e error

	if segCtx == nil {
		dataKeyOutput, e = k.kmsClient.GenerateDataKeyWithoutPlaintext(&dataKeyInput)
	} else {
		dataKeyOutput, e = k.kmsClient.GenerateDataKeyWithoutPlaintextWithContext(segCtx, &dataKeyInput)
	}

	if e != nil {
		err = errors.New("GenerateDataKeyAes256 with KMS Failed: (Gen Data Key) " + e.Error())
		return "", err
	}

	// return encrypted key via cipherKey
	cipherKey = util.ByteToHex(dataKeyOutput.CiphertextBlob)
	return cipherKey, nil
}

// EncryptWithDataKeyAes256 will encrypt plainText using cipherKey that was generated via GenerateDataKeyAes256()
//
// cipherKey = encrypted data key in hex (must use KMS CMK to decrypt such key)
func (k *KMS) EncryptWithDataKeyAes256(plainText string, cipherKey string) (cipherText string, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-EncryptWithDataKeyAes256", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-EncryptWithDataKeyAes256-AES-KMS-KeyName", k.AesKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-EncryptWithDataKeyAes256-PlainText-Length", len(plainText))
			_ = seg.Seg.AddMetadata("KMS-EncryptWithDataKeyAes256-CipherKey-Length", len(cipherKey))
			_ = seg.Seg.AddMetadata("KMS-EncryptWithDataKeyAes256-Result-CipherText-Length", len(cipherText))

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("EncryptWithDataKeyAes256 with KMS Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(k.AesKmsKeyName) <= 0 {
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
	keyId := "alias/" + k.AesKmsKeyName
	cipherBytes, ce := util.HexToByte(cipherKey)

	if ce != nil {
		err = errors.New("EncryptWithDataKeyAes256 with KMS Failed: (Unmarshal CipherKey Hex To Byte) " + ce.Error())
		return "", err
	}

	// decrypt cipherKey using kms cmk
	var dataKeyOutput *kms.DecryptOutput
	var e error

	if segCtx == nil {
		dataKeyOutput, e = k.kmsClient.Decrypt(&kms.DecryptInput{
			EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
			KeyId:               aws.String(keyId),
			CiphertextBlob:      cipherBytes,
		})
	} else {
		dataKeyOutput, e = k.kmsClient.DecryptWithContext(segCtx,
			&kms.DecryptInput{
				EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
				KeyId:               aws.String(keyId),
				CiphertextBlob:      cipherBytes,
			})
	}

	if e != nil {
		err = errors.New("EncryptWithDataKeyAes256 with KMS Failed: (Decrypt Data Key) " + e.Error())
		return "", err
	}

	// perform encryption action using decrypted plaintext data key
	buf, e := crypto.AesGcmEncrypt(plainText, string(dataKeyOutput.Plaintext))

	// clean up data key from memory immediately
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
func (k *KMS) DecryptWithDataKeyAes256(cipherText string, cipherKey string) (plainText string, err error) {
	var segCtx context.Context
	segCtx = nil

	seg := xray.NewSegmentNullable("KMS-DecryptWithDataKeyAes256", k._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("KMS-DecryptWithDataKeyAes256-AES-KMS-KeyName", k.AesKmsKeyName)
			_ = seg.Seg.AddMetadata("KMS-DecryptWithDataKeyAes256-CipherText-Length", len(cipherText))
			_ = seg.Seg.AddMetadata("KMS-DecryptWithDataKeyAes256-CipherKey-Length", len(cipherKey))
			_ = seg.Seg.AddMetadata("KMS-DecryptWithDataKeyAes256-Result-PlainText-Length", len(plainText))

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if k.kmsClient == nil {
		err = errors.New("DecryptWithDataKeyAes256 with KMS Failed: " + "KMS Client is Required")
		return "", err
	}

	if len(k.AesKmsKeyName) <= 0 {
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
	keyId := "alias/" + k.AesKmsKeyName
	cipherBytes, ce := util.HexToByte(cipherKey)

	if ce != nil {
		err = errors.New("DecryptWithDataKeyAes256 with KMS Failed: (Unmarshal CipherKey Hex To Byte) " + ce.Error())
		return "", err
	}

	// decrypt cipherKey using kms cmk
	var dataKeyOutput *kms.DecryptOutput
	var e error

	if segCtx == nil {
		dataKeyOutput, e = k.kmsClient.Decrypt(&kms.DecryptInput{
			EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
			KeyId:               aws.String(keyId),
			CiphertextBlob:      cipherBytes,
		})
	} else {
		dataKeyOutput, e = k.kmsClient.DecryptWithContext(segCtx,
			&kms.DecryptInput{
				EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
				KeyId:               aws.String(keyId),
				CiphertextBlob:      cipherBytes,
			})
	}

	if e != nil {
		err = errors.New("DecryptWithDataKeyAes256 with KMS Failed: (Decrypt Data Key) " + e.Error())
		return "", err
	}

	// perform decryption action using decrypted plaintext data key
	buf, e := crypto.AesGcmDecrypt(cipherText, string(dataKeyOutput.Plaintext))

	// clean up data key from memory immediately
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
	// validate
	if k.kmsClient == nil {
		err = errors.New("ImportECCP256SignVerifyKey with KMS Failed: " + "KMS Client is Required")
		return "", err
	}

	svc := k.kmsClient

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

	cOutput, err := svc.CreateKey(cInput)
	if err != nil {
		return "", err
	}

	keyID := aws.StringValue(cOutput.KeyMetadata.KeyId)

	if keyID == "" {
		return "", errors.New("key ID is empty")
	}

	// Step 1: Get an import token and public key from KMS
	getParams := &kms.GetParametersForImportInput{
		KeyId:             aws.String(keyID),
		WrappingAlgorithm: aws.String(kms.AlgorithmSpecRsaesOaepSha256),
		WrappingKeySpec:   aws.String(kms.KeySpecRsa2048),
	}
	resp, err := svc.GetParametersForImport(getParams)
	if err != nil {
		return "", err
	}

	derKey, err := x509.MarshalPKCS8PrivateKey(eccPvk)
	if err != nil {
		return "", err
	}

	// Step 2: Encrypt the key material using the provided public key
	pubKey, err := x509.ParsePKIXPublicKey(resp.PublicKey)
	if err != nil {
		return "", err
	}
	rsaPubKey, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("public key is not RSA")
	}
	encryptedKeyMaterial, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaPubKey, derKey, nil)
	if err != nil {
		return "", err
	}

	//Step 3: Import the encrypted key material into KMS
	importParams := &kms.ImportKeyMaterialInput{
		EncryptedKeyMaterial: encryptedKeyMaterial,
		ExpirationModel:      aws.String(kms.ExpirationModelTypeKeyMaterialDoesNotExpire),
		ImportToken:          resp.ImportToken,
		KeyId:                aws.String(keyID),
		ValidTo:              nil,
	}
	_, err = svc.ImportKeyMaterial(importParams)
	if err != nil {
		return "", err
	}

	aliasName := "alias/" + keyAlias // Change to your desired alias name

	aliasInput := &kms.CreateAliasInput{
		AliasName:   aws.String(aliasName),
		TargetKeyId: aws.String(keyID),
	}

	_, err = k.kmsClient.CreateAlias(aliasInput)
	if err != nil {
		return "", err
	}

	return aws.StringValue(cOutput.KeyMetadata.Arn), nil
}
