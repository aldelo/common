package redisdatatype

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

// go:generate gen-enumer -type RedisDataType

type RedisDataType int

const (
	UNKNOWN RedisDataType = 0
	String  RedisDataType = 1
	Bool    RedisDataType = 2
	Int     RedisDataType = 3
	Int64   RedisDataType = 4
	Float64 RedisDataType = 5
	Bytes   RedisDataType = 6
	Json    RedisDataType = 7
	Time    RedisDataType = 8
)
