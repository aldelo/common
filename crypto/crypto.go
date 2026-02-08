package crypto

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
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"

	util "github.com/aldelo/common"
	"github.com/aldelo/common/ascii"
)

// ================================================================================================================
// KEY HELPERS
// ================================================================================================================

// Generate32ByteRandomKey will generate a random 32 byte key based on passphrase and random salt,
// passphrase does not need to be any specific length
func Generate32ByteRandomKey(passphrase string) (string, error) {
	// validate passphrase
	if len(passphrase) == 0 {
		return "", errors.New("Passphrase is Required")
	}

	// generate salt
	salt := make([]byte, 32)

	// populate salt
	if _, err1 := io.ReadFull(rand.Reader, salt); err1 != nil {
		return "", err1
	}

	// generate key
	key, err2 := scrypt.Key([]byte(passphrase), salt, 32768, 8, 1, 32)

	if err2 != nil {
		return "", err2
	}

	return util.ByteToHex(key), nil
}

// ================================================================================================================
// FNV HELPERS
// ================================================================================================================

// FnvHashDigit returns persistent hash digit value, limited by the digit limit parameter
func FnvHashDigit(data string, digitLimit int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(data))

	if digitLimit < 2 {
		digitLimit = 2
	}

	return int(h.Sum32()%uint32(digitLimit-1) + 1)
}

// ================================================================================================================
// MD5 HELPERS
// ================================================================================================================

// Md5 hashing
func Md5(data string, salt string) string {
	b := []byte(data + salt)
	return fmt.Sprintf("%X", md5.Sum(b))
}

// ================================================================================================================
// SHA256 HELPERS
// ================================================================================================================

// Sha256 hashing (always 64 bytes output)
func Sha256(data string, salt string) string {
	b := []byte(data + salt)
	return fmt.Sprintf("%X", sha256.Sum256(b))
}

// ================================================================================================================
// BCRYPT HELPERS
// ================================================================================================================

// PasswordHash uses BCrypt to hash the given password and return a corresponding hash,
// suggested cost = 13 （440ms),
// if cost is left as 0, then default 13 is assumed
func PasswordHash(password string, cost int) (string, error) {
	if cost <= 0 {
		cost = 13
	} else if cost < 12 {
		cost = 12
	} else if cost > 31 {
		cost = 31
	}

	b, err := bcrypt.GenerateFromPassword([]byte(password), cost)

	if err != nil {
		return "", err
	}

	return string(b), nil
}

// PasswordVerify uses BCrypt to verify the input password against a prior hash version to see if match
func PasswordVerify(password string, hash string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))

	if err != nil {
		return false, err
	}

	return true, nil
}

// ================================================================================================================
// AES-GCM HELPERS
// ================================================================================================================

// AesGcmEncrypt will encrypt using aes gcm 256 bit,
// passphrase must be 32 bytes, if over 32 bytes, it be truncated,
// encrypted data is represented in hex value
func AesGcmEncrypt(data string, passphrase string) (string, error) {
	// ensure data has value
	if len(data) == 0 {
		return "", errors.New("Data to Encrypt is Required")
	}

	// ensure passphrase is 32 bytes
	// note： passphrase is just ascii, not hex, client should not convert to hex
	if len(passphrase) < 32 {
		return "", errors.New("Passphrase Must Be 32 Bytes")
	}

	// cut the passphrase to 32 bytes only
	passphrase = util.Left(passphrase, 32)

	// convert data and passphrase into byte array
	text := []byte(data)
	key := []byte(passphrase)

	// generate a new aes cipher using the 32 bytes key
	c, err1 := aes.NewCipher(key)

	// on error stop
	if err1 != nil {
		return "", err1
	}

	// using gcm (Galois/Counter Mode) for symmetric key cryptographic block ciphers
	// https://en.wikipedia.org/wiki/Galois/Counter_Mode
	gcm, err2 := cipher.NewGCM(c)

	// on error stop
	if err2 != nil {
		return "", err2
	}

	// create a new byte array the size of the nonce,
	// which must be passed to Seal
	nonce := make([]byte, gcm.NonceSize())

	// populate our nonce with a cryptographically secure random sequence
	if _, err3 := io.ReadFull(rand.Reader, nonce); err3 != nil {
		return "", err3
	}

	// encrypt our text using Seal function,
	// Seal encrypts and authenticates plaintext,
	// authenticates the additional data and appends the result to dst,
	// returning the updated slice,
	// the nonce must be NonceSize() bytes long and unique for all time, for a given key

	// return util.ByteToHex(gcm.Seal(nonce, nonce, text, nil)), nil
	return util.ByteToHex(gcm.Seal(nonce, nonce, text, nil)), nil
}

// AesGcmDecrypt will decrypt using aes gcm 256 bit,
// passphrase must be 32 bytes, if over 32 bytes, it be truncated
func AesGcmDecrypt(data string, passphrase string) (string, error) {
	// ensure data has value
	if len(data) == 0 {
		return "", errors.New("Data to Decrypt is Required")
	}

	// ensure passphrase is 32 bytes
	// note： passphrase is just ascii, not hex, client should not convert to hex
	if len(passphrase) < 32 {
		return "", errors.New("Passphrase Must Be 32 Bytes")
	}

	// cut the passphrase to 32 bytes only
	passphrase = util.Left(passphrase, 32)

	// convert data and passphrase into byte array
	text, err := util.HexToByte(data)

	if err != nil {
		return "", err
	}

	key := []byte(passphrase)

	// create aes cipher
	c, err1 := aes.NewCipher(key)

	// on error stop
	if err1 != nil {
		return "", err1
	}

	// create gcm (Galois/Counter Mode) to decrypt
	gcm, err2 := cipher.NewGCM(c)

	// on error stop
	if err2 != nil {
		return "", err2
	}

	// create nonce
	nonceSize := gcm.NonceSize()

	if len(text) < nonceSize {
		return "", errors.New("Cipher Text Smaller Than Nonce Size")
	}

	// get cipher text
	nonce, text := text[:nonceSize], text[nonceSize:]

	// decrypt cipher text
	plaintext, err3 := gcm.Open(nil, nonce, text, nil)

	// on error stop
	if err3 != nil {
		return "", err3
	}

	// return decrypted text
	return string(plaintext), nil
}

// ================================================================================================================
// AES-CFB HELPERS
// ================================================================================================================

// AesCfbEncrypt will encrypt using aes cfb 256 bit,
// passphrase must be 32 bytes, if over 32 bytes, it be truncated,
// encrypted data is represented in hex value
func AesCfbEncrypt(data string, passphrase string) (string, error) {
	// ensure data has value
	if len(data) == 0 {
		return "", errors.New("Data to Encrypt is Required")
	}

	// ensure passphrase is 32 bytes
	if len(passphrase) < 32 {
		return "", errors.New("Passphrase Must Be 32 Bytes")
	}

	// cut the passphrase to 32 bytes only
	passphrase = util.Left(passphrase, 32)

	// convert data and passphrase into byte array
	text := []byte(data)
	key := []byte(passphrase)

	// generate a new aes cipher using the 32 bytes key
	c, err1 := aes.NewCipher(key)

	// on error stop
	if err1 != nil {
		return "", err1
	}

	// iv needs to be unique, but doesn't have to be secure,
	// its common to put it at the beginning of the cipher text
	cipherText := make([]byte, aes.BlockSize+len(text))

	iv := cipherText[:aes.BlockSize]

	if _, err2 := io.ReadFull(rand.Reader, iv); err2 != nil {
		return "", err2
	}

	// encrypt
	stream := cipher.NewCFBEncrypter(c, iv)
	stream.XORKeyStream(cipherText[aes.BlockSize:], text)

	// return encrypted data
	return util.ByteToHex(cipherText), nil
}

// AesCfbDecrypt will decrypt using aes cfb 256 bit,
// passphrase must be 32 bytes, if over 32 bytes, it be truncated
func AesCfbDecrypt(data string, passphrase string) (string, error) {
	// ensure data has value
	if len(data) == 0 {
		return "", errors.New("Data to Decrypt is Required")
	}

	// ensure passphrase is 32 bytes
	if len(passphrase) < 32 {
		return "", errors.New("Passphrase Must Be 32 Bytes")
	}

	// cut the passphrase to 32 bytes only
	passphrase = util.Left(passphrase, 32)

	// convert data and passphrase into byte array
	text, err := util.HexToByte(data)

	if err != nil {
		return "", err
	}

	key := []byte(passphrase)

	// create aes cipher
	c, err1 := aes.NewCipher(key)

	// on error stop
	if err1 != nil {
		return "", err1
	}

	// ensure cipher block size appropriate
	if len(text) < aes.BlockSize {
		return "", errors.New("Cipher Text Block Size Too Short")
	}

	// iv needs to be unique, doesn't have to secured,
	// it's common to put iv at the beginning of the cipher text
	iv := text[:aes.BlockSize]
	text = text[aes.BlockSize:]

	// decrypt
	stream := cipher.NewCFBDecrypter(c, iv)

	// XORKeyStream can work in-place if two arguments are the same
	stream.XORKeyStream(text, text)

	// return decrypted data
	return string(text), nil
}

// ================================================================================================================
// AES-CBC HELPERS
// ================================================================================================================

// AesCbcEncrypt will encrypt using aes cbc 256 bit,
// passphrase must be 32 bytes, if over 32 bytes, it be truncated,
// encrypted data is represented in hex value
func AesCbcEncrypt(data string, passphrase string) (string, error) {
	// ensure data has value
	if len(data) == 0 {
		return "", errors.New("Data to Encrypt is Required")
	}

	// ensure passphrase is 32 bytes
	if len(passphrase) < 32 {
		return "", errors.New("Passphrase Must Be 32 Bytes")
	}

	// cut the passphrase to 32 bytes only
	passphrase = util.Left(passphrase, 32)

	// in cbc, data must be in block size of aes (multiples of 16 bytes),
	// otherwise padding needs to be performed
	if len(data)%aes.BlockSize != 0 {
		// pad with blank spaces up to 16 bytes
		data = util.Padding(data, util.NextFixedLength(data, aes.BlockSize), true, ascii.AsciiToString(ascii.NUL))
	}

	// convert data and passphrase into byte array
	text := []byte(data)
	key := []byte(passphrase)

	// generate a new aes cipher using the 32 bytes key
	c, err1 := aes.NewCipher(key)

	// on error stop
	if err1 != nil {
		return "", err1
	}

	// iv needs to be unique, but doesn't have to be secure,
	// its common to put it at the beginning of the cipher text
	cipherText := make([]byte, aes.BlockSize+len(text))

	// create iv, length must be equal to block size
	iv := cipherText[:aes.BlockSize]

	if _, err2 := io.ReadFull(rand.Reader, iv); err2 != nil {
		return "", err2
	}

	// encrypt
	mode := cipher.NewCBCEncrypter(c, iv)
	mode.CryptBlocks(cipherText[aes.BlockSize:], text)

	// return encrypted data
	return util.ByteToHex(cipherText), nil
}

// AesCbcDecrypt will decrypt using aes cbc 256 bit,
// passphrase must be 32 bytes, if over 32 bytes, it be truncated
func AesCbcDecrypt(data string, passphrase string) (string, error) {
	// ensure data has value
	if len(data) == 0 {
		return "", errors.New("Data to Decrypt is Required")
	}

	// ensure passphrase is 32 bytes
	if len(passphrase) < 32 {
		return "", errors.New("Passphrase Must Be 32 Bytes")
	}

	// cut the passphrase to 32 bytes only
	passphrase = util.Left(passphrase, 32)

	// convert data and passphrase into byte array
	text, err := util.HexToByte(data)

	if err != nil {
		return "", err
	}

	key := []byte(passphrase)

	// create aes cipher
	c, err1 := aes.NewCipher(key)

	// on error stop
	if err1 != nil {
		return "", err1
	}

	// ensure cipher block size appropriate
	if len(text) < aes.BlockSize {
		return "", errors.New("Cipher Text Block Size Too Short")
	}

	// iv needs to be unique, doesn't have to secured,
	// it's common to put iv at the beginning of the cipher text
	iv := text[:aes.BlockSize]
	text = text[aes.BlockSize:]

	// cbc mode always work in whole blocks
	if len(text)%aes.BlockSize != 0 {
		return "", errors.New("Cipher Text Must Be In Multiple of Block Size")
	}

	// decrypt
	mode := cipher.NewCBCDecrypter(c, iv)
	mode.CryptBlocks(text, text)

	// return decrypted data
	return strings.ReplaceAll(string(text[:]), ascii.AsciiToString(ascii.NUL), ""), nil
}

// ================================================================================================================
// HMAC HELPERS
// ================================================================================================================

// AppendHmac will calculate the hmac for the given encrypted data based on the given key,
// and append the Hmac to the end of the encrypted data and return the newly assembled encrypted data with hmac
// key must be 32 bytes
func AppendHmac(encryptedData string, key string) (string, error) {
	// ensure data has value
	if len(encryptedData) == 0 {
		return "", errors.New("Data is Required")
	}

	// ensure key is 32 bytes
	if len(key) < 32 {
		return "", errors.New("Key Must Be 32 Bytes")
	}

	data := []byte(encryptedData)

	// cut the key to 32 bytes only
	key = util.Left(key, 32)

	// new hmac
	macProducer := hmac.New(sha256.New, []byte(key))
	macProducer.Write(data)

	// generate mac
	strMac := util.ByteToHex(macProducer.Sum(nil))
	mac := []byte(strMac)

	// append and return
	return string(append(data, mac...)), nil
}

// ValidateHmac will verify if the appended hmac validates against the message based on the given key,
// and parse the hmac out and return the actual message if hmac validation succeeds,
// if hmac validation fails, then blank is returned and the error contains the failure reason
func ValidateHmac(encryptedDataWithHmac string, key string) (string, error) {
	// ensure data has value
	if len(encryptedDataWithHmac) <= 64 { // hex is 2x the byte, so 32 normally is now 64
		return "", errors.New("Data with HMAC is Required")
	}

	// ensure key is 32 bytes
	if len(key) < 32 {
		return "", errors.New("Key Must Be 32 Bytes")
	}

	// cut the key to 32 bytes only
	key = util.Left(key, 32)

	// data to byte
	data := []byte(encryptedDataWithHmac)

	// parse message
	message := data[:len(data)-64]
	mac, err := util.HexToByte(string(data[len(data)-64:]))

	if err != nil {
		return "", err
	}

	// new mac
	macProducer := hmac.New(sha256.New, []byte(key))
	macProducer.Write(message)

	// get calculated mac
	calculatedMac := macProducer.Sum(nil)

	if hmac.Equal(mac, calculatedMac) {
		// hmac match, return encrypted data without hmac
		return string(message), nil
	}

	// if process gets here, then hmac failed
	return "", errors.New("HMAC Verification Failed")
}

// ================================================================================================================
// RSA HELPERS
// ================================================================================================================

// There are two format types of private key PEM file (between the -----BEGIN ... and -----END ... lines).
//
// 1. PKCS#1 format - RFC 8017
//    Describes how RSA public and private keys are encoded.
//
//    -----BEGIN RSA PRIVATE KEY-----
//    (base64 data)
//    -----END RSA PRIVATE KEY-----
//
// 2. PKCS#8 format - RFC 5958
//    Defines a generic container for any kind of private key, not just RSA (e.g., RSA, EC, DSA, Ed25519, etc.).
//
//    -----BEGIN PRIVATE KEY-----
//    (base64 data)
//    -----END PRIVATE KEY-----

// RsaCreateKey generates the private and public key pair,
// expressed in hex code value
func RsaCreateKey() (privateKey string, publicKey string, err error) {
	// generate new private key
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)

	if err != nil {
		return "", "", err
	}

	// get the public key
	rsaPublicKey := &rsaKey.PublicKey

	// get private key in bytes
	bytesPrivateKey := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)})

	// get public key in bytes
	bytesPublicKey := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(rsaPublicKey)})

	// return result
	return util.ByteToHex(bytesPrivateKey), util.ByteToHex(bytesPublicKey), nil
}

// rsaPrivateKeyFromHex converts hex string private key into rsa private key object
func rsaPrivateKeyFromHex(privateKeyHex string) (*rsa.PrivateKey, error) {
	// convert hex to bytes
	bytesPrivateKey, err := util.HexToByte(privateKeyHex)

	if err != nil {
		return nil, err
	}

	// convert byte to object
	block, _ := pem.Decode(bytesPrivateKey)

	if block == nil {
		return nil, errors.New("RSA Private Key From Hex Fail: " + "Pem Block Nil")
	}

	// Remove deprecated x509.IsEncryptedPEMBlock and x509.DecryptPEMBlock usage.
	// This function no longer supports encrypted PEM blocks. If you need to handle
	// encrypted PEMs, decrypt them first using golang.org/x/crypto/pkcs12 or similar
	// modern crypto libraries before calling this function.
	b := block.Bytes

	// parse key
	switch block.Type {
	case "RSA PRIVATE KEY":
		// PKCS#1
		return x509.ParsePKCS1PrivateKey(b)

	case "PRIVATE KEY":
		// PKCS#8
		key, err := x509.ParsePKCS8PrivateKey(b)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("RSA Private Key From Hex Fail: parsed key is not an RSA private key")
		}
		return rsaKey, nil

	default:
		return nil, fmt.Errorf("RSA Private Key From Hex Fail: Unsupported Key Type: %s", block.Type)
	}
}

// rsaPrivateKeyFromPem converts pkcs1 string private key into rsa private key object
func rsaPrivateKeyFromPem(privateKeyPem string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privateKeyPem))

	if block == nil {
		return nil, errors.New("RSA Private Key From Pem Fail: " + "Pem Block Nil")
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		// PKCS#1
		return x509.ParsePKCS1PrivateKey(block.Bytes)

	case "PRIVATE KEY":
		// PKCS#8
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("RSA Private Key From Pem Fail: parsed key is not an RSA private key")
		}
		return rsaKey, nil

	default:
		return nil, fmt.Errorf("RSA Private Key From Pem Fail: Unsupported Key Type: %s", block.Type)
	}
}

// rsaPublicKeyFromHex converts hex string public key into rsa public key object
func rsaPublicKeyFromHex(publicKeyHex string) (*rsa.PublicKey, error) {
	// convert hex to bytes
	bytesPublicKey, err := util.HexToByte(publicKeyHex)

	if err != nil {
		return nil, err
	}

	// convert byte to object
	block, _ := pem.Decode(bytesPublicKey)

	if block == nil {
		return nil, errors.New("RSA Public Key From Hex Fail: " + "Pem Block Nil")
	}

	// Note: Encrypted PEM blocks are no longer supported due to deprecated x509.DecryptPEMBlock.
	// Use modern crypto libraries like golang.org/x/crypto/pkcs12 to decrypt before calling this function.
	b := block.Bytes

	// parse key
	key, err1 := x509.ParsePKCS1PublicKey(b)

	if err1 != nil {
		return nil, err1
	}

	// return public key
	return key, nil
}

// rsaPublicKeyFromPem converts certificate string public key into rsa public key object
func rsaPublicKeyFromPem(publicKeyPem string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKeyPem))

	if block == nil {
		return nil, errors.New("RSA Public Key From Pem Fail: " + "Pem Block Nil")
	}

	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		return nil, err
	} else {
		return key.(*rsa.PublicKey), nil
	}
}

// RsaPublicKeyEncrypt will encrypt given data using rsa public key,
// encrypted data is represented in hex value
//
// publicKeyHexOrPem = can be either HEX or PEM
func RsaPublicKeyEncrypt(data string, publicKeyHexOrPem string) (string, error) {
	// data must not exceed 190 bytes
	if len(data) > 190 {
		return "", errors.New("RSA OAEP 2048 SHA-256 Public Key Encrypt Data Must Not Exceed 190 Bytes")
	}

	if len(data) == 0 {
		return "", errors.New("Data To Encrypt is Required")
	}

	// get public key
	var publicKey *rsa.PublicKey
	var err error

	if (util.Left(publicKeyHexOrPem, 30) == "-----BEGIN RSA PUBLIC KEY-----" && util.Right(publicKeyHexOrPem, 28) == "-----END RSA PUBLIC KEY-----") ||
		(util.Left(publicKeyHexOrPem, 26) == "-----BEGIN PUBLIC KEY-----" && util.Right(publicKeyHexOrPem, 24) == "-----END PUBLIC KEY-----") {
		// get public key from pem
		publicKey, err = rsaPublicKeyFromPem(publicKeyHexOrPem)
	} else {
		// get public key from hex
		publicKey, err = rsaPublicKeyFromHex(publicKeyHexOrPem)
	}

	if err != nil {
		return "", err
	}

	// convert data into byte array
	msg := []byte(data)

	// create hash
	hash := sha256.New()

	// encrypt
	cipherText, err1 := rsa.EncryptOAEP(hash, rand.Reader, publicKey, msg, nil)

	if err1 != nil {
		return "", err1
	}

	// return encrypted value
	return util.ByteToHex(cipherText), nil
}

// RsaPrivateKeyDecrypt will decrypt rsa public key encrypted data using its corresponding rsa private key
//
// privateKeyHexOrPem = can be either HEX or PEM
func RsaPrivateKeyDecrypt(data string, privateKeyHexOrPem string) (string, error) {
	// data to decrypt must exist
	if len(data) == 0 {
		return "", errors.New("Data To Decrypt is Required")
	}

	// get private key
	var privateKey *rsa.PrivateKey
	var err error

	if (util.Left(privateKeyHexOrPem, 31) == "-----BEGIN RSA PRIVATE KEY-----" && util.Right(privateKeyHexOrPem, 29) == "-----END RSA PRIVATE KEY-----") ||
		(util.Left(privateKeyHexOrPem, 27) == "-----BEGIN PRIVATE KEY-----" && util.Right(privateKeyHexOrPem, 25) == "-----END PRIVATE KEY-----") {
		// get private key from pem text
		privateKey, err = rsaPrivateKeyFromPem(privateKeyHexOrPem)
	} else {
		// get private key from hex
		privateKey, err = rsaPrivateKeyFromHex(privateKeyHexOrPem)
	}

	if err != nil {
		return "", err
	}

	// convert hex data into byte array
	msg, err1 := util.HexToByte(data)

	if err1 != nil {
		return "", err1
	}

	// create hash
	hash := sha256.New()

	// decrypt
	plainText, err2 := rsa.DecryptOAEP(hash, rand.Reader, privateKey, msg, nil)

	if err2 != nil {
		return "", err2
	}

	// return decrypted value
	return string(plainText), nil
}

// RsaPrivateKeySign will sign the plaintext data using the given private key,
// NOTE: data must be plain text before encryption as signature verification is against plain text data
// signature is returned via hex
//
// privateKeyHexOrPem = can be either HEX or PEM
func RsaPrivateKeySign(data string, privateKeyHexOrPem string) (string, error) {
	// data is required
	if len(data) == 0 {
		return "", errors.New("Data To Sign is Required")
	}

	// get private key
	var privateKey *rsa.PrivateKey
	var err error

	if (util.Left(privateKeyHexOrPem, 31) == "-----BEGIN RSA PRIVATE KEY-----" && util.Right(privateKeyHexOrPem, 29) == "-----END RSA PRIVATE KEY-----") ||
		(util.Left(privateKeyHexOrPem, 27) == "-----BEGIN PRIVATE KEY-----" && util.Right(privateKeyHexOrPem, 25) == "-----END PRIVATE KEY-----") {
		// get private key from pem text
		privateKey, err = rsaPrivateKeyFromPem(privateKeyHexOrPem)
	} else {
		privateKey, err = rsaPrivateKeyFromHex(privateKeyHexOrPem)
	}

	if err != nil {
		return "", err
	}

	// convert data to byte array
	msg := []byte(data)

	// define hash
	h := sha256.New()
	h.Write(msg)
	d := h.Sum(nil)

	signature, err1 := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, d)

	if err1 != nil {
		return "", err1
	}

	// return signature
	return util.ByteToHex(signature), nil
}

// RsaPublicKeyVerify will verify the plaintext data using the given public key,
// NOTE: data must be plain text before encryption as signature verification is against plain text data
// if verification is successful, nil is returned, otherwise error is returned
//
// publicKeyHexOrPem = can be either HEX or PEM
func RsaPublicKeyVerify(data string, publicKeyHexOrPem string, signatureHex string) error {
	// data is required
	if len(data) == 0 {
		return errors.New("Data To Verify is Required")
	}

	// get public key
	var publicKey *rsa.PublicKey
	var err error

	if (util.Left(publicKeyHexOrPem, 30) == "-----BEGIN RSA PUBLIC KEY-----" && util.Right(publicKeyHexOrPem, 28) == "-----END RSA PUBLIC KEY-----") ||
		(util.Left(publicKeyHexOrPem, 26) == "-----BEGIN PUBLIC KEY-----" && util.Right(publicKeyHexOrPem, 24) == "-----END PUBLIC KEY-----") {
		// get public key from pem
		publicKey, err = rsaPublicKeyFromPem(publicKeyHexOrPem)
	} else {
		// get public key from hex
		publicKey, err = rsaPublicKeyFromHex(publicKeyHexOrPem)
	}

	if err != nil {
		return err
	}

	// convert data to byte array
	msg := []byte(data)

	// define hash
	h := sha256.New()
	h.Write(msg)
	d := h.Sum(nil)

	sig, sigErr := util.HexToByte(signatureHex)
	if sigErr != nil {
		return sigErr
	}

	err1 := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, d, sig)

	if err1 != nil {
		return err1
	}

	// verified
	return nil
}

// RsaPublicKeyEncryptAndPrivateKeySign will encrypt given data using recipient's rsa public key,
// and then using sender's rsa private key to sign,
// NOTE: data represents the plaintext data,
// encrypted data and signature are represented in hex values
//
// recipientPublicKeyHexOrPem = can be either HEX or PEM
// senderPrivateKeyHexOrPem = can be either HEX or PEM
func RsaPublicKeyEncryptAndPrivateKeySign(data string, recipientPublicKeyHexOrPem string, senderPrivateKeyHexOrPem string) (encryptedData string, signature string, err error) {
	encryptedData, err = RsaPublicKeyEncrypt(data, recipientPublicKeyHexOrPem)

	if err != nil {
		encryptedData = ""
		return "", "", err
	}

	signature, err = RsaPrivateKeySign(data, senderPrivateKeyHexOrPem)

	if err != nil {
		encryptedData = ""
		signature = ""
		return "", "", err
	}

	// return success values
	return encryptedData, signature, nil
}

// RsaPrivateKeyDecryptAndPublicKeyVerify will decrypt given data using recipient's rsa private key,
// and then using sender's rsa public key to verify if the signature given is a match,
// NOTE: data represents the encrypted data
//
// recipientPrivateKeyHexOrPem = can be either HEX or PEM
// senderPublicKeyHexOrPem = can be either HEX or PEM
func RsaPrivateKeyDecryptAndPublicKeyVerify(data string, recipientPrivateKeyHexOrPem string, signatureHex string, senderPublicKeyHexOrPem string) (plaintext string, verified bool, err error) {
	plaintext, err = RsaPrivateKeyDecrypt(data, recipientPrivateKeyHexOrPem)

	if err != nil {
		plaintext = ""
		return "", false, err
	}

	err = RsaPublicKeyVerify(plaintext, senderPublicKeyHexOrPem, signatureHex)

	if err != nil {
		plaintext = ""
		verified = false
		return "", false, err
	}

	// return success values
	return plaintext, true, nil
}

// ================================================================================================================
// RSA+AES DYNAMIC ENCRYPT/DECRYPT HELPERS
// ================================================================================================================

// RsaAesPublicKeyEncryptAndSign is a simplified wrapper method to generate a random AES key, then encrypt plainText using AES GCM,
// and then sign plain text data using sender's private key,
// and then using recipient's public key to encrypt the dynamic aes key,
// and finally compose the encrypted payload that encapsulates a full envelop:
//
//	<STX>RsaPublicKeyEncryptedAESKeyData + AesGcmEncryptedPayload(PlainTextData<VT>SenderPublicKey<VT>PlainTextDataSignature)<ETX>
//
//	warning: VT is used in encrypted payload as separator, make sure to escape VT if it is to be used inside the plainTextData <<< IMPORTANT
//
// recipientPublicKeyHexOrPem = can be either HEX or PEM
// senderPublicKeyHexOrPem = can be either HEX or PEM
// senderPrivateKeyHexOrPem = can be either HEX or PEM
func RsaAesPublicKeyEncryptAndSign(plainText string, recipientPublicKeyHexOrPem string, senderPublicKeyHexOrPem string, senderPrivateKeyHexOrPem string) (encryptedData string, err error) {
	// validate inputs
	if util.LenTrim(plainText) == 0 {
		return "", errors.New("Data To Encrypt is Required")
	}

	if util.LenTrim(recipientPublicKeyHexOrPem) == 0 {
		return "", errors.New("Recipient Public Key is Required")
	}

	if util.LenTrim(senderPublicKeyHexOrPem) == 0 {
		return "", errors.New("Sender Public Key is Required")
	}

	if util.LenTrim(senderPrivateKeyHexOrPem) == 0 {
		return "", errors.New("Sender Private Key is Required")
	}

	//
	// generate random aes key for data encryption during this session
	//
	aesKey, err1 := Generate32ByteRandomKey(Sha256(util.NewUUID(), util.CurrentDateTime()))

	if err1 != nil {
		return "", errors.New("Dynamic AES New Key Error: " + err1.Error())
	}

	//
	// rsa sender private key sign plain text data
	//
	signature, err2 := RsaPrivateKeySign(plainText, senderPrivateKeyHexOrPem)

	if err2 != nil {
		return "", errors.New("Dynamic AES Siganture Error: " + err2.Error())
	}

	//
	// encrypt plain text data using aes key with aes gcm,
	// note: payload format = plainText<VT>senderPublicKeyHex<VT>plainTextSignature
	//
	aesEncryptedData, err3 := AesGcmEncrypt(plainText+ascii.AsciiToString(ascii.VT)+senderPublicKeyHexOrPem+ascii.AsciiToString(ascii.VT)+signature, aesKey)

	if err3 != nil {
		return "", errors.New("Dynamic AES Data Encrypt Error: " + err3.Error())
	}

	//
	// now protect the aesKey with recipient's public key using rsa encrypt
	//
	aesEncryptedKey, err4 := RsaPublicKeyEncrypt(aesKey, recipientPublicKeyHexOrPem)

	if err4 != nil {
		return "", errors.New("Dynamic AES Key Encrypt Error: " + err4.Error())
	}

	//
	// compose output encrypted payload
	// note: aesEncryptedKey_512_Bytes_Always + aesEncryptedData_Variable_Bytes + 64BytesRecipientPublicKeyHashWithSalt of 'TPK@2019' (TPK@2019 doesn't represent anything, just for backward compatibility in prior encrypted data)
	//       parse first 512 bytes of payload = aes encrypted key (use recipient rsa private key to decrypt)
	//       parse rest other than first 512 bytes of payload = aes encrypted data, using aes key to decrypt, contains plaintext<VT>senderPublicKey<VT>siganture
	//
	encryptedPayload := ascii.AsciiToString(ascii.STX) + aesEncryptedKey + aesEncryptedData + Sha256(recipientPublicKeyHexOrPem, "TPK@2019") + ascii.AsciiToString(ascii.ETX) // wrap in STX and ETX envelop to denote start and end of the encrypted payload

	//
	// send encrypted payload data to caller
	//
	return encryptedPayload, nil
}

// RsaAesParseTPKHashFromEncryptedPayload will get the public key TPK hash from the embedded encrypted data string
func RsaAesParseTPKHashFromEncryptedPayload(encryptedData string) string {
	// strip STX and ETX out
	data := util.Right(encryptedData, len(encryptedData)-1)
	data = util.Left(data, len(data)-1)

	// last 64 bytes of the encrypted data is the public key hash
	data = util.Right(data, 64)

	// get actual tpk key
	return data
}

// RsaAesPrivateKeyDecryptAndVerify is a simplified wrapper method to decrypt incoming encrypted payload envelop that was previously encrypted using the RsaAesPublicKeyEncryptAndSign(),
// this function will use recipient's private key to decrypt the rsa encrypted dynamic aes key and then using the dynamic aes key to decrypt the aes encrypted data payload,
// this function will then parse the decrypted payload and perform a verification of signature using the sender's public key
//
// usage tip: the sender's public key can then be used to encrypt the return data back to the sender as a reply using RsaAesPublicKeyEncryptedAndSign(),
//
//	in this usage pattern, only the public key is used in each messaging cycle, while the aes key is dynamically generated each time and no prior knowledge of it is known,
//	since the public key encrypted data cannot be decrypted unless with private key, then as long as the private key is protected, then the messaging pipeline will be secured,
//	furthermore, by using sender private key sign and sender public key verify into the message authentication, we further ensure the plain text data is coming from the expected source
//
// recipientPrivateKeyHexOrPem = can be either HEX or PEM
func RsaAesPrivateKeyDecryptAndVerify(encryptedData string, recipientPrivateKeyHexOrPem string) (plainText string, senderPublicKeyHexOrPem string, err error) {
	// validate inputs
	if util.LenTrim(encryptedData) <= 578 {
		return "", "", errors.New("Encrypted Payload Envelop Not Valid: Must Exceed 578 Bytes")
	}

	if util.Left(encryptedData, 1) != ascii.AsciiToString(ascii.STX) {
		return "", "", errors.New("Encrypted Payload Envelop Not Valid: No STX Found")
	}

	if util.Right(encryptedData, 1) != ascii.AsciiToString(ascii.ETX) {
		return "", "", errors.New("Encrypted Payload Envelop Not Valid: No ETX Found")
	}

	if util.LenTrim(recipientPrivateKeyHexOrPem) == 0 {
		return "", "", errors.New("Recipient Private Key is Required")
	}

	// strip STX and ETX out
	data := util.Right(encryptedData, len(encryptedData)-1)
	data = util.Left(data, len(data)-1)

	// parse rsa public key encrypted aes key data
	aesKeyEncrypted := util.Left(data, 512)

	// parse aes key encrypted data
	aesDataEncrypted := util.Right(data, len(data)-512)

	// parse tpk hash from aes encrypted data
	// we don't need the tpk hash for the purpose of this method call
	// the tpk hash is only used by extracting and matching registry of keys and is used outside of this decrypter
	aesDataEncrypted = util.Left(aesDataEncrypted, len(aesDataEncrypted)-64)

	//
	// decrypt aes key encrypted using rsa private key of recipient
	//
	aesKey, err1 := RsaPrivateKeyDecrypt(aesKeyEncrypted, recipientPrivateKeyHexOrPem)

	if err1 != nil {
		return "", "", errors.New("Decrypt AES Key Error: " + err1.Error())
	}

	//
	// decrypt aes data using aes key that was just decrypted
	//
	aesData, err2 := AesGcmDecrypt(aesDataEncrypted, aesKey)

	if err2 != nil {
		return "", "", errors.New("Decrypt AES Data Error: " + err2.Error())
	}

	if util.LenTrim(aesData) <= 1028 {
		return "", "", errors.New("Decrypted AES Data Not Valid: " + "Expected More Bytes")
	}

	//
	// parse decrypted data into elements using VT as separator
	// expects 3 parts exactly
	//
	parts := strings.Split(aesData, ascii.AsciiToString(ascii.VT))

	if len(parts) != 3 {
		return "", "", errors.New("Decrypted AES Data Not Valid: " + "Expected 3 Parts Exactly, " + util.Itoa(len(parts)) + " Parts Returned Instead" + aesData)
	}

	plainText = parts[0]
	senderPublicKeyHexOrPem = parts[1]
	signature := parts[2]

	if util.LenTrim(plainText) == 0 {
		plainText = ""
		senderPublicKeyHexOrPem = ""

		return "", "", errors.New("Decrypted AES Data Not Valid: " + "Empty String Not Expected")
	}

	if util.LenTrim(senderPublicKeyHexOrPem) < 512 {
		plainText = ""
		senderPublicKeyHexOrPem = ""

		return "", "", errors.New("Decrypted AES Data Not Valid: " + "Sender Public Key Not Complete")
	}

	if util.LenTrim(signature) < 512 {
		plainText = ""
		senderPublicKeyHexOrPem = ""

		return "", "", errors.New("Decrypted AES Data Not Valid: " + "Sender Signature Not Complete")
	}

	//
	// verify plain text data against signature using sender public key hex
	//
	if err3 := RsaPublicKeyVerify(plainText, senderPublicKeyHexOrPem, signature); err3 != nil {
		plainText = ""
		senderPublicKeyHexOrPem = ""

		return "", "", errors.New("Decrypted AES Data Not Valid: (Signature Not Authenticated) " + err3.Error())
	}

	//
	// decrypt completed successful
	//
	return plainText, senderPublicKeyHexOrPem, nil
}
