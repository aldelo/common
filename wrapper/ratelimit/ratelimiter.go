package ratelimit

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

import (
	"go.uber.org/ratelimit"
	"time"
)

// RateLimiter struct wraps ratelimit package
//
// RateLimitPerson = 0 = unlimited = no rate limit
type RateLimiter struct {
	// configuration options
	RateLimitPerSecond     int
	InitializeWithoutSlack bool

	// rate limiter client
	limiterClient ratelimit.Limiter
}

// Init will setup the rate limit for use
func (r *RateLimiter) Init() {
	// validate
	if r.RateLimitPerSecond < 0 {
		r.RateLimitPerSecond = 0
	}

	if r.RateLimitPerSecond > 0 {
		// limited
		if !r.InitializeWithoutSlack {
			// with slack (allow initial spike consideration)
			r.limiterClient = ratelimit.New(r.RateLimitPerSecond)
		} else {
			// no slack (disallow initial spike consideration)
			r.limiterClient = ratelimit.New(r.RateLimitPerSecond, ratelimit.WithoutSlack)
		}
	} else {
		// unlimited
		r.limiterClient = ratelimit.NewUnlimited()
	}
}

// Take is called by each method needing rate limit applied,
// based on the rate limit per second setting, given amount of time is slept before process continues,
// for example, 1 second rate limit 100 = 10 milliseconds per call, this causes each call to Take() sleep for 10 milliseconds,
// if rate limit is unlimited, then no sleep delay will occur (thus no rate limit applied)
//
// in other words, each call to take blocks for certain amount of time per rate limit per second configured,
// call to Take() returns time.Time for the Take() that took place
func (r *RateLimiter) Take() time.Time {
	return r.limiterClient.Take()
}
