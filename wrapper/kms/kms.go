package kms

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

// =================================================================================================================
// AWS CREDENTIAL:
//		use $> aws configure (to set aws access key and secret to target machine)
//		Store AWS Access ID and Secret Key into Default Profile Using '$ aws configure' cli
// =================================================================================================================

import (
	util "github.com/aldelo/common"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/crypto"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
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
	AesKmsKeyName string
	RsaKmsKeyName string
	SignatureKmsKeyName string

	// store aws session object
	sess *session.Session

	// store kms client object
	kmsClient *kms.KMS
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the KMS service
func (k *KMS) Connect() error {
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
			Region:      aws.String(k.AwsRegion.Key()),
			HTTPClient:  httpCli,
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

// ----------------------------------------------------------------------------------------------------------------
// kms-cmk encrypt/decrypt via aes 256 functions
// ----------------------------------------------------------------------------------------------------------------

// EncryptViaCmkAes256 will use kms cmk to encrypt plainText using aes 256 symmetric kms cmk key, and return cipherText string,
// the cipherText can only be decrypted with aes 256 symmetric kms cmk key
func (k *KMS) EncryptViaCmkAes256(plainText string) (cipherText string, err error) {
	// validate
	if k.kmsClient == nil {
		return "", errors.New("EncryptViaCmkAes256 with KMS CMK Failed: " + "KMS Client is Required")
	}

	if len(k.AesKmsKeyName) <= 0 {
		return "", errors.New("EncryptViaCmkAes256 with KMS CMK Failed: " + "AES KMS Key Name is Required")
	}

	if len(plainText) <= 0 {
		return "", errors.New("EncryptViaCmkAes256 with KMS CMK Failed: " + "PlainText is Required")
	}

	// prepare key info
	keyId := "alias/" + k.AesKmsKeyName

	// encrypt symmetric using kms cmk
	encryptedOutput, e := k.kmsClient.Encrypt(&kms.EncryptInput{
		EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
		KeyId: aws.String(keyId),
		Plaintext: []byte(plainText),
	})

	if e != nil {
		return "", errors.New("EncryptViaCmkAes256 with KMS CMK Failed: (Symmetric Encrypt) " + e.Error())
	}

	// return encrypted cipher text blob
	return util.ByteToHex(encryptedOutput.CiphertextBlob), nil
}

// ReEncryptViaCmkAes256 will re-encrypt sourceCipherText using the new targetKmsKeyName via kms, (must be targeting aes 256 key)
// the re-encrypted cipherText is then returned
func (k *KMS) ReEncryptViaCmkAes256(sourceCipherText string, targetKmsKeyName string) (targetCipherText string, err error) {
	// validate
	if k.kmsClient == nil {
		return "", errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: " + "KMS Client is Required")
	}

	if len(k.AesKmsKeyName) <= 0 {
		return "", errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: " + "AES KMS Key Name is Required")
	}

	if len(sourceCipherText) <= 0 {
		return "", errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: " + "Source CipherText is Required")
	}

	if len(targetKmsKeyName) <= 0 {
		return "", errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: " + "Target KMS Key Name is Required")
	}

	// prepare key info
	keyId := "alias/" + k.AesKmsKeyName

	// convert hex to bytes
	cipherBytes, ce := util.HexToByte(sourceCipherText)

	if ce != nil {
		return "", errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: (Unmarshal Source CipherText Hex To Byte) " + ce.Error())
	}

	// re-encrypt symmetric kms cmk
	reEncryptOutput, e := k.kmsClient.ReEncrypt(&kms.ReEncryptInput{
		SourceEncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
		SourceKeyId: aws.String(keyId),
		DestinationEncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
		DestinationKeyId: aws.String("alias/" + targetKmsKeyName),
		CiphertextBlob: cipherBytes,
	})

	if e != nil {
		return "", errors.New("ReEncryptViaCmkAes256 with KMS CMK Failed: (Symmetric ReEncrypt) " + e.Error())
	}

	// return encrypted cipher text blob
	return util.ByteToHex(reEncryptOutput.CiphertextBlob), nil
}

// DecryptViaCmkAes256 will use kms cmk to decrypt cipherText using symmetric aes 256 kms cmk key, and return plainText string,
// the cipherText can only be decrypted with the symmetric aes 256 kms cmk key
func (k *KMS) DecryptViaCmkAes256(cipherText string) (plainText string, err error) {
	// validate
	if k.kmsClient == nil {
		return "", errors.New("DecryptViaCmkAes256 with KMS CMK Failed: " + "KMS Client is Required")
	}

	if len(k.AesKmsKeyName) <= 0 {
		return "", errors.New("DecryptViaCmkAes256 with KMS CMK Failed: " + "AES KMS Key Name is Required")
	}

	if len(cipherText) <= 0 {
		return "", errors.New("DecryptViaCmkAes256 with KMS CMK Failed: " + "Cipher Text is Required")
	}

	// prepare key info
	keyId := "alias/" + k.AesKmsKeyName
	cipherBytes, ce := util.HexToByte(cipherText)

	if ce != nil {
		return "", errors.New("DecryptViaCmkAes256 with KMS CMK Failed: (Unmarshal CipherText Hex To Byte) " + ce.Error())
	}

	// decrypt symmetric using kms cmk
	decryptedOutput, e := k.kmsClient.Decrypt(&kms.DecryptInput{
		EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
		KeyId: aws.String(keyId),
		CiphertextBlob: cipherBytes,
	})

	if e != nil {
		return "", errors.New("DecryptViaCmkAes256 with KMS CMK Failed: (Symmetric Decrypt) " + e.Error())
	}

	// return decrypted cipher text blob
	return string(decryptedOutput.Plaintext), nil
}

// ----------------------------------------------------------------------------------------------------------------
// kms-cmk encrypt/decrypt via rsa 2048 public/private key functions
// ----------------------------------------------------------------------------------------------------------------

// EncryptViaCmkRsa2048 will use kms cmk to encrypt plainText with asymmetric rsa 2048 kms cmk public key, and return cipherText string,
// the cipherText can only be decrypted with the paired asymmetric rsa 2048 kms cmk private key
//
// *** To Encrypt using Public Key Outside of KMS ***
//		1) Copy Public Key from AWS KMS for the given RSA CMK
//		2) Using External RSA Public Key Crypto Encrypt Function with the given Public Key to Encrypt
func (k *KMS) EncryptViaCmkRsa2048(plainText string) (cipherText string, err error) {
	// validate
	if k.kmsClient == nil {
		return "", errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "KMS Client is Required")
	}

	if len(k.RsaKmsKeyName) <= 0 {
		return "", errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "RSA KMS Key Name is Required")
	}

	if len(plainText) <= 0 {
		return "", errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "PlainText is Required")
	}

	if len(plainText) > 214 {
		return "", errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: " + "PlainText Cannot Exceed 214 Bytes")
	}

	// prepare key info
	keyId := "alias/" + k.RsaKmsKeyName

	// encrypt asymmetric using kms cmk
	encryptedOutput, e := k.kmsClient.Encrypt(&kms.EncryptInput{
		EncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
		KeyId: aws.String(keyId),
		Plaintext: []byte(plainText),
	})

	if e != nil {
		return "", errors.New("EncryptViaCmkRsa2048 with KMS CMK Failed: (Asymmetric Encrypt) " + e.Error())
	}

	// return encrypted cipher text blob
	return util.ByteToHex(encryptedOutput.CiphertextBlob), nil
}

// ReEncryptViaCmkRsa2048 will re-encrypt sourceCipherText using the new targetKmsKeyName via kms, (must be targeting rsa 2048 key)
// the re-encrypted cipherText is then returned
func (k *KMS) ReEncryptViaCmkRsa2048(sourceCipherText string, targetKmsKeyName string) (targetCipherText string, err error) {
	// validate
	if k.kmsClient == nil {
		return "", errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: " + "KMS Client is Required")
	}

	if len(k.RsaKmsKeyName) <= 0 {
		return "", errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: " + "RSA KMS Key Name is Required")
	}

	if len(sourceCipherText) <= 0 {
		return "", errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: " + "Source CipherText is Required")
	}

	if len(targetKmsKeyName) <= 0 {
		return "", errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: " + "Target KMS Key Name is Required")
	}

	// prepare key info
	keyId := "alias/" + k.RsaKmsKeyName

	// convert hex to bytes
	cipherBytes, ce := util.HexToByte(sourceCipherText)

	if ce != nil {
		return "", errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: (Unmarshal Source CipherText Hex To Byte) " + ce.Error())
	}

	// re-encrypt asymmetric kms cmk
	reEncryptOutput, e := k.kmsClient.ReEncrypt(&kms.ReEncryptInput{
		SourceEncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
		SourceKeyId: aws.String(keyId),
		DestinationEncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
		DestinationKeyId: aws.String("alias/" + targetKmsKeyName),
		CiphertextBlob: cipherBytes,
	})

	if e != nil {
		return "", errors.New("ReEncryptViaCmkRsa2048 with KMS CMK Failed: (Asymmetric ReEncrypt) " + e.Error())
	}

	// return encrypted cipher text blob
	return util.ByteToHex(reEncryptOutput.CiphertextBlob), nil
}

// DecryptViaCmkRsa2048 will use kms cmk to decrypt cipherText using asymmetric rsa 2048 kms cmk private key, and return plainText string,
// the cipherText can only be decrypted with the asymmetric rsa 2048 kms cmk private key
func (k *KMS) DecryptViaCmkRsa2048(cipherText string) (plainText string, err error) {
	// validate
	if k.kmsClient == nil {
		return "", errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: " + "KMS Client is Required")
	}

	if len(k.RsaKmsKeyName) <= 0 {
		return "", errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: " + "RSA KMS Key Name is Required")
	}

	if len(cipherText) <= 0 {
		return "", errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: " + "Cipher Text is Required")
	}

	// prepare key info
	keyId := "alias/" + k.RsaKmsKeyName
	cipherBytes, ce := util.HexToByte(cipherText)

	if ce != nil {
		return "", errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: (Unmarshal CipherText Hex To Byte) " + ce.Error())
	}

	// decrypt symmetric using kms cmk
	decryptedOutput, e := k.kmsClient.Decrypt(&kms.DecryptInput{
		EncryptionAlgorithm: aws.String("RSAES_OAEP_SHA_256"),
		KeyId: aws.String(keyId),
		CiphertextBlob: cipherBytes,
	})

	if e != nil {
		return "", errors.New("DecryptViaCmkRsa2048 with KMS CMK Failed: (Asymmetric Decrypt) " + e.Error())
	}

	// return decrypted cipher text blob
	return string(decryptedOutput.Plaintext), nil
}

// ----------------------------------------------------------------------------------------------------------------
// kms-cmk sign/verify via rsa 2048 private/public key functions
// ----------------------------------------------------------------------------------------------------------------

// SignViaCmkRsa2048 will sign dataToSign using KMS CMK RSA Sign/Verify Key (Private Key on KMS will be used to securely sign)
func (k *KMS) SignViaCmkRsa2048(dataToSign string) (signature string, err error) {
	// validate
	if k.kmsClient == nil {
		return "", errors.New("SignViaCmkRsa2048 with KMS Failed: " + "KMS Client is Required")
	}

	if len(k.SignatureKmsKeyName) <= 0 {
		return "", errors.New("SignViaCmkRsa2048 with KMS Failed: " + "Signature KMS Key Name is Required")
	}

	if len(dataToSign) <= 0 {
		return "", errors.New("SignViaCmkRsa2048 with KMS Failed: " + "Data To Sign is Required")
	}

	// prepare key info
	keyId := "alias/" + k.SignatureKmsKeyName

	// perform sign action using kms rsa sign/verify cmk
	signOutput, e := k.kmsClient.Sign(&kms.SignInput{
		KeyId: aws.String(keyId),
		SigningAlgorithm: aws.String("RSASSA_PKCS1_V1_5_SHA_256"),
		MessageType: aws.String("RAW"),
		Message: []byte(dataToSign),
	})

	if e != nil {
		return "", errors.New("SignViaCmkRsa2048 with KMS Failed: (Sign Action) " + e.Error())
	}

	// return signature
	return util.ByteToHex(signOutput.Signature), nil
}

// VerifyViaCmkRsa2048 will verify dataToVerify with signature using KMS CMK RSA Sign/Verify Key (Public Key on KMS will be used securely to verify)
//
// signatureToVerify = prior signed signature in hex to verify against the dataToVerify parameter
//
// *** To Verify using Public Key Outside of KMS ***
//		1) Copy Public Key from AWS KMS for the given RSA CMK
//		2) Using External RSA Public Key Crypto Verify Function with the given Public Key to Verify
func (k *KMS) VerifyViaCmkRsa2048(dataToVerify string, signatureToVerify string) (signatureValid bool, err error) {
	// validate
	if k.kmsClient == nil {
		return false, errors.New("VerifyViaCmkRsa2048 with KMS Failed: " + "KMS Client is Required")
	}

	if len(k.SignatureKmsKeyName) <= 0 {
		return false, errors.New("VerifyViaCmkRsa2048 with KMS Failed: " + "Signature KMS Key Name is Required")
	}

	if len(dataToVerify) <= 0 {
		return false, errors.New("VerifyViaCmkRsa2048 with KMS Failed: " + "Data To Verify is Required")
	}

	if len(signatureToVerify) <= 0 {
		return false, errors.New("VerifyViaCmkRsa2048 with KMS Failed: " + "Signature To Verify is Required")
	}

	// prepare key info
	keyId := "alias/" + k.SignatureKmsKeyName
	signatureBytes, ce := util.HexToByte(signatureToVerify)

	if ce != nil {
		return false, errors.New("VerifyViaCmkRsa2048 with KMS Failed: (Marshal SignatureToVerify Hex To Byte) " + ce.Error())
	}

	// perform verify action using kms rsa sign/verify cmk
	verifyOutput, e := k.kmsClient.Verify(&kms.VerifyInput{
		KeyId: aws.String(keyId),
		SigningAlgorithm: aws.String("RSASSA_PKCS1_V1_5_SHA_256"),
		MessageType: aws.String("RAW"),
		Message: []byte(dataToVerify),
		Signature: signatureBytes,
	})

	if e != nil {
		return false, errors.New("VerifyViaCmkRsa2048 with KMS Failed: (Verify Action) " + e.Error())
	}

	// return verify result
	return *verifyOutput.SignatureValid, nil
}

// ----------------------------------------------------------------------------------------------------------------
// kms data-key encrypt/decrypt aes 256 functions
// ----------------------------------------------------------------------------------------------------------------

// GenerateDataKeyAes256 will return an encrypted data key generated by kms cmk,
// this data key is encrypted, and able to decrypt only via kms cmk (therefore it is safe to store in memory or at rest)
//
// cipherKey = encrypted data key in hex (must use KMS CMK to decrypt such key)
func (k *KMS) GenerateDataKeyAes256() (cipherKey string, err error) {
	// validate
	if k.kmsClient == nil {
		return "", errors.New("GenerateDataKeyAes256 with KMS Failed: " + "KMS Client is Required")
	}

	if len(k.AesKmsKeyName) <= 0 {
		return "", errors.New("GenerateDataKeyAes256 with KMS Failed: " + "AES KMS Key Name is Required")
	}

	// prepare key info
	keyId := "alias/" + k.AesKmsKeyName
	keySpec := "AES_256" // always use AES 256
	dataKeyInput := kms.GenerateDataKeyWithoutPlaintextInput{
		KeyId: aws.String(keyId),
		KeySpec: aws.String(keySpec),
	}

	// generate data key
	dataKeyOutput, e := k.kmsClient.GenerateDataKeyWithoutPlaintext(&dataKeyInput)

	if e != nil {
		return "", errors.New("GenerateDataKeyAes256 with KMS Failed: (Gen Data Key) " + e.Error())
	}

	// return encrypted key via cipherKey
	return util.ByteToHex(dataKeyOutput.CiphertextBlob), nil
}

// EncryptWithDataKeyAes256 will encrypt plainText using cipherKey that was generated via GenerateDataKeyAes256()
//
// cipherKey = encrypted data key in hex (must use KMS CMK to decrypt such key)
func (k *KMS) EncryptWithDataKeyAes256(plainText string, cipherKey string) (cipherText string, err error) {
	// validate
	if k.kmsClient == nil {
		return "", errors.New("EncryptWithDataKeyAes256 with KMS Failed: " + "KMS Client is Required")
	}

	if len(k.AesKmsKeyName) <= 0 {
		return "", errors.New("EncryptWithDataKeyAes256 with KMS Failed: " + "AES KMS Key Name is Required")
	}

	if len(plainText) <= 0 {
		return "", errors.New("EncryptWithDataKeyAes256 with KMS Failed: " + "PlainText is Required")
	}

	if len(cipherKey) <= 0 {
		return "", errors.New("EncryptWithDataKeyAes256 with KMS Failed: " + "CipherKey is Required")
	}

	// prepare key info
	keyId := "alias/" + k.AesKmsKeyName
	cipherBytes, ce := util.HexToByte(cipherKey)

	if ce != nil {
		return "", errors.New("EncryptWithDataKeyAes256 with KMS Failed: (Unmarshal CipherKey Hex To Byte) " + ce.Error())
	}

	// decrypt cipherKey using kms cmk
	dataKeyOutput, e := k.kmsClient.Decrypt(&kms.DecryptInput{
		EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
		KeyId: aws.String(keyId),
		CiphertextBlob: cipherBytes,
	})

	if e != nil {
		return "", errors.New("EncryptWithDataKeyAes256 with KMS Failed: (Decrypt Data Key) " + e.Error())
	}

	// perform encryption action using decrypted plaintext data key
	buf, e := crypto.AesGcmEncrypt(plainText, string(dataKeyOutput.Plaintext))

	// clean up data key from memory immediately
	dataKeyOutput.SetPlaintext([]byte{})
	dataKeyOutput = nil

	// evaluate encrypted result
	if e != nil {
		return "", errors.New("EncryptWithDataKeyAes256 with KMS Failed: (Encrypt Data) " + e.Error())
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
	// validate
	if k.kmsClient == nil {
		return "", errors.New("DecryptWithDataKeyAes256 with KMS Failed: " + "KMS Client is Required")
	}

	if len(k.AesKmsKeyName) <= 0 {
		return "", errors.New("DecryptWithDataKeyAes256 with KMS Failed: " + "AES KMS Key Name is Required")
	}

	if len(cipherText) <= 0 {
		return "", errors.New("DecryptWithDataKeyAes256 with KMS Failed: " + "CipherText is Required")
	}

	if len(cipherKey) <= 0 {
		return "", errors.New("DecryptWithDataKeyAes256 with KMS Failed: " + "CipherKey is Required")
	}

	// prepare key info
	keyId := "alias/" + k.AesKmsKeyName
	cipherBytes, ce := util.HexToByte(cipherKey)

	if ce != nil {
		return "", errors.New("DecryptWithDataKeyAes256 with KMS Failed: (Unmarshal CipherKey Hex To Byte) " + ce.Error())
	}

	// decrypt cipherKey using kms cmk
	dataKeyOutput, e := k.kmsClient.Decrypt(&kms.DecryptInput{
		EncryptionAlgorithm: aws.String("SYMMETRIC_DEFAULT"),
		KeyId: aws.String(keyId),
		CiphertextBlob: cipherBytes,
	})

	if e != nil {
		return "", errors.New("DecryptWithDataKeyAes256 with KMS Failed: (Decrypt Data Key) " + e.Error())
	}

	// perform decryption action using decrypted plaintext data key
	buf, e := crypto.AesGcmDecrypt(cipherText, string(dataKeyOutput.Plaintext))

	// clean up data key from memory immediately
	dataKeyOutput.SetPlaintext([]byte{})
	dataKeyOutput = nil

	// evaluate decrypted result
	if e != nil {
		return "", errors.New("DecryptWithDataKeyAes256 with KMS Failed: (Decrypt Data) " + e.Error())
	} else {
		plainText = buf
	}

	// return decrypted data
	return plainText, nil
}

