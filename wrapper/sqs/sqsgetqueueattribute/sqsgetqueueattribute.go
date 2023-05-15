package sqsgetqueueattribute

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

// DO NOT EDIT - auto generated via tool

// go:generate gen-enumer -type SQSGetQueueAttribute

type SQSGetQueueAttribute int

const (
	UNKNOWN                               SQSGetQueueAttribute = 0
	All                                   SQSGetQueueAttribute = 1
	ApproximateNumberOfMessages           SQSGetQueueAttribute = 2
	ApproximateNumberOfMessagesDelayed    SQSGetQueueAttribute = 3
	ApproximateNumberOfMessagesNotVisible SQSGetQueueAttribute = 4
	CreatedTimestamp                      SQSGetQueueAttribute = 5
	DelaySeconds                          SQSGetQueueAttribute = 6
	LastModifiedTimestamp                 SQSGetQueueAttribute = 7
	MaximumMessageSize                    SQSGetQueueAttribute = 8
	MessageRetentionPeriod                SQSGetQueueAttribute = 9
	Policy                                SQSGetQueueAttribute = 10
	QueueArn                              SQSGetQueueAttribute = 11
	ReceiveMessageWaitTimeSeconds         SQSGetQueueAttribute = 12
	RedrivePolicy                         SQSGetQueueAttribute = 13
	DeadLetterTargetArn                   SQSGetQueueAttribute = 14
	MaxReceiveCount                       SQSGetQueueAttribute = 15
	VisibilityTimeout                     SQSGetQueueAttribute = 16
	KmsMasterKeyId                        SQSGetQueueAttribute = 17
	KmsDataKeyReusePeriodSeconds          SQSGetQueueAttribute = 18
	FifoQueue                             SQSGetQueueAttribute = 19
	ContentBasedDeduplication             SQSGetQueueAttribute = 20
)

const (
	_SQSGetQueueAttributeKey_0  = "UNKNOWN"
	_SQSGetQueueAttributeKey_1  = "All"
	_SQSGetQueueAttributeKey_2  = "ApproximateNumberOfMessages"
	_SQSGetQueueAttributeKey_3  = "ApproximateNumberOfMessagesDelayed"
	_SQSGetQueueAttributeKey_4  = "ApproximateNumberOfMessagesNotVisible"
	_SQSGetQueueAttributeKey_5  = "CreatedTimestamp"
	_SQSGetQueueAttributeKey_6  = "DelaySeconds"
	_SQSGetQueueAttributeKey_7  = "LastModifiedTimestamp"
	_SQSGetQueueAttributeKey_8  = "MaximumMessageSize"
	_SQSGetQueueAttributeKey_9  = "MessageRetentionPeriod"
	_SQSGetQueueAttributeKey_10 = "Policy"
	_SQSGetQueueAttributeKey_11 = "QueueArn"
	_SQSGetQueueAttributeKey_12 = "ReceiveMessageWaitTimeSeconds"
	_SQSGetQueueAttributeKey_13 = "RedrivePolicy"
	_SQSGetQueueAttributeKey_14 = "deadLetterTargetArn"
	_SQSGetQueueAttributeKey_15 = "maxReceiveCount"
	_SQSGetQueueAttributeKey_16 = "VisibilityTimeout"
	_SQSGetQueueAttributeKey_17 = "KmsMasterKeyId"
	_SQSGetQueueAttributeKey_18 = "KmsDataKeyReusePeriodSeconds"
	_SQSGetQueueAttributeKey_19 = "FifoQueue"
	_SQSGetQueueAttributeKey_20 = "ContentBasedDeduplication"
)

const (
	_SQSGetQueueAttributeCaption_0  = "UNKNOWN"
	_SQSGetQueueAttributeCaption_1  = "All"
	_SQSGetQueueAttributeCaption_2  = "ApproximateNumberOfMessages"
	_SQSGetQueueAttributeCaption_3  = "ApproximateNumberOfMessagesDelayed"
	_SQSGetQueueAttributeCaption_4  = "ApproximateNumberOfMessagesNotVisible"
	_SQSGetQueueAttributeCaption_5  = "CreatedTimestamp"
	_SQSGetQueueAttributeCaption_6  = "DelaySeconds"
	_SQSGetQueueAttributeCaption_7  = "LastModifiedTimestamp"
	_SQSGetQueueAttributeCaption_8  = "MaximumMessageSize"
	_SQSGetQueueAttributeCaption_9  = "MessageRetentionPeriod"
	_SQSGetQueueAttributeCaption_10 = "Policy"
	_SQSGetQueueAttributeCaption_11 = "QueueArn"
	_SQSGetQueueAttributeCaption_12 = "ReceiveMessageWaitTimeSeconds"
	_SQSGetQueueAttributeCaption_13 = "RedrivePolicy"
	_SQSGetQueueAttributeCaption_14 = "DeadLetterTargetArn"
	_SQSGetQueueAttributeCaption_15 = "MaxReceiveCount"
	_SQSGetQueueAttributeCaption_16 = "VisibilityTimeout"
	_SQSGetQueueAttributeCaption_17 = "KmsMasterKeyId"
	_SQSGetQueueAttributeCaption_18 = "KmsDataKeyReusePeriodSeconds"
	_SQSGetQueueAttributeCaption_19 = "FifoQueue"
	_SQSGetQueueAttributeCaption_20 = "ContentBasedDeduplication"
)

const (
	_SQSGetQueueAttributeDescription_0  = "UNKNOWN"
	_SQSGetQueueAttributeDescription_1  = "All"
	_SQSGetQueueAttributeDescription_2  = "ApproximateNumberOfMessages"
	_SQSGetQueueAttributeDescription_3  = "ApproximateNumberOfMessagesDelayed"
	_SQSGetQueueAttributeDescription_4  = "ApproximateNumberOfMessagesNotVisible"
	_SQSGetQueueAttributeDescription_5  = "CreatedTimestamp"
	_SQSGetQueueAttributeDescription_6  = "DelaySeconds"
	_SQSGetQueueAttributeDescription_7  = "LastModifiedTimestamp"
	_SQSGetQueueAttributeDescription_8  = "MaximumMessageSize"
	_SQSGetQueueAttributeDescription_9  = "MessageRetentionPeriod"
	_SQSGetQueueAttributeDescription_10 = "Policy"
	_SQSGetQueueAttributeDescription_11 = "QueueArn"
	_SQSGetQueueAttributeDescription_12 = "ReceiveMessageWaitTimeSeconds"
	_SQSGetQueueAttributeDescription_13 = "RedrivePolicy"
	_SQSGetQueueAttributeDescription_14 = "DeadLetterTargetArn"
	_SQSGetQueueAttributeDescription_15 = "MaxReceiveCount"
	_SQSGetQueueAttributeDescription_16 = "VisibilityTimeout"
	_SQSGetQueueAttributeDescription_17 = "KmsMasterKeyId"
	_SQSGetQueueAttributeDescription_18 = "KmsDataKeyReusePeriodSeconds"
	_SQSGetQueueAttributeDescription_19 = "FifoQueue"
	_SQSGetQueueAttributeDescription_20 = "ContentBasedDeduplication"
)
