package snsgettopicattribute

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

// go:generate gen-enumer -type SNSGetTopicAttribute

type SNSGetTopicAttribute int

const (
	UNKNOWN                 SNSGetTopicAttribute = 0
	DeliveryPolicy          SNSGetTopicAttribute = 1
	DisplayName             SNSGetTopicAttribute = 2
	Owner                   SNSGetTopicAttribute = 3
	Policy                  SNSGetTopicAttribute = 4
	SubscriptionsConfirmed  SNSGetTopicAttribute = 5
	SubscriptionsDeleted    SNSGetTopicAttribute = 6
	SubscriptionsPending    SNSGetTopicAttribute = 7
	TopicArn                SNSGetTopicAttribute = 8
	EffectiveDeliveryPolicy SNSGetTopicAttribute = 9
	KmsMasterKeyId          SNSGetTopicAttribute = 10
)

const (
	_SNSGetTopicAttributeKey_0  = "UNKNOWN"
	_SNSGetTopicAttributeKey_1  = "DeliveryPolicy"
	_SNSGetTopicAttributeKey_2  = "DisplayName"
	_SNSGetTopicAttributeKey_3  = "Owner"
	_SNSGetTopicAttributeKey_4  = "Policy"
	_SNSGetTopicAttributeKey_5  = "SubscriptionsConfirmed"
	_SNSGetTopicAttributeKey_6  = "SubscriptionsDeleted"
	_SNSGetTopicAttributeKey_7  = "SubscriptionsPending"
	_SNSGetTopicAttributeKey_8  = "TopicArn"
	_SNSGetTopicAttributeKey_9  = "EffectiveDeliveryPolicy"
	_SNSGetTopicAttributeKey_10 = "KmsMasterKeyId"
)

const (
	_SNSGetTopicAttributeCaption_0  = "UNKNOWN"
	_SNSGetTopicAttributeCaption_1  = "DeliveryPolicy"
	_SNSGetTopicAttributeCaption_2  = "DisplayName"
	_SNSGetTopicAttributeCaption_3  = "Owner"
	_SNSGetTopicAttributeCaption_4  = "Policy"
	_SNSGetTopicAttributeCaption_5  = "SubscriptionsConfirmed"
	_SNSGetTopicAttributeCaption_6  = "SubscriptionsDeleted"
	_SNSGetTopicAttributeCaption_7  = "SubscriptionsPending"
	_SNSGetTopicAttributeCaption_8  = "TopicArn"
	_SNSGetTopicAttributeCaption_9  = "EffectiveDeliveryPolicy"
	_SNSGetTopicAttributeCaption_10 = "KmsMasterKeyId"
)

const (
	_SNSGetTopicAttributeDescription_0  = "UNKNOWN"
	_SNSGetTopicAttributeDescription_1  = "DeliveryPolicy"
	_SNSGetTopicAttributeDescription_2  = "DisplayName"
	_SNSGetTopicAttributeDescription_3  = "Owner"
	_SNSGetTopicAttributeDescription_4  = "Policy"
	_SNSGetTopicAttributeDescription_5  = "SubscriptionsConfirmed"
	_SNSGetTopicAttributeDescription_6  = "SubscriptionsDeleted"
	_SNSGetTopicAttributeDescription_7  = "SubscriptionsPending"
	_SNSGetTopicAttributeDescription_8  = "TopicArn"
	_SNSGetTopicAttributeDescription_9  = "EffectiveDeliveryPolicy"
	_SNSGetTopicAttributeDescription_10 = "KmsMasterKeyId"
)
