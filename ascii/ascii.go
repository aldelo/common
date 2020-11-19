package ascii

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
	"fmt"
	"strconv"
	"strings"
)

// ascii definition
// use string(...) to convert the const into string value
const (
	NUL = 0x00 		// '\0' Null
	SOH = 0x01 		//      Start of Header
	STX = 0x02 		//      Start of Text
	ETX = 0x03 		//      End of Text
	EOT = 0x04 		//      End of Transmission
	ENQ = 0x05 		//      Enquiry
	ACK = 0x06 		//      Acknowledgement
	BEL = 0x07 		// '\a' Bell
	BS  = 0x08 		// '\b' Backspace
	HT  = 0x09 		// '\t' Horizontal Tab
	LF  = 0x0A 		// '\n' Line Feed
	VT  = 0x0B 		// '\v' Vertical Tab
	FF  = 0x0C 		// '\f' Form Feed
	CR  = 0x0D 		// '\r' Carriage Return
	SO  = 0x0E 		//      Shift Out
	SI  = 0x0F 		//      Shift In
	DLE = 0x10 		//      Device Idle
	DC1 = 0x11 		//      Device Control 1
	DC2 = 0x12 		//      Device Control 2
	DC3 = 0x13 		//      Device Control 3
	DC4 = 0x14 		//      Device Control 4
	NAK = 0x15 		//      Negative Ack
	SYN = 0x16 		//      Synchronize
	ETB = 0x17 		//      End of Transmission Block
	CAN = 0x18 		//      Cancel
	EM  = 0x19 		//      End of Medium
	SUB = 0x1A 		//      Substitute
	ESC = 0x1B 		// '\e' Escape
	FS  = 0x1C 		//      Field Separator
	GS  = 0x1D 		//      Group Separator
	RS  = 0x1E 		//      Record Separator
	US  = 0x1F 		//      Unit Separator
	SP  = 0x20 		//      Space
	DEL = 0x7F 		//      Delete
	COMMA = 0x2C	// Comma
	COLON = 0x3A	// Colon
	PIPE = 0x7C		// Pipe

)

func AsciiToString(i int) string {
	return string(rune(i))
}

// calculate the LRC value for input string,
// returns blank LRC to indicate error condition (see error for reason)
//
// parameters:
// 		data = includes the STX and ETX but not LRC if exists
//
// returns:
// 		string = LRC string value
func GetLRC(data string) (string, error) {
	if len(strings.Trim(data, " ")) <= 1 {
		return "", fmt.Errorf("Data is Required for LRC Calculation")
	}

	// LRC check excludes STX
	firstChar := data[:1]

	if firstChar == AsciiToString(STX) {
		data = data[1:]
	}

	// excluding STX, must be 2 or more chars
	if len(data) < 2 {
		return "", fmt.Errorf("Data Must Be 2 Characters or More for LRC Calculation")
	}

	lrcBytes := []byte(data)
	lrc := byte(0)

	// loop through each element, XOR product of element and next adjacent element and continue
	for i, v := range lrcBytes {
		if i == 0 {
			lrc = v
		} else {
			lrc ^= v
		}
	}

	// return lrc value
	return string(lrc), nil
}

// IsLRCValid checks if the input data that contains the entire string, including STX ETX and LRC, that its LRC is valid for the content of the data
func IsLRCValid(data string) bool {
	if len(data) <= 2 {
		return false
	}

	if calcLrc, err := GetLRC(data[:len(data)-1]); err != nil || len(calcLrc) == 0 {
		return false
	} else {
		return data[len(data)-1:] == calcLrc
	}
}

// IsCreditCardMod10Valid performs modulo 10 credit card number validation
func IsCreditCardMod10Valid(cardNumber string) (bool, error) {
	cardNumber = strings.Trim(cardNumber, " ")

	if len(cardNumber) < 5 {
		return false, fmt.Errorf("Card Number Must Be Greater or Equal to 5 Digits")
	}

	if _, err := strconv.ParseUint(cardNumber, 10, 64); err != nil {
		return false, fmt.Errorf("Card Number Must Be Numeric")
	}

	source := cardNumber[:len(cardNumber)-1]
	checkDigit := cardNumber[len(cardNumber)-1:]

	result := 0
	multiplier := 2

	// loop through each element from right to left,
	// multiple element by value of 2, and 1 alternating
	for i := len(source) - 1; i >= 0; i-- {
		if temp, e := strconv.Atoi(string(source[i])); e == nil {
			temp *= multiplier

			if multiplier == 2 {
				multiplier = 1
			} else {
				multiplier = 2
			}

			for {
				if temp < 10 {
					result += temp
					break
				}

				buf := strconv.Itoa(temp)
				x, _ := strconv.Atoi(buf[:1])
				y, _ := strconv.Atoi(buf[len(buf)-1:])
				temp = x + y
			}
		}
	}

	// find the next highest multiple of 10
	multiplier = result % 10

	if multiplier > 0 {
		multiplier = result / 10 + 1
	} else {
		multiplier = result / 10
	}

	// get check digit
	result = multiplier * 10 - result

	if chk, err := strconv.Atoi(checkDigit); err != nil {
		return false, fmt.Errorf("Convert Check Digit Failed: %s", err)
	} else {
		return chk == result, nil
	}
}

// EnvelopWithStxEtxLrc will take content data, wrap with STX, ETX, and calculate LRC to append
//
// contentData = do not include STX, ETX, LRC
func EnvelopWithStxEtxLrc(contentData string) string {
	if len(contentData) == 0 {
		return ""
	}

	if contentData[:1] != AsciiToString(STX) {
		contentData = AsciiToString(STX) + contentData
	}

	if len(contentData) >= 2 {
		removeLast := false
		d := contentData[:2]

		if d[:1] == AsciiToString(ETX) {
			removeLast = true
		} else if d[len(d)-1:] != AsciiToString(ETX) {
			contentData += AsciiToString(ETX)
		}

		if removeLast {
			contentData = contentData[:len(contentData)-1]
		}
	} else {
		contentData += AsciiToString(ETX)
	}

	lrc, _ := GetLRC(contentData)
	return contentData + lrc
}

// StripStxEtxLrcFromEnvelop removes STX ETX and LRC from envelopment and returns content data,
// this method will validate LRC, if LRC fails, blank is returned
func StripStxEtxLrcFromEnvelop(envelopData string) string {
	if len(envelopData) == 0 {
		return ""
	}

	if ok := IsLRCValid(envelopData); ok {
		// remove lrc
		envelopData = envelopData[:len(envelopData)-1]

		if len(envelopData) == 0 {
			return ""
		}

		// remove stx
		if envelopData[:1] == AsciiToString(STX) {
			if len(envelopData) > 1 {
				envelopData = envelopData[1:]
			} else {
				return ""
			}
		}

		// remove etx
		if envelopData[len(envelopData)-1:] == AsciiToString(ETX) {
			if len(envelopData) > 1 {
				envelopData = envelopData[:len(envelopData)-1]
			} else {
				return ""
			}
		}

		return envelopData
	} else {
		return ""
	}
}

// ControlCharToWord converts non-printable control char to word
func ControlCharToWord(data string) string {
	data = strings.ReplaceAll(data, AsciiToString(STX), "[STX]")
	data = strings.ReplaceAll(data, AsciiToString(ETX), "[ETX]")
	data = strings.ReplaceAll(data, AsciiToString(ETB), "[ETB]")
	data = strings.ReplaceAll(data, AsciiToString(ACK), "[ACK]")
	data = strings.ReplaceAll(data, AsciiToString(NAK), "[NAK]")
	data = strings.ReplaceAll(data, AsciiToString(ENQ), "[ENQ]")
	data = strings.ReplaceAll(data, AsciiToString(DLE), "[DLE]")
	data = strings.ReplaceAll(data, AsciiToString(DC1), "[DC1]")
	data = strings.ReplaceAll(data, AsciiToString(DC2), "[DC2]")
	data = strings.ReplaceAll(data, AsciiToString(DC3), "[DC3]")
	data = strings.ReplaceAll(data, AsciiToString(DC4), "[DC4]")
	data = strings.ReplaceAll(data, AsciiToString(FS), "[FS]")
	data = strings.ReplaceAll(data, AsciiToString(US), "[US]")
	data = strings.ReplaceAll(data, AsciiToString(GS), "[GS]")
	data = strings.ReplaceAll(data, AsciiToString(RS), "[RS]")
	data = strings.ReplaceAll(data, AsciiToString(BS), "[BS]")
	data = strings.ReplaceAll(data, AsciiToString(BEL), "[BEL]")
	data = strings.ReplaceAll(data, AsciiToString(DEL), "[DEL]")
	data = strings.ReplaceAll(data, AsciiToString(EOT), "[EOT]")
	data = strings.ReplaceAll(data, AsciiToString(COMMA), "[COMMA]")
	data = strings.ReplaceAll(data, AsciiToString(COLON), "[COLON]")
	data = strings.ReplaceAll(data, AsciiToString(PIPE), "[PIPE]")
	data = strings.ReplaceAll(data, AsciiToString(NUL), "[NULL]")
	data = strings.ReplaceAll(data, AsciiToString(SOH), "[SOH]")
	data = strings.ReplaceAll(data, AsciiToString(HT), "[HT]")
	data = strings.ReplaceAll(data, AsciiToString(LF), "[LF]")
	data = strings.ReplaceAll(data, AsciiToString(VT), "[VT]")
	data = strings.ReplaceAll(data, AsciiToString(FF), "[FF]")
	data = strings.ReplaceAll(data, AsciiToString(CR), "[CR]")
	data = strings.ReplaceAll(data, AsciiToString(SO), "[SO]")
	data = strings.ReplaceAll(data, AsciiToString(SI), "[SI]")
	data = strings.ReplaceAll(data, AsciiToString(SP), "[SP]")
	data = strings.ReplaceAll(data, AsciiToString(SYN), "[SYN]")
	data = strings.ReplaceAll(data, AsciiToString(CAN), "[CAN]")
	data = strings.ReplaceAll(data, AsciiToString(EM), "[EM]")
	data = strings.ReplaceAll(data, AsciiToString(SUB), "[SUB]")
	data = strings.ReplaceAll(data, AsciiToString(ESC), "[ESC]")

	return data
}

// ControlCharToASCII converts non-printable control char represented in word to ascii non-printable form
func ControlCharToASCII(data string) string {
	data = strings.ReplaceAll(data, "[STX]", AsciiToString(STX))
	data = strings.ReplaceAll(data, "[ETX]", AsciiToString(ETX))
	data = strings.ReplaceAll(data, "[ETB]", AsciiToString(ETB))
	data = strings.ReplaceAll(data, "[ACK]", AsciiToString(ACK))
	data = strings.ReplaceAll(data, "[NAK]", AsciiToString(NAK))
	data = strings.ReplaceAll(data, "[ENQ]", AsciiToString(ENQ))
	data = strings.ReplaceAll(data, "[DLE]", AsciiToString(DLE))
	data = strings.ReplaceAll(data, "[DC1]", AsciiToString(DC1))
	data = strings.ReplaceAll(data, "[DC2]", AsciiToString(DC2))
	data = strings.ReplaceAll(data, "[DC3]", AsciiToString(DC3))
	data = strings.ReplaceAll(data, "[DC4]", AsciiToString(DC4))
	data = strings.ReplaceAll(data, "[FS]", AsciiToString(FS))
	data = strings.ReplaceAll(data, "[US]", AsciiToString(US))
	data = strings.ReplaceAll(data, "[GS]", AsciiToString(GS))
	data = strings.ReplaceAll(data, "[RS]", AsciiToString(RS))
	data = strings.ReplaceAll(data, "[BS]", AsciiToString(BS))
	data = strings.ReplaceAll(data, "[BEL]", AsciiToString(BEL))
	data = strings.ReplaceAll(data, "[DEL]", AsciiToString(DEL))
	data = strings.ReplaceAll(data, "[EOT]", AsciiToString(EOT))
	data = strings.ReplaceAll(data, "[COMMA]", AsciiToString(COMMA))
	data = strings.ReplaceAll(data, "[COLON]", AsciiToString(COLON))
	data = strings.ReplaceAll(data, "[PIPE]", AsciiToString(PIPE))
	data = strings.ReplaceAll(data, "[NULL]", AsciiToString(NUL))
	data = strings.ReplaceAll(data, "[SOH]", AsciiToString(SOH))
	data = strings.ReplaceAll(data, "[HT]", AsciiToString(HT))
	data = strings.ReplaceAll(data, "[LF]", AsciiToString(LF))
	data = strings.ReplaceAll(data, "[VT]", AsciiToString(VT))
	data = strings.ReplaceAll(data, "[FF]", AsciiToString(FF))
	data = strings.ReplaceAll(data, "[CR]", AsciiToString(CR))
	data = strings.ReplaceAll(data, "[SO]", AsciiToString(SO))
	data = strings.ReplaceAll(data, "[SI]", AsciiToString(SI))
	data = strings.ReplaceAll(data, "[SP]", AsciiToString(SP))
	data = strings.ReplaceAll(data, "[SYN]", AsciiToString(SYN))
	data = strings.ReplaceAll(data, "[CAN]", AsciiToString(CAN))
	data = strings.ReplaceAll(data, "[EM]", AsciiToString(EM))
	data = strings.ReplaceAll(data, "[SUB]", AsciiToString(SUB))
	data = strings.ReplaceAll(data, "[ESC]", AsciiToString(ESC))

	return data
}

// EscapeNonPrintable converts non printable \x00 - \x1f to pseudo escaped format
func EscapeNonPrintable(data string) string {
	data = strings.Replace(data, AsciiToString(NUL), "[NUL_00]", -1)
	data = strings.Replace(data, AsciiToString(SOH), "[SOH_01]", -1)
	data = strings.Replace(data, AsciiToString(STX), "[STX_02]", -1)
	data = strings.Replace(data, AsciiToString(ETX), "[ETX_03]", -1)
	data = strings.Replace(data, AsciiToString(EOT), "[EOT_04]", -1)
	data = strings.Replace(data, AsciiToString(ENQ), "[ENQ_05]", -1)
	data = strings.Replace(data, AsciiToString(ACK), "[ACK_06]", -1)
	data = strings.Replace(data, AsciiToString(BEL), "[BEL_07]", -1)
	data = strings.Replace(data, AsciiToString(BS), "[BS_08]", -1)
	data = strings.Replace(data, AsciiToString(HT), "[HT_09]", -1)
	data = strings.Replace(data, AsciiToString(LF), "[LF_0A]", -1)
	data = strings.Replace(data, AsciiToString(VT), "[VT_0B]", -1)
	data = strings.Replace(data, AsciiToString(FF), "[FF_0C]", -1)
	data = strings.Replace(data, AsciiToString(CR), "[CR_0D]", -1)
	data = strings.Replace(data, AsciiToString(SO), "[SO_0E]", -1)
	data = strings.Replace(data, AsciiToString(SI), "[SI_0F]", -1)
	data = strings.Replace(data, AsciiToString(DLE), "[DLE_10]", -1)
	data = strings.Replace(data, AsciiToString(DC1), "[DC1_11]", -1)
	data = strings.Replace(data, AsciiToString(DC2), "[DC2_12]", -1)
	data = strings.Replace(data, AsciiToString(DC3), "[DC3_13]", -1)
	data = strings.Replace(data, AsciiToString(DC4), "[DC4_14]", -1)
	data = strings.Replace(data, AsciiToString(NAK), "[NAK_15]", -1)
	data = strings.Replace(data, AsciiToString(SYN), "[SYN_16]", -1)
	data = strings.Replace(data, AsciiToString(ETB), "[ETB_17]", -1)
	data = strings.Replace(data, AsciiToString(CAN), "[CAN_18]", -1)
	data = strings.Replace(data, AsciiToString(EM), "[EM_19]", -1)
	data = strings.Replace(data, AsciiToString(SUB), "[SUB_1A]", -1)
	data = strings.Replace(data, AsciiToString(ESC), "[ESC_1B]", -1)
	data = strings.Replace(data, AsciiToString(CR), "[CR_1C]", -1)
	data = strings.Replace(data, AsciiToString(FS), "[FS_1D]", -1)
	data = strings.Replace(data, AsciiToString(RS), "[RS_1E]", -1)
	data = strings.Replace(data, AsciiToString(US), "[US_1F]", -1)

	return data
}

// UnescapeNonPrintable converts pseudo escaped back to non printable form
func UnescapeNonPrintable(data string) string {
	data = strings.Replace(data, "[NUL_00]", AsciiToString(NUL), -1)
	data = strings.Replace(data,  "[SOH_01]", AsciiToString(SOH),-1)
	data = strings.Replace(data,  "[STX_02]", AsciiToString(STX),-1)
	data = strings.Replace(data,  "[ETX_03]", AsciiToString(ETX),-1)
	data = strings.Replace(data,  "[EOT_04]", AsciiToString(EOT),-1)
	data = strings.Replace(data,  "[ENQ_05]", AsciiToString(ENQ),-1)
	data = strings.Replace(data,  "[ACK_06]", AsciiToString(ACK),-1)
	data = strings.Replace(data,  "[BEL_07]", AsciiToString(BEL),-1)
	data = strings.Replace(data,  "[BS_08]", AsciiToString(BS),-1)
	data = strings.Replace(data,  "[HT_09]", AsciiToString(HT),-1)
	data = strings.Replace(data,  "[LF_0A]", AsciiToString(LF),-1)
	data = strings.Replace(data,  "[VT_0B]", AsciiToString(VT),-1)
	data = strings.Replace(data,  "[FF_0C]", AsciiToString(FF),-1)
	data = strings.Replace(data,  "[CR_0D]", AsciiToString(CR),-1)
	data = strings.Replace(data,  "[SO_0E]", AsciiToString(SO),-1)
	data = strings.Replace(data,  "[SI_0F]", AsciiToString(SI),-1)
	data = strings.Replace(data,  "[DLE_10]", AsciiToString(DLE),-1)
	data = strings.Replace(data,  "[DC1_11]", AsciiToString(DC1),-1)
	data = strings.Replace(data,  "[DC2_12]", AsciiToString(DC2),-1)
	data = strings.Replace(data,  "[DC3_13]", AsciiToString(DC3),-1)
	data = strings.Replace(data,  "[DC4_14]", AsciiToString(DC4),-1)
	data = strings.Replace(data,  "[NAK_15]", AsciiToString(NAK),-1)
	data = strings.Replace(data,  "[SYN_16]", AsciiToString(SYN),-1)
	data = strings.Replace(data,  "[ETB_17]", AsciiToString(ETB),-1)
	data = strings.Replace(data,  "[CAN_18]", AsciiToString(CAN),-1)
	data = strings.Replace(data,  "[EM_19]", AsciiToString(EM),-1)
	data = strings.Replace(data,  "[SUB_1A]", AsciiToString(SUB),-1)
	data = strings.Replace(data,  "[ESC_1B]", AsciiToString(ESC),-1)
	data = strings.Replace(data,  "[CR_1C]", AsciiToString(CR),-1)
	data = strings.Replace(data,  "[FS_1D]", AsciiToString(FS),-1)
	data = strings.Replace(data,  "[RS_1E]", AsciiToString(RS),-1)
	data = strings.Replace(data,  "[US_1F]", AsciiToString(US),-1)

	return data
}