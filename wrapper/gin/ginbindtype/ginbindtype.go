package ginbindtype

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

//go:generate gen-enumer -type GinBindType

type GinBindType int

const (
	UNKNOWN      GinBindType = 0
	BindHeader   GinBindType = 1
	BindJson     GinBindType = 2
	BindQuery    GinBindType = 3
	BindUri      GinBindType = 4
	BindXml      GinBindType = 5
	BindYaml     GinBindType = 6
	BindProtoBuf GinBindType = 7
	BindPostForm GinBindType = 8
)

const (
	_GinBindTypeKey_0 = "UNKNOWN"
	_GinBindTypeKey_1 = "BindHeader"
	_GinBindTypeKey_2 = "BindJson"
	_GinBindTypeKey_3 = "BindQuery"
	_GinBindTypeKey_4 = "BindUri"
	_GinBindTypeKey_5 = "BindXml"
	_GinBindTypeKey_6 = "BindYaml"
	_GinBindTypeKey_7 = "BindProtoBuf"
	_GinBindTypeKey_8 = "BindPostForm"
)

const (
	_GinBindTypeCaption_0 = "UNKNOWN"
	_GinBindTypeCaption_1 = "BindHeader"
	_GinBindTypeCaption_2 = "BindJson"
	_GinBindTypeCaption_3 = "BindQuery"
	_GinBindTypeCaption_4 = "BindUri"
	_GinBindTypeCaption_5 = "BindXml"
	_GinBindTypeCaption_6 = "BindYaml"
	_GinBindTypeCaption_7 = "BindProtoBuf"
	_GinBindTypeCaption_8 = "BindPostForm"
)

const (
	_GinBindTypeDescription_0 = "UNKNOWN"
	_GinBindTypeDescription_1 = "BindHeader"
	_GinBindTypeDescription_2 = "BindJson"
	_GinBindTypeDescription_3 = "BindQuery"
	_GinBindTypeDescription_4 = "BindUri"
	_GinBindTypeDescription_5 = "BindXml"
	_GinBindTypeDescription_6 = "BindYaml"
	_GinBindTypeDescription_7 = "BindProtoBuf"
	_GinBindTypeDescription_8 = "BindPostForm"
)
