package helper

import (
	"fmt"
	"strings"
)

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

type EmvTlvTag struct {
	TagName string
	TagHexValueCount int
	TagHexValue string
	TagDecodedValue string
}

// getEmvTags returns list of emv tags used by this helper,
// future updates may add to this emv tag list
func getEmvTags() []string {
	return []string{
		"4F", "50", "56", "57", "5A", "82", "84", "95", "9B", "9C",
		"5F24", "5F25", "5F2D", "5F30", "5F34", "5F20",
		"9F07", "9F08", "9F09", "9F11", "9F12", "9F0D", "9F0E", "9F0F",
		"9F10", "9F1A", "9F26", "9F27", "9F33", "9F34", "9F35", "9F36", "9F37", "9F39", "9F40",
	}
}

// ParseEmvTlvTags accepts a hex payload of emv tlv data string,
// performs parsing of emv tags (2 and 4 digit hex as found in getEmvTags()),
// the expected emvTlvTagsPayload is tag hex + tag value len in hex + tag value in hex, data is composed without any other delimiters
//
// Reference Info:
// 		EMVLab Emv Tag Search = http://www.emvlab.org/emvtags/
// 		EMVLab Emv Tags Decode Sample = http://www.emvlab.org/tlvutils/?data=6F2F840E325041592E5359532E4444463031A51DBF0C1A61184F07A0000000031010500A564953412044454249548701019000
// 		Hex To String Decoder = http://www.convertstring.com/EncodeDecode/HexDecode
// 		---
// 		Stack Overflow Article = https://stackoverflow.com/questions/36740699/decode-emv-tlv-data
// 		Stack Overflow Article = https://stackoverflow.com/questions/15059580/reading-emv-card-using-ppse-and-not-pse/19593841#19593841
func ParseEmvTlvTags(emvTlvTagsPayload string) (foundList []*EmvTlvTag, err error) {
	// validate
	emvTlvTagsPayload, _ = ExtractAlphaNumeric(Replace(emvTlvTagsPayload, " ", ""))
	emvTlvTagsPayload = strings.ToUpper(emvTlvTagsPayload)

	if LenTrim(emvTlvTagsPayload) < 6 {
		return nil, fmt.Errorf("EMV TLV Tags Payload Must Be 6 Digits or More")
	}

	if len(emvTlvTagsPayload) % 2 != 0 {
		return nil, fmt.Errorf("EMV TLV Tags Payload Must Be Formatted as Double HEX")
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
		buf, e := HexToString(Mid(emvTlvTagsPayload, 2, 2))
		if e != nil {
			return nil, e
		}
		left2HexValueCount, _ := ParseInt32(buf)
		if left2HexValueCount < 0 {
			left2HexValueCount = 0
		}

		mid2 := Mid(emvTlvTagsPayload, 2, 2)
		buf, e = HexToString(Mid(emvTlvTagsPayload, 4, 2))
		if e != nil {
			return nil, e
		}
		mid2HexValueCount, _ := ParseInt32(buf)
		if mid2HexValueCount < 0 {
			mid2HexValueCount = 0
		}

		left4 := Left(emvTlvTagsPayload, 4)
		buf, e = HexToString(Mid(emvTlvTagsPayload, 4, 2))
		if e != nil {
			return nil, e
		}
		left4HexValueCount, _ := ParseInt32(buf)
		if left4HexValueCount < 0 {
			left4HexValueCount = 0
		}

		checkMid4 := false
		mid4 := ""
		mid4HexvalueCount := 0

		if len(emvTlvTagsPayload) >= 8 {
			mid4 = Mid(emvTlvTagsPayload, 2, 4)
			buf, e = HexToString(Mid(emvTlvTagsPayload, 6, 2))
			if e != nil {
				return nil, e
			}
			mid4HexvalueCount, _ = ParseInt32(buf)
			if mid4HexvalueCount < 0 {
				mid4HexvalueCount = 0
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
					tagValHex = Left(emvTlvTagsPayload, tagValLen * 2)

					if tagValDecoded, err = HexToString(tagValHex); err != nil {
						return nil, err
					}

					// remove tag value from payload
					emvTlvTagsPayload = Right(emvTlvTagsPayload, len(emvTlvTagsPayload)-tagValLen * 2)

					// matched, finalize tag found
					matchFound = true

					foundList = append(foundList, &EmvTlvTag{
						TagName: t,
						TagHexValueCount: tagValLen,
						TagHexValue: tagValHex,
						TagDecodedValue: tagValDecoded,
					})

					processedTags = append(processedTags, t)
				}
			}
		}

		// after searching left most 2 char, and 4 char, if still cannot find a match for a corresponding hex,
		// then the first 4 char need to be skipped (need to remove first 4 char of payload)
		if !matchFound {
			emvTlvTagsPayload = Right(emvTlvTagsPayload, len(emvTlvTagsPayload)-4)
		}
	}

	// parsing completed
	return foundList, nil
}