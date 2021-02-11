package sqssetqueueattribute

/*
 * Copyright 2020-2021 Aldelo, LP
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

// go:generate gen-enumer -type SQSSetQueueAttribute

type SQSSetQueueAttribute int

const (
	UNKNOWN                       SQSSetQueueAttribute = 0
	DelaySeconds                  SQSSetQueueAttribute = 1
	MaximumMessageSize            SQSSetQueueAttribute = 2
	MessageRetentionPeriod        SQSSetQueueAttribute = 3
	Policy                        SQSSetQueueAttribute = 4
	ReceiveMessageWaitTimeSeconds SQSSetQueueAttribute = 5
	RedrivePolicy                 SQSSetQueueAttribute = 6
	DeadLetterTargetArn           SQSSetQueueAttribute = 7
	MaxReceiveCount               SQSSetQueueAttribute = 8
	VisibilityTimeout             SQSSetQueueAttribute = 9
	KmsMasterKeyId                SQSSetQueueAttribute = 10
	KmsDataKeyReusePeriodSeconds  SQSSetQueueAttribute = 11
	ContentBasedDeduplication     SQSSetQueueAttribute = 13
)

const (
	_SQSSetQueueAttributeKey_0  = "UNKNOWN"
	_SQSSetQueueAttributeKey_1  = "DelaySeconds"
	_SQSSetQueueAttributeKey_2  = "MaximumMessageSize"
	_SQSSetQueueAttributeKey_3  = "MessageRetentionPeriod"
	_SQSSetQueueAttributeKey_4  = "Policy"
	_SQSSetQueueAttributeKey_5  = "ReceiveMessageWaitTimeSeconds"
	_SQSSetQueueAttributeKey_6  = "RedrivePolicy"
	_SQSSetQueueAttributeKey_7  = "deadLetterTargetArn"
	_SQSSetQueueAttributeKey_8  = "maxReceiveCount"
	_SQSSetQueueAttributeKey_9  = "VisibilityTimeout"
	_SQSSetQueueAttributeKey_10 = "KmsMasterKeyId"
	_SQSSetQueueAttributeKey_11 = "KmsDataKeyReusePeriodSeconds"
	_SQSSetQueueAttributeKey_12 = "ContentBasedDeduplication"
)

const (
	_SQSSetQueueAttributeCaption_0  = "UNKNOWN"
	_SQSSetQueueAttributeCaption_1  = "DelaySeconds"
	_SQSSetQueueAttributeCaption_2  = "MaximumMessageSize"
	_SQSSetQueueAttributeCaption_3  = "MessageRetentionPeriod"
	_SQSSetQueueAttributeCaption_4  = "Policy"
	_SQSSetQueueAttributeCaption_5  = "ReceiveMessageWaitTimeSeconds"
	_SQSSetQueueAttributeCaption_6  = "RedrivePolicy"
	_SQSSetQueueAttributeCaption_7  = "DeadLetterTargetArn"
	_SQSSetQueueAttributeCaption_8  = "MaxReceiveCount"
	_SQSSetQueueAttributeCaption_9  = "VisibilityTimeout"
	_SQSSetQueueAttributeCaption_10 = "KmsMasterKeyId"
	_SQSSetQueueAttributeCaption_11 = "KmsDataKeyReusePeriodSeconds"
	_SQSSetQueueAttributeCaption_12 = "ContentBasedDeduplication"
)

const (
	_SQSSetQueueAttributeDescription_0  = "UNKNOWN"
	_SQSSetQueueAttributeDescription_1  = "DelaySeconds"
	_SQSSetQueueAttributeDescription_2  = "MaximumMessageSize"
	_SQSSetQueueAttributeDescription_3  = "MessageRetentionPeriod"
	_SQSSetQueueAttributeDescription_4  = "Policy"
	_SQSSetQueueAttributeDescription_5  = "ReceiveMessageWaitTimeSeconds"
	_SQSSetQueueAttributeDescription_6  = "RedrivePolicy"
	_SQSSetQueueAttributeDescription_7  = "DeadLetterTargetArn"
	_SQSSetQueueAttributeDescription_8  = "MaxReceiveCount"
	_SQSSetQueueAttributeDescription_9  = "VisibilityTimeout"
	_SQSSetQueueAttributeDescription_10 = "KmsMasterKeyId"
	_SQSSetQueueAttributeDescription_11 = "KmsDataKeyReusePeriodSeconds"
	_SQSSetQueueAttributeDescription_12 = "ContentBasedDeduplication"
)
