package helper

import (
	"fmt"
	"strconv"
	"strings"
)

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

type EmvTlvTag struct {
	TagName          string
	TagHexValueCount int
	TagHexValue      string
	TagDecodedValue  string
}

// getEmvTags returns list of emv tags used by this helper,
// future updates may add to this emv tag list
func getEmvTags() []string {
	return []string{
		"4F", "50", "56", "57", "5A", "82", "84", "95", "9B", "9C",
		"5F24", "5F25", "5F2D", "5F30", "5F34", "5F20",
		"9F07", "9F08", "9F09", "9F11", "9F12", "9F0D", "9F0E", "9F0F",
		"9F10", "9F1A", "9F26", "9F27", "9F33", "9F34", "9F35", "9F36", "9F37", "9F39", "9F40",
		"DF78", "DF79",
	}
}

// ParseEmvTlvTags accepts a hex payload of emv tlv data string,
// performs parsing of emv tags (2 and 4 digit hex as found in getEmvTags()),
// the expected emvTlvTagsPayload is tag hex + tag value len in hex + tag value in hex, data is composed without any other delimiters
//
// Reference Info:
//
//	EMVLab Emv Tag Search = http://www.emvlab.org/emvtags/
//	EMVLab Emv Tags Decode Sample = http://www.emvlab.org/tlvutils/?data=6F2F840E325041592E5359532E4444463031A51DBF0C1A61184F07A0000000031010500A564953412044454249548701019000
//	Hex To String Decoder = http://www.convertstring.com/EncodeDecode/HexDecode
//	---
//	Stack Overflow Article = https://stackoverflow.com/questions/36740699/decode-emv-tlv-data
//	Stack Overflow Article = https://stackoverflow.com/questions/15059580/reading-emv-card-using-ppse-and-not-pse/19593841#19593841
func ParseEmvTlvTags(emvTlvTagsPayload string) (foundList []*EmvTlvTag, err error) {
	// validate
	emvTlvTagsPayload, _ = ExtractAlphaNumeric(Replace(emvTlvTagsPayload, " ", ""))
	emvTlvTagsPayload = strings.ToUpper(emvTlvTagsPayload)

	if LenTrim(emvTlvTagsPayload) < 6 {
		return nil, fmt.Errorf("EMV TLV Tags Payload Must Be 6 Digits or More")
	}

	if len(emvTlvTagsPayload)%2 != 0 {
		return nil, fmt.Errorf("EMV TLV Tags Payload Must Be Formatted as Double HEX")
	}

	// helper to parse hex length bytes without ASCII conversion
	parseLen := func(hexLen string) (int, error) {
		v, e := strconv.ParseInt(hexLen, 16, 32)
		if e != nil {
			return 0, e
		}
		if v < 0 {
			return 0, nil
		}
		return int(v), nil
	}

	// get search tags
	searchTags := getEmvTags()

	if len(searchTags) == 0 {
		return nil, fmt.Errorf("EMV Tags To Search is Required")
	}

	// store emv tags already processed
	var processedTags []string

	// loop until all emv tlv tags payload are processed
	for len(emvTlvTagsPayload) >= 6 {
		// get left 2 char, mid 2 char, and left 4 char, from left to match against emv search tags
		left2 := Left(emvTlvTagsPayload, 2)
		left2HexValueCount, e := parseLen(Mid(emvTlvTagsPayload, 2, 2))
		if e != nil {
			return nil, e
		}

		mid2 := Mid(emvTlvTagsPayload, 2, 2)
		mid2HexValueCount, e := parseLen(Mid(emvTlvTagsPayload, 4, 2))
		if e != nil {
			return nil, e
		}

		left4 := Left(emvTlvTagsPayload, 4)
		left4HexValueCount, e := parseLen(Mid(emvTlvTagsPayload, 4, 2))
		if e != nil {
			return nil, e
		}

		checkMid4 := false
		mid4 := ""
		mid4HexvalueCount := 0

		if len(emvTlvTagsPayload) >= 8 {
			mid4 = Mid(emvTlvTagsPayload, 2, 4)
			mid4HexvalueCount, e = parseLen(Mid(emvTlvTagsPayload, 6, 2))
			if e != nil {
				return nil, e
			}
			checkMid4 = true
		}

		// loop through tags to search
		matchFound := false

		for _, t := range searchTags {
			if LenTrim(t) > 0 && !StringSliceContains(&processedTags, t) && (len(t) == 2 || len(t) == 4) {
				tagLenRemove := 0
				tagValLen := 0
				tagValHex := ""
				tagValDecoded := ""

				if len(t) == 2 {
					// 2
					if strings.ToUpper(left2) == strings.ToUpper(t) && left2HexValueCount > 0 {
						tagLenRemove = 4
						tagValLen = left2HexValueCount
					} else if strings.ToUpper(mid2) == strings.ToUpper(t) && mid2HexValueCount > 0 {
						tagLenRemove = 6
						tagValLen = mid2HexValueCount
					}
				} else {
					// 4
					if strings.ToUpper(left4) == strings.ToUpper(t) && left4HexValueCount > 0 {
						tagLenRemove = 6
						tagValLen = left4HexValueCount
					} else if checkMid4 && len(mid4) > 0 && strings.ToUpper(mid4) == strings.ToUpper(t) && mid4HexvalueCount > 0 {
						tagLenRemove = 8
						tagValLen = mid4HexvalueCount
					}
				}

				if tagLenRemove > 0 && tagValLen > 0 {
					// remove left x (tag and size)
					emvTlvTagsPayload = Right(emvTlvTagsPayload, len(emvTlvTagsPayload)-tagLenRemove)

					// get tag value hex
					tagValHex = Left(emvTlvTagsPayload, tagValLen*2)

					if tagValDecoded, err = HexToString(tagValHex); err != nil {
						return nil, err
					}

					// remove tag value from payload
					emvTlvTagsPayload = Right(emvTlvTagsPayload, len(emvTlvTagsPayload)-tagValLen*2)

					// matched, finalize tag found
					matchFound = true

					foundList = append(foundList, &EmvTlvTag{
						TagName:          t,
						TagHexValueCount: tagValLen,
						TagHexValue:      tagValHex,
						TagDecodedValue:  tagValDecoded,
					})

					processedTags = append(processedTags, t)
				}
			}
		}

		// after searching left most 2 char, and 4 char, if still cannot find a match for a corresponding hex,
		// then the first 2 char need to be skipped (need to remove first 2 char of payload)
		if !matchFound {
			emvTlvTagsPayload = Right(emvTlvTagsPayload, len(emvTlvTagsPayload)-2)
		}
	}

	// parsing completed
	return foundList, nil
}

// ParseEmvTlvTagNamesOnly accepts a hex payload of emv tlv names string,
// performs parsing of emv tags (2 and 4 digit hex as found in getEmvTags()),
// the expected emvTlvTagsPayload is tag hex names appended one after another, without delimiters, no other tag values in the string
//
// Reference Info:
//
//	EMVLab Emv Tag Search = http://www.emvlab.org/emvtags/
//	EMVLab Emv Tags Decode Sample = http://www.emvlab.org/tlvutils/?data=6F2F840E325041592E5359532E4444463031A51DBF0C1A61184F07A0000000031010500A564953412044454249548701019000
//	Hex To String Decoder = http://www.convertstring.com/EncodeDecode/HexDecode
//	---
//	Stack Overflow Article = https://stackoverflow.com/questions/36740699/decode-emv-tlv-data
//	Stack Overflow Article = https://stackoverflow.com/questions/15059580/reading-emv-card-using-ppse-and-not-pse/19593841#19593841
func ParseEmvTlvTagNamesOnly(emvTlvTagNamesPayload string) (foundList []string, err error) {
	// validate
	emvTlvTagNamesPayload, _ = ExtractAlphaNumeric(Replace(emvTlvTagNamesPayload, " ", ""))
	emvTlvTagNamesPayload = strings.ToUpper(emvTlvTagNamesPayload)

	if LenTrim(emvTlvTagNamesPayload) < 2 {
		return nil, fmt.Errorf("EMV TLV Tags Payload Must Be 2 Digits or More")
	}

	if len(emvTlvTagNamesPayload)%2 != 0 {
		return nil, fmt.Errorf("EMV TLV Tags Payload Must Be Formatted as Double HEX")
	}

	// get search tags
	searchTags := getEmvTags()

	if len(searchTags) == 0 {
		return nil, fmt.Errorf("EMV Tags To Search is Required")
	}

	// loop until all emv tlv tags payload are processed
	for len(emvTlvTagNamesPayload) >= 2 {
		// get left 2 char, and left 4 char, from left to match against emv search tags
		left2 := Left(emvTlvTagNamesPayload, 2)

		if StringSliceContains(&searchTags, left2) {
			// left 2 match
			foundList = append(foundList, left2)
			emvTlvTagNamesPayload = Right(emvTlvTagNamesPayload, len(emvTlvTagNamesPayload)-2)
			continue
		}

		if len(emvTlvTagNamesPayload) >= 4 {
			left4 := Left(emvTlvTagNamesPayload, 4)

			if StringSliceContains(&searchTags, left4) {
				// left 4 match
				foundList = append(foundList, left4)
				emvTlvTagNamesPayload = Right(emvTlvTagNamesPayload, len(emvTlvTagNamesPayload)-4)
				continue
			}
		}

		// left 2 and 4 no match, remove first 2 char
		emvTlvTagNamesPayload = Right(emvTlvTagNamesPayload, len(emvTlvTagNamesPayload)-2)
	}

	// parsing completed
	return foundList, nil
}

// cn = compressed numeric data element, consists of 2 numeric digits in hex 0 - 9,
//
//	left justified, padded with trailing F
//
// ---
// DFA001 = PAN key entered (cn)
// DFA002 = CVV/CID (cn)
// DFA003 = Expiry Date (YYMM) (cn)
// DFA004 = Raw MSR Track 2 with Start and End Sentinel (ascii)
// DFA005 = Raw MSR Track 1 with Start and End Sentinel (ascii)
// 57 = Track 2 Equivalent Data
// 5A = PAN (cn)
// 9F6B = Track 2 Data
// 56 = Track 1 Data
// 9F1F = Track 1 Discretionary Data
// 9F20 = Track 2 Discretionary Data
func getEncryptedTlvTags() []string {
	return []string{
		"DFA001", "DFA002", "DFA003", "DFA004", "DFA005",
		"57", "5A", "9F6B", "56", "9F1F", "9F20",
	}
}

func getEncryptedTlvTagsAscii() []string {
	return []string{
		"DFA004", "DFA005",
	}
}

// ParseEncryptedTlvTags accepts a hex payload of encrypted tlv data string,
// performs parsing of emv tags (2, 4 and 6 digit hex as found in getEncryptedTlvTags()),
// the expected encryptedTlvTagsPayload is tag hex + tag value len in hex + tag value in hex, data is composed without any other delimiters
//
// Reference Info:
//
//	EMVLab Emv Tag Search = http://www.emvlab.org/emvtags/
//	EMVLab Emv Tags Decode Sample = http://www.emvlab.org/tlvutils/?data=6F2F840E325041592E5359532E4444463031A51DBF0C1A61184F07A0000000031010500A564953412044454249548701019000
//	Hex To String Decoder = http://www.convertstring.com/EncodeDecode/HexDecode
//	---
//	Stack Overflow Article = https://stackoverflow.com/questions/36740699/decode-emv-tlv-data
//	Stack Overflow Article = https://stackoverflow.com/questions/15059580/reading-emv-card-using-ppse-and-not-pse/19593841#19593841
func ParseEncryptedTlvTags(encryptedTlvTagsPayload string) (foundList []*EmvTlvTag, err error) {
	// validate
	if LenTrim(encryptedTlvTagsPayload) < 6 {
		return nil, fmt.Errorf("Encrypted TLV Tags Payload Must Be 6 Digits or More")
	}

	// helper to parse hex length bytes without ASCII conversion
	parseLen := func(hexLen string) (int, error) {
		v, e := strconv.ParseInt(hexLen, 16, 32)
		if e != nil {
			return 0, e
		}
		if v < 0 {
			return 0, nil
		}
		return int(v), nil
	}

	// get search tags
	searchTags := getEncryptedTlvTags()

	if len(searchTags) == 0 {
		return nil, fmt.Errorf("Encrypted TLV Tags To Search is Required")
	}

	asciiTags := getEncryptedTlvTagsAscii()

	// store emv tags already processed
	var processedTags []string

	// loop until all tlv tags payload are processed
	for len(encryptedTlvTagsPayload) >= 6 {
		// get left 2 char, mid 2 char, and left 4 char, from left to match against emv search tags
		// get left 6 char (for DF tags)
		left2 := Left(encryptedTlvTagsPayload, 2)
		left2HexValueCount, e := parseLen(Mid(encryptedTlvTagsPayload, 2, 2)) // FIX
		if e != nil {
			return nil, e
		}

		mid2 := Mid(encryptedTlvTagsPayload, 2, 2)
		mid2HexValueCount, e := parseLen(Mid(encryptedTlvTagsPayload, 4, 2)) // FIX
		if e != nil {
			return nil, e
		}

		left4 := Left(encryptedTlvTagsPayload, 4)
		left4HexValueCount, e := parseLen(Mid(encryptedTlvTagsPayload, 4, 2)) // FIX
		if e != nil {
			return nil, e
		}

		checkMid4 := false
		mid4 := ""
		mid4HexValueCount := 0

		if len(encryptedTlvTagsPayload) >= 8 {
			mid4 = Mid(encryptedTlvTagsPayload, 2, 4)
			mid4HexValueCount, e = parseLen(Mid(encryptedTlvTagsPayload, 6, 2))
			if e != nil {
				return nil, e
			}
			checkMid4 = true
		}

		checkLeft6 := false
		left6 := ""
		left6HexValueCount := 0

		if len(encryptedTlvTagsPayload) >= 8 {
			left6 = Left(encryptedTlvTagsPayload, 6)
			left6HexValueCount, e = parseLen(Mid(encryptedTlvTagsPayload, 6, 2))
			if e != nil {
				return nil, e
			}
			checkLeft6 = true
		}

		// loop through tags to search
		matchFound := false

		for _, t := range searchTags {
			if LenTrim(t) > 0 && !StringSliceContains(&processedTags, t) && (len(t) == 2 || len(t) == 4 || len(t) == 6) {
				tagLenRemove := 0
				tagValLen := 0
				tagValHex := ""
				tagValDecoded := ""

				if len(t) == 2 {
					// 2
					if strings.ToUpper(left2) == strings.ToUpper(t) && left2HexValueCount > 0 {
						tagLenRemove = 4
						tagValLen = left2HexValueCount
					} else if strings.ToUpper(mid2) == strings.ToUpper(t) && mid2HexValueCount > 0 {
						tagLenRemove = 6
						tagValLen = mid2HexValueCount
					}
				} else if len(t) == 4 {
					// 4
					if strings.ToUpper(left4) == strings.ToUpper(t) && left4HexValueCount > 0 {
						tagLenRemove = 6
						tagValLen = left4HexValueCount
					} else if checkMid4 && len(mid4) > 0 && strings.ToUpper(mid4) == strings.ToUpper(t) && mid4HexValueCount > 0 {
						tagLenRemove = 8
						tagValLen = mid4HexValueCount
					}
				} else if checkLeft6 {
					// 6
					if strings.ToUpper(left6) == strings.ToUpper(t) && left6HexValueCount > 0 {
						tagLenRemove = 8
						tagValLen = left6HexValueCount
					}
				}

				if tagLenRemove > 0 && tagValLen > 0 {
					// remove left x (tag and size)
					encryptedTlvTagsPayload = Right(encryptedTlvTagsPayload, len(encryptedTlvTagsPayload)-tagLenRemove)

					// get tag value hex
					if !StringSliceContains(&asciiTags, t) {
						// hex
						tagValHex = Left(encryptedTlvTagsPayload, tagValLen*2)

						if tagValDecoded, err = HexToString(tagValHex); err != nil {
							return nil, err
						}

						// remove tag value from payload
						encryptedTlvTagsPayload = Right(encryptedTlvTagsPayload, len(encryptedTlvTagsPayload)-tagValLen*2)
					} else {
						// ascii
						tagValHex = Left(encryptedTlvTagsPayload, tagValLen)
						tagValDecoded = tagValHex

						// remove tag value from payload
						encryptedTlvTagsPayload = Right(encryptedTlvTagsPayload, len(encryptedTlvTagsPayload)-tagValLen)
					}

					// matched, finalize tag found
					matchFound = true

					foundList = append(foundList, &EmvTlvTag{
						TagName:          t,
						TagHexValueCount: tagValLen,
						TagHexValue:      tagValHex,
						TagDecodedValue:  tagValDecoded,
					})

					processedTags = append(processedTags, t)
				}
			}
		}

		// after searching left most 2 char, 4 char, 6 char, if still cannot find a match for a corresponding hex,
		// then the first 2 char need to be skipped (need to remove first 2 char of payload)
		if !matchFound {
			encryptedTlvTagsPayload = Right(encryptedTlvTagsPayload, len(encryptedTlvTagsPayload)-2)
		}
	}

	// parsing completed
	return foundList, nil
}
