package redisradiusunit

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

// go:generate gen-enumer -type RedisRadiusUnit

type RedisRadiusUnit int

const (
	UNKNOWN    RedisRadiusUnit = 0
	Meters     RedisRadiusUnit = 1
	Kilometers RedisRadiusUnit = 2
	Miles      RedisRadiusUnit = 3
	Feet       RedisRadiusUnit = 4
)

const (
	_RedisRadiusUnitKey_0 = "UNKNOWN"
	_RedisRadiusUnitKey_1 = "m"
	_RedisRadiusUnitKey_2 = "km"
	_RedisRadiusUnitKey_3 = "mi"
	_RedisRadiusUnitKey_4 = "ft"
)
