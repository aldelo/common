package snsgetsubscriptionattribute

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

// go:generate gen-enumer -type SNSGetSubscriptionAttribute

type SNSGetSubscriptionAttribute int

const (
	UNKNOWN                      SNSGetSubscriptionAttribute = 0
	ConfirmationWasAuthenticated SNSGetSubscriptionAttribute = 1
	DeliveryPolicy               SNSGetSubscriptionAttribute = 2
	EffectiveDeliveryPolicy      SNSGetSubscriptionAttribute = 3
	FilterPolicy                 SNSGetSubscriptionAttribute = 4
	Owner                        SNSGetSubscriptionAttribute = 5
	PendingConfirmation          SNSGetSubscriptionAttribute = 6
	RawMessageDelivery           SNSGetSubscriptionAttribute = 7
	RedrivePolicy                SNSGetSubscriptionAttribute = 8
	SubscriptionArn              SNSGetSubscriptionAttribute = 9
	TopicArn                     SNSGetSubscriptionAttribute = 10
)

const (
	_SNSGetSubscriptionAttributeKey_0  = "UNKNOWN"
	_SNSGetSubscriptionAttributeKey_1  = "ConfirmationWasAuthenticated"
	_SNSGetSubscriptionAttributeKey_2  = "DeliveryPolicy"
	_SNSGetSubscriptionAttributeKey_3  = "EffectiveDeliveryPolicy"
	_SNSGetSubscriptionAttributeKey_4  = "FilterPolicy"
	_SNSGetSubscriptionAttributeKey_5  = "Owner"
	_SNSGetSubscriptionAttributeKey_6  = "PendingConfirmation"
	_SNSGetSubscriptionAttributeKey_7  = "RawMessageDelivery"
	_SNSGetSubscriptionAttributeKey_8  = "RedrivePolicy"
	_SNSGetSubscriptionAttributeKey_9  = "SubscriptionArn"
	_SNSGetSubscriptionAttributeKey_10 = "TopicArn"
)

const (
	_SNSGetSubscriptionAttributeCaption_0  = "UNKNOWN"
	_SNSGetSubscriptionAttributeCaption_1  = "ConfirmationWasAuthenticated"
	_SNSGetSubscriptionAttributeCaption_2  = "DeliveryPolicy"
	_SNSGetSubscriptionAttributeCaption_3  = "EffectiveDeliveryPolicy"
	_SNSGetSubscriptionAttributeCaption_4  = "FilterPolicy"
	_SNSGetSubscriptionAttributeCaption_5  = "Owner"
	_SNSGetSubscriptionAttributeCaption_6  = "PendingConfirmation"
	_SNSGetSubscriptionAttributeCaption_7  = "RawMessageDelivery"
	_SNSGetSubscriptionAttributeCaption_8  = "RedrivePolicy"
	_SNSGetSubscriptionAttributeCaption_9  = "SubscriptionArn"
	_SNSGetSubscriptionAttributeCaption_10 = "TopicArn"
)

const (
	_SNSGetSubscriptionAttributeDescription_0  = "UNKNOWN"
	_SNSGetSubscriptionAttributeDescription_1  = "ConfirmationWasAuthenticated"
	_SNSGetSubscriptionAttributeDescription_2  = "DeliveryPolicy"
	_SNSGetSubscriptionAttributeDescription_3  = "EffectiveDeliveryPolicy"
	_SNSGetSubscriptionAttributeDescription_4  = "FilterPolicy"
	_SNSGetSubscriptionAttributeDescription_5  = "Owner"
	_SNSGetSubscriptionAttributeDescription_6  = "PendingConfirmation"
	_SNSGetSubscriptionAttributeDescription_7  = "RawMessageDelivery"
	_SNSGetSubscriptionAttributeDescription_8  = "RedrivePolicy"
	_SNSGetSubscriptionAttributeDescription_9  = "SubscriptionArn"
	_SNSGetSubscriptionAttributeDescription_10 = "TopicArn"
)
