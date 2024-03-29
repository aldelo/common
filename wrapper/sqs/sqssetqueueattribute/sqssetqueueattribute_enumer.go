// Code Generated By gen-enumer For "Enum Type: SQSSetQueueAttribute" - DO NOT EDIT;

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

package sqssetqueueattribute

import (
	"fmt"
	"strconv"
)

// enum names constants
const (
	_SQSSetQueueAttributeName_0  = "UNKNOWN"
	_SQSSetQueueAttributeName_1  = "DelaySeconds"
	_SQSSetQueueAttributeName_2  = "MaximumMessageSize"
	_SQSSetQueueAttributeName_3  = "MessageRetentionPeriod"
	_SQSSetQueueAttributeName_4  = "Policy"
	_SQSSetQueueAttributeName_5  = "ReceiveMessageWaitTimeSeconds"
	_SQSSetQueueAttributeName_6  = "RedrivePolicy"
	_SQSSetQueueAttributeName_7  = "DeadLetterTargetArn"
	_SQSSetQueueAttributeName_8  = "MaxReceiveCount"
	_SQSSetQueueAttributeName_9  = "VisibilityTimeout"
	_SQSSetQueueAttributeName_10 = "KmsMasterKeyId"
	_SQSSetQueueAttributeName_11 = "KmsDataKeyReusePeriodSeconds"
	_SQSSetQueueAttributeName_12 = "ContentBasedDeduplication"
)

// var declares of enum indexes
var (
	_SQSSetQueueAttributeIndex_0  = [...]uint8{0, 7}
	_SQSSetQueueAttributeIndex_1  = [...]uint8{0, 12}
	_SQSSetQueueAttributeIndex_2  = [...]uint8{0, 18}
	_SQSSetQueueAttributeIndex_3  = [...]uint8{0, 22}
	_SQSSetQueueAttributeIndex_4  = [...]uint8{0, 6}
	_SQSSetQueueAttributeIndex_5  = [...]uint8{0, 29}
	_SQSSetQueueAttributeIndex_6  = [...]uint8{0, 13}
	_SQSSetQueueAttributeIndex_7  = [...]uint8{0, 19}
	_SQSSetQueueAttributeIndex_8  = [...]uint8{0, 15}
	_SQSSetQueueAttributeIndex_9  = [...]uint8{0, 17}
	_SQSSetQueueAttributeIndex_10 = [...]uint8{0, 14}
	_SQSSetQueueAttributeIndex_11 = [...]uint8{0, 28}
	_SQSSetQueueAttributeIndex_12 = [...]uint8{0, 25}
)

func (i SQSSetQueueAttribute) String() string {
	switch {
	case i == UNKNOWN:
		return _SQSSetQueueAttributeName_0
	case i == DelaySeconds:
		return _SQSSetQueueAttributeName_1
	case i == MaximumMessageSize:
		return _SQSSetQueueAttributeName_2
	case i == MessageRetentionPeriod:
		return _SQSSetQueueAttributeName_3
	case i == Policy:
		return _SQSSetQueueAttributeName_4
	case i == ReceiveMessageWaitTimeSeconds:
		return _SQSSetQueueAttributeName_5
	case i == RedrivePolicy:
		return _SQSSetQueueAttributeName_6
	case i == DeadLetterTargetArn:
		return _SQSSetQueueAttributeName_7
	case i == MaxReceiveCount:
		return _SQSSetQueueAttributeName_8
	case i == VisibilityTimeout:
		return _SQSSetQueueAttributeName_9
	case i == KmsMasterKeyId:
		return _SQSSetQueueAttributeName_10
	case i == KmsDataKeyReusePeriodSeconds:
		return _SQSSetQueueAttributeName_11
	case i == ContentBasedDeduplication:
		return _SQSSetQueueAttributeName_12
	default:
		return ""
	}
}

var _SQSSetQueueAttributeValues = []SQSSetQueueAttribute{
	0,  // UNKNOWN
	1,  // DelaySeconds
	2,  // MaximumMessageSize
	3,  // MessageRetentionPeriod
	4,  // Policy
	5,  // ReceiveMessageWaitTimeSeconds
	6,  // RedrivePolicy
	7,  // DeadLetterTargetArn
	8,  // MaxReceiveCount
	9,  // VisibilityTimeout
	10, // KmsMasterKeyId
	11, // KmsDataKeyReusePeriodSeconds
	13, // ContentBasedDeduplication
}

var _SQSSetQueueAttributeNameToValueMap = map[string]SQSSetQueueAttribute{
	_SQSSetQueueAttributeName_0[0:7]:   0,  // UNKNOWN
	_SQSSetQueueAttributeName_1[0:12]:  1,  // DelaySeconds
	_SQSSetQueueAttributeName_2[0:18]:  2,  // MaximumMessageSize
	_SQSSetQueueAttributeName_3[0:22]:  3,  // MessageRetentionPeriod
	_SQSSetQueueAttributeName_4[0:6]:   4,  // Policy
	_SQSSetQueueAttributeName_5[0:29]:  5,  // ReceiveMessageWaitTimeSeconds
	_SQSSetQueueAttributeName_6[0:13]:  6,  // RedrivePolicy
	_SQSSetQueueAttributeName_7[0:19]:  7,  // DeadLetterTargetArn
	_SQSSetQueueAttributeName_8[0:15]:  8,  // MaxReceiveCount
	_SQSSetQueueAttributeName_9[0:17]:  9,  // VisibilityTimeout
	_SQSSetQueueAttributeName_10[0:14]: 10, // KmsMasterKeyId
	_SQSSetQueueAttributeName_11[0:28]: 11, // KmsDataKeyReusePeriodSeconds
	_SQSSetQueueAttributeName_12[0:25]: 13, // ContentBasedDeduplication
}

var _SQSSetQueueAttributeValueToKeyMap = map[SQSSetQueueAttribute]string{
	0:  _SQSSetQueueAttributeKey_0,  // UNKNOWN
	1:  _SQSSetQueueAttributeKey_1,  // DelaySeconds
	2:  _SQSSetQueueAttributeKey_2,  // MaximumMessageSize
	3:  _SQSSetQueueAttributeKey_3,  // MessageRetentionPeriod
	4:  _SQSSetQueueAttributeKey_4,  // Policy
	5:  _SQSSetQueueAttributeKey_5,  // ReceiveMessageWaitTimeSeconds
	6:  _SQSSetQueueAttributeKey_6,  // RedrivePolicy
	7:  _SQSSetQueueAttributeKey_7,  // DeadLetterTargetArn
	8:  _SQSSetQueueAttributeKey_8,  // MaxReceiveCount
	9:  _SQSSetQueueAttributeKey_9,  // VisibilityTimeout
	10: _SQSSetQueueAttributeKey_10, // KmsMasterKeyId
	11: _SQSSetQueueAttributeKey_11, // KmsDataKeyReusePeriodSeconds
	13: _SQSSetQueueAttributeKey_12, // ContentBasedDeduplication
}

var _SQSSetQueueAttributeValueToCaptionMap = map[SQSSetQueueAttribute]string{
	0:  _SQSSetQueueAttributeCaption_0,  // UNKNOWN
	1:  _SQSSetQueueAttributeCaption_1,  // DelaySeconds
	2:  _SQSSetQueueAttributeCaption_2,  // MaximumMessageSize
	3:  _SQSSetQueueAttributeCaption_3,  // MessageRetentionPeriod
	4:  _SQSSetQueueAttributeCaption_4,  // Policy
	5:  _SQSSetQueueAttributeCaption_5,  // ReceiveMessageWaitTimeSeconds
	6:  _SQSSetQueueAttributeCaption_6,  // RedrivePolicy
	7:  _SQSSetQueueAttributeCaption_7,  // DeadLetterTargetArn
	8:  _SQSSetQueueAttributeCaption_8,  // MaxReceiveCount
	9:  _SQSSetQueueAttributeCaption_9,  // VisibilityTimeout
	10: _SQSSetQueueAttributeCaption_10, // KmsMasterKeyId
	11: _SQSSetQueueAttributeCaption_11, // KmsDataKeyReusePeriodSeconds
	13: _SQSSetQueueAttributeCaption_12, // ContentBasedDeduplication
}

var _SQSSetQueueAttributeValueToDescriptionMap = map[SQSSetQueueAttribute]string{
	0:  _SQSSetQueueAttributeDescription_0,  // UNKNOWN
	1:  _SQSSetQueueAttributeDescription_1,  // DelaySeconds
	2:  _SQSSetQueueAttributeDescription_2,  // MaximumMessageSize
	3:  _SQSSetQueueAttributeDescription_3,  // MessageRetentionPeriod
	4:  _SQSSetQueueAttributeDescription_4,  // Policy
	5:  _SQSSetQueueAttributeDescription_5,  // ReceiveMessageWaitTimeSeconds
	6:  _SQSSetQueueAttributeDescription_6,  // RedrivePolicy
	7:  _SQSSetQueueAttributeDescription_7,  // DeadLetterTargetArn
	8:  _SQSSetQueueAttributeDescription_8,  // MaxReceiveCount
	9:  _SQSSetQueueAttributeDescription_9,  // VisibilityTimeout
	10: _SQSSetQueueAttributeDescription_10, // KmsMasterKeyId
	11: _SQSSetQueueAttributeDescription_11, // KmsDataKeyReusePeriodSeconds
	13: _SQSSetQueueAttributeDescription_12, // ContentBasedDeduplication
}

// Valid returns 'true' if the value is listed in the SQSSetQueueAttribute enum map definition, 'false' otherwise
func (i SQSSetQueueAttribute) Valid() bool {
	for _, v := range _SQSSetQueueAttributeValues {
		if i == v {
			return true
		}
	}

	return false
}

// ParseByName retrieves a SQSSetQueueAttribute enum value from the enum string name,
// throws an error if the param is not part of the enum
func (i SQSSetQueueAttribute) ParseByName(s string) (SQSSetQueueAttribute, error) {
	if val, ok := _SQSSetQueueAttributeNameToValueMap[s]; ok {
		// parse ok
		return val, nil
	}

	// error
	return -1, fmt.Errorf("Enum Name of %s Not Expected In SQSSetQueueAttribute Values List", s)
}

// ParseByKey retrieves a SQSSetQueueAttribute enum value from the enum string key,
// throws an error if the param is not part of the enum
func (i SQSSetQueueAttribute) ParseByKey(s string) (SQSSetQueueAttribute, error) {
	for k, v := range _SQSSetQueueAttributeValueToKeyMap {
		if v == s {
			// parse ok
			return k, nil
		}
	}

	// error
	return -1, fmt.Errorf("Enum Key of %s Not Expected In SQSSetQueueAttribute Keys List", s)
}

// Key retrieves a SQSSetQueueAttribute enum string key
func (i SQSSetQueueAttribute) Key() string {
	if val, ok := _SQSSetQueueAttributeValueToKeyMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Caption retrieves a SQSSetQueueAttribute enum string caption
func (i SQSSetQueueAttribute) Caption() string {
	if val, ok := _SQSSetQueueAttributeValueToCaptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Description retrieves a SQSSetQueueAttribute enum string description
func (i SQSSetQueueAttribute) Description() string {
	if val, ok := _SQSSetQueueAttributeValueToDescriptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// IntValue gets the intrinsic enum integer value
func (i SQSSetQueueAttribute) IntValue() int {
	return int(i)
}

// IntString gets the intrinsic enum integer value represented in string format
func (i SQSSetQueueAttribute) IntString() string {
	return strconv.Itoa(int(i))
}

// ValueSlice returns all values of the enum SQSSetQueueAttribute in a slice
func (i SQSSetQueueAttribute) ValueSlice() []SQSSetQueueAttribute {
	return _SQSSetQueueAttributeValues
}

// NameMap returns all names of the enum SQSSetQueueAttribute in a K:name,V:SQSSetQueueAttribute map
func (i SQSSetQueueAttribute) NameMap() map[string]SQSSetQueueAttribute {
	return _SQSSetQueueAttributeNameToValueMap
}

// KeyMap returns all keys of the enum SQSSetQueueAttribute in a K:SQSSetQueueAttribute,V:key map
func (i SQSSetQueueAttribute) KeyMap() map[SQSSetQueueAttribute]string {
	return _SQSSetQueueAttributeValueToKeyMap
}

// CaptionMap returns all captions of the enum SQSSetQueueAttribute in a K:SQSSetQueueAttribute,V:caption map
func (i SQSSetQueueAttribute) CaptionMap() map[SQSSetQueueAttribute]string {
	return _SQSSetQueueAttributeValueToCaptionMap
}

// DescriptionMap returns all descriptions of the enum SQSSetQueueAttribute in a K:SQSSetQueueAttribute,V:description map
func (i SQSSetQueueAttribute) DescriptionMap() map[SQSSetQueueAttribute]string {
	return _SQSSetQueueAttributeValueToDescriptionMap
}
