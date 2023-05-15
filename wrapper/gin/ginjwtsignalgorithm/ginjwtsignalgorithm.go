package ginjwtsignalgorithm

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

// go:generate gen-enumer -type GinJwtSignAlgorithm

type GinJwtSignAlgorithm int

const (
	UNKNOWN GinJwtSignAlgorithm = 0
	HS256   GinJwtSignAlgorithm = 1
	HS384   GinJwtSignAlgorithm = 2
	HS512   GinJwtSignAlgorithm = 3
	RS256   GinJwtSignAlgorithm = 4
	RS384   GinJwtSignAlgorithm = 5
	RS512   GinJwtSignAlgorithm = 6
)

const (
	_GinJwtSignAlgorithmKey_0 = "UNKNOWN"
	_GinJwtSignAlgorithmKey_1 = "HS256"
	_GinJwtSignAlgorithmKey_2 = "HS384"
	_GinJwtSignAlgorithmKey_3 = "HS512"
	_GinJwtSignAlgorithmKey_4 = "RS256"
	_GinJwtSignAlgorithmKey_5 = "RS384"
	_GinJwtSignAlgorithmKey_6 = "RS512"
)

const (
	_GinJwtSignAlgorithmCaption_0 = "UNKNOWN"
	_GinJwtSignAlgorithmCaption_1 = "HS256"
	_GinJwtSignAlgorithmCaption_2 = "HS384"
	_GinJwtSignAlgorithmCaption_3 = "HS512"
	_GinJwtSignAlgorithmCaption_4 = "RS256"
	_GinJwtSignAlgorithmCaption_5 = "RS384"
	_GinJwtSignAlgorithmCaption_6 = "RS512"
)

const (
	_GinJwtSignAlgorithmDescription_0 = "UNKNOWN"
	_GinJwtSignAlgorithmDescription_1 = "HS256"
	_GinJwtSignAlgorithmDescription_2 = "HS384"
	_GinJwtSignAlgorithmDescription_3 = "HS512"
	_GinJwtSignAlgorithmDescription_4 = "RS256"
	_GinJwtSignAlgorithmDescription_5 = "RS384"
	_GinJwtSignAlgorithmDescription_6 = "RS512"
)
