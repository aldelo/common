package sqscreatequeueattribute

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

// DO NOT EDIT - auto generated via tool

// go:generate gen-enumer -type SQSCreateQueueAttribute

type SQSCreateQueueAttribute int

const (
	UNKNOWN                       SQSCreateQueueAttribute = 0
	DelaySeconds                  SQSCreateQueueAttribute = 1
	MaximumMessageSize            SQSCreateQueueAttribute = 2
	MessageRetentionPeriod        SQSCreateQueueAttribute = 3
	Policy                        SQSCreateQueueAttribute = 4
	ReceiveMessageWaitTimeSeconds SQSCreateQueueAttribute = 5
	RedrivePolicy                 SQSCreateQueueAttribute = 6
	DeadLetterTargetArn           SQSCreateQueueAttribute = 7
	MaxReceiveCount               SQSCreateQueueAttribute = 8
	VisibilityTimeout             SQSCreateQueueAttribute = 9
	KmsMasterKeyId                SQSCreateQueueAttribute = 10
	KmsDataKeyReusePeriodSeconds  SQSCreateQueueAttribute = 11
	FifoQueue                     SQSCreateQueueAttribute = 12
	ContentBasedDeduplication     SQSCreateQueueAttribute = 13
)

const (
	_SQSCreateQueueAttributeKey_0  = "UNKNOWN"
	_SQSCreateQueueAttributeKey_1  = "DelaySeconds"
	_SQSCreateQueueAttributeKey_2  = "MaximumMessageSize"
	_SQSCreateQueueAttributeKey_3  = "MessageRetentionPeriod"
	_SQSCreateQueueAttributeKey_4  = "Policy"
	_SQSCreateQueueAttributeKey_5  = "ReceiveMessageWaitTimeSeconds"
	_SQSCreateQueueAttributeKey_6  = "RedrivePolicy"
	_SQSCreateQueueAttributeKey_7  = "deadLetterTargetArn"
	_SQSCreateQueueAttributeKey_8  = "maxReceiveCount"
	_SQSCreateQueueAttributeKey_9  = "VisibilityTimeout"
	_SQSCreateQueueAttributeKey_10 = "KmsMasterKeyId"
	_SQSCreateQueueAttributeKey_11 = "KmsDataKeyReusePeriodSeconds"
	_SQSCreateQueueAttributeKey_12 = "FifoQueue"
	_SQSCreateQueueAttributeKey_13 = "ContentBasedDeduplication"
)

const (
	_SQSCreateQueueAttributeCaption_0  = "UNKNOWN"
	_SQSCreateQueueAttributeCaption_1  = "DelaySeconds"
	_SQSCreateQueueAttributeCaption_2  = "MaximumMessageSize"
	_SQSCreateQueueAttributeCaption_3  = "MessageRetentionPeriod"
	_SQSCreateQueueAttributeCaption_4  = "Policy"
	_SQSCreateQueueAttributeCaption_5  = "ReceiveMessageWaitTimeSeconds"
	_SQSCreateQueueAttributeCaption_6  = "RedrivePolicy"
	_SQSCreateQueueAttributeCaption_7  = "DeadLetterTargetArn"
	_SQSCreateQueueAttributeCaption_8  = "MaxReceiveCount"
	_SQSCreateQueueAttributeCaption_9  = "VisibilityTimeout"
	_SQSCreateQueueAttributeCaption_10 = "KmsMasterKeyId"
	_SQSCreateQueueAttributeCaption_11 = "KmsDataKeyReusePeriodSeconds"
	_SQSCreateQueueAttributeCaption_12 = "FifoQueue"
	_SQSCreateQueueAttributeCaption_13 = "ContentBasedDeduplication"
)

const (
	_SQSCreateQueueAttributeDescription_0  = "UNKNOWN"
	_SQSCreateQueueAttributeDescription_1  = "DelaySeconds"
	_SQSCreateQueueAttributeDescription_2  = "MaximumMessageSize"
	_SQSCreateQueueAttributeDescription_3  = "MessageRetentionPeriod"
	_SQSCreateQueueAttributeDescription_4  = "Policy"
	_SQSCreateQueueAttributeDescription_5  = "ReceiveMessageWaitTimeSeconds"
	_SQSCreateQueueAttributeDescription_6  = "RedrivePolicy"
	_SQSCreateQueueAttributeDescription_7  = "DeadLetterTargetArn"
	_SQSCreateQueueAttributeDescription_8  = "MaxReceiveCount"
	_SQSCreateQueueAttributeDescription_9  = "VisibilityTimeout"
	_SQSCreateQueueAttributeDescription_10 = "KmsMasterKeyId"
	_SQSCreateQueueAttributeDescription_11 = "KmsDataKeyReusePeriodSeconds"
	_SQSCreateQueueAttributeDescription_12 = "FifoQueue"
	_SQSCreateQueueAttributeDescription_13 = "ContentBasedDeduplication"
)
