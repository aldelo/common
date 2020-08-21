// Code Generated By gen-enumer For "Enum Type: SNSPlatformApplicationAttribute" - DO NOT EDIT;

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

package snsplatformapplicationattribute

import (
	"fmt"
	"strconv"
)

// enum names constants
const (
	_SNSPlatformApplicationAttributeName_0 = "UNKNOWN"
	_SNSPlatformApplicationAttributeName_1 = "PlatformCredential"
	_SNSPlatformApplicationAttributeName_2 = "PlatformPrincipal"
	_SNSPlatformApplicationAttributeName_3 = "EventEndpointCreated"
	_SNSPlatformApplicationAttributeName_4 = "EventEndpointDeleted"
	_SNSPlatformApplicationAttributeName_5 = "EventEndpointUpdated"
	_SNSPlatformApplicationAttributeName_6 = "EventDeliveryFailure"
	_SNSPlatformApplicationAttributeName_7 = "SuccessFeedbackRoleArn"
	_SNSPlatformApplicationAttributeName_8 = "FailureFeedbackRoleArn"
	_SNSPlatformApplicationAttributeName_9 = "SuccessFeedbackSampleRate"
)

// var declares of enum indexes
var (
	_SNSPlatformApplicationAttributeIndex_0 = [...]uint8{0, 7}
	_SNSPlatformApplicationAttributeIndex_1 = [...]uint8{0, 18}
	_SNSPlatformApplicationAttributeIndex_2 = [...]uint8{0, 17}
	_SNSPlatformApplicationAttributeIndex_3 = [...]uint8{0, 20}
	_SNSPlatformApplicationAttributeIndex_4 = [...]uint8{0, 20}
	_SNSPlatformApplicationAttributeIndex_5 = [...]uint8{0, 20}
	_SNSPlatformApplicationAttributeIndex_6 = [...]uint8{0, 20}
	_SNSPlatformApplicationAttributeIndex_7 = [...]uint8{0, 22}
	_SNSPlatformApplicationAttributeIndex_8 = [...]uint8{0, 22}
	_SNSPlatformApplicationAttributeIndex_9 = [...]uint8{0, 25}
)

func (i SNSPlatformApplicationAttribute) String() string {
	switch {
	case i == UNKNOWN:
		return _SNSPlatformApplicationAttributeName_0
	case i == PlatformCredential:
		return _SNSPlatformApplicationAttributeName_1
	case i == PlatformPrincipal:
		return _SNSPlatformApplicationAttributeName_2
	case i == EventEndpointCreated:
		return _SNSPlatformApplicationAttributeName_3
	case i == EventEndpointDeleted:
		return _SNSPlatformApplicationAttributeName_4
	case i == EventEndpointUpdated:
		return _SNSPlatformApplicationAttributeName_5
	case i == EventDeliveryFailure:
		return _SNSPlatformApplicationAttributeName_6
	case i == SuccessFeedbackRoleArn:
		return _SNSPlatformApplicationAttributeName_7
	case i == FailureFeedbackRoleArn:
		return _SNSPlatformApplicationAttributeName_8
	case i == SuccessFeedbackSampleRate:
		return _SNSPlatformApplicationAttributeName_9
	default:
		return ""
	}
}

var _SNSPlatformApplicationAttributeValues = []SNSPlatformApplicationAttribute{
	0, // UNKNOWN
	1, // PlatformCredential
	2, // PlatformPrincipal
	3, // EventEndpointCreated
	4, // EventEndpointDeleted
	5, // EventEndpointUpdated
	6, // EventDeliveryFailure
	7, // SuccessFeedbackRoleArn
	8, // FailureFeedbackRoleArn
	9, // SuccessFeedbackSampleRate
}

var _SNSPlatformApplicationAttributeNameToValueMap = map[string]SNSPlatformApplicationAttribute{
	_SNSPlatformApplicationAttributeName_0[0:7]:  0, // UNKNOWN
	_SNSPlatformApplicationAttributeName_1[0:18]: 1, // PlatformCredential
	_SNSPlatformApplicationAttributeName_2[0:17]: 2, // PlatformPrincipal
	_SNSPlatformApplicationAttributeName_3[0:20]: 3, // EventEndpointCreated
	_SNSPlatformApplicationAttributeName_4[0:20]: 4, // EventEndpointDeleted
	_SNSPlatformApplicationAttributeName_5[0:20]: 5, // EventEndpointUpdated
	_SNSPlatformApplicationAttributeName_6[0:20]: 6, // EventDeliveryFailure
	_SNSPlatformApplicationAttributeName_7[0:22]: 7, // SuccessFeedbackRoleArn
	_SNSPlatformApplicationAttributeName_8[0:22]: 8, // FailureFeedbackRoleArn
	_SNSPlatformApplicationAttributeName_9[0:25]: 9, // SuccessFeedbackSampleRate
}

var _SNSPlatformApplicationAttributeValueToKeyMap = map[SNSPlatformApplicationAttribute]string{
	0: _SNSPlatformApplicationAttributeKey_0, // UNKNOWN
	1: _SNSPlatformApplicationAttributeKey_1, // PlatformCredential
	2: _SNSPlatformApplicationAttributeKey_2, // PlatformPrincipal
	3: _SNSPlatformApplicationAttributeKey_3, // EventEndpointCreated
	4: _SNSPlatformApplicationAttributeKey_4, // EventEndpointDeleted
	5: _SNSPlatformApplicationAttributeKey_5, // EventEndpointUpdated
	6: _SNSPlatformApplicationAttributeKey_6, // EventDeliveryFailure
	7: _SNSPlatformApplicationAttributeKey_7, // SuccessFeedbackRoleArn
	8: _SNSPlatformApplicationAttributeKey_8, // FailureFeedbackRoleArn
	9: _SNSPlatformApplicationAttributeKey_9, // SuccessFeedbackSampleRate
}

var _SNSPlatformApplicationAttributeValueToCaptionMap = map[SNSPlatformApplicationAttribute]string{
	0: _SNSPlatformApplicationAttributeCaption_0, // UNKNOWN
	1: _SNSPlatformApplicationAttributeCaption_1, // PlatformCredential
	2: _SNSPlatformApplicationAttributeCaption_2, // PlatformPrincipal
	3: _SNSPlatformApplicationAttributeCaption_3, // EventEndpointCreated
	4: _SNSPlatformApplicationAttributeCaption_4, // EventEndpointDeleted
	5: _SNSPlatformApplicationAttributeCaption_5, // EventEndpointUpdated
	6: _SNSPlatformApplicationAttributeCaption_6, // EventDeliveryFailure
	7: _SNSPlatformApplicationAttributeCaption_7, // SuccessFeedbackRoleArn
	8: _SNSPlatformApplicationAttributeCaption_8, // FailureFeedbackRoleArn
	9: _SNSPlatformApplicationAttributeCaption_9, // SuccessFeedbackSampleRate
}

var _SNSPlatformApplicationAttributeValueToDescriptionMap = map[SNSPlatformApplicationAttribute]string{
	0: _SNSPlatformApplicationAttributeDescription_0, // UNKNOWN
	1: _SNSPlatformApplicationAttributeDescription_1, // PlatformCredential
	2: _SNSPlatformApplicationAttributeDescription_2, // PlatformPrincipal
	3: _SNSPlatformApplicationAttributeDescription_3, // EventEndpointCreated
	4: _SNSPlatformApplicationAttributeDescription_4, // EventEndpointDeleted
	5: _SNSPlatformApplicationAttributeDescription_5, // EventEndpointUpdated
	6: _SNSPlatformApplicationAttributeDescription_6, // EventDeliveryFailure
	7: _SNSPlatformApplicationAttributeDescription_7, // SuccessFeedbackRoleArn
	8: _SNSPlatformApplicationAttributeDescription_8, // FailureFeedbackRoleArn
	9: _SNSPlatformApplicationAttributeDescription_9, // SuccessFeedbackSampleRate
}

// Valid returns 'true' if the value is listed in the SNSPlatformApplicationAttribute enum map definition, 'false' otherwise
func (i SNSPlatformApplicationAttribute) Valid() bool {
	for _, v := range _SNSPlatformApplicationAttributeValues {
		if i == v {
			return true
		}
	}

	return false
}

// ParseByName retrieves a SNSPlatformApplicationAttribute enum value from the enum string name,
// throws an error if the param is not part of the enum
func (i SNSPlatformApplicationAttribute) ParseByName(s string) (SNSPlatformApplicationAttribute, error) {
	if val, ok := _SNSPlatformApplicationAttributeNameToValueMap[s]; ok {
		// parse ok
		return val, nil
	}

	// error
	return -1, fmt.Errorf("Enum Name of %s Not Expected In SNSPlatformApplicationAttribute Values List", s)
}

// ParseByKey retrieves a SNSPlatformApplicationAttribute enum value from the enum string key,
// throws an error if the param is not part of the enum
func (i SNSPlatformApplicationAttribute) ParseByKey(s string) (SNSPlatformApplicationAttribute, error) {
	for k, v := range _SNSPlatformApplicationAttributeValueToKeyMap {
		if v == s {
			// parse ok
			return k, nil
		}
	}

	// error
	return -1, fmt.Errorf("Enum Key of %s Not Expected In SNSPlatformApplicationAttribute Keys List", s)
}

// Key retrieves a SNSPlatformApplicationAttribute enum string key
func (i SNSPlatformApplicationAttribute) Key() string {
	if val, ok := _SNSPlatformApplicationAttributeValueToKeyMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Caption retrieves a SNSPlatformApplicationAttribute enum string caption
func (i SNSPlatformApplicationAttribute) Caption() string {
	if val, ok := _SNSPlatformApplicationAttributeValueToCaptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Description retrieves a SNSPlatformApplicationAttribute enum string description
func (i SNSPlatformApplicationAttribute) Description() string {
	if val, ok := _SNSPlatformApplicationAttributeValueToDescriptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// IntValue gets the intrinsic enum integer value
func (i SNSPlatformApplicationAttribute) IntValue() int {
	return int(i)
}

// IntString gets the intrinsic enum integer value represented in string format
func (i SNSPlatformApplicationAttribute) IntString() string {
	return strconv.Itoa(int(i))
}

// ValueSlice returns all values of the enum SNSPlatformApplicationAttribute in a slice
func (i SNSPlatformApplicationAttribute) ValueSlice() []SNSPlatformApplicationAttribute {
	return _SNSPlatformApplicationAttributeValues
}

// NameMap returns all names of the enum SNSPlatformApplicationAttribute in a K:name,V:SNSPlatformApplicationAttribute map
func (i SNSPlatformApplicationAttribute) NameMap() map[string]SNSPlatformApplicationAttribute {
	return _SNSPlatformApplicationAttributeNameToValueMap
}

// KeyMap returns all keys of the enum SNSPlatformApplicationAttribute in a K:SNSPlatformApplicationAttribute,V:key map
func (i SNSPlatformApplicationAttribute) KeyMap() map[SNSPlatformApplicationAttribute]string {
	return _SNSPlatformApplicationAttributeValueToKeyMap
}

// CaptionMap returns all captions of the enum SNSPlatformApplicationAttribute in a K:SNSPlatformApplicationAttribute,V:caption map
func (i SNSPlatformApplicationAttribute) CaptionMap() map[SNSPlatformApplicationAttribute]string {
	return _SNSPlatformApplicationAttributeValueToCaptionMap
}

// DescriptionMap returns all descriptions of the enum SNSPlatformApplicationAttribute in a K:SNSPlatformApplicationAttribute,V:description map
func (i SNSPlatformApplicationAttribute) DescriptionMap() map[SNSPlatformApplicationAttribute]string {
	return _SNSPlatformApplicationAttributeValueToDescriptionMap
}