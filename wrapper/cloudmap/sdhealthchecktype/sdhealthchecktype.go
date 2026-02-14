package sdhealthchecktype

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

// go:generate gen-enumer -type SdHealthCheckType

type SdHealthCheckType int

const (
	UNKNOWN SdHealthCheckType = 0
	HTTP    SdHealthCheckType = 1
	HTTPS   SdHealthCheckType = 2
	TCP     SdHealthCheckType = 3
)

const (
	_SdHealthCheckTypeKey_0 = "UNKNOWN"
	_SdHealthCheckTypeKey_1 = "HTTP"
	_SdHealthCheckTypeKey_2 = "HTTPS"
	_SdHealthCheckTypeKey_3 = "TCP"
)

const (
	_SdHealthCheckTypeCaption_0 = "UNKNOWN"
	_SdHealthCheckTypeCaption_1 = "HTTP"
	_SdHealthCheckTypeCaption_2 = "HTTPS"
	_SdHealthCheckTypeCaption_3 = "TCP"
)

const (
	_SdHealthCheckTypeDescription_0 = "UNKNOWN"
	_SdHealthCheckTypeDescription_1 = "HTTP"
	_SdHealthCheckTypeDescription_2 = "HTTPS"
	_SdHealthCheckTypeDescription_3 = "TCP"
)
