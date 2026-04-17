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

// FnvHashDigit returns a deterministic FNV-1a hash digit derived from data,
// clamped to the inclusive range [1, digitLimit-1]. It NEVER returns 0 and
// NEVER returns digitLimit-1 — the result lives strictly inside the open
// interval (0, digitLimit).
//
// Concretely, the returned value is (fnv32a(data) mod (digitLimit-1)) + 1.
// If digitLimit < 2 it is silently raised to 2, so the minimum output range
// is the single value {1}.
//
// IMPORTANT: callers that need a uniform 0-based distribution over [0, N)
// (e.g., modulo-based shard assignment, 0-indexed bucketing, array indices)
// will see skew — bucket 0 is never filled and bucket digitLimit-1 is never
// filled. For those use cases, derive the index differently (e.g.,
// int(fnv32a(data) % uint32(N))) rather than calling this function.
//
// Preserved as-is for v1.x per workspace rule #10 (contract stability across
// 36+ consumer repos). Any change to the range would require a coordinated
// major-version release.
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

// Md5 hashes the given data+salt with the MD5 algorithm and returns the
// uppercase hex digest.
//
// Deprecated: MD5 is cryptographically broken. It is vulnerable to
// collision attacks (Wang & Yu, 2005; chosen-prefix attacks since 2008)
// and must NOT be used for:
//
//   - password hashing (use [PasswordHash], which wraps bcrypt)
//   - digital signatures or any integrity check where an attacker may
//     influence the input (use [Sha256] or [Sha512])
//   - security tokens, session IDs, or anything that must be
//     unforgeable (use [Sha256] / [Sha512] with a keyed HMAC, or a
//     CSPRNG)
//
// The only remaining safe use is non-security checksums (e.g., a cache
// key that protects against accidental corruption, not malicious input).
// If you need a non-cryptographic hash for that purpose, prefer a
// dedicated one such as xxhash or fnv.
//
// This function remains callable in v1.x for backward compatibility and
// will be removed in v2.0.0. Consumers are expected to migrate to
// [Sha256] or [Sha512] before the v2.0.0 cut. See CHANGELOG.md and the
// "Compatibility and Stability" section of README.md for the migration
// policy.
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
// encrypted data is represented in hex value.
//
// IMPORTANT: passphrase MUST be a high-entropy 32-byte key (e.g., output of a
// cryptographic random generator), NOT a human-memorizable password. For
// password-based encryption, derive the key via scrypt or argon2id first.
// This function uses raw byte-slice truncation, not a key derivation function.
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
// passphrase must be 32 bytes, if over 32 bytes, it be truncated.
//
// IMPORTANT: passphrase MUST be a high-entropy 32-byte key (e.g., output of a
// cryptographic random generator), NOT a human-memorizable password. For
// password-based encryption, derive the key via scrypt or argon2id first.
// This function uses raw byte-slice truncation, not a key derivation function.
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

// Deprecated: AesCfbEncrypt uses CFB mode which is an unauthenticated stream cipher.
// An attacker who can modify ciphertext can produce controlled plaintext changes
// (bit-flipping attack) without detection. Use AesGcmEncrypt instead, which provides
// both confidentiality and authentication (AEAD). The CFB functions remain callable
// through the entire v1.x series per workspace rule #10; removal is scheduled for v2.0.0.
//
// AesCfbEncrypt will encrypt using aes cfb 256 bit,
// passphrase must be 32 bytes, if over 32 bytes, it be truncated,
// encrypted data is represented in hex value.
//
// IMPORTANT: passphrase MUST be a high-entropy 32-byte key, NOT a password.
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

// Deprecated: AesCfbDecrypt uses CFB mode which is an unauthenticated stream cipher.
// See AesCfbEncrypt for details. Use AesGcmDecrypt instead.
//
// AesCfbDecrypt will decrypt using aes cfb 256 bit,
// passphrase must be 32 bytes, if over 32 bytes, it be truncated.
//
// IMPORTANT: passphrase MUST be a high-entropy 32-byte key, NOT a password.
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
// encrypted data is represented in hex value.
//
// Deprecated: AesCbcEncrypt uses NUL-byte padding (0x00) to align plaintext to
// the AES block size, and its paired AesCbcDecrypt strips ALL trailing NUL
// bytes from the decrypted output. This causes silent data corruption for any
// plaintext whose last byte is legitimately 0x00 — the trailing NUL is
// indistinguishable from padding and is removed on decrypt. CBC also lacks
// authentication, so a tampered ciphertext decrypts without error and returns
// attacker-influenced plaintext.
//
// Use AesGcmEncrypt / AesGcmDecrypt instead. AES-GCM is an authenticated
// encryption mode (AEAD) that:
//   - detects tampering via an authentication tag (decrypt returns an error
//     on any bit flip),
//   - requires no padding (so arbitrary byte sequences including embedded or
//     trailing NUL bytes round-trip exactly), and
//   - is the current default for symmetric bulk encryption per NIST SP 800-38D.
//
// AesCbcEncrypt remains fully supported for the v1.x release series per
// workspace rule #10 (observable-contract stability across minor versions).
// Scheduled for removal in v2.0.0. See
// _src/docs/repos/common/findings/remediation-report-2026-04-11-release-readiness.md
// P0-4 for the full hazard analysis.
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
// passphrase must be 32 bytes, if over 32 bytes, it be truncated.
//
// Deprecated: AesCbcDecrypt strips ALL trailing NUL bytes (0x00) from the
// decrypted output via strings.ReplaceAll. This is how AesCbcEncrypt's
// NUL-padding is removed, but it also removes legitimate trailing NUL bytes
// that were part of the original plaintext — silent data corruption. CBC
// additionally lacks authentication, so a tampered or truncated ciphertext
// decrypts without error and returns attacker-influenced plaintext.
//
// Use AesGcmDecrypt (paired with AesGcmEncrypt) instead. AES-GCM is AEAD:
// it returns an error on any ciphertext tampering and preserves arbitrary
// byte sequences (including embedded or trailing NULs) exactly.
//
// AesCbcDecrypt remains fully supported for the v1.x release series per
// workspace rule #10 (observable-contract stability across minor versions).
// Scheduled for removal in v2.0.0. See
// _src/docs/repos/common/findings/remediation-report-2026-04-11-release-readiness.md
// P0-4 for the full hazard analysis.
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
// and append the Hmac to the end of the encrypted data and return the newly assembled encrypted data with hmac.
//
// IMPORTANT: key MUST be a high-entropy 32-byte key (e.g., the AES key used to encrypt
// the data), NOT a human-memorizable password. This function uses raw byte-slice
// truncation, not a key derivation function.
func AppendHmac(encryptedData string, key string) (string, error) {
	// ensure data has value
	if len(encryptedData) == 0 {
		return "", errors.New("Data is Required")
	}

	// ensure key is at least 32 bytes
	if len(key) < 32 {
		return "", errors.New("Key Must Be At Least 32 Bytes")
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
// if hmac validation fails, then blank is returned and the error contains the failure reason.
//
// IMPORTANT: key MUST be a high-entropy 32-byte key (e.g., the AES key used to encrypt
// the data), NOT a human-memorizable password. This function uses raw byte-slice
// truncation, not a key derivation function.
func ValidateHmac(encryptedDataWithHmac string, key string) (string, error) {
	// ensure data has value
	if len(encryptedDataWithHmac) <= 64 { // hex is 2x the byte, so 32 normally is now 64
		return "", errors.New("Data with HMAC is Required")
	}

	// ensure key is at least 32 bytes
	if len(key) < 32 {
		return "", errors.New("Key Must Be At Least 32 Bytes")
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

	// Check for encrypted PEM blocks and return explicit error
	// Encrypted blocks have DEK-Info header or Proc-Type: 4,ENCRYPTED
	if _, encrypted := block.Headers["DEK-Info"]; encrypted {
		return nil, errors.New("RSA Public Key From Hex Fail: Encrypted PEM blocks are no longer supported. Use golang.org/x/crypto/pkcs12 or similar to decrypt before calling this function")
	}
	if procType, ok := block.Headers["Proc-Type"]; ok && strings.Contains(procType, "ENCRYPTED") {
		return nil, errors.New("RSA Public Key From Hex Fail: Encrypted PEM blocks are no longer supported. Use golang.org/x/crypto/pkcs12 or similar to decrypt before calling this function")
	}

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

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("RSA Public Key From Pem Fail: parsed key is not an RSA public key")
	}
	return rsaKey, nil
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
//
// Deprecated: this function uses two brittle constructions that its
// replacement avoids:
//
//  1. The trailing 64-char hex value appended to the envelope is
//     Sha256(recipientPublicKey, "TPK@2019") — a public, unkeyed hash that
//     any attacker can recompute. It is used only as a recipient-key lookup
//     identifier (see RsaAesParseTPKHashFromEncryptedPayload), but its
//     presence alongside the ciphertext invites readers to mistake it for
//     an integrity tag. There is no envelope-level keyed integrity binding
//     the RSA-wrapped AES key to the AES-GCM ciphertext.
//  2. The AES-GCM plaintext is the 0x0B (VT) delimited triple
//     plainText<VT>senderPublicKey<VT>signature. If the caller's plainText
//     itself contains a VT byte, the decrypt-side parser (strings.Split)
//     splits incorrectly and the envelope is unrecoverable — a silent
//     footgun that the godoc warns about but is easy to trip on.
//
// Use RsaAesPublicKeyEncryptAndSignHmac instead. It keeps the same
// RSA-OAEP-SHA256 key wrapping + AES-GCM payload encryption, but:
//   - replaces the VT-delimited triple with length-prefixed fields so
//     arbitrary byte sequences (including embedded VT / NUL / control
//     bytes) round-trip exactly, and
//   - appends a real HMAC-SHA256 tag keyed by the per-envelope AES key
//     over the entire RSA-wrapped-key || AES-GCM-ciphertext body, with
//     a 2-byte "V2" format marker so V1 and V2 payloads are unambiguously
//     distinguishable at decrypt time.
//
// This function remains fully callable through the entire v1.x release
// series per workspace rule #10 (observable-contract stability). Removal
// is scheduled for v2.0.0. See
// _src/docs/repos/common/findings/remediation-report-2026-04-11-release-readiness.md
// P0-5 for the full hazard analysis.
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
		return "", fmt.Errorf("Dynamic AES New Key Error: %w", err1)
	}

	//
	// rsa sender private key sign plain text data
	//
	signature, err2 := RsaPrivateKeySign(plainText, senderPrivateKeyHexOrPem)

	if err2 != nil {
		return "", fmt.Errorf("Dynamic AES Siganture Error: %w", err2)
	}

	//
	// encrypt plain text data using aes key with aes gcm,
	// note: payload format = plainText<VT>senderPublicKeyHex<VT>plainTextSignature
	//
	aesEncryptedData, err3 := AesGcmEncrypt(plainText+ascii.AsciiToString(ascii.VT)+senderPublicKeyHexOrPem+ascii.AsciiToString(ascii.VT)+signature, aesKey)

	if err3 != nil {
		return "", fmt.Errorf("Dynamic AES Data Encrypt Error: %w", err3)
	}

	//
	// now protect the aesKey with recipient's public key using rsa encrypt
	//
	aesEncryptedKey, err4 := RsaPublicKeyEncrypt(aesKey, recipientPublicKeyHexOrPem)

	if err4 != nil {
		return "", fmt.Errorf("Dynamic AES Key Encrypt Error: %w", err4)
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
//
// Deprecated: this function is the decrypt counterpart of the deprecated
// RsaAesPublicKeyEncryptAndSign and shares its two hazards:
//
//  1. It trusts the trailing 64-char TPK hash as a routing identifier but
//     performs NO envelope-level keyed integrity check before handing the
//     AES-GCM ciphertext to the decrypter. GCM itself authenticates the
//     inner plaintext, but a tampered rsaEncryptedAesKey (or a tampered
//     concatenation of the two) is not detected until GCM decryption fails
//     — and even then, an attacker who can re-wrap a chosen AES key under
//     the recipient's public key can substitute the entire payload at the
//     price of only the GCM tag.
//  2. It parses the decrypted inner plaintext with strings.Split on 0x0B
//     (VT), so any envelope whose original plaintext contained a VT byte
//     decodes into the wrong number of parts and fails with a confusing
//     "Expected 3 Parts Exactly" error instead of returning the caller's
//     data.
//
// Use RsaAesPrivateKeyDecryptAndVerifyHmac instead. It verifies an
// HMAC-SHA256 tag over rsaEncryptedAesKey || aesGcmEncryptedBody BEFORE
// invoking the GCM decrypter (fail-fast on tampering), and parses the
// inner plaintext with length-prefixed fields so arbitrary byte sequences
// round-trip exactly.
//
// This function remains fully callable through the entire v1.x release
// series per workspace rule #10 (observable-contract stability). Removal
// is scheduled for v2.0.0. See
// _src/docs/repos/common/findings/remediation-report-2026-04-11-release-readiness.md
// P0-5 for the full hazard analysis.
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
		return "", "", fmt.Errorf("Decrypt AES Key Error: %w", err1)
	}

	//
	// decrypt aes data using aes key that was just decrypted
	//
	aesData, err2 := AesGcmDecrypt(aesDataEncrypted, aesKey)

	if err2 != nil {
		return "", "", fmt.Errorf("Decrypt AES Data Error: %w", err2)
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
		return "", "", errors.New("Decrypted AES Data Not Valid: " + "Expected 3 Parts Exactly, " + util.Itoa(len(parts)) + " Parts Returned Instead")
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

		return "", "", fmt.Errorf("Decrypted AES Data Not Valid: (Signature Not Authenticated) %w", err3)
	}

	//
	// decrypt completed successful
	//
	return plainText, senderPublicKeyHexOrPem, nil
}

// =====================================================================
// P0-5 — RSA/AES envelope V2 (HMAC-keyed sibling pair)
//
// The functions below are the additive v1.7.9 replacement for
// RsaAesPublicKeyEncryptAndSign / RsaAesPrivateKeyDecryptAndVerify. The
// V1 pair remains fully callable and its wire format is unchanged — per
// workspace rule #10 (observable-contract stability). This V2 pair is
// meant for NEW callers; existing callers should migrate before v2.0.0
// (at which point V1 is scheduled for removal).
//
// V2 envelope wire format:
//
//	<STX> V2 <rsaEncryptedAesKey : 512 hex chars>
//	        <aesGcmEncryptedBodyHex : variable>
//	        <hmacSha256Hex : 64 chars>
//	<ETX>
//
// The leading literal "V2" is the format marker. 'V' is not a hex char,
// so a V1 decrypter fed a V2 payload fails at its "aes key must be 512
// hex chars" check, and a V2 decrypter fed a V1 payload fails at its
// "first two bytes must be V2" check. V1 and V2 payloads are therefore
// unambiguously distinguishable and never cross-decode.
//
// Inside the AES-GCM body, the plaintext is NOT a VT-delimited triple.
// It is three length-prefixed fields concatenated:
//
//	encodeLenPrefixedField(plainText) +
//	encodeLenPrefixedField(senderPublicKeyHexOrPem) +
//	encodeLenPrefixedField(signature)
//
// Each field starts with an 8-char uppercase-hex length (uint32, so the
// per-field cap is 4 GiB — more than enough for any realistic payload)
// followed by the field's raw bytes. Because the length is explicit,
// arbitrary byte sequences (including embedded 0x0B/VT, 0x00/NUL, and
// any other control byte) round-trip exactly. This directly closes the
// V1 "plainText must not contain VT" footgun.
//
// Integrity is provided by HMAC-SHA256 over
//
//	rsaEncryptedAesKey || aesGcmEncryptedBody
//
// keyed by the per-envelope 32-byte AES key. This AES key is already
// protected end-to-end via RSA-OAEP-SHA256 wrapping (same as V1), so
// anyone who could recompute the HMAC must also already possess the
// recipient's private key and therefore already be able to decrypt
// everything anyway — the HMAC binds the RSA-wrapped key to the GCM
// body at the envelope level, detects tampering of either component
// BEFORE the GCM decrypter is invoked (fail-fast), and removes any
// reliance on the unkeyed TPK hash for integrity purposes. The TPK
// hash is intentionally NOT present in V2; recipient-key lookup in V2
// deployments can be done with an out-of-band identifier or by trying
// known recipient keys in order.
//
// Rationale (why HMAC even though GCM is already AEAD): GCM's AEAD tag
// covers only the inner plaintext once you have the right key. The
// HMAC covers the whole envelope (wrapped-key || body) with the same
// symmetric key, so a bit-flip in the RSA-wrapped-key portion — which
// would otherwise only surface as "RSA decrypt produced garbage key,
// GCM tag failed, return generic error" — is detected and rejected
// before any RSA private-key operation is even attempted. This both
// speeds up the hot path on rejection and avoids leaking timing
// information about RSA decrypt failures.
// =====================================================================

// encodeLenPrefixedField encodes s as an 8-char uppercase-hex uint32
// length followed by the raw bytes of s. Used by the V2 envelope's
// AES-GCM inner plaintext to frame the (plainText, senderPublicKey,
// signature) triple without an ambiguous delimiter. Length-prefixing
// lets the decoder handle arbitrary byte sequences — including any
// control byte that would have broken a VT-delimited format.
//
// The 8-hex-char length caps any single field at (2^32 - 1) bytes
// (~4 GiB), which is far beyond any realistic envelope payload.
func encodeLenPrefixedField(s string) string {
	return fmt.Sprintf("%08X", uint32(len(s))) + s
}

// decodeLenPrefixedField reads one length-prefixed field starting at
// offset. It returns the field's string value and the next offset to
// continue parsing from. An error is returned if the buffer is too
// short to contain either the length prefix or the declared field
// payload.
func decodeLenPrefixedField(data string, offset int) (field string, nextOffset int, err error) {
	if offset+8 > len(data) {
		return "", 0, errors.New("length prefix truncated")
	}
	var n uint32
	if _, scanErr := fmt.Sscanf(data[offset:offset+8], "%08X", &n); scanErr != nil {
		return "", 0, fmt.Errorf("length prefix not valid hex: %w", scanErr)
	}
	start := offset + 8
	end := start + int(n)
	if end > len(data) {
		return "", 0, errors.New("field payload truncated")
	}
	return data[start:end], end, nil
}

// hmacSha256Hex computes HMAC-SHA256(key, data) and returns the 64-char
// lowercase-hex digest. The helper exists so both the V2 encrypter and
// the V2 decrypter compute the same thing from the same inputs; callers
// outside the V2 envelope should not use it.
func hmacSha256Hex(key []byte, data string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

// RsaAesPublicKeyEncryptAndSignHmac is the v1.7.9 additive replacement
// for the deprecated RsaAesPublicKeyEncryptAndSign. It preserves the
// same security goals (hybrid RSA-wrapped AES key + AES-GCM body +
// sender signature verified by the recipient) but fixes two V1
// hazards:
//
//  1. V1's trailing 64-char value is an unkeyed hash that can be
//     recomputed by any attacker and provides no integrity. V2 appends
//     an HMAC-SHA256 tag keyed by the per-envelope AES key, binding
//     rsaEncryptedAesKey and aesGcmEncryptedBody together so tampering
//     of either is detected before GCM decrypt is attempted.
//  2. V1's inner plaintext is the 0x0B-delimited triple
//     plainText<VT>senderPublicKey<VT>signature. If plainText contains
//     a VT byte, the decrypt-side parser splits incorrectly and the
//     envelope is unrecoverable. V2 uses length-prefixed fields so
//     arbitrary byte sequences (VT, NUL, any control byte, binary
//     data) round-trip exactly.
//
// recipientPublicKeyHexOrPem, senderPublicKeyHexOrPem,
// senderPrivateKeyHexOrPem = may each be HEX or PEM. Same as V1.
//
// Returns an envelope starting with STX, the literal "V2" format
// marker, the hex-encoded RSA-wrapped AES key, the hex-encoded AES-GCM
// ciphertext, the 64-char HMAC-SHA256 tag, and a trailing ETX. The
// envelope can be safely transported as a single printable string
// (hex-only body plus two ASCII control framing bytes).
func RsaAesPublicKeyEncryptAndSignHmac(plainText string, recipientPublicKeyHexOrPem string, senderPublicKeyHexOrPem string, senderPrivateKeyHexOrPem string) (encryptedData string, err error) {
	// validate inputs — same checks V1 performs, same error messages
	// so migrating callers don't have to distinguish failure modes
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

	// per-envelope random 32-byte AES key (same derivation as V1)
	aesKey, err1 := Generate32ByteRandomKey(Sha256(util.NewUUID(), util.CurrentDateTime()))
	if err1 != nil {
		return "", fmt.Errorf("Dynamic AES New Key Error: %w", err1)
	}

	// sign the raw plain text with sender private key
	signature, err2 := RsaPrivateKeySign(plainText, senderPrivateKeyHexOrPem)
	if err2 != nil {
		return "", fmt.Errorf("Dynamic AES Siganture Error: %w", err2)
	}

	// V2 inner plaintext = length-prefixed triple
	// (plainText, senderPublicKey, signature). NO VT delimiter.
	innerPlaintext := encodeLenPrefixedField(plainText) +
		encodeLenPrefixedField(senderPublicKeyHexOrPem) +
		encodeLenPrefixedField(signature)

	aesEncryptedBody, err3 := AesGcmEncrypt(innerPlaintext, aesKey)
	if err3 != nil {
		return "", fmt.Errorf("Dynamic AES Data Encrypt Error: %w", err3)
	}

	// wrap the AES key under recipient's public key (RSA-OAEP-SHA256)
	aesEncryptedKey, err4 := RsaPublicKeyEncrypt(aesKey, recipientPublicKeyHexOrPem)
	if err4 != nil {
		return "", fmt.Errorf("Dynamic AES Key Encrypt Error: %w", err4)
	}

	// HMAC binds rsaEncryptedAesKey || aesEncryptedBody at envelope
	// level, keyed by the per-envelope AES key. Anyone who can
	// recompute this tag must already possess the AES key (which
	// means they already can decrypt the body) so the HMAC adds no
	// confidentiality burden — it is purely an integrity binding.
	hmacTag := hmacSha256Hex([]byte(aesKey), aesEncryptedKey+aesEncryptedBody)

	// compose V2 envelope
	encryptedPayload := ascii.AsciiToString(ascii.STX) +
		"V2" +
		aesEncryptedKey +
		aesEncryptedBody +
		hmacTag +
		ascii.AsciiToString(ascii.ETX)

	return encryptedPayload, nil
}

// RsaAesPrivateKeyDecryptAndVerifyHmac is the v1.7.9 additive
// replacement for the deprecated RsaAesPrivateKeyDecryptAndVerify. It
// decrypts envelopes produced by RsaAesPublicKeyEncryptAndSignHmac.
//
// Verification order (fail-fast on tampering):
//  1. Strip STX/ETX.
//  2. Check the literal "V2" format marker. A V1 payload has its
//     first byte equal to a hex char of the RSA-wrapped AES key, not
//     'V', so it is rejected here with a clear error instead of
//     partially parsing and failing later.
//  3. Split the body into rsaEncryptedAesKey (512 hex chars) ||
//     aesGcmEncryptedBody (variable) || hmacTag (64 hex chars).
//  4. RSA-decrypt rsaEncryptedAesKey with recipient's private key to
//     recover the per-envelope AES key.
//  5. Recompute HMAC-SHA256(aesKey, rsaEncryptedAesKey ||
//     aesGcmEncryptedBody) and compare against hmacTag in constant
//     time. REJECT if they do not match — do NOT invoke AES-GCM
//     decrypt on a tampered envelope.
//  6. AES-GCM decrypt aesGcmEncryptedBody with aesKey.
//  7. Parse the decrypted inner plaintext as three length-prefixed
//     fields (plainText, senderPublicKeyHexOrPem, signature).
//  8. RSA-verify signature against plainText using
//     senderPublicKeyHexOrPem.
//
// Only after all eight steps pass does the function return the
// plainText and senderPublicKeyHexOrPem to the caller.
//
// recipientPrivateKeyHexOrPem = may be HEX or PEM.
func RsaAesPrivateKeyDecryptAndVerifyHmac(encryptedData string, recipientPrivateKeyHexOrPem string) (plainText string, senderPublicKeyHexOrPem string, err error) {
	// minimum envelope size sanity check:
	//   STX(1) + "V2"(2) + rsaKey(512) + body(>=1) + hmac(64) + ETX(1)
	//   = 581 bytes minimum. Any payload shorter than 581 cannot be
	//   a V2 envelope.
	if util.LenTrim(encryptedData) < 581 {
		return "", "", errors.New("Encrypted Payload Envelop Not Valid: Must Be At Least 581 Bytes (V2)")
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

	// strip STX and ETX
	data := util.Right(encryptedData, len(encryptedData)-1)
	data = util.Left(data, len(data)-1)

	// V2 format marker — this is the cross-version isolation check.
	// A V1 payload has no "V2" here (its first two bytes are hex
	// chars from the RSA-wrapped AES key), so it is rejected here.
	if util.Left(data, 2) != "V2" {
		return "", "", errors.New("Encrypted Payload Envelop Not Valid: V2 Format Marker Missing (is this a V1 payload? use RsaAesPrivateKeyDecryptAndVerify instead)")
	}
	data = util.Right(data, len(data)-2)

	// sanity: must have room for rsaKey(512) + >=1 body + hmac(64)
	if len(data) < 512+1+64 {
		return "", "", errors.New("Encrypted Payload Envelop Not Valid: Body Too Short After V2 Marker")
	}

	// split into rsaEncryptedAesKey || aesGcmEncryptedBody || hmacTag
	aesKeyEncrypted := util.Left(data, 512)
	afterKey := util.Right(data, len(data)-512)
	hmacTag := util.Right(afterKey, 64)
	aesBodyEncrypted := util.Left(afterKey, len(afterKey)-64)

	// RSA-decrypt the AES key with recipient private key
	aesKey, err1 := RsaPrivateKeyDecrypt(aesKeyEncrypted, recipientPrivateKeyHexOrPem)
	if err1 != nil {
		return "", "", fmt.Errorf("Decrypt AES Key Error: %w", err1)
	}

	// CRITICAL: verify HMAC BEFORE invoking AES-GCM decrypt. A
	// tampered envelope must fail here with a clear integrity error
	// instead of producing a GCM tag mismatch (which looks like a
	// key-derivation bug to most callers and leaks nothing about
	// what was tampered with).
	expectedTag := hmacSha256Hex([]byte(aesKey), aesKeyEncrypted+aesBodyEncrypted)
	if !hmac.Equal([]byte(expectedTag), []byte(hmacTag)) {
		return "", "", errors.New("Encrypted Payload Integrity Check Failed: HMAC-SHA256 Mismatch")
	}

	// HMAC passed → AES-GCM decrypt the body
	innerPlaintext, err2 := AesGcmDecrypt(aesBodyEncrypted, aesKey)
	if err2 != nil {
		return "", "", fmt.Errorf("Decrypt AES Data Error: %w", err2)
	}

	// parse three length-prefixed fields
	pt, off, errF := decodeLenPrefixedField(innerPlaintext, 0)
	if errF != nil {
		return "", "", fmt.Errorf("Decrypted AES Data Not Valid: plainText field: %w", errF)
	}
	spk, off, errF := decodeLenPrefixedField(innerPlaintext, off)
	if errF != nil {
		return "", "", fmt.Errorf("Decrypted AES Data Not Valid: senderPublicKey field: %w", errF)
	}
	sig, off, errF := decodeLenPrefixedField(innerPlaintext, off)
	if errF != nil {
		return "", "", fmt.Errorf("Decrypted AES Data Not Valid: signature field: %w", errF)
	}
	if off != len(innerPlaintext) {
		return "", "", errors.New("Decrypted AES Data Not Valid: trailing bytes after signature field")
	}

	if util.LenTrim(pt) == 0 {
		return "", "", errors.New("Decrypted AES Data Not Valid: Empty String Not Expected")
	}
	if util.LenTrim(spk) < 512 {
		return "", "", errors.New("Decrypted AES Data Not Valid: Sender Public Key Not Complete")
	}
	if util.LenTrim(sig) < 512 {
		return "", "", errors.New("Decrypted AES Data Not Valid: Sender Signature Not Complete")
	}

	// verify sender signature against plainText
	if err3 := RsaPublicKeyVerify(pt, spk, sig); err3 != nil {
		return "", "", fmt.Errorf("Decrypted AES Data Not Valid: (Signature Not Authenticated) %w", err3)
	}

	return pt, spk, nil
}
