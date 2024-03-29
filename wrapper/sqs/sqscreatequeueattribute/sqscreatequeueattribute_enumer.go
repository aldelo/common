// Code Generated By gen-enumer For "Enum Type: SQSCreateQueueAttribute" - DO NOT EDIT;

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

package sqscreatequeueattribute

import (
	"fmt"
	"strconv"
)

// enum names constants
const (
	_SQSCreateQueueAttributeName_0  = "UNKNOWN"
	_SQSCreateQueueAttributeName_1  = "DelaySeconds"
	_SQSCreateQueueAttributeName_2  = "MaximumMessageSize"
	_SQSCreateQueueAttributeName_3  = "MessageRetentionPeriod"
	_SQSCreateQueueAttributeName_4  = "Policy"
	_SQSCreateQueueAttributeName_5  = "ReceiveMessageWaitTimeSeconds"
	_SQSCreateQueueAttributeName_6  = "RedrivePolicy"
	_SQSCreateQueueAttributeName_7  = "DeadLetterTargetArn"
	_SQSCreateQueueAttributeName_8  = "MaxReceiveCount"
	_SQSCreateQueueAttributeName_9  = "VisibilityTimeout"
	_SQSCreateQueueAttributeName_10 = "KmsMasterKeyId"
	_SQSCreateQueueAttributeName_11 = "KmsDataKeyReusePeriodSeconds"
	_SQSCreateQueueAttributeName_12 = "FifoQueue"
	_SQSCreateQueueAttributeName_13 = "ContentBasedDeduplication"
)

// var declares of enum indexes
var (
	_SQSCreateQueueAttributeIndex_0  = [...]uint8{0, 7}
	_SQSCreateQueueAttributeIndex_1  = [...]uint8{0, 12}
	_SQSCreateQueueAttributeIndex_2  = [...]uint8{0, 18}
	_SQSCreateQueueAttributeIndex_3  = [...]uint8{0, 22}
	_SQSCreateQueueAttributeIndex_4  = [...]uint8{0, 6}
	_SQSCreateQueueAttributeIndex_5  = [...]uint8{0, 29}
	_SQSCreateQueueAttributeIndex_6  = [...]uint8{0, 13}
	_SQSCreateQueueAttributeIndex_7  = [...]uint8{0, 19}
	_SQSCreateQueueAttributeIndex_8  = [...]uint8{0, 15}
	_SQSCreateQueueAttributeIndex_9  = [...]uint8{0, 17}
	_SQSCreateQueueAttributeIndex_10 = [...]uint8{0, 14}
	_SQSCreateQueueAttributeIndex_11 = [...]uint8{0, 28}
	_SQSCreateQueueAttributeIndex_12 = [...]uint8{0, 9}
	_SQSCreateQueueAttributeIndex_13 = [...]uint8{0, 25}
)

func (i SQSCreateQueueAttribute) String() string {
	switch {
	case i == UNKNOWN:
		return _SQSCreateQueueAttributeName_0
	case i == DelaySeconds:
		return _SQSCreateQueueAttributeName_1
	case i == MaximumMessageSize:
		return _SQSCreateQueueAttributeName_2
	case i == MessageRetentionPeriod:
		return _SQSCreateQueueAttributeName_3
	case i == Policy:
		return _SQSCreateQueueAttributeName_4
	case i == ReceiveMessageWaitTimeSeconds:
		return _SQSCreateQueueAttributeName_5
	case i == RedrivePolicy:
		return _SQSCreateQueueAttributeName_6
	case i == DeadLetterTargetArn:
		return _SQSCreateQueueAttributeName_7
	case i == MaxReceiveCount:
		return _SQSCreateQueueAttributeName_8
	case i == VisibilityTimeout:
		return _SQSCreateQueueAttributeName_9
	case i == KmsMasterKeyId:
		return _SQSCreateQueueAttributeName_10
	case i == KmsDataKeyReusePeriodSeconds:
		return _SQSCreateQueueAttributeName_11
	case i == FifoQueue:
		return _SQSCreateQueueAttributeName_12
	case i == ContentBasedDeduplication:
		return _SQSCreateQueueAttributeName_13
	default:
		return ""
	}
}

var _SQSCreateQueueAttributeValues = []SQSCreateQueueAttribute{
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
	12, // FifoQueue
	13, // ContentBasedDeduplication
}

var _SQSCreateQueueAttributeNameToValueMap = map[string]SQSCreateQueueAttribute{
	_SQSCreateQueueAttributeName_0[0:7]:   0,  // UNKNOWN
	_SQSCreateQueueAttributeName_1[0:12]:  1,  // DelaySeconds
	_SQSCreateQueueAttributeName_2[0:18]:  2,  // MaximumMessageSize
	_SQSCreateQueueAttributeName_3[0:22]:  3,  // MessageRetentionPeriod
	_SQSCreateQueueAttributeName_4[0:6]:   4,  // Policy
	_SQSCreateQueueAttributeName_5[0:29]:  5,  // ReceiveMessageWaitTimeSeconds
	_SQSCreateQueueAttributeName_6[0:13]:  6,  // RedrivePolicy
	_SQSCreateQueueAttributeName_7[0:19]:  7,  // DeadLetterTargetArn
	_SQSCreateQueueAttributeName_8[0:15]:  8,  // MaxReceiveCount
	_SQSCreateQueueAttributeName_9[0:17]:  9,  // VisibilityTimeout
	_SQSCreateQueueAttributeName_10[0:14]: 10, // KmsMasterKeyId
	_SQSCreateQueueAttributeName_11[0:28]: 11, // KmsDataKeyReusePeriodSeconds
	_SQSCreateQueueAttributeName_12[0:9]:  12, // FifoQueue
	_SQSCreateQueueAttributeName_13[0:25]: 13, // ContentBasedDeduplication
}

var _SQSCreateQueueAttributeValueToKeyMap = map[SQSCreateQueueAttribute]string{
	0:  _SQSCreateQueueAttributeKey_0,  // UNKNOWN
	1:  _SQSCreateQueueAttributeKey_1,  // DelaySeconds
	2:  _SQSCreateQueueAttributeKey_2,  // MaximumMessageSize
	3:  _SQSCreateQueueAttributeKey_3,  // MessageRetentionPeriod
	4:  _SQSCreateQueueAttributeKey_4,  // Policy
	5:  _SQSCreateQueueAttributeKey_5,  // ReceiveMessageWaitTimeSeconds
	6:  _SQSCreateQueueAttributeKey_6,  // RedrivePolicy
	7:  _SQSCreateQueueAttributeKey_7,  // DeadLetterTargetArn
	8:  _SQSCreateQueueAttributeKey_8,  // MaxReceiveCount
	9:  _SQSCreateQueueAttributeKey_9,  // VisibilityTimeout
	10: _SQSCreateQueueAttributeKey_10, // KmsMasterKeyId
	11: _SQSCreateQueueAttributeKey_11, // KmsDataKeyReusePeriodSeconds
	12: _SQSCreateQueueAttributeKey_12, // FifoQueue
	13: _SQSCreateQueueAttributeKey_13, // ContentBasedDeduplication
}

var _SQSCreateQueueAttributeValueToCaptionMap = map[SQSCreateQueueAttribute]string{
	0:  _SQSCreateQueueAttributeCaption_0,  // UNKNOWN
	1:  _SQSCreateQueueAttributeCaption_1,  // DelaySeconds
	2:  _SQSCreateQueueAttributeCaption_2,  // MaximumMessageSize
	3:  _SQSCreateQueueAttributeCaption_3,  // MessageRetentionPeriod
	4:  _SQSCreateQueueAttributeCaption_4,  // Policy
	5:  _SQSCreateQueueAttributeCaption_5,  // ReceiveMessageWaitTimeSeconds
	6:  _SQSCreateQueueAttributeCaption_6,  // RedrivePolicy
	7:  _SQSCreateQueueAttributeCaption_7,  // DeadLetterTargetArn
	8:  _SQSCreateQueueAttributeCaption_8,  // MaxReceiveCount
	9:  _SQSCreateQueueAttributeCaption_9,  // VisibilityTimeout
	10: _SQSCreateQueueAttributeCaption_10, // KmsMasterKeyId
	11: _SQSCreateQueueAttributeCaption_11, // KmsDataKeyReusePeriodSeconds
	12: _SQSCreateQueueAttributeCaption_12, // FifoQueue
	13: _SQSCreateQueueAttributeCaption_13, // ContentBasedDeduplication
}

var _SQSCreateQueueAttributeValueToDescriptionMap = map[SQSCreateQueueAttribute]string{
	0:  _SQSCreateQueueAttributeDescription_0,  // UNKNOWN
	1:  _SQSCreateQueueAttributeDescription_1,  // DelaySeconds
	2:  _SQSCreateQueueAttributeDescription_2,  // MaximumMessageSize
	3:  _SQSCreateQueueAttributeDescription_3,  // MessageRetentionPeriod
	4:  _SQSCreateQueueAttributeDescription_4,  // Policy
	5:  _SQSCreateQueueAttributeDescription_5,  // ReceiveMessageWaitTimeSeconds
	6:  _SQSCreateQueueAttributeDescription_6,  // RedrivePolicy
	7:  _SQSCreateQueueAttributeDescription_7,  // DeadLetterTargetArn
	8:  _SQSCreateQueueAttributeDescription_8,  // MaxReceiveCount
	9:  _SQSCreateQueueAttributeDescription_9,  // VisibilityTimeout
	10: _SQSCreateQueueAttributeDescription_10, // KmsMasterKeyId
	11: _SQSCreateQueueAttributeDescription_11, // KmsDataKeyReusePeriodSeconds
	12: _SQSCreateQueueAttributeDescription_12, // FifoQueue
	13: _SQSCreateQueueAttributeDescription_13, // ContentBasedDeduplication
}

// Valid returns 'true' if the value is listed in the SQSCreateQueueAttribute enum map definition, 'false' otherwise
func (i SQSCreateQueueAttribute) Valid() bool {
	for _, v := range _SQSCreateQueueAttributeValues {
		if i == v {
			return true
		}
	}

	return false
}

// ParseByName retrieves a SQSCreateQueueAttribute enum value from the enum string name,
// throws an error if the param is not part of the enum
func (i SQSCreateQueueAttribute) ParseByName(s string) (SQSCreateQueueAttribute, error) {
	if val, ok := _SQSCreateQueueAttributeNameToValueMap[s]; ok {
		// parse ok
		return val, nil
	}

	// error
	return -1, fmt.Errorf("Enum Name of %s Not Expected In SQSCreateQueueAttribute Values List", s)
}

// ParseByKey retrieves a SQSCreateQueueAttribute enum value from the enum string key,
// throws an error if the param is not part of the enum
func (i SQSCreateQueueAttribute) ParseByKey(s string) (SQSCreateQueueAttribute, error) {
	for k, v := range _SQSCreateQueueAttributeValueToKeyMap {
		if v == s {
			// parse ok
			return k, nil
		}
	}

	// error
	return -1, fmt.Errorf("Enum Key of %s Not Expected In SQSCreateQueueAttribute Keys List", s)
}

// Key retrieves a SQSCreateQueueAttribute enum string key
func (i SQSCreateQueueAttribute) Key() string {
	if val, ok := _SQSCreateQueueAttributeValueToKeyMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Caption retrieves a SQSCreateQueueAttribute enum string caption
func (i SQSCreateQueueAttribute) Caption() string {
	if val, ok := _SQSCreateQueueAttributeValueToCaptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// Description retrieves a SQSCreateQueueAttribute enum string description
func (i SQSCreateQueueAttribute) Description() string {
	if val, ok := _SQSCreateQueueAttributeValueToDescriptionMap[i]; ok {
		// found
		return val
	} else {
		// not found
		return ""
	}
}

// IntValue gets the intrinsic enum integer value
func (i SQSCreateQueueAttribute) IntValue() int {
	return int(i)
}

// IntString gets the intrinsic enum integer value represented in string format
func (i SQSCreateQueueAttribute) IntString() string {
	return strconv.Itoa(int(i))
}

// ValueSlice returns all values of the enum SQSCreateQueueAttribute in a slice
func (i SQSCreateQueueAttribute) ValueSlice() []SQSCreateQueueAttribute {
	return _SQSCreateQueueAttributeValues
}

// NameMap returns all names of the enum SQSCreateQueueAttribute in a K:name,V:SQSCreateQueueAttribute map
func (i SQSCreateQueueAttribute) NameMap() map[string]SQSCreateQueueAttribute {
	return _SQSCreateQueueAttributeNameToValueMap
}

// KeyMap returns all keys of the enum SQSCreateQueueAttribute in a K:SQSCreateQueueAttribute,V:key map
func (i SQSCreateQueueAttribute) KeyMap() map[SQSCreateQueueAttribute]string {
	return _SQSCreateQueueAttributeValueToKeyMap
}

// CaptionMap returns all captions of the enum SQSCreateQueueAttribute in a K:SQSCreateQueueAttribute,V:caption map
func (i SQSCreateQueueAttribute) CaptionMap() map[SQSCreateQueueAttribute]string {
	return _SQSCreateQueueAttributeValueToCaptionMap
}

// DescriptionMap returns all descriptions of the enum SQSCreateQueueAttribute in a K:SQSCreateQueueAttribute,V:description map
func (i SQSCreateQueueAttribute) DescriptionMap() map[SQSCreateQueueAttribute]string {
	return _SQSCreateQueueAttributeValueToDescriptionMap
}
