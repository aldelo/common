// Code Generated By gen-enumer For "Enum Type: GinJwtSignAlgorithm" - DO NOT EDIT;

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

package ginjwtsignalgorithm

import (
	"fmt"
	"strconv"
)

// enum names constants
const (
	_GinJwtSignAlgorithmName_0 = "UNKNOWN"
	_GinJwtSignAlgorithmName_1 = "HS256"
	_GinJwtSignAlgorithmName_2 = "HS384"
	_GinJwtSignAlgorithmName_3 = "HS512"
	_GinJwtSignAlgorithmName_4 = "RS256"
	_GinJwtSignAlgorithmName_5 = "RS384"
	_GinJwtSignAlgorithmName_6 = "RS512"
)

// var declares of enum indexes
var (
	_GinJwtSignAlgorithmIndex_0 = [...]uint8{0, 7}
	_GinJwtSignAlgorithmIndex_1 = [...]uint8{0, 5}
	_GinJwtSignAlgorithmIndex_2 = [...]uint8{0, 5}
	_GinJwtSignAlgorithmIndex_3 = [...]uint8{0, 5}
	_GinJwtSignAlgorithmIndex_4 = [...]uint8{0, 5}
	_GinJwtSignAlgorithmIndex_5 = [...]uint8{0, 5}
	_GinJwtSignAlgorithmIndex_6 = [...]uint8{0, 5}
)

func (i GinJwtSignAlgorithm) String() string {
	switch {
	case i == UNKNOWN:
		return _GinJwtSignAlgorithmName_0
	case i == HS256:
		return _GinJwtSignAlgorithmName_1
	case i == HS384:
		return _GinJwtSignAlgorithmName_2
	case i == HS512:
		return _GinJwtSignAlgorithmName_3
	case i == RS256:
		return _GinJwtSignAlgorithmName_4
	case i == RS384:
		return _GinJwtSignAlgorithmName_5
	case i == RS512:
		return _GinJwtSignAlgorithmName_6
	default:
		return ""
	}
}

var _GinJwtSignAlgorithmValues = []GinJwtSignAlgorithm{
	0, // UNKNOWN
	1, // HS256
	2, // HS384
	3, // HS512
	4, // RS256
	5, // RS384
	6, // RS512
}

var _GinJwtSignAlgorithmNameToValueMap = map[string]GinJwtSignAlgorithm{
	_GinJwtSignAlgorithmName_0[0:7]: 0, // UNKNOWN
	_GinJwtSignAlgorithmName_1[0:5]: 1, // HS256
	_GinJwtSignAlgorithmName_2[0:5]: 2, // HS384
	_GinJwtSignAlgorithmName_3[0:5]: 3, // HS512
	_GinJwtSignAlgorithmName_4[0:5]: 4, // RS256
	_GinJwtSignAlgorithmName_5[0:5]: 5, // RS384
	_GinJwtSignAlgorithmName_6[0:5]: 6, // RS512
}

var _GinJwtSignAlgorithmValueToKeyMap = map[GinJwtSignAlgorithm]string{
	0: _GinJwtSignAlgorithmKey_0, // UNKNOWN
	1: _GinJwtSignAlgorithmKey_1, // HS256
	2: _GinJwtSignAlgorithmKey_2, // HS384
	3: _GinJwtSignAlgorithmKey_3, // HS512
	4: _GinJwtSignAlgorithmKey_4, // RS256
	5: _GinJwtSignAlgorithmKey_5, // RS384
	6: _GinJwtSignAlgorithmKey_6, // RS512
}

var _GinJwtSignAlgorithmValueToCaptionMap = map[GinJwtSignAlgorithm]string{
	0: _GinJwtSignAlgorithmCaption_0, // UNKNOWN
	1: _GinJwtSignAlgorithmCaption_1, // HS256
	2: _GinJwtSignAlgorithmCaption_2, // HS384
	3: _GinJwtSignAlgorithmCaption_3, // HS512
	4: _GinJwtSignAlgorithmCaption_4, // RS256
	5: _GinJwtSignAlgorithmCaption_5, // RS384
	6: _GinJwtSignAlgorithmCaption_6, // RS512
}

var _GinJwtSignAlgorithmValueToDescriptionMap = map[GinJwtSignAlgorithm]string{
	0: _GinJwtSignAlgorithmDescription_0, // UNKNOWN
	1: _GinJwtSignAlgorithmDescription_1, // HS256
	2: _GinJwtSignAlgorithmDescription_2, // HS384
	3: _GinJwtSignAlgorithmDescription_3, // HS512
	4: _GinJwtSignAlgorithmDescription_4, // RS256
	5: _GinJwtSignAlgorithmDescription_5, // RS384
	6: _GinJwtSignAlgorithmDescription_6, // RS512
}

// Valid returns 'true' if the value is listed in the GinJwtSignAlgorithm enum map definition, 'false' otherwise
func (i GinJwtSignAlgorithm) Valid() bool {
	for _, v := range _GinJwtSignAlgorithmValues {
		if i == v {
			return true
		}
	}

	return false
}

// ParseByName retrieves a GinJwtSignAlgorithm enum value from the enum string name,
// throws an error if the param is not part of the enum
func (i GinJwtSignAlgorithm) ParseByName(s string) (GinJwtSignAlgorithm, error) {
	if val, ok := _GinJwtSignAlgorithmNameToValueMap[s]; ok {
		// parse ok
		return val, nil
	}

	// error
	return -1, fmt.Errorf("Enum Name of %s Not Expected In GinJwtSignAlgorithm Values List", s)
}

// ParseByKey retrieves a GinJwtSignAlgorithm enum value from the enum string key,
// throws an error if the param is not part of the enum
func (i GinJwtSignAlgorithm) ParseByKey(s string) (GinJwtSignAlgorithm, error) {
	for k, v := range _GinJwtSignAlgorithmValueToKeyMap {
		if v == s {
			// parse ok
			return k, nil
		}
	}

	// error
	return -1, fmt.Errorf("Enum Key of %s Not Expected In GinJwtSignAlgorithm Keys List", s)
}

// Key retrieves a GinJwtSignAlgorithm enum string key
func (i GinJwtSignAlgorithm) Key() string {
	if val, ok := _GinJwtSignAlgorithmValueToKeyMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Caption retrieves a GinJwtSignAlgorithm enum string caption
func (i GinJwtSignAlgorithm) Caption() string {
	if val, ok := _GinJwtSignAlgorithmValueToCaptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Description retrieves a GinJwtSignAlgorithm enum string description
func (i GinJwtSignAlgorithm) Description() string {
	if val, ok := _GinJwtSignAlgorithmValueToDescriptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// IntValue gets the intrinsic enum integer value
func (i GinJwtSignAlgorithm) IntValue() int {
	return int(i)
}

// IntString gets the intrinsic enum integer value represented in string format
func (i GinJwtSignAlgorithm) IntString() string {
	return strconv.Itoa(int(i))
}

// ValueSlice returns all values of the enum GinJwtSignAlgorithm in a slice
func (i GinJwtSignAlgorithm) ValueSlice() []GinJwtSignAlgorithm {
	return _GinJwtSignAlgorithmValues
}

// NameMap returns all names of the enum GinJwtSignAlgorithm in a K:name,V:GinJwtSignAlgorithm map
func (i GinJwtSignAlgorithm) NameMap() map[string]GinJwtSignAlgorithm {
	return _GinJwtSignAlgorithmNameToValueMap
}

// KeyMap returns all keys of the enum GinJwtSignAlgorithm in a K:GinJwtSignAlgorithm,V:key map
func (i GinJwtSignAlgorithm) KeyMap() map[GinJwtSignAlgorithm]string {
	return _GinJwtSignAlgorithmValueToKeyMap
}

// CaptionMap returns all captions of the enum GinJwtSignAlgorithm in a K:GinJwtSignAlgorithm,V:caption map
func (i GinJwtSignAlgorithm) CaptionMap() map[GinJwtSignAlgorithm]string {
	return _GinJwtSignAlgorithmValueToCaptionMap
}

// DescriptionMap returns all descriptions of the enum GinJwtSignAlgorithm in a K:GinJwtSignAlgorithm,V:description map
func (i GinJwtSignAlgorithm) DescriptionMap() map[GinJwtSignAlgorithm]string {
	return _GinJwtSignAlgorithmValueToDescriptionMap
}
