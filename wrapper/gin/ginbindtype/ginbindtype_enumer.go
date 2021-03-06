// Code Generated By gen-enumer For "Enum Type: GinBindType" - DO NOT EDIT;

package ginbindtype

import (
	"fmt"
	"strconv"
)

// enum names constants
const (
	_GinBindTypeName_0 = "UNKNOWN"
	_GinBindTypeName_1 = "BindHeader"
	_GinBindTypeName_2 = "BindJson"
	_GinBindTypeName_3 = "BindQuery"
	_GinBindTypeName_4 = "BindUri"
	_GinBindTypeName_5 = "BindXml"
	_GinBindTypeName_6 = "BindYaml"
	_GinBindTypeName_7 = "BindProtoBuf"
	_GinBindTypeName_8 = "BindPostForm"
)

// var declares of enum indexes
var (
	_GinBindTypeIndex_0 = [...]uint8{0, 7}
	_GinBindTypeIndex_1 = [...]uint8{0, 10}
	_GinBindTypeIndex_2 = [...]uint8{0, 8}
	_GinBindTypeIndex_3 = [...]uint8{0, 9}
	_GinBindTypeIndex_4 = [...]uint8{0, 7}
	_GinBindTypeIndex_5 = [...]uint8{0, 7}
	_GinBindTypeIndex_6 = [...]uint8{0, 8}
	_GinBindTypeIndex_7 = [...]uint8{0, 12}
	_GinBindTypeIndex_8 = [...]uint8{0, 12}
)

func (i GinBindType) String() string {
	switch {
	case i == UNKNOWN:
		return _GinBindTypeName_0
	case i == BindHeader:
		return _GinBindTypeName_1
	case i == BindJson:
		return _GinBindTypeName_2
	case i == BindQuery:
		return _GinBindTypeName_3
	case i == BindUri:
		return _GinBindTypeName_4
	case i == BindXml:
		return _GinBindTypeName_5
	case i == BindYaml:
		return _GinBindTypeName_6
	case i == BindProtoBuf:
		return _GinBindTypeName_7
	case i == BindPostForm:
		return _GinBindTypeName_8
	default:
		return ""
	}
}

var _GinBindTypeValues = []GinBindType{
	0, // UNKNOWN
	1, // BindHeader
	2, // BindJson
	3, // BindQuery
	4, // BindUri
	5, // BindXml
	6, // BindYaml
	7, // BindProtoBuf
	8, // BindPostForm
}

var _GinBindTypeNameToValueMap = map[string]GinBindType{
	_GinBindTypeName_0[0:7]:  0, // UNKNOWN
	_GinBindTypeName_1[0:10]: 1, // BindHeader
	_GinBindTypeName_2[0:8]:  2, // BindJson
	_GinBindTypeName_3[0:9]:  3, // BindQuery
	_GinBindTypeName_4[0:7]:  4, // BindUri
	_GinBindTypeName_5[0:7]:  5, // BindXml
	_GinBindTypeName_6[0:8]:  6, // BindYaml
	_GinBindTypeName_7[0:12]: 7, // BindProtoBuf
	_GinBindTypeName_8[0:12]: 8, // BindPostForm
}

var _GinBindTypeValueToKeyMap = map[GinBindType]string{
	0: _GinBindTypeKey_0, // UNKNOWN
	1: _GinBindTypeKey_1, // BindHeader
	2: _GinBindTypeKey_2, // BindJson
	3: _GinBindTypeKey_3, // BindQuery
	4: _GinBindTypeKey_4, // BindUri
	5: _GinBindTypeKey_5, // BindXml
	6: _GinBindTypeKey_6, // BindYaml
	7: _GinBindTypeKey_7, // BindProtoBuf
	8: _GinBindTypeKey_8, // BindPostForm
}

var _GinBindTypeValueToCaptionMap = map[GinBindType]string{
	0: _GinBindTypeCaption_0, // UNKNOWN
	1: _GinBindTypeCaption_1, // BindHeader
	2: _GinBindTypeCaption_2, // BindJson
	3: _GinBindTypeCaption_3, // BindQuery
	4: _GinBindTypeCaption_4, // BindUri
	5: _GinBindTypeCaption_5, // BindXml
	6: _GinBindTypeCaption_6, // BindYaml
	7: _GinBindTypeCaption_7, // BindProtoBuf
	8: _GinBindTypeCaption_8, // BindPostForm
}

var _GinBindTypeValueToDescriptionMap = map[GinBindType]string{
	0: _GinBindTypeDescription_0, // UNKNOWN
	1: _GinBindTypeDescription_1, // BindHeader
	2: _GinBindTypeDescription_2, // BindJson
	3: _GinBindTypeDescription_3, // BindQuery
	4: _GinBindTypeDescription_4, // BindUri
	5: _GinBindTypeDescription_5, // BindXml
	6: _GinBindTypeDescription_6, // BindYaml
	7: _GinBindTypeDescription_7, // BindProtoBuf
	8: _GinBindTypeDescription_8, // BindPostForm
}

// Valid returns 'true' if the value is listed in the GinBindType enum map definition, 'false' otherwise
func (i GinBindType) Valid() bool {
	for _, v := range _GinBindTypeValues {
		if i == v {
			return true
		}
	}

	return false
}

// ParseByName retrieves a GinBindType enum value from the enum string name,
// throws an error if the param is not part of the enum
func (i GinBindType) ParseByName(s string) (GinBindType, error) {
	if val, ok := _GinBindTypeNameToValueMap[s]; ok {
		// parse ok
		return val, nil
	}

	// error
	return -1, fmt.Errorf("Enum Name of %s Not Expected In GinBindType Values List", s)
}

// ParseByKey retrieves a GinBindType enum value from the enum string key,
// throws an error if the param is not part of the enum
func (i GinBindType) ParseByKey(s string) (GinBindType, error) {
	for k, v := range _GinBindTypeValueToKeyMap {
		if v == s {
			// parse ok
			return k, nil
		}
	}

	// error
	return -1, fmt.Errorf("Enum Key of %s Not Expected In GinBindType Keys List", s)
}

// Key retrieves a GinBindType enum string key
func (i GinBindType) Key() string {
	if val, ok := _GinBindTypeValueToKeyMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Caption retrieves a GinBindType enum string caption
func (i GinBindType) Caption() string {
	if val, ok := _GinBindTypeValueToCaptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Description retrieves a GinBindType enum string description
func (i GinBindType) Description() string {
	if val, ok := _GinBindTypeValueToDescriptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// IntValue gets the intrinsic enum integer value
func (i GinBindType) IntValue() int {
	return int(i)
}

// IntString gets the intrinsic enum integer value represented in string format
func (i GinBindType) IntString() string {
	return strconv.Itoa(int(i))
}

// ValueSlice returns all values of the enum GinBindType in a slice
func (i GinBindType) ValueSlice() []GinBindType {
	return _GinBindTypeValues
}

// NameMap returns all names of the enum GinBindType in a K:name,V:GinBindType map
func (i GinBindType) NameMap() map[string]GinBindType {
	return _GinBindTypeNameToValueMap
}

// KeyMap returns all keys of the enum GinBindType in a K:GinBindType,V:key map
func (i GinBindType) KeyMap() map[GinBindType]string {
	return _GinBindTypeValueToKeyMap
}

// CaptionMap returns all captions of the enum GinBindType in a K:GinBindType,V:caption map
func (i GinBindType) CaptionMap() map[GinBindType]string {
	return _GinBindTypeValueToCaptionMap
}

// DescriptionMap returns all descriptions of the enum GinBindType in a K:GinBindType,V:description map
func (i GinBindType) DescriptionMap() map[GinBindType]string {
	return _GinBindTypeValueToDescriptionMap
}
