// Code Generated By gen-enumer For "Enum Type: SdHealthCheckType" - DO NOT EDIT;

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

package sdhealthchecktype

import (
	"fmt"
	"strconv"
)

// enum names constants
const (
	_SdHealthCheckTypeName_0 = "UNKNOWN"
	_SdHealthCheckTypeName_1 = "HTTP"
	_SdHealthCheckTypeName_2 = "HTTPS"
	_SdHealthCheckTypeName_3 = "TCP"
)

// var declares of enum indexes
var (
	_SdHealthCheckTypeIndex_0 = [...]uint8{0, 7}
	_SdHealthCheckTypeIndex_1 = [...]uint8{0, 4}
	_SdHealthCheckTypeIndex_2 = [...]uint8{0, 5}
	_SdHealthCheckTypeIndex_3 = [...]uint8{0, 3}
)

func (i SdHealthCheckType) String() string {
	switch {
	case i == UNKNOWN:
		return _SdHealthCheckTypeName_0
	case i == HTTP:
		return _SdHealthCheckTypeName_1
	case i == HTTPS:
		return _SdHealthCheckTypeName_2
	case i == TCP:
		return _SdHealthCheckTypeName_3
	default:
		return ""
	}
}

var _SdHealthCheckTypeValues = []SdHealthCheckType{
	0, // UNKNOWN
	1, // HTTP
	2, // HTTPS
	3, // TCP
}

var _SdHealthCheckTypeNameToValueMap = map[string]SdHealthCheckType{
	_SdHealthCheckTypeName_0[0:7]: 0, // UNKNOWN
	_SdHealthCheckTypeName_1[0:4]: 1, // HTTP
	_SdHealthCheckTypeName_2[0:5]: 2, // HTTPS
	_SdHealthCheckTypeName_3[0:3]: 3, // TCP
}

var _SdHealthCheckTypeValueToKeyMap = map[SdHealthCheckType]string{
	0: _SdHealthCheckTypeKey_0, // UNKNOWN
	1: _SdHealthCheckTypeKey_1, // HTTP
	2: _SdHealthCheckTypeKey_2, // HTTPS
	3: _SdHealthCheckTypeKey_3, // TCP
}

var _SdHealthCheckTypeValueToCaptionMap = map[SdHealthCheckType]string{
	0: _SdHealthCheckTypeCaption_0, // UNKNOWN
	1: _SdHealthCheckTypeCaption_1, // HTTP
	2: _SdHealthCheckTypeCaption_2, // HTTPS
	3: _SdHealthCheckTypeCaption_3, // TCP
}

var _SdHealthCheckTypeValueToDescriptionMap = map[SdHealthCheckType]string{
	0: _SdHealthCheckTypeDescription_0, // UNKNOWN
	1: _SdHealthCheckTypeDescription_1, // HTTP
	2: _SdHealthCheckTypeDescription_2, // HTTPS
	3: _SdHealthCheckTypeDescription_3, // TCP
}

// Valid returns 'true' if the value is listed in the SdHealthCheckType enum map definition, 'false' otherwise
func (i SdHealthCheckType) Valid() bool {
	for _, v := range _SdHealthCheckTypeValues {
		if i == v {
			return true
		}
	}

	return false
}

// ParseByName retrieves a SdHealthCheckType enum value from the enum string name,
// throws an error if the param is not part of the enum
func (i SdHealthCheckType) ParseByName(s string) (SdHealthCheckType, error) {
	if val, ok := _SdHealthCheckTypeNameToValueMap[s]; ok {
		// parse ok
		return val, nil
	}

	// error
	return -1, fmt.Errorf("Enum Name of %s Not Expected In SdHealthCheckType Values List", s)
}

// ParseByKey retrieves a SdHealthCheckType enum value from the enum string key,
// throws an error if the param is not part of the enum
func (i SdHealthCheckType) ParseByKey(s string) (SdHealthCheckType, error) {
	for k, v := range _SdHealthCheckTypeValueToKeyMap {
		if v == s {
			// parse ok
			return k, nil
		}
	}

	// error
	return -1, fmt.Errorf("Enum Key of %s Not Expected In SdHealthCheckType Keys List", s)
}

// Key retrieves a SdHealthCheckType enum string key
func (i SdHealthCheckType) Key() string {
	if val, ok := _SdHealthCheckTypeValueToKeyMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Caption retrieves a SdHealthCheckType enum string caption
func (i SdHealthCheckType) Caption() string {
	if val, ok := _SdHealthCheckTypeValueToCaptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Description retrieves a SdHealthCheckType enum string description
func (i SdHealthCheckType) Description() string {
	if val, ok := _SdHealthCheckTypeValueToDescriptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// IntValue gets the intrinsic enum integer value
func (i SdHealthCheckType) IntValue() int {
	return int(i)
}

// IntString gets the intrinsic enum integer value represented in string format
func (i SdHealthCheckType) IntString() string {
	return strconv.Itoa(int(i))
}

// ValueSlice returns all values of the enum SdHealthCheckType in a slice
func (i SdHealthCheckType) ValueSlice() []SdHealthCheckType {
	return _SdHealthCheckTypeValues
}

// NameMap returns all names of the enum SdHealthCheckType in a K:name,V:SdHealthCheckType map
func (i SdHealthCheckType) NameMap() map[string]SdHealthCheckType {
	return _SdHealthCheckTypeNameToValueMap
}

// KeyMap returns all keys of the enum SdHealthCheckType in a K:SdHealthCheckType,V:key map
func (i SdHealthCheckType) KeyMap() map[SdHealthCheckType]string {
	return _SdHealthCheckTypeValueToKeyMap
}

// CaptionMap returns all captions of the enum SdHealthCheckType in a K:SdHealthCheckType,V:caption map
func (i SdHealthCheckType) CaptionMap() map[SdHealthCheckType]string {
	return _SdHealthCheckTypeValueToCaptionMap
}

// DescriptionMap returns all descriptions of the enum SdHealthCheckType in a K:SdHealthCheckType,V:description map
func (i SdHealthCheckType) DescriptionMap() map[SdHealthCheckType]string {
	return _SdHealthCheckTypeValueToDescriptionMap
}
