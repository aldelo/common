package snsprotocol

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

// go:generate gen-enumer -type SNSProtocol

type SNSProtocol int

const (
	UNKNOWN             SNSProtocol = 0
	Http                SNSProtocol = 1
	Https               SNSProtocol = 2
	Email               SNSProtocol = 3
	EmailJson           SNSProtocol = 4
	Sms                 SNSProtocol = 5
	Sqs                 SNSProtocol = 6
	ApplicationEndpoint SNSProtocol = 7
	Lambda              SNSProtocol = 8
)

const (
	_SNSProtocolKey_0 = "UNKNOWN"
	_SNSProtocolKey_1 = "http"
	_SNSProtocolKey_2 = "https"
	_SNSProtocolKey_3 = "email"
	_SNSProtocolKey_4 = "email-json"
	_SNSProtocolKey_5 = "sms"
	_SNSProtocolKey_6 = "sqs"
	_SNSProtocolKey_7 = "application"
	_SNSProtocolKey_8 = "lambda"
)

const (
	_SNSProtocolCaption_0 = "UNKNOWN"
	_SNSProtocolCaption_1 = "Http"
	_SNSProtocolCaption_2 = "Https"
	_SNSProtocolCaption_3 = "Email"
	_SNSProtocolCaption_4 = "EmailJson"
	_SNSProtocolCaption_5 = "Sms"
	_SNSProtocolCaption_6 = "Sqs"
	_SNSProtocolCaption_7 = "ApplicationEndpoint"
	_SNSProtocolCaption_8 = "Lambda"
)

const (
	_SNSProtocolDescription_0 = "UNKNOWN"
	_SNSProtocolDescription_1 = "Http"
	_SNSProtocolDescription_2 = "Https"
	_SNSProtocolDescription_3 = "Email"
	_SNSProtocolDescription_4 = "EmailJson"
	_SNSProtocolDescription_5 = "Sms"
	_SNSProtocolDescription_6 = "Sqs"
	_SNSProtocolDescription_7 = "ApplicationEndpoint"
	_SNSProtocolDescription_8 = "Lambda"
)
