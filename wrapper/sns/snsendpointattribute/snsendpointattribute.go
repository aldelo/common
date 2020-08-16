package snsendpointattribute

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

// go:generate gen-enumer -type SNSEndpointAttribute

type SNSEndpointAttribute int

const (
	UNKNOWN        SNSEndpointAttribute = 0
	CustomUserData SNSEndpointAttribute = 1
	Enabled        SNSEndpointAttribute = 2
	Token          SNSEndpointAttribute = 3
)

const (
	_SNSEndpointAttributeKey_0 = "UNKNOWN"
	_SNSEndpointAttributeKey_1 = "CustomUserData"
	_SNSEndpointAttributeKey_2 = "Enabled"
	_SNSEndpointAttributeKey_3 = "Token"
)

const (
	_SNSEndpointAttributeCaption_0 = "UNKNOWN"
	_SNSEndpointAttributeCaption_1 = "CustomUserData"
	_SNSEndpointAttributeCaption_2 = "Enabled"
	_SNSEndpointAttributeCaption_3 = "Token"
)

const (
	_SNSEndpointAttributeDescription_0 = "UNKNOWN"
	_SNSEndpointAttributeDescription_1 = "CustomUserData"
	_SNSEndpointAttributeDescription_2 = "Enabled"
	_SNSEndpointAttributeDescription_3 = "Token"
)
