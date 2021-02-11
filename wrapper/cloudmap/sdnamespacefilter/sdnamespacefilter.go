package sdnamespacefilter

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

// go:generate gen-enumer -type SdNamespaceFilter

type SdNamespaceFilter int

const (
	UNKNOWN             SdNamespaceFilter = 0
	PublicDnsNamespace  SdNamespaceFilter = 1
	PrivateDnsNamespace SdNamespaceFilter = 2
	Both                SdNamespaceFilter = 3
)

const (
	_SdNamespaceFilterKey_0 = "UNKNOWN"
	_SdNamespaceFilterKey_1 = "PublicDnsNamespace"
	_SdNamespaceFilterKey_2 = "PrivateDnsNamespace"
	_SdNamespaceFilterKey_3 = "Both"
)

const (
	_SdNamespaceFilterCaption_0 = "UNKNOWN"
	_SdNamespaceFilterCaption_1 = "PublicDnsNamespace"
	_SdNamespaceFilterCaption_2 = "PrivateDnsNamespace"
	_SdNamespaceFilterCaption_3 = "Both"
)

const (
	_SdNamespaceFilterDescription_0 = "UNKNOWN"
	_SdNamespaceFilterDescription_1 = "PublicDnsNamespace"
	_SdNamespaceFilterDescription_2 = "PrivateDnsNamespace"
	_SdNamespaceFilterDescription_3 = "Both"
)
