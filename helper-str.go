package helper

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
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"math"
	"reflect"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// unified typed-nil detector to enforce non-nil contracts
func isNilInterface(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Slice, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

// LenTrim returns length of space trimmed string s
func LenTrim(s string) int {
	trimmed := strings.TrimSpace(s)        // avoid double-trimming and reuse
	return utf8.RuneCountInString(trimmed) // count runes, not bytes, for correct Unicode length
}

// NextFixedLength calculates the next fixed length total block size.
// handle blockSize <= 0 safely and fix exact-multiple off-by-one.
func NextFixedLength(data string, blockSize int) int {
	if blockSize <= 0 {
		return 0
	}

	n := utf8.RuneCountInString(data) //use rune count to align with other rune-aware helpers
	if n == 0 {
		return blockSize
	}

	blocks := (n + blockSize - 1) / blockSize
	return blocks * blockSize
}

// Left returns the left side of string indicated by variable l (size of substring)
// negative or zero length returns empty string; otherwise clamp to length.
func Left(s string, l int) string {
	if l <= 0 {
		return ""
	}

	r := []rune(s)
	if len(r) <= l {
		return s
	}

	return string(r[:l])
}

// Right returns the right side of string indicated by variable l (size of substring)
// negative or zero length returns empty string; otherwise clamp to length.
func Right(s string, l int) string {
	if l <= 0 {
		return ""
	}

	r := []rune(s)
	if len(r) <= l {
		return s
	}

	return string(r[len(r)-l:])
}

// Mid returns the middle of string indicated by variable start and l positions (size of substring)
// robust bounds handling; negative start/length yield empty; clamp end to len(s); return "" when out of range.
func Mid(s string, start int, l int) string {
	if l <= 0 || start < 0 {
		return ""
	}

	r := []rune(s)
	if start >= len(r) {
		return ""
	}

	end := start + l
	if end > len(r) {
		end = len(r)
	}

	return string(r[start:end])
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
	if oldChar == "" { // prevent unbounded growth when oldChar is empty
		return s
	}
	return strings.Replace(s, oldChar, newChar, -1)
}

// Trim gets the left and right space trimmed input string s
func Trim(s string) string {
	return strings.TrimSpace(s)
}

// RightTrimLF will remove linefeed (return char) from the right most char and return result string
func RightTrimLF(s string) string {
	if len(s) == 0 {
		return s
	}

	if strings.HasSuffix(s, "\r\n") {
		return s[:len(s)-2]
	}

	if strings.HasSuffix(s, "\n") {
		return s[:len(s)-1]
	}

	if strings.HasSuffix(s, "\r") { // also trim lone carriage return
		return s[:len(s)-1]
	}

	return s
}

// Padding will pad the data with specified char either to the left or right
func Padding(data string, totalSize int, padRight bool, padChar string) string {
	result := data

	diff := totalSize - utf8.RuneCountInString(data)
	if diff > 0 {
		var pChar string
		if len(padChar) == 0 {
			pChar = " "
		} else {
			pChar = string([]rune(padChar)[0])
		}

		pad := strings.Repeat(pChar, diff)
		if padRight {
			result += pad
		} else {
			result = pad + result
		}
	}

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
	if len(delimiter) == 0 { // guard empty delimiter to avoid unexpected split behavior
		return ""
	}

	if !strings.Contains(source, delimiter) { // honor contract—return blank when delimiter not found
		return ""
	}

	ar := strings.Split(source, delimiter)

	if len(ar) == 0 {
		return ""
	}

	// honor the contract strictly: only -1 means "last element"
	if index == -1 {
		return ar[len(ar)-1]
	}
	if index < -1 {
		return ""
	}

	if len(ar) > index {
		return ar[index]
	}

	return ""
}

// SliceStringToCSVString unboxes slice of string into comma separated string
func SliceStringToCSVString(source []string, spaceAfterComma bool) string {
	if len(source) == 0 { // fast path and avoids nil/empty loop
		return ""
	}

	var b strings.Builder // avoid LenTrim(output) sentinel; handle whitespace-only first entry safely
	for i, v := range source {
		if i > 0 {
			b.WriteString(",")
			if spaceAfterComma {
				b.WriteString(" ")
			}
		}

		field := v // ensure proper CSV quoting
		if strings.IndexAny(field, ",\"\r\n") != -1 {
			field = `"` + strings.ReplaceAll(field, `"`, `""`) + `"`
		}

		b.WriteString(field)
	}

	return b.String()
}

// ParseKeyValue will parse the input string using specified delimiter (= is default),
// result is set in the key and val fields
func ParseKeyValue(s string, delimiter string, key *string, val *string) error {
	// guard against nil pointers to avoid panic
	if key == nil || val == nil {
		return fmt.Errorf("Key and Val pointers are required")
	}

	// allow minimal key/value pairs (e.g., "a=") and avoid over-strict length check
	if len(delimiter) == 0 {
		delimiter = "="
	}

	idx := strings.Index(s, delimiter) // single pass lookup to validate presence
	if idx == -1 {
		*key = ""
		*val = ""
		return fmt.Errorf("Delimiter Not Found in Source Data")
	}

	parts := strings.SplitN(s, delimiter, 2)
	if len(parts) != 2 {
		*key = ""
		*val = ""
		return fmt.Errorf("Parsed Parts Must Equal 2")
	}

	*key = Trim(parts[0])
	*val = Trim(parts[1])
	return nil
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

// ExtractHex will extract only A-F, a-f, and 0-9 out of string to be returned
func ExtractHex(s string) (string, error) {
	exp, err := regexp.Compile("[^A-Fa-f0-9]+")

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
// require non-empty input and anchor regex to full string
func IsAlphanumericOnly(s string) bool {
	if len(s) == 0 {
		return false
	}

	exp, err := regexp.Compile("^[A-Za-z0-9]+$")
	if err != nil {
		return false
	}

	return exp.MatchString(s)
}

// IsAlphanumericAndSpaceOnly checks if the input string is A-Z, a-z, 0-9, and space
// require non-empty input and anchor regex to full string
func IsAlphanumericAndSpaceOnly(s string) bool {
	if len(s) == 0 {
		return false
	}

	exp, err := regexp.Compile("^[A-Za-z0-9 ]+$")
	if err != nil {
		return false
	}

	return exp.MatchString(s)
}

// IsBase64Only checks if the input string is a-z, A-Z, 0-9, +, /, =
// require non-empty input and anchor regex to full string
func IsBase64Only(s string) bool {
	clean := stripBase64Whitespace(s) // validate after whitespace removal
	if len(clean) == 0 {
		return false
	}

	// FReject impossible base64 lengths (len % 4 == 1); pad only valid short cases.
	switch m := len(clean) % 4; m {
	case 1:
		return false
	case 2, 3:
		clean += strings.Repeat("=", 4-m)
	}

	exp, err := regexp.Compile(`^[A-Za-z0-9+/]+={0,2}$`)
	if err != nil {
		return false
	}

	if !exp.MatchString(clean) {
		return false
	}

	// actual decode validation to reject syntactically valid but undecodable data
	if _, err := base64.StdEncoding.DecodeString(clean); err != nil {
		return false
	}

	return true
}

// IsHexOnly checks if the input string is a-f, A-F, 0-9
// require non-empty input and anchor regex to full string
func IsHexOnly(s string) bool {
	if len(s) == 0 {
		return false
	}

	exp, err := regexp.Compile("^[A-Fa-f0-9]+$")
	if err != nil {
		return false
	}

	// Removed even-length enforcement so validation matches the stated contract
	return exp.MatchString(s)
}

// IsNumericIntOnly checks if the input string is 0-9 only
// require non-empty input and anchor regex to full string
func IsNumericIntOnly(s string) bool {
	if len(s) == 0 {
		return false
	}

	exp, err := regexp.Compile("^[0-9]+$")
	if err != nil {
		return false
	}

	return exp.MatchString(s)
}

// IsNumericFloat64 checks if string is float
func IsNumericFloat64(s string, positiveOnly bool) bool {
	if LenTrim(s) == 0 {
		return false
	}

	clean := strings.TrimSpace(s)
	if f, ok := ParseFloat64(clean); !ok {
		return false
	} else {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return false
		}
		if positiveOnly && f < 0 {
			return false
		}
		return true
	}
}

// IsNumericIntAndNegativeSignOnly checks if the input string is 0-9 and possibly with lead negative sign only
func IsNumericIntAndNegativeSignOnly(s string) bool {
	if len(s) == 0 {
		return false
	}

	r := []rune(s) // use rune-aware logic
	if r[0] == '-' {
		if len(r) == 1 {
			return false
		}
		return IsNumericIntOnly(string(r[1:]))
	}

	return IsNumericIntOnly(string(r))
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

// helper to strip common whitespace in base64 payloads
func stripBase64Whitespace(data string) string {
	return strings.Map(func(r rune) rune { // remove all Unicode whitespace
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, data)
}

// Base64StdDecode will decode given data from base 64 standard encoded string
func Base64StdDecode(data string) (string, error) {
	clean := stripBase64Whitespace(data)
	if len(clean) == 0 { // reject empty input instead of silently returning empty
		return "", fmt.Errorf("base64 data is required")
	}

	padded := clean
	// Reject impossible base64 lengths (len % 4 == 1); pad only valid short cases.
	switch m := len(clean) % 4; m {
	case 1:
		return "", fmt.Errorf("invalid base64 length")
	case 2, 3:
		padded += strings.Repeat("=", 4-m)
	}

	b, err := base64.StdEncoding.DecodeString(padded)
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
	clean := stripBase64Whitespace(data)
	if len(clean) == 0 { // reject empty input instead of silently returning empty
		return "", fmt.Errorf("base64url data is required")
	}

	padded := clean
	// Reject impossible base64url lengths (len % 4 == 1); pad only valid short cases.
	switch m := len(clean) % 4; m {
	case 1:
		return "", fmt.Errorf("invalid base64url length")
	case 2, 3:
		padded += strings.Repeat("=", 4-m)
	}

	b, err := base64.URLEncoding.DecodeString(padded)
	if err != nil {
		// fallback for strictly unpadded inputs
		b, err = base64.RawURLEncoding.DecodeString(clean) // use whitespace-stripped input
		if err != nil {
			return "", err
		}
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
	buf = strings.Replace(buf, "<FS>", "=", -1)
	buf = strings.Replace(buf, "<GS>", "\n", -1)

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
	// use xml.EscapeText to avoid double-escaping and to cover all XML entities safely
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(data)); err != nil {
		return ""
	}
	// preserve legacy behavior for '%' -> &#37;
	return strings.ReplaceAll(buf.String(), "%", "&#37;")
}

// XMLFromEscaped will un-escape the data whose &gt; &lt; &amp; &#37; &apos; &quot; are converted to > < & % ' "
func XMLFromEscaped(data string) string {
	// rely on html.UnescapeString to handle standard entities comprehensively
	return html.UnescapeString(data)
}

// MarshalXMLCompact will accept an input variable, typically struct with xml struct tags, to serialize from object into xml string
// reject typed-nil inputs consistently
//
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// special struct field:
//
//	XMLName xml.Name `xml:"ElementName"`
//
// struct xml tags:
//
//	`xml:"AttributeName,attr"` or `xml:",attr"` 		<<< Attribute Instead of Element
//	`xml:"ElementName"`								<<< XML Element Name
//	`xml:"OuterElementName>InnerElementName"` 		<<< Outer XML Grouping By OuterElementName
//	`xml:",cdata"`									<<< <![CDATA[...]]
//	`xml:",innerxml"`								    <<< Write as Inner XML Verbatim and Not Subject to Marshaling
//	`xml:",comment"`									<<< Write as Comment, and Not Contain "--" Within Value
//	`xml:"...,omitempty"`								<<< Omit This Line if Empty Value (false, 0, nil, zero length array)
//	`xml:"-"` <<< Omit From XML Marshal
func MarshalXMLCompact(v interface{}) (string, error) {
	if isNilInterface(v) {
		return "", fmt.Errorf("Object For XML Marshal Must Not Be Nil")
	}

	b, err := xml.Marshal(v)

	if err != nil {
		return "", err
	}

	return string(b), nil
}

// MarshalXMLIndent will accept an input variable, typically struct with xml struct tags, to serialize from object into xml string with indented formatting
// reject typed-nil inputs consistently
//
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// special struct field:
//
//	XMLName xml.Name `xml:"ElementName"`
//
// struct xml tags:
//
//	`xml:"AttributeName,attr"` or `xml:",attr"` 		<<< Attribute Instead of Element
//	`xml:"ElementName"`								<<< XML Element Name
//	`xml:"OuterElementName>InnerElementName"` 		<<< Outer XML Grouping By OuterElementName
//	`xml:",cdata"`									<<< <![CDATA[...]]
//	`xml:",innerxml"`								    <<< Write as Inner XML Verbatim and Not Subject to Marshaling
//	`xml:",comment"`									<<< Write as Comment, and Not Contain "--" Within Value
//	`xml:"...,omitempty"`								<<< Omit This Line if Empty Value (false, 0, nil, zero length array)
//	`xml:"-"` <<< Omit From XML Marshal
func MarshalXMLIndent(v interface{}) (string, error) {
	if isNilInterface(v) {
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
// reject typed-nil targets up front for consistent error reporting
//
// *** PASS PARAMETER AS "&v" IN ORDER TO BE WRITABLE ***
//
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// if unmarshal is successful, nil is returned, otherwise error info is returned
func UnmarshalXML(xmlData string, v interface{}) error {
	if isNilInterface(v) {
		return fmt.Errorf("Target object for XML Unmarshal must not be nil")
	}

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
	// canonical escaping via encoding/json to cover quotes and control chars
	b, err := json.Marshal(data)
	if err != nil || len(b) < 2 {
		return ""
	}
	// json.Marshal for a string returns a quoted JSON string; strip the outer quotes
	return string(b[1 : len(b)-1])
}

// JsonFromEscaped will unescape the json data that may be special character escaped
func JsonFromEscaped(data string) string {
	// canonical unescape via encoding/json; tolerate missing surrounding quotes
	wrapped := data
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		wrapped = `"` + data + `"`
	}

	var out string
	if err := json.Unmarshal([]byte(wrapped), &out); err != nil {
		return ""
	}
	return out
}

// MarshalJSONCompact will accept an input variable, typically struct with json struct tags, to serialize from object into json string with compact formatting
// reject typed-nil inputs consistently
//
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// struct json tags:
//
//	`json:"ElementName"`								<<< JSON Element Name
//	`json:"...,omitempty"`							<<< Omit This Line if Empty Value (false, 0, nil, zero length array)
//	`json:"-"` <<< Omit From JSON Marshal
func MarshalJSONCompact(v interface{}) (string, error) {
	if isNilInterface(v) {
		return "", fmt.Errorf("Object For JSON Marshal Must Not Be Nil")
	}

	b, err := json.Marshal(v)

	if err != nil {
		return "", err
	}

	return string(b), nil
}

// MarshalJSONIndent will accept an input variable, typically struct with json struct tags, to serialize from object into json string with indented formatting
// reject typed-nil inputs consistently
//
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// struct json tags:
//
//	`json:"ElementName"`								<<< JSON Element Name
//	`json:"...,omitempty"`							<<< Omit This Line if Empty Value (false, 0, nil, zero length array)
//	`json:"-"` <<< Omit From JSON Marshal
func MarshalJSONIndent(v interface{}) (string, error) {
	if isNilInterface(v) {
		return "", fmt.Errorf("Object For JSON Marshal Must Not Be Nil")
	}

	b, err := json.MarshalIndent(v, "", "  ")

	if err != nil {
		return "", err
	}

	return string(b), nil
}

// UnmarshalJSON will accept input json data string and deserialize into target object indicated by parameter v
// reject typed-nil targets up front for consistent error reporting
//
// *** PASS PARAMETER AS "&v" IN ORDER TO BE WRITABLE ***
// *** v interface{} MUST BE initialized first ***
// *** STRUCT FIELDS MUST BE EXPORTED FOR MARSHAL AND UNMARSHAL ***
//
// if unmarshal is successful, nil is returned, otherwise error info is returned
func UnmarshalJSON(jsonData string, v interface{}) error {
	if isNilInterface(v) {
		return fmt.Errorf("Target object for JSON Unmarshal must not be nil")
	}

	if LenTrim(jsonData) == 0 {
		return fmt.Errorf("JSON Data is Required")
	}

	return json.Unmarshal([]byte(jsonData), v)
}
