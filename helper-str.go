package helper

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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/aldelo/common/ascii"
	"html"
	"regexp"
	"strings"
)

// LenTrim returns length of space trimmed string s
func LenTrim(s string) int {
	return len(strings.TrimSpace(s))
}

// NextFixedLength calculates the next fixed length total block size,
// for example, if block size is 16, then the total size should be 16, 32, 48 and so on based on data length
func NextFixedLength(data string, blockSize int) int {
	blocks := (len(data) / blockSize) + 1
	blocks = blocks * blockSize

	return blocks
}

// Left returns the left side of string indicated by variable l (size of substring)
func Left(s string, l int) string {
	if len(s) <= l {
		return s
	}

	if l <= 0 {
		return s
	}

	return s[0:l]
}

// Right returns the right side of string indicated by variable l (size of substring)
func Right(s string, l int) string {
	if len(s) <= l {
		return s
	}

	if l <= 0 {
		return s
	}

	return s[len(s)-l:]
}

// Mid returns the middle of string indicated by variable start and l positions (size of substring)
func Mid(s string, start int, l int) string {
	if len(s) <= l {
		return s
	}

	if l <= 0 {
		return s
	}

	if start > len(s)-1 {
		return s
	}

	if (len(s) - start) < l {
		return s
	}

	return s[start:l+start]
}

// Reverse a string
func Reverse(s string) string {
	chars := []rune(s)
	for i, j := 0, len(chars)-1; i < j; i, j = i+1, j-1 {
		chars[i], chars[j] = chars[j], chars[i]
	}
	return string(chars)
}

// Replace will replace old char with new char and return the replaced string
func Replace(s string, oldChar string, newChar string) string {
	return strings.Replace(s, oldChar, newChar, -1)
}

// Trim gets the left and right space trimmed input string s
func Trim(s string) string {
	return strings.TrimSpace(s)
}

// RightTrimLF will remove linefeed (return char) from the right most char and return result string
func RightTrimLF(s string) string {
	if LenTrim(s) > 0 {
		if Right(s, 2) == "\r\n" {
			return Left(s, len(s)-2)
		}

		if Right(s, 1) == "\n" {
			return Left(s, len(s)-1)
		}
	}

	return s
}

// Padding will pad the data with specified char either to the left or right
func Padding(data string, totalSize int, padRight bool, padChar string) string {
	var result string
	result = data

	b := []byte(data)
	diff := totalSize - len(b)

	if diff > 0 {
		var pChar string

		if len(padChar) == 0 {
			pChar = " "
		} else {
			pChar = string(padChar[0])
		}

		pad := strings.Repeat(pChar, diff)

		if padRight {
			result += pad
		} else {
			result = pad + result
		}
	}

	// return result
	return result
}

// PadLeft will pad data with space to the left
func PadLeft(data string, totalSize int) string {
	return Padding(data, totalSize, false, " ")
}

// PadRight will pad data with space to the right
func PadRight(data string, totalSize int) string {
	return Padding(data, totalSize, true, " ")
}

// PadZero pads zero to the left by default
func PadZero(data string, totalSize int, padRight ...bool) string {
	right := GetFirstBoolOrDefault(false, padRight...)
	return Padding(data, totalSize, right, "0")
}

// PadX pads X to the left by default
func PadX(data string, totalSize int, padRight ...bool) string {
	right := GetFirstBoolOrDefault(false, padRight...)
	return Padding(data, totalSize, right, "X")
}

// SplitString will split the source string using delimiter, and return the element indicated by index,
// if nothing is found, blank is returned,
// index = -1 returns last index
func SplitString(source string, delimiter string, index int) string {
	ar := strings.Split(source, delimiter)

	if len(ar) > 0 {
		if index <= -1 {
			return ar[len(ar)-1]
		} else {
			if len(ar) > index {
				return ar[index]
			} else {
				return ""
			}
		}
	}

	return ""
}

// SliceStringToCSVString unboxes slice of string into comma separated string
func SliceStringToCSVString(source []string, spaceAfterComma bool) string {
	output := ""

	for _, v := range source {
		if LenTrim(output) > 0 {
			output += ","

			if spaceAfterComma {
				output += " "
			}
		}

		output += v
	}

	return output
}

// ParseKeyValue will parse the input string using specified delimiter (= is default),
// result is set in the key and val fields
func ParseKeyValue(s string, delimiter string, key *string, val *string) error {
	if len(s) <= 2 {
		*key = ""
		*val = ""
		return fmt.Errorf("Source Data Must Exceed 2 Characters")
	}

	if len(delimiter) == 0 {
		delimiter = "="
	} else {
		delimiter = string(delimiter[0])
	}

	if strings.Contains(s, delimiter) {
		p := strings.Split(s, delimiter)

		if len(p) == 2 {
			*key = Trim(p[0])
			*val = Trim(p[1])
			return nil
		}

		// parts not valid
		*key = ""
		*val = ""
		return fmt.Errorf("Parsed Parts Must Equal 2")
	}

	// no delimiter found
	*key = ""
	*val = ""
	return fmt.Errorf("Delimiter Not Found in Source Data")
}

// ExtractNumeric will extract only 0-9 out of string to be returned
func ExtractNumeric(s string) (string, error) {
	exp, err := regexp.Compile("[^0-9]+")

	if err != nil {
		return "", err
	}

	return exp.ReplaceAllString(s, ""), nil
}

// ExtractAlpha will extract A-Z out of string to be returned
func ExtractAlpha(s string) (string, error) {
	exp, err := regexp.Compile("[^A-Za-z]+")

	if err != nil {
		return "", err
	}

	return exp.ReplaceAllString(s, ""), nil
}

// ExtractAlphaNumeric will extract only A-Z, a-z, and 0-9 out of string to be returned
func ExtractAlphaNumeric(s string) (string, error) {
	exp, err := regexp.Compile("[^A-Za-z0-9]+")

	if err != nil {
		return "", err
	}

	return exp.ReplaceAllString(s, ""), nil
}

// ExtractAlphaNumericUnderscoreDash will extract only A-Z, a-z, 0-9, _, - out of string to be returned
func ExtractAlphaNumericUnderscoreDash(s string) (string, error) {
	exp, err := regexp.Compile("[^A-Za-z0-9_-]+")

	if err != nil {
		return "", err
	}

	return exp.ReplaceAllString(s, ""), nil
}

// ExtractAlphaNumericPrintableSymbols will extra A-Z, a-z, 0-9, and printable symbols
func ExtractAlphaNumericPrintableSymbols(s string) (string, error) {
	exp, err := regexp.Compile("[^ -~]+")

	if err != nil {
		return "", err
	}

	return exp.ReplaceAllString(s, ""), nil
}

// ExtractByRegex will extract string based on regex expression,
// any regex match will be replaced with blank
func ExtractByRegex(s string, regexStr string) (string, error) {
	exp, err := regexp.Compile(regexStr)

	if err != nil {
		return "", err
	}

	return exp.ReplaceAllString(s, ""), nil
}

// ================================================================================================================
// TYPE CHECK HELPERS
// ================================================================================================================

// IsAlphanumericOnly checks if the input string is A-Z, a-z, and 0-9 only
func IsAlphanumericOnly(s string) bool {
	exp, err := regexp.Compile("[A-Za-z0-9]+")

	if err != nil {
		return false
	}

	if len(exp.ReplaceAllString(s, "")) > 0 {
		// has non alphanumeric
		return false
	} else {
		// alphanumeric only
		return true
	}
}

// IsAlphanumericAndSpaceOnly checks if the input string is A-Z, a-z, 0-9, and space
func IsAlphanumericAndSpaceOnly(s string) bool {
	exp, err := regexp.Compile("[A-Za-z0-9 ]+")

	if err != nil {
		return false
	}

	if len(exp.ReplaceAllString(s, "")) > 0 {
		// has non alphanumeric and space
		return false
	} else {
		// alphanumeric and space only
		return true
	}
}

// IsBase64Only checks if the input string is a-z, A-Z, 0-9, +, /, =
func IsBase64Only(s string) bool {
	exp, err := regexp.Compile("[A-Za-z0-9+/=]+")

	if err != nil {
		return false
	}

	if len(exp.ReplaceAllString(s, "")) > 0 {
		// has non base 64
		return false
	} else {
		// base 64 only
		return true
	}
}

// IsHexOnly checks if the input string is a-f, A-F, 0-9
func IsHexOnly(s string) bool {
	exp, err := regexp.Compile("[A-Fa-f0-9]+")

	if err != nil {
		return false
	}

	if len(exp.ReplaceAllString(s, "")) > 0 {
		// has non hex
		return false
	} else {
		// hex only
		return true
	}
}

// IsNumericIntOnly checks if the input string is 0-9 only
func IsNumericIntOnly(s string) bool {
	exp, err := regexp.Compile("[0-9]+")

	if err != nil {
		return false
	}

	if len(exp.ReplaceAllString(s, "")) > 0 {
		// has non numeric
		return false
	} else {
		// numeric only
		return true
	}
}

// IsNumericIntAndNegativeSignOnly checks if the input string is 0-9 and possibly with lead negative sign only
func IsNumericIntAndNegativeSignOnly(s string) bool {
	if len(s) == 0 {
		return false
	}

	if !IsNumericIntOnly(Left(s, 1)) && Left(s, 1) != "-" {
		return false
	}

	if len(s) > 1 {
		v := Right(s, len(s)-1)

		if !IsNumericIntOnly(v) {
			return false
		} else {
			return true
		}
	} else {
		if s == "-" {
			return false
		} else {
			return true
		}
	}
}

// ================================================================================================================
// HEX HELPERS
// ================================================================================================================

// StringToHex converts string into hex
func StringToHex(data string) string {
	return strings.ToUpper(hex.EncodeToString([]byte(data)))
}

// ByteToHex converts byte array into hex
func ByteToHex(data []byte) string {
	return strings.ToUpper(hex.EncodeToString(data))
}

// HexToString converts hex data into string
func HexToString(hexData string) (string, error) {
	data, err := hex.DecodeString(hexData)

	if err != nil {
		return "", err
	}

	return string(data), nil
}

// HexToByte converts hex data into byte array
func HexToByte(hexData string) ([]byte, error) {
	data, err := hex.DecodeString(hexData)

	if err != nil {
		return []byte{}, err
	}

	return data, nil
}

// ================================================================================================================
// BASE64 HELPERS
// ================================================================================================================

// Base64StdEncode will encode given data into base 64 standard encoded string
func Base64StdEncode(data string) string {
	return base64.StdEncoding.EncodeToString([]byte(data))
}

// Base64StdDecode will decode given data from base 64 standard encoded string
func Base64StdDecode(data string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(data)

	if err != nil {
		return "", err
	}

	return string(b), nil
}

// Base64UrlEncode will encode given data into base 64 url encoded string
func Base64UrlEncode(data string) string {
	return base64.URLEncoding.EncodeToString([]byte(data))
}

// Base64UrlDecode will decode given data from base 64 url encoded string
func Base64UrlDecode(data string) (string, error) {
	b, err := base64.URLEncoding.DecodeString(data)

	if err != nil {
		return "", err
	}

	return string(b), nil
}

// ================================================================================================================
// HTML HELPERS
// ================================================================================================================

// HTMLDecode will unescape html tags and extended tags relevant to our apps
func HTMLDecode(s string) string {
	buf := html.UnescapeString(s)

	buf = strings.Replace(buf, "&amp;#39;", "'", -1)
	buf = strings.Replace(buf, "&amp;lt;", "<", -1)
	buf = strings.Replace(buf, "&amp;gt;", ">", -1)
	buf = strings.Replace(buf, "&amp;quot;", "\"", -1)
	buf = strings.Replace(buf, "&amp;apos;", "'", -1)
	buf = strings.Replace(buf, "&amp;#169;", "©", -1)
	buf = strings.Replace(buf, "&#39;", "'", -1)
	buf = strings.Replace(buf, "&lt;", "<", -1)
	buf = strings.Replace(buf, "&gt;", ">", -1)
	buf = strings.Replace(buf, "&quot;", "\"", -1)
	buf = strings.Replace(buf, "&apos;", "'", -1)
	buf = strings.Replace(buf, "&#169;", "©", -1)
	buf = strings.Replace(buf, "&lt;FS&gt;", "=", -1)
	buf = strings.Replace(buf, "&lt;GS&gt;", "\n", -1)

	return buf
}

// HTMLEncode will escape html tags
func HTMLEncode(s string) string {
	return html.EscapeString(s)
}

// ================================================================================================================
// XML HELPERS
// ================================================================================================================

// XMLToEscaped will escape the data whose xml special chars > < & % ' " are escaped into &gt; &lt; &amp; &#37; &apos; &quot;
func XMLToEscaped(data string) string {
	var r string

	r = strings.Replace(data, "&", "&amp;", -1)
	r = strings.Replace(r, ">", "&gt;", -1)
	r = strings.Replace(r, "<", "&lt;", -1)
	r = strings.Replace(r, "%", "&#37;", -1)
	r = strings.Replace(r, "'", "&apos;", -1)
	r = strings.Replace(r, "\"", "&quot;", -1)

	return r
}

// XMLFromEscaped will un-escape the data whose &gt; &lt; &amp; &#37; &apos; &quot; are converted to > < & % ' "
func XMLFromEscaped(data string) string {
	var r string

	r = strings.Replace(data, "&amp;", "&", -1)
	r = strings.Replace(r, "&gt;", ">", -1)
	r = strings.Replace(r, "&lt;", "<", -1)
	r = strings.Replace(r, "&#37;", "%", -1)
	r = strings.Replace(r, "&apos;", "'", -1)
	r = strings.Replace(r, "&quot;", "\"", -1)

	return r
}

// MarshalXMLCompact will accept an input variable, typically struct with xml struct tags, to serialize from object into xml string
//
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// special struct field:
//    XMLName xml.Name `xml:"ElementName"`
//
// struct xml tags:
//    `xml:"AttributeName,attr"` or `xml:",attr"` 		<<< Attribute Instead of Element
//    `xml:"ElementName"`								<<< XML Element Name
//    `xml:"OuterElementName>InnerElementName"` 		<<< Outer XML Grouping By OuterElementName
//    `xml:",cdata"`									<<< <![CDATA[...]]
//    `xml:",innerxml"`								    <<< Write as Inner XML Verbatim and Not Subject to Marshaling
//    `xml:",comment"`									<<< Write as Comment, and Not Contain "--" Within Value
//    `xml:"...,omitempty"`								<<< Omit This Line if Empty Value (false, 0, nil, zero length array)
//    `xml:"-"` <<< Omit From XML Marshal
func MarshalXMLCompact(v interface{}) (string, error) {
	if v == nil {
		return "", fmt.Errorf("Object For XML Marshal Must Not Be Nil")
	}

	b, err := xml.Marshal(v)

	if err != nil {
		return "", err
	}

	return string(b), nil
}

// MarshalXMLIndent will accept an input variable, typically struct with xml struct tags, to serialize from object into xml string with indented formatting
//
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// special struct field:
//    XMLName xml.Name `xml:"ElementName"`
//
// struct xml tags:
//    `xml:"AttributeName,attr"` or `xml:",attr"` 		<<< Attribute Instead of Element
//    `xml:"ElementName"`								<<< XML Element Name
//    `xml:"OuterElementName>InnerElementName"` 		<<< Outer XML Grouping By OuterElementName
//    `xml:",cdata"`									<<< <![CDATA[...]]
//    `xml:",innerxml"`								    <<< Write as Inner XML Verbatim and Not Subject to Marshaling
//    `xml:",comment"`									<<< Write as Comment, and Not Contain "--" Within Value
//    `xml:"...,omitempty"`								<<< Omit This Line if Empty Value (false, 0, nil, zero length array)
//    `xml:"-"` <<< Omit From XML Marshal
func MarshalXMLIndent(v interface{}) (string, error) {
	if v == nil {
		return "", fmt.Errorf("Object For XML Marshal Must Not Be Nil")
	}

	b, err := xml.MarshalIndent(v, "", "  ")

	if err != nil {
		return "", err
	}

	return string(b), nil
}

// MarshalXML with option for indent or compact
func MarshalXML(v interface{}, indentXML bool) (string, error) {
	if indentXML {
		return MarshalXMLIndent(v)
	} else {
		return MarshalXMLCompact(v)
	}
}

// UnmarshalXML will accept input xml data string and deserialize into target object indicated by parameter v
//
// *** PASS PARAMETER AS "&v" IN ORDER TO BE WRITABLE ***
//
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// if unmarshal is successful, nil is returned, otherwise error info is returned
func UnmarshalXML(xmlData string, v interface{}) error {
	if LenTrim(xmlData) == 0 {
		return fmt.Errorf("XML Data is Required")
	}

	return xml.Unmarshal([]byte(xmlData), v)
}

// ================================================================================================================
// ENCODING JSON HELPERS
// ================================================================================================================

// JsonToEscaped will escape the data whose json special chars are escaped
func JsonToEscaped(data string) string {
	var r string

	r = strings.Replace(data, `\`, `\\`, -1)
	r = strings.Replace(r, string(rune(ascii.BS)), `\b`, -1)
	r = strings.Replace(r, string(rune(ascii.FF)), `\f`, -1)
	r = strings.Replace(r, string(rune(ascii.LF)), `\n`, -1)
	r = strings.Replace(r, string(rune(ascii.CR)), `\r`, -1)
	r = strings.Replace(r, string(rune(ascii.HT)), `\t`, -1)

	return r
}

// MarshalJSONCompact will accept an input variable, typically struct with json struct tags, to serialize from object into json string with compact formatting
//
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// struct json tags:
//    `json:"ElementName"`								<<< JSON Element Name
//    `json:"...,omitempty"`							<<< Omit This Line if Empty Value (false, 0, nil, zero length array)
//    `json:"-"` <<< Omit From JSON Marshal
func MarshalJSONCompact(v interface{}) (string, error) {
	if v == nil {
		return "", fmt.Errorf("Object For JSON Marshal Must Not Be Nil")
	}

	b, err := json.Marshal(v)

	if err != nil {
		return "", err
	}

	return string(b), nil
}

// MarshalJSONIndent will accept an input variable, typically struct with json struct tags, to serialize from object into json string with indented formatting
//
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// struct json tags:
//    `json:"ElementName"`								<<< JSON Element Name
//    `json:"...,omitempty"`							<<< Omit This Line if Empty Value (false, 0, nil, zero length array)
//    `json:"-"` <<< Omit From JSON Marshal
func MarshalJSONIndent(v interface{}) (string, error) {
	if v == nil {
		return "", fmt.Errorf("Object For JSON Marshal Must Not Be Nil")
	}

	b, err := json.MarshalIndent(v, "", "  ")

	if err != nil {
		return "", err
	}

	return string(b), nil
}

// UnmarshalJSON will accept input json data string and deserialize into target object indicated by parameter v
//
// *** PASS PARAMETER AS "&v" IN ORDER TO BE WRITABLE ***
//
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// if unmarshal is successful, nil is returned, otherwise error info is returned
func UnmarshalJSON(jsonData string, v interface{}) error {
	if LenTrim(jsonData) == 0 {
		return fmt.Errorf("JSON Data is Required")
	}

	return json.Unmarshal([]byte(jsonData), v)
}