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

	// BER-TLV length decoder with bounds checks and long-form support (up to 3 bytes)
	parseLen := func(payload string, offset int) (val int, lenHeaderHex int, err error) {
		if offset+2 > len(payload) {
			return 0, 0, fmt.Errorf("EMV length header at offset %d exceeds payload", offset)
		}

		firstByteStr := payload[offset : offset+2]
		firstByte, e := strconv.ParseInt(firstByteStr, 16, 32)
		if e != nil {
			return 0, 0, e
		}

		if firstByte < 0x80 {
			return int(firstByte), 2, nil // short form: 1 length byte => 2 hex chars
		}

		if firstByte == 0x80 {
			return 0, 0, fmt.Errorf("EMV indefinite length not supported at offset %d", offset)
		}

		numLenBytes := int(firstByte & 0x7F)
		if numLenBytes == 0 || numLenBytes > 3 {
			return 0, 0, fmt.Errorf("EMV length uses %d bytes at offset %d; max 3 supported", numLenBytes, offset)
		}

		if offset+2+numLenBytes*2 > len(payload) {
			return 0, 0, fmt.Errorf("EMV length header at offset %d exceeds payload", offset)
		}

		lengthHex := payload[offset+2 : offset+2+numLenBytes*2]
		val64, e := strconv.ParseInt(lengthHex, 16, 32)
		if e != nil {
			return 0, 0, e
		}
		if val64 < 0 {
			return 0, 0, fmt.Errorf("EMV length must be non-negative at offset %d", offset)
		}

		return int(val64), (1 + numLenBytes) * 2, nil // header hex chars consumed
	}

	// get search tags
	searchTags := getEmvTags()

	if len(searchTags) == 0 {
		return nil, fmt.Errorf("EMV Tags To Search is Required")
	}

	// loop until all emv tlv tags payload are processed
	for len(emvTlvTagsPayload) >= 6 {
		// get left 2 char, mid 2 char, and left 4 char, from left to match against emv search tags
		left2 := Left(emvTlvTagsPayload, 2)
		left4 := ""
		if len(emvTlvTagsPayload) >= 4 {
			left4 = Left(emvTlvTagsPayload, 4)
		}

		mid2 := ""
		if len(emvTlvTagsPayload) >= 4 {
			mid2 = Mid(emvTlvTagsPayload, 2, 2)
		}

		mid4 := ""
		canCheckMid4 := false
		if len(emvTlvTagsPayload) >= 8 {
			mid4 = Mid(emvTlvTagsPayload, 2, 4)
			canCheckMid4 = true
		}

		matchFound := false

		for _, t := range searchTags {
			if LenTrim(t) == 0 || (len(t) != 2 && len(t) != 4) {
				continue
			}

			tagLenRemove := 0
			tagValLen := 0

			switch len(t) {
			case 2:
				if strings.EqualFold(left2, t) {
					var hdr int
					tagValLen, hdr, err = parseLen(emvTlvTagsPayload, 2)
					if err != nil {
						return nil, err
					}
					tagLenRemove = 2 + hdr
				} else if strings.EqualFold(mid2, t) {
					var hdr int
					tagValLen, hdr, err = parseLen(emvTlvTagsPayload, 4)
					if err != nil {
						return nil, err
					}
					tagLenRemove = 2 + 2 + hdr
				}
			case 4:
				if strings.EqualFold(left4, t) {
					var hdr int
					tagValLen, hdr, err = parseLen(emvTlvTagsPayload, 4)
					if err != nil {
						return nil, err
					}
					tagLenRemove = 4 + hdr
				} else if canCheckMid4 && strings.EqualFold(mid4, t) {
					var hdr int
					tagValLen, hdr, err = parseLen(emvTlvTagsPayload, 6)
					if err != nil {
						return nil, err
					}
					tagLenRemove = 2 + 4 + hdr
				}
			}

			if tagLenRemove > 0 && tagValLen >= 0 {
				if tagLenRemove > len(emvTlvTagsPayload) {
					return nil, fmt.Errorf("EMV tag %s length header exceeds payload", t)
				}

				emvTlvTagsPayload = Right(emvTlvTagsPayload, len(emvTlvTagsPayload)-tagLenRemove)

				need := tagValLen * 2
				if need > len(emvTlvTagsPayload) {
					return nil, fmt.Errorf("EMV tag %s value length %d exceeds remaining payload", t, tagValLen)
				}

				tagValHex := Left(emvTlvTagsPayload, need)

				tagValDecoded := ""
				if tagValDecoded, err = HexToString(tagValHex); err != nil {
					return nil, err
				}

				emvTlvTagsPayload = Right(emvTlvTagsPayload, len(emvTlvTagsPayload)-need)

				matchFound = true

				foundList = append(foundList, &EmvTlvTag{
					TagName:          t,
					TagHexValueCount: tagValLen,
					TagHexValue:      tagValHex,
					TagDecodedValue:  tagValDecoded,
				})

				break
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
	// normalize and validate payload
	encryptedTlvTagsPayload, _ = ExtractAlphaNumeric(Replace(encryptedTlvTagsPayload, " ", ""))
	encryptedTlvTagsPayload = strings.ToUpper(encryptedTlvTagsPayload)

	// validate
	if LenTrim(encryptedTlvTagsPayload) < 6 {
		return nil, fmt.Errorf("Encrypted TLV Tags Payload Must Be 6 Digits or More")
	}

	if len(encryptedTlvTagsPayload)%2 != 0 {
		return nil, fmt.Errorf("Encrypted TLV Tags Payload Must Be Formatted as Double HEX")
	}

	// BER-TLV length decoder with bounds checks and long-form support (up to 3 bytes)
	parseLen := func(payload string, offset int) (val int, lenHeaderHex int, err error) {
		if offset+2 > len(payload) {
			return 0, 0, fmt.Errorf("Encrypted length header at offset %d exceeds payload", offset)
		}

		firstByteStr := payload[offset : offset+2]
		firstByte, e := strconv.ParseInt(firstByteStr, 16, 32)
		if e != nil {
			return 0, 0, e
		}

		if firstByte < 0x80 {
			return int(firstByte), 2, nil // short form
		}

		if firstByte == 0x80 {
			return 0, 0, fmt.Errorf("Encrypted TLV indefinite length not supported at offset %d", offset)
		}

		numLenBytes := int(firstByte & 0x7F)
		if numLenBytes == 0 || numLenBytes > 3 {
			return 0, 0, fmt.Errorf("Encrypted TLV length uses %d bytes at offset %d; max 3 supported", numLenBytes, offset)
		}

		if offset+2+numLenBytes*2 > len(payload) {
			return 0, 0, fmt.Errorf("Encrypted length header at offset %d exceeds payload", offset)
		}

		lengthHex := payload[offset+2 : offset+2+numLenBytes*2]
		val64, e := strconv.ParseInt(lengthHex, 16, 32)
		if e != nil {
			return 0, 0, e
		}
		if val64 < 0 {
			return 0, 0, fmt.Errorf("Encrypted TLV length must be non-negative at offset %d", offset)
		}

		return int(val64), (1 + numLenBytes) * 2, nil
	}

	// get search tags
	searchTags := getEncryptedTlvTags()

	if len(searchTags) == 0 {
		return nil, fmt.Errorf("Encrypted TLV Tags To Search is Required")
	}

	asciiTags := getEncryptedTlvTagsAscii()

	// loop until all tlv tags payload are processed
	for len(encryptedTlvTagsPayload) >= 6 {
		// get left 2 char, mid 2 char, and left 4 char, from left to match against emv search tags
		// get left 6 char (for DF tags)
		left2 := Left(encryptedTlvTagsPayload, 2)
		left2HexValueCount, left2LenHdr, e := parseLen(encryptedTlvTagsPayload, 2)
		if e != nil {
			return nil, e
		}

		mid2 := Mid(encryptedTlvTagsPayload, 2, 2)
		mid2HexValueCount, mid2LenHdr, e := parseLen(encryptedTlvTagsPayload, 4)
		if e != nil {
			return nil, e
		}

		left4 := Left(encryptedTlvTagsPayload, 4)
		left4HexValueCount, left4LenHdr, e := parseLen(encryptedTlvTagsPayload, 4)
		if e != nil {
			return nil, e
		}

		checkMid4 := false
		mid4 := ""
		mid4HexValueCount := 0
		mid4LenHdr := 0

		if len(encryptedTlvTagsPayload) >= 8 {
			mid4 = Mid(encryptedTlvTagsPayload, 2, 4)
			mid4HexValueCount, mid4LenHdr, e = parseLen(encryptedTlvTagsPayload, 6)
			if e != nil {
				return nil, e
			}
			checkMid4 = true
		}

		checkLeft6 := false
		left6 := ""
		left6HexValueCount := 0
		left6LenHdr := 0

		if len(encryptedTlvTagsPayload) >= 8 {
			left6 = Left(encryptedTlvTagsPayload, 6)
			left6HexValueCount, left6LenHdr, e = parseLen(encryptedTlvTagsPayload, 6)
			if e != nil {
				return nil, e
			}
			checkLeft6 = true
		}

		// loop through tags to search
		matchFound := false

		for _, t := range searchTags {
			if LenTrim(t) > 0 && (len(t) == 2 || len(t) == 4 || len(t) == 6) {
				tagLenRemove := 0
				tagValLen := 0
				tagValHex := ""
				tagValDecoded := ""

				if len(t) == 2 {
					// 2
					if strings.ToUpper(left2) == strings.ToUpper(t) && left2HexValueCount >= 0 {
						tagLenRemove = 2 + left2LenHdr
						tagValLen = left2HexValueCount
					} else if strings.ToUpper(mid2) == strings.ToUpper(t) && mid2HexValueCount >= 0 {
						tagLenRemove = 2 + 2 + mid2LenHdr
						tagValLen = mid2HexValueCount
					}
				} else if len(t) == 4 {
					// 4
					if strings.ToUpper(left4) == strings.ToUpper(t) && left4HexValueCount >= 0 {
						tagLenRemove = 4 + left4LenHdr
						tagValLen = left4HexValueCount
					} else if checkMid4 && len(mid4) > 0 && strings.ToUpper(mid4) == strings.ToUpper(t) && mid4HexValueCount >= 0 {
						tagLenRemove = 2 + 4 + mid4LenHdr
						tagValLen = mid4HexValueCount
					}
				} else if checkLeft6 {
					// 6
					if strings.ToUpper(left6) == strings.ToUpper(t) && left6HexValueCount >= 0 {
						tagLenRemove = 6 + left6LenHdr
						tagValLen = left6HexValueCount
					}
				}

				if tagLenRemove > 0 && tagValLen >= 0 {
					// bounds check for header removal
					if tagLenRemove > len(encryptedTlvTagsPayload) {
						return nil, fmt.Errorf("Encrypted tag %s length header exceeds payload", t)
					}

					// remove left x (tag and size)
					encryptedTlvTagsPayload = Right(encryptedTlvTagsPayload, len(encryptedTlvTagsPayload)-tagLenRemove)

					// get tag value hex
					if !StringSliceContains(&asciiTags, t) {
						// hex
						need := tagValLen * 2

						// bounds check for value length
						if need > len(encryptedTlvTagsPayload) {
							return nil, fmt.Errorf("Encrypted tag %s value length %d exceeds remaining payload", t, tagValLen)
						}
						tagValHex = Left(encryptedTlvTagsPayload, need)

						if tagValDecoded, err = HexToString(tagValHex); err != nil {
							return nil, err
						}

						// remove tag value from payload
						encryptedTlvTagsPayload = Right(encryptedTlvTagsPayload, len(encryptedTlvTagsPayload)-need)
					} else {
						// ascii
						need := tagValLen * 2
						// bounds check for ascii value length
						if need > len(encryptedTlvTagsPayload) {
							return nil, fmt.Errorf("Encrypted tag %s ASCII value length %d exceeds remaining payload", t, tagValLen)
						}
						tagValHex = Left(encryptedTlvTagsPayload, need)
						if tagValDecoded, err = HexToString(tagValHex); err != nil { // decode hex to ASCII
							return nil, err
						}
						encryptedTlvTagsPayload = Right(encryptedTlvTagsPayload, len(encryptedTlvTagsPayload)-need)
					}

					// matched, finalize tag found
					matchFound = true

					foundList = append(foundList, &EmvTlvTag{
						TagName:          t,
						TagHexValueCount: tagValLen,
						TagHexValue:      tagValHex,
						TagDecodedValue:  tagValDecoded,
					})

					// stop scanning with stale cursor; re-evaluate cursors on next outer iteration
					break
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
