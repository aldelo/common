package redis

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

// =================================================================================================================
// AWS CREDENTIAL:
//		use $> aws configure (to set aws access key and secret to target machine)
//		Store AWS Access ID and Secret Key into Default Profile Using '$ aws configure' cli
//
// To Install & Setup AWS CLI on Host:
//		1) https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2-linux.html
//				On Ubuntu, if host does not have zip and unzip:
//					$> sudo apt install zip
//					$> sudo apt install unzip
//				On Ubuntu, to install AWS CLI v2:
//					$> curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
//					$> unzip awscliv2.zip
//					$> sudo ./aws/install
//		2) $> aws configure set region awsRegionName --profile default
// 		3) $> aws configure
//				follow prompts to enter Access ID and Secret Key
//
// AWS Region Name Reference:
//		us-west-2, us-east-1, ap-northeast-1, etc
//		See: https://docs.aws.amazon.com/general/latest/gr/rande.html
// =================================================================================================================

import (
	"context"
	"crypto/tls"
	"log"
	"strings"
	"sync"

	"errors"
	"time"

	"github.com/aldelo/common/wrapper/redis/redisbitop"
	"github.com/aldelo/common/wrapper/redis/redisdatatype"
	"github.com/aldelo/common/wrapper/redis/rediskeytype"
	"github.com/aldelo/common/wrapper/redis/redisradiusunit"
	"github.com/aldelo/common/wrapper/redis/redissetcondition"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/go-redis/redis/v8"

	util "github.com/aldelo/common"
)

// keysDeprecationOnce ensures the KEYS deprecation warning is logged at most once per process lifetime.
var keysDeprecationOnce sync.Once

// KeysDeprecationHook is a callback invoked the first time the deprecated Keys method is called.
// It defaults to logging a warning via the standard log package.
// Tests may replace this to capture the deprecation event without inspecting log output.
var KeysDeprecationHook func() = func() {
	log.Println("[WARN] Redis UTILS.Keys() is deprecated: the KEYS command performs an O(N) full keyspace scan " +
		"that blocks the Redis event loop and can cause latency spikes in production. " +
		"Use UTILS.ScanKeys() or UTILS.Scan() instead, which use the cursor-based SCAN command.")
}

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// Redis defines wrapper struct to handle redis interactions with AWS ElasticCache Redis service (using go-redis package)
//
// IMPORTANT
//
//	AWS ELASTICACHE REDIS lives within the VPC that the cluster is launched
//	Access is allowed ONLY WITHIN EC2 in the same VPC
//	There is no external public access since AWS Redis uses private IP only
//
// Dev Testing
//
//	Test On AWS EC2 (via SSH into EC2) since elastic cache redis is deployed within vpc access only
//
// Reference Info
//  1. Redis Commands Documentation = https://redis.io/commands
//  2. Go-Redis Documentation = https://pkg.go.dev/github.com/go-redis/redis?tab=doc
type Redis struct {
	// config fields
	AwsRedisWriterEndpoint string
	AwsRedisReaderEndpoint string

	// TLS is supported by Redis starting with version 6 as an optional feature
	EnableTLS bool

	// Connection pool configuration (optional, defaults will be used if not set)
	// PoolSize: maximum number of socket connections (default: 10 total connections)
	// MinIdleConns: minimum number of idle connections (default: 3)
	// ReadTimeout: timeout for read operations (default: 3 seconds)
	// WriteTimeout: timeout for write operations (default: 3 seconds)
	PoolSize     int
	MinIdleConns int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// client connection fields
	cnWriter   *redis.Client
	cnReader   *redis.Client
	cnAreReady bool

	// objects containing wrapped functions
	BIT        *BIT
	LIST       *LIST
	HASH       *HASH
	SET        *SET
	SORTED_SET *SORTED_SET
	GEO        *GEO
	STREAM     *STREAM
	PUBSUB     *PUBSUB
	PIPELINE   *PIPELINE
	TTL        *TTL
	UTILS      *UTILS

	_parentSegment *xray.XRayParentSegment

	connectMU sync.RWMutex
}

// connectionSnapshot captures the connection state under a read lock,
// so that callers can safely use cnWriter/cnReader without racing against Connect/Disconnect.
type redisConnSnapshot struct {
	ready         bool
	writer        *redis.Client
	reader        *redis.Client
	parentSegment *xray.XRayParentSegment
}

func (r *Redis) connSnapshot() redisConnSnapshot {
	r.connectMU.RLock()
	snap := redisConnSnapshot{
		ready:         r.cnAreReady,
		writer:        r.cnWriter,
		reader:        r.cnReader,
		parentSegment: r._parentSegment,
	}
	r.connectMU.RUnlock()
	return snap
}

// BIT defines redis BIT operations
type BIT struct {
	core *Redis
}

// SET defines redis SET operations
type SET struct {
	core *Redis
}

// SORTED_SET defines SORTED SET operations
type SORTED_SET struct {
	core *Redis
}

// LIST defines redis LIST operations
type LIST struct {
	core *Redis
}

// HASH defines redis HASH operations
type HASH struct {
	core *Redis
}

// GEO defines redis GEO operations
type GEO struct {
	core *Redis
}

// STREAM defines redis STREAM operations
type STREAM struct {
	core *Redis
}

// PUBSUB defines redis PUB SUB operations
type PUBSUB struct {
	core *Redis
}

// PIPELINE defines batched pipeline patterns
type PIPELINE struct {
	core *Redis
}

// TTL defines TTL and Persist related redis patterns
type TTL struct {
	core *Redis
}

// UTILS defines redis helper functions
type UTILS struct {
	core *Redis
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// connection functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish connection to aws elasticCache redis writer and reader endpoint connections.
func (r *Redis) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if r == nil {
		return errors.New("Redis Connect Failed: Redis receiver is nil")
	}
	r.connectMU.Lock()
	defer r.connectMU.Unlock()

	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			r._parentSegment = parentSegment[0]
		}

		seg := xray.NewSegment("Redis-Connect", r._parentSegment)
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Writer-Endpoint", r.AwsRedisWriterEndpoint))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Reader-Endpoint", r.AwsRedisReaderEndpoint))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = r.connectInternal()
		return err
	} else {
		return r.connectInternal()
	}
}

// connectInternal performs the actual redis writer and reader connection
func (r *Redis) connectInternal() error {
	r.cnAreReady = false

	// Sub-struct pointers (BIT, LIST, etc.) are NOT nilled here.
	// Concurrent callers may hold references to them without locks.
	// cnAreReady=false (set above) ensures all concurrent operations
	// fail safely via the !snap.ready check in connSnapshot().
	// Fresh sub-struct instances are created after successful connection below.

	if r.cnWriter != nil {
		_ = r.cnWriter.Close()
		r.cnWriter = nil
	}

	if r.cnReader != nil {
		_ = r.cnReader.Close()
		r.cnReader = nil
	}

	// validate
	if util.LenTrim(r.AwsRedisWriterEndpoint) <= 0 {
		// writer endpoint works against the primary node
		return errors.New("Connect To Redis Failed: " + "Writer Endpoint is Required")
	}

	if util.LenTrim(r.AwsRedisReaderEndpoint) <= 0 {
		// reader endpoint is cluster level that works against all shards for read only access
		return errors.New("Connect To Redis Failed: " + "Reader Endpoint is Required")
	}

	// establish new writer redis client
	// Apply configurable pool settings with sensible defaults
	poolSize := r.PoolSize
	if poolSize <= 0 {
		poolSize = 10 // default: 10 total connections
	}
	minIdleConns := r.MinIdleConns
	if minIdleConns <= 0 {
		minIdleConns = 3 // default: minimum 3 idle connections
	}
	readTimeout := r.ReadTimeout
	if readTimeout <= 0 {
		readTimeout = 3 * time.Second // default: 3 second read timeout
	}
	writeTimeout := r.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = 3 * time.Second // default: 3 second write timeout
	}

	optWriter := &redis.Options{
		Addr:         r.AwsRedisWriterEndpoint, // redis endpoint url and port
		Password:     "",                       // no password set
		DB:           0,                        // use default DB
		ReadTimeout:  readTimeout,              // time after read operation timeout
		WriteTimeout: writeTimeout,             // time after write operation timeout
		PoolSize:     poolSize,                 // maximum socket connections
		MinIdleConns: minIdleConns,             // minimum number of idle connections to keep
	}
	if r.EnableTLS {
		optWriter.TLSConfig = &tls.Config{InsecureSkipVerify: false}
	}
	r.cnWriter = redis.NewClient(optWriter)

	if r.cnWriter == nil {
		return errors.New("Connect To Redis Failed: (Writer Endpoint) " + "Obtain Client Yielded Nil")
	}

	// establish new reader redis client
	optReader := &redis.Options{
		Addr:         r.AwsRedisReaderEndpoint, // redis endpoint url and port
		Password:     "",                       // no password set
		DB:           0,                        // use default DB
		ReadTimeout:  readTimeout,              // time after read operation timeout
		WriteTimeout: writeTimeout,             // time after write operation timeout
		PoolSize:     poolSize,                 // maximum socket connections
		MinIdleConns: minIdleConns,             // minimum number of idle connections to keep
	}
	if r.EnableTLS {
		optReader.TLSConfig = &tls.Config{InsecureSkipVerify: false}
	}
	r.cnReader = redis.NewClient(optReader)

	if r.cnReader == nil {
		_ = r.cnWriter.Close()
		r.cnWriter = nil
		return errors.New("Connect To Redis Failed: (Reader Endpoint) " + "Obtain Client Yielded Nil")
	}

	// bounded ping to avoid indefinite hangs
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.cnWriter.Ping(ctx).Err(); err != nil {
		_ = r.cnWriter.Close()
		r.cnWriter = nil
		_ = r.cnReader.Close()
		r.cnReader = nil
		return errors.New("Connect To Redis Failed: (Writer Endpoint) ping failed: " + err.Error())
	}
	if err := r.cnReader.Ping(ctx).Err(); err != nil {
		_ = r.cnWriter.Close()
		r.cnWriter = nil
		_ = r.cnReader.Close()
		r.cnReader = nil
		return errors.New("Connect To Redis Failed: (Reader Endpoint) ping failed: " + err.Error())
	}

	// once writer and readers are all connected, set reference to helper struct objects
	r.BIT = new(BIT)
	r.BIT.core = r

	r.LIST = new(LIST)
	r.LIST.core = r

	r.HASH = new(HASH)
	r.HASH.core = r

	r.SET = new(SET)
	r.SET.core = r

	r.SORTED_SET = new(SORTED_SET)
	r.SORTED_SET.core = r

	r.GEO = new(GEO)
	r.GEO.core = r

	r.STREAM = new(STREAM)
	r.STREAM.core = r

	r.PUBSUB = new(PUBSUB)
	r.PUBSUB.core = r

	r.PIPELINE = new(PIPELINE)
	r.PIPELINE.core = r

	r.TTL = new(TTL)
	r.TTL.core = r

	r.UTILS = new(UTILS)
	r.UTILS.core = r

	// ready
	r.cnAreReady = true

	// success
	return nil
}

// Disconnect will close aws redis writer and reader endpoints
func (r *Redis) Disconnect() {
	if r == nil {
		return
	}
	r.connectMU.Lock()
	defer r.connectMU.Unlock()

	r.cnAreReady = false

	// Note: sub-struct pointers (BIT, LIST, etc.) and their .core fields are NOT
	// nilled here. They are read without locks by callers (e.g., r.BIT.SetBit(...)),
	// so nilling them would create a data race. The cnAreReady=false flag, captured
	// atomically by connSnapshot(), is sufficient to make all operations return an
	// error safely. connectInternal() handles full cleanup/recreation under the write lock.

	if r.cnWriter != nil {
		_ = r.cnWriter.Close()
		r.cnWriter = nil
	}

	if r.cnReader != nil {
		_ = r.cnReader.Close()
		r.cnReader = nil
	}
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (r *Redis) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	if r == nil {
		return
	}
	r.connectMU.Lock()
	r._parentSegment = parentSegment
	r.connectMU.Unlock()
}

// ----------------------------------------------------------------------------------------------------------------
// Cmd Result and Error Handling functions
// ----------------------------------------------------------------------------------------------------------------

// handleStatusCmd evaluates redis StatusCmd struct, and returns error struct if applicable,
// error = nil indicates success
func (r *Redis) handleStatusCmd(statusCmd *redis.StatusCmd, errorTextPrefix ...string) error {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if statusCmd == nil {
		return errors.New(prefix + "Redis StatusCmd Result Yielded Nil")
	} else {
		if statusCmd.Err() != nil {
			// has error encountered
			return errors.New(prefix + statusCmd.Err().Error())
		} else {
			// no error encountered
			return nil
		}
	}
}

// handleStringStatusCmd evaluates redis StringStatusCmd struct, and returns error struct if applicable,
// error = nil indicates success
func (r *Redis) handleStringStatusCmd(statusCmd *redis.StatusCmd, errorTextPrefix ...string) (val string, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if statusCmd == nil {
		return "", false, errors.New(prefix + "Redis StatusCmd Result Yielded Nil")
	} else {
		if statusCmd.Err() != nil {
			if statusCmd.Err() == redis.Nil {
				// not found
				return "", true, nil
			} else {
				// has error encountered
				return "", false, errors.New(prefix + statusCmd.Err().Error())
			}
		} else {
			// no error encountered
			if val, err = statusCmd.Result(); err != nil {
				return "", false, errors.New(prefix + "[Result to String Errored] " + err.Error())
			} else {
				return val, false, nil
			}
		}
	}
}

// handleBoolCmd evaluates redis BoolCmd struct, and returns error struct if applicable,
// error = nil indicates success
func (r *Redis) handleBoolCmd(boolCmd *redis.BoolCmd, errorTextPrefix ...string) (val bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if boolCmd == nil {
		return false, errors.New(prefix + "Redis BoolCmd Result Yielded Nil")
	} else {
		if boolCmd.Err() != nil {
			// has error encountered
			return false, errors.New(prefix + boolCmd.Err().Error())
		} else {
			// no error encountered
			return boolCmd.Val(), nil
		}
	}
}

// handleIntCmd evaluates redis IntCmd struct, and returns error struct if applicable,
// error = nil indicates success
func (r *Redis) handleIntCmd(intCmd *redis.IntCmd, errorTextPrefix ...string) (val int64, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if intCmd == nil {
		return 0, false, errors.New(prefix + "Redis IntCmd Result Yielded Nil")
	} else {
		if intCmd.Err() != nil {
			if intCmd.Err() == redis.Nil {
				// not found
				return 0, true, nil
			} else {
				// other error
				return 0, false, errors.New(prefix + intCmd.Err().Error())
			}
		} else {
			// no error encountered
			if val, err = intCmd.Result(); err != nil {
				return 0, false, errors.New(prefix + "[Result to Int64 Errored] " + err.Error())
			} else {
				return val, false, nil
			}
		}
	}
}

// handleIntCmd2 evaluates redis IntCmd struct, and returns error struct if result is 0 or actual error
func (r *Redis) handleIntCmd2(intCmd *redis.IntCmd, errorTextPrefix ...string) error {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if intCmd == nil {
		return errors.New(prefix + "Redis IntCmd Result Yielded Nil")
	} else {
		if intCmd.Err() != nil {
			return errors.New(prefix + intCmd.Err().Error())
		} else {
			// no error encountered
			if val, err := intCmd.Result(); err != nil {
				return errors.New(prefix + "[Result to Int64 Errored] " + err.Error())
			} else {
				if val == 0 {
					// fail
					return errors.New(prefix + "[No Records Affected] Action Yielded No Change")
				} else {
					// success
					return nil
				}
			}
		}
	}
}

// handleFloatCmd evaluates redis FloatCmd struct, and returns error struct if applicable,
// error = nil indicates success
func (r *Redis) handleFloatCmd(floatCmd *redis.FloatCmd, errorTextPrefix ...string) (val float64, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if floatCmd == nil {
		return 0.00, false, errors.New(prefix + "Redis FloatCmd Result Yielded Nil")
	} else {
		if floatCmd.Err() != nil {
			if floatCmd.Err() == redis.Nil {
				// not found
				return 0.00, true, nil
			} else {
				// other error
				return 0.00, false, errors.New(prefix + floatCmd.Err().Error())
			}
		} else {
			// no error encountered
			if val, err = floatCmd.Result(); err != nil {
				return 0.00, false, errors.New(prefix + "[Result to Float64 Errored] " + err.Error())
			} else {
				return val, false, nil
			}
		}
	}
}

// handleTimeCmd evaluates redis TimeCmd struct, and returns error struct if applicable,
// error = nil indicates success
func (r *Redis) handleTimeCmd(timeCmd *redis.TimeCmd, errorTextPrefix ...string) (val time.Time, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if timeCmd == nil {
		return time.Time{}, false, errors.New(prefix + "Redis TimeCmd Result Yielded Nil")
	} else {
		if timeCmd.Err() != nil {
			if timeCmd.Err() == redis.Nil {
				// not found
				return time.Time{}, true, nil
			} else {
				// other error
				return time.Time{}, false, errors.New(prefix + timeCmd.Err().Error())
			}
		} else {
			// no error encountered
			if val, err = timeCmd.Result(); err != nil {
				return time.Time{}, false, errors.New(prefix + "[Result to Time Errored] " + err.Error())
			} else {
				return val, false, nil
			}
		}
	}
}

// handleDurationCmd evaluates redis DurationCmd struct, and returns error struct if applicable,
// error = nil indicates success
func (r *Redis) handleDurationCmd(durationCmd *redis.DurationCmd, errorTextPrefix ...string) (val time.Duration, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if durationCmd == nil {
		return 0, false, errors.New(prefix + "Redis DurationCmd Result Yielded Nil")
	} else {
		if durationCmd.Err() != nil {
			if durationCmd.Err() == redis.Nil {
				// not found
				return 0, true, nil
			} else {
				// other error
				return 0, false, errors.New(prefix + durationCmd.Err().Error())
			}
		} else {
			// no error encountered
			if val, err = durationCmd.Result(); err != nil {
				return 0, false, errors.New(prefix + "[Result to Duration Errored] " + err.Error())
			} else {
				return val, false, nil
			}
		}
	}
}

// handleStringCmd2 evaluates redis StringCmd struct, and returns error struct if applicable,
// error = nil indicates success
func (r *Redis) handleStringCmd2(stringCmd *redis.StringCmd, errorTextPrefix ...string) (val string, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if stringCmd == nil {
		return "", false, errors.New(prefix + "Redis StringCmd Result Yielded Nil")
	} else {
		if stringCmd.Err() != nil {
			if stringCmd.Err() == redis.Nil {
				// not found
				return "", true, nil
			} else {
				// other error
				return "", false, errors.New(prefix + stringCmd.Err().Error())
			}
		} else {
			// no error encountered
			if val, err = stringCmd.Result(); err != nil {
				return "", false, errors.New(prefix + "[Result to String Errored] " + err.Error())
			} else {
				return val, false, nil
			}
		}
	}
}

// handleStringCmd evaluates redis StringCmd struct, and related results and error if applicable
func (r *Redis) handleStringCmd(stringCmd *redis.StringCmd, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}, errorTextPrefix ...string) (notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if !outputDataType.Valid() || outputDataType == redisdatatype.UNKNOWN {
		return false, errors.New(prefix + "Result Output Data Type is Required")
	}

	if outputObjectPtr == nil {
		return false, errors.New(prefix + "Result Output Data Object Pointer is Required")
	}

	if stringCmd == nil {
		return false, errors.New(prefix + "Redis StringCmd Result Yielded Nil")
	} else {
		if stringCmd.Err() != nil {
			// has error encountered
			if stringCmd.Err() == redis.Nil {
				// not found
				return true, nil
			} else {
				// other error
				return false, errors.New(prefix + stringCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			switch outputDataType {
			case redisdatatype.Bool:
				// result to bool
				dst, ok := outputObjectPtr.(*bool)
				if !ok {
					return false, errors.New(prefix + "[Result to Bool Errored] Output Object Pointer Type Assertion to *bool Failed")
				}
				if val, e := stringCmd.Result(); e != nil {
					return false, errors.New(prefix + "[Result to Bool Errored] " + e.Error())
				} else {
					// success
					if output, success := util.ParseBool(val); !success {
						return false, errors.New(prefix + "[Result to Bool Errored] Parse Str to Bool Not OK")
					} else {
						*dst = output
						return false, nil
					}
				}
			case redisdatatype.Int:
				// result to int
				dst, ok := outputObjectPtr.(*int)
				if !ok {
					return false, errors.New(prefix + "[Result to Int Errored] Output Object Pointer Type Assertion to *int Failed")
				}
				if val, e := stringCmd.Int(); e != nil {
					return false, errors.New(prefix + "[Result to Int Errored] " + e.Error())
				} else {
					// success
					*dst = val
					return false, nil
				}
			case redisdatatype.Int64:
				// result to int64
				dst, ok := outputObjectPtr.(*int64)
				if !ok {
					return false, errors.New(prefix + "[Result to Int64 Errored] Output Object Pointer Type Assertion to *int64 Failed")
				}
				if val, e := stringCmd.Int64(); e != nil {
					return false, errors.New(prefix + "[Result to Int64 Errored] " + e.Error())
				} else {
					// success
					*dst = val
					return false, nil
				}
			case redisdatatype.Float64:
				// result to float64
				dst, ok := outputObjectPtr.(*float64)
				if !ok {
					return false, errors.New(prefix + "[Result to Float64 Errored] Output Object Pointer Type Assertion to *float64 Failed")
				}
				if val, e := stringCmd.Float64(); e != nil {
					return false, errors.New(prefix + "[Result to Float64 Errored] " + e.Error())
				} else {
					// success
					*dst = val
					return false, nil
				}
			case redisdatatype.Bytes:
				// result to []byte
				dst, ok := outputObjectPtr.(*[]byte)
				if !ok {
					return false, errors.New(prefix + "[Result to Bytes Errored] Output Object Pointer Type Assertion to *[]byte Failed")
				}
				if val, e := stringCmd.Bytes(); e != nil {
					return false, errors.New(prefix + "[Result to Bytes Errored] " + e.Error())
				} else {
					// success
					*dst = val
					return false, nil
				}
			case redisdatatype.Json:
				// result to json
				if str, e := stringCmd.Result(); e != nil {
					return false, errors.New(prefix + "[Result to Json Errored] " + e.Error())
				} else {
					// ready to unmarshal json to object
					// found str value,
					// unmarshal to json
					if util.LenTrim(str) <= 0 {
						return false, nil
					} else {
						if err = util.UnmarshalJSON(str, outputObjectPtr); err != nil {
							// unmarshal error
							return false, errors.New(prefix + "[Result to Json Errored] Unmarshal Json Failed " + err.Error())
						} else {
							// unmarshal success
							return false, nil
						}
					}
				}
			case redisdatatype.Time:
				// result to int
				dst, ok := outputObjectPtr.(*time.Time)
				if !ok {
					return false, errors.New(prefix + "[Result to Time Errored] Output Object Pointer Type Assertion to *time.Time Failed")
				}
				if val, e := stringCmd.Time(); e != nil {
					return false, errors.New(prefix + "[Result to Time Errored] " + e.Error())
				} else {
					// success
					*dst = val
					return false, nil
				}
			default:
				// default is string
				dst, ok := outputObjectPtr.(*string)
				if !ok {
					return false, errors.New(prefix + "[Result to String Errored] Output Object Pointer Type Assertion to *string Failed")
				}
				if str, e := stringCmd.Result(); e != nil {
					return false, errors.New(prefix + "[Result to String Errored] " + e.Error())
				} else {
					// success
					*dst = str
					return false, nil
				}
			}
		}
	}
}

// handleSliceCmd evaluates redis SliceCmd struct, and related results and error if applicable
func (r *Redis) handleSliceCmd(sliceCmd *redis.SliceCmd, errorTextPrefix ...string) (outputSlice []interface{}, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if sliceCmd == nil {
		return nil, false, errors.New(prefix + "Redis SliceCmd Result Yielded Nil")
	} else {
		if sliceCmd.Err() != nil {
			// has error encountered
			if sliceCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + sliceCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputSlice, err = sliceCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to Slice Errored] " + err.Error())
			} else {
				// success
				return outputSlice, false, nil
			}
		}
	}
}

// handleStringSliceCmd evaluates redis StringSliceCmd struct, and related results and error if applicable
func (r *Redis) handleStringSliceCmd(stringSliceCmd *redis.StringSliceCmd, errorTextPrefix ...string) (outputSlice []string, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if stringSliceCmd == nil {
		return nil, false, errors.New(prefix + "Redis StringSliceCmd Result Yielded Nil")
	} else {
		if stringSliceCmd.Err() != nil {
			// has error encountered
			if stringSliceCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + stringSliceCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputSlice, err = stringSliceCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to String Slice Errored] " + err.Error())
			} else {
				// success
				return outputSlice, false, nil
			}
		}
	}
}

// handleIntSliceCmd evaluates redis IntSliceCmd struct, and related results and error if applicable
func (r *Redis) handleIntSliceCmd(intSliceCmd *redis.IntSliceCmd, errorTextPrefix ...string) (outputSlice []int64, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if intSliceCmd == nil {
		return nil, false, errors.New(prefix + "Redis IntSliceCmd Result Yielded Nil")
	} else {
		if intSliceCmd.Err() != nil {
			// has error encountered
			if intSliceCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + intSliceCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputSlice, err = intSliceCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to Int64 Slice Errored] " + err.Error())
			} else {
				// success
				return outputSlice, false, nil
			}
		}
	}
}

// handleBoolSliceCmd evaluates redis BoolSliceCmd struct, and related results and error if applicable
func (r *Redis) handleBoolSliceCmd(boolSliceCmd *redis.BoolSliceCmd, errorTextPrefix ...string) (outputSlice []bool, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if boolSliceCmd == nil {
		return nil, false, errors.New(prefix + "Redis BoolSliceCmd Result Yielded Nil")
	} else {
		if boolSliceCmd.Err() != nil {
			// has error encountered
			if boolSliceCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + boolSliceCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputSlice, err = boolSliceCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to Bool Slice Errored] " + err.Error())
			} else {
				// success
				if len(outputSlice) > 0 {
					return outputSlice, false, nil
				} else {
					return nil, true, nil
				}
			}
		}
	}
}

// handleZSliceCmd evaluates redis ZSliceCmd struct, and related results and error if applicable
func (r *Redis) handleZSliceCmd(zSliceCmd *redis.ZSliceCmd, errorTextPrefix ...string) (outputSlice []redis.Z, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if zSliceCmd == nil {
		return nil, false, errors.New(prefix + "Redis ZSliceCmd Result Yielded Nil")
	} else {
		if zSliceCmd.Err() != nil {
			// has error encountered
			if zSliceCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + zSliceCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputSlice, err = zSliceCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to Z Slice Errored] " + err.Error())
			} else {
				// success
				if len(outputSlice) > 0 {
					return outputSlice, false, nil
				} else {
					return nil, true, nil
				}
			}
		}
	}
}

// handleXMessageSliceCmd evaluates redis XMessageSliceCmd struct, and related results and error if applicable
func (r *Redis) handleXMessageSliceCmd(xmessageSliceCmd *redis.XMessageSliceCmd, errorTextPrefix ...string) (outputSlice []redis.XMessage, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if xmessageSliceCmd == nil {
		return nil, false, errors.New(prefix + "Redis XMessageSliceCmd Result Yielded Nil")
	} else {
		if xmessageSliceCmd.Err() != nil {
			// has error encountered
			if xmessageSliceCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + xmessageSliceCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputSlice, err = xmessageSliceCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to XMessage Slice Errored] " + err.Error())
			} else {
				// success
				if len(outputSlice) > 0 {
					return outputSlice, false, nil
				} else {
					return nil, true, nil
				}
			}
		}
	}
}

// handleXStreamSliceCmd evaluates redis XStreamSliceCmd struct, and related results and error if applicable
func (r *Redis) handleXStreamSliceCmd(xstreamSliceCmd *redis.XStreamSliceCmd, errorTextPrefix ...string) (outputSlice []redis.XStream, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if xstreamSliceCmd == nil {
		return nil, false, errors.New(prefix + "Redis XStreamSliceCmd Result Yielded Nil")
	} else {
		if xstreamSliceCmd.Err() != nil {
			// has error encountered
			if xstreamSliceCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + xstreamSliceCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputSlice, err = xstreamSliceCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to XStream Slice Errored] " + err.Error())
			} else {
				// success
				if len(outputSlice) > 0 {
					return outputSlice, false, nil
				} else {
					return nil, true, nil
				}
			}
		}
	}
}

// handleXInfoGroupsCmd evaluates redis XInfoGroupsCmd struct, and related results and error if applicable
func (r *Redis) handleXInfoGroupsCmd(xinfoGroupsCmd *redis.XInfoGroupsCmd, errorTextPrefix ...string) (outputSlice []redis.XInfoGroup, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if xinfoGroupsCmd == nil {
		return nil, false, errors.New(prefix + "Redis XInfoGroupsCmd Result Yielded Nil")
	} else {
		if xinfoGroupsCmd.Err() != nil {
			// has error encountered
			if xinfoGroupsCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + xinfoGroupsCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputSlice, err = xinfoGroupsCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to XInfoGroups Errored] " + err.Error())
			} else {
				// success
				if len(outputSlice) > 0 {
					return outputSlice, false, nil
				} else {
					return nil, true, nil
				}
			}
		}
	}
}

// handleScanCmd evaluates redis ScanCmd struct, and returns error struct if applicable,
// error = nil indicates success
func (r *Redis) handleScanCmd(scanCmd *redis.ScanCmd, errorTextPrefix ...string) (keys []string, cursor uint64, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if scanCmd == nil {
		return nil, 0, errors.New(prefix + "Redis ScanCmd Result Yielded Nil")
	} else {
		if scanCmd.Err() != nil {
			if scanCmd.Err() == redis.Nil {
				// not found
				return nil, 0, nil
			} else {
				// other error
				return nil, 0, errors.New(prefix + scanCmd.Err().Error())
			}
		} else {
			// no error encountered
			return scanCmd.Result()
		}
	}
}

// handleXPendingCmd evaluates redis XPendingCmd struct, and related results and error if applicable
func (r *Redis) handleXPendingCmd(xpendingCmd *redis.XPendingCmd, errorTextPrefix ...string) (output *redis.XPending, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if xpendingCmd == nil {
		return nil, false, errors.New(prefix + "Redis XPendingCmd Result Yielded Nil")
	} else {
		if xpendingCmd.Err() != nil {
			// has error encountered
			if xpendingCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + xpendingCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if output, err = xpendingCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to XPending Errored] " + err.Error())
			} else {
				// success
				if output != nil {
					return output, false, nil
				} else {
					return nil, true, nil
				}
			}
		}
	}
}

// handleXPendingExtCmd evaluates redis XPendingExtCmd struct, and related results and error if applicable
func (r *Redis) handleXPendingExtCmd(xpendingExtCmd *redis.XPendingExtCmd, errorTextPrefix ...string) (outputSlice []redis.XPendingExt, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if xpendingExtCmd == nil {
		return nil, false, errors.New(prefix + "Redis XPendingExtCmd Result Yielded Nil")
	} else {
		if xpendingExtCmd.Err() != nil {
			// has error encountered
			if xpendingExtCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + xpendingExtCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputSlice, err = xpendingExtCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to XPendingExt Errored] " + err.Error())
			} else {
				// success
				if len(outputSlice) > 0 {
					return outputSlice, false, nil
				} else {
					return nil, true, nil
				}
			}
		}
	}
}

// handleStringIntMapCmd evaluates redis StringIntMapCmd struct, and related results and error if applicable
func (r *Redis) handleStringIntMapCmd(stringIntMapCmd *redis.StringIntMapCmd, errorTextPrefix ...string) (outputMap map[string]int64, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if stringIntMapCmd == nil {
		return nil, false, errors.New(prefix + "Redis String-Int-Map Result Yielded Nil")
	} else {
		if stringIntMapCmd.Err() != nil {
			// has error encountered
			if stringIntMapCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + stringIntMapCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputMap, err = stringIntMapCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to String-Int-Map Errored] " + err.Error())
			} else {
				// success
				return outputMap, false, nil
			}
		}
	}
}

// handleStringStringMapCmd evaluates redis StringStringMapCmd struct, and related results and error if applicable
func (r *Redis) handleStringStringMapCmd(stringStringMapCmd *redis.StringStringMapCmd, errorTextPrefix ...string) (outputMap map[string]string, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if stringStringMapCmd == nil {
		return nil, false, errors.New(prefix + "Redis String-String-Map Result Yielded Nil")
	} else {
		if stringStringMapCmd.Err() != nil {
			// has error encountered
			if stringStringMapCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + stringStringMapCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputMap, err = stringStringMapCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to String-String-Map Errored] " + err.Error())
			} else {
				// success
				return outputMap, false, nil
			}
		}
	}
}

// handleStringStructMapCmd evaluates redis StringStructMapCmd struct, and related results and error if applicable
func (r *Redis) handleStringStructMapCmd(stringStructMapCmd *redis.StringStructMapCmd, errorTextPrefix ...string) (outputMap map[string]struct{}, notFound bool, err error) {
	prefix := ""

	if len(errorTextPrefix) > 0 {
		prefix = errorTextPrefix[0]
	}

	if stringStructMapCmd == nil {
		return nil, false, errors.New(prefix + "Redis String-Struct-Map Result Yielded Nil")
	} else {
		if stringStructMapCmd.Err() != nil {
			// has error encountered
			if stringStructMapCmd.Err() == redis.Nil {
				// not found
				return nil, true, nil
			} else {
				// other error
				return nil, false, errors.New(prefix + stringStructMapCmd.Err().Error())
			}
		} else {
			// no error, evaluate result
			if outputMap, err = stringStructMapCmd.Result(); err != nil {
				// error
				return nil, false, errors.New(prefix + "[Result to String-Struct-Map Errored] " + err.Error())
			} else {
				// success
				return outputMap, false, nil
			}
		}
	}
}

// ----------------------------------------------------------------------------------------------------------------
// Set and Get functions
// ----------------------------------------------------------------------------------------------------------------

// SetBase is helper to set value into redis by key.
//
// notes
//
//	setCondition = support for SetNX and SetXX
func (r *Redis) SetBase(key string, val interface{}, setCondition redissetcondition.RedisSetCondition, expires ...time.Duration) (err error) {
	if r == nil {
		return errors.New("Redis SetBase Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Set", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Set-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Set-Value", val))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Set-Condition", setCondition.Caption()))

			if len(expires) > 0 {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Set-Expire-Seconds", expires[0].Seconds()))
			} else {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Set-Expire-Seconds", "Not Defined"))
			}

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = r.setBaseInternal(snap, key, val, setCondition, expires...)
		return err
	} else {
		return r.setBaseInternal(snap, key, val, setCondition, expires...)
	}
}

// setBaseInternal is helper to set value into redis by key
//
// notes
//
//	setCondition = support for SetNX and SetXX
func (r *Redis) setBaseInternal(snap redisConnSnapshot, key string, val interface{}, setCondition redissetcondition.RedisSetCondition, expires ...time.Duration) error {
	// validate
	if !snap.ready {
		return errors.New("Redis Set Failed: " + "Endpoint Connections Not Ready")
	}

	if util.LenTrim(key) <= 0 {
		return errors.New("Redis Set Failed: " + "Key is Required")
	}

	// persist value to redis
	var expireDuration time.Duration

	if len(expires) > 0 {
		expireDuration = expires[0]
	}

	switch setCondition {
	case redissetcondition.Normal:
		cmd := snap.writer.Set(snap.writer.Context(), key, val, expireDuration)
		return r.handleStatusCmd(cmd, "Redis Set Failed: (Set Method) ")

	case redissetcondition.SetIfExists:
		cmd := snap.writer.SetXX(snap.writer.Context(), key, val, expireDuration)
		if val, err := r.handleBoolCmd(cmd, "Redis Set Failed: (SetXX Method) "); err != nil {
			return err
		} else {
			if val {
				// success
				return nil
			} else {
				// not success
				return errors.New("Redis Set Failed: (SetXX Method) " + "Key Was Not Set")
			}
		}

	case redissetcondition.SetIfNotExists:
		cmd := snap.writer.SetNX(snap.writer.Context(), key, val, expireDuration)
		if val, err := r.handleBoolCmd(cmd, "Redis Set Failed: (SetNX Method) "); err != nil {
			return err
		} else {
			if val {
				// success
				return nil
			} else {
				// not success
				return errors.New("Redis Set Failed: (SetNX Method) " + "Key Was Not Set")
			}
		}

	default:
		return errors.New("Redis Set Failed: (Set Method) " + "SetCondition Not Expected")
	}
}

// Set sets string value into redis by key
func (r *Redis) Set(key string, val string, expires ...time.Duration) error {
	if r == nil {
		return errors.New("Redis Set Failed: Redis receiver is nil")
	}
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetBool sets boolean value into redis by key
func (r *Redis) SetBool(key string, val bool, expires ...time.Duration) error {
	if r == nil {
		return errors.New("Redis SetBool Failed: Redis receiver is nil")
	}
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetInt sets int value into redis by key
func (r *Redis) SetInt(key string, val int, expires ...time.Duration) error {
	if r == nil {
		return errors.New("Redis SetInt Failed: Redis receiver is nil")
	}
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetInt64 sets int64 value into redis by key
func (r *Redis) SetInt64(key string, val int64, expires ...time.Duration) error {
	if r == nil {
		return errors.New("Redis SetInt64 Failed: Redis receiver is nil")
	}
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetFloat64 sets float64 value into redis by key
func (r *Redis) SetFloat64(key string, val float64, expires ...time.Duration) error {
	if r == nil {
		return errors.New("Redis SetFloat64 Failed: Redis receiver is nil")
	}
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetBytes sets []byte value into redis by key
func (r *Redis) SetBytes(key string, val []byte, expires ...time.Duration) error {
	if r == nil {
		return errors.New("Redis SetBytes Failed: Redis receiver is nil")
	}
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetJson sets Json object into redis by key (Json object is marshaled into string and then saved to redis)
func (r *Redis) SetJson(key string, jsonObject interface{}, expires ...time.Duration) error {
	if r == nil {
		return errors.New("Redis SetJson Failed: Redis receiver is nil")
	}
	if val, err := util.MarshalJSONCompact(jsonObject); err != nil {
		return errors.New("Redis Set Failed: (Marshal Json) " + err.Error())
	} else {
		return r.SetBase(key, val, redissetcondition.Normal, expires...)
	}
}

// SetTime sets time.Time value into redis by key
func (r *Redis) SetTime(key string, val time.Time, expires ...time.Duration) error {
	if r == nil {
		return errors.New("Redis SetTime Failed: Redis receiver is nil")
	}
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// GetBase is internal helper to get value from redis.
func (r *Redis) GetBase(key string) (cmd *redis.StringCmd, notFound bool, err error) {
	if r == nil {
		return nil, false, errors.New("Redis GetBase Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Get", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Get-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Get-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Get-Value-Cmd", cmd))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		cmd, notFound, err = r.getBaseInternal(snap, key)
		return cmd, notFound, err
	} else {
		return r.getBaseInternal(snap, key)
	}
}

// getBaseInternal is internal helper to get value from redis.
func (r *Redis) getBaseInternal(snap redisConnSnapshot, key string) (cmd *redis.StringCmd, notFound bool, err error) {
	// validate
	if !snap.ready {
		return nil, false, errors.New("Redis Get Failed: " + "Endpoint Connections Not Ready")
	}

	if util.LenTrim(key) <= 0 {
		return nil, false, errors.New("Redis Get Failed: " + "Key is Required")
	}

	// get value from redis
	cmd = snap.reader.Get(snap.reader.Context(), key)

	if cmd.Err() != nil {
		if cmd.Err() == redis.Nil {
			// not found
			return nil, true, nil
		} else {
			// other error
			return nil, false, errors.New("Redis Get Failed: (Get Method) " + cmd.Err().Error())
		}
	} else {
		// value found - return actual StringCmd
		return cmd, false, nil
	}
}

// Get gets string value from redis by key
func (r *Redis) Get(key string) (val string, notFound bool, err error) {
	if r == nil {
		return "", false, errors.New("Redis Get Failed: Redis receiver is nil")
	}
	var cmd *redis.StringCmd

	if cmd, notFound, err = r.GetBase(key); err != nil {
		return "", notFound, err
	} else {
		if notFound {
			return "", notFound, err
		} else {
			if val, err = cmd.Result(); err != nil {
				return "", false, err
			} else {
				// found string value
				return val, false, nil
			}
		}
	}
}

// GetBool gets bool value from redis by key
func (r *Redis) GetBool(key string) (val bool, notFound bool, err error) {
	if r == nil {
		return false, false, errors.New("Redis GetBool Failed: Redis receiver is nil")
	}
	var cmd *redis.StringCmd

	if cmd, notFound, err = r.GetBase(key); err != nil {
		return false, notFound, err
	} else {
		if notFound {
			return false, notFound, err
		} else {
			var valStr string

			if valStr, err = cmd.Result(); err != nil {
				return false, false, err
			} else {
				// found string value,
				// convert to bool
				b, success := util.ParseBool(valStr)

				if success {
					return b, false, nil
				} else {
					return false, true, nil
				}
			}
		}
	}
}

// GetInt gets int value from redis by key
func (r *Redis) GetInt(key string) (val int, notFound bool, err error) {
	if r == nil {
		return 0, false, errors.New("Redis GetInt Failed: Redis receiver is nil")
	}
	var cmd *redis.StringCmd

	if cmd, notFound, err = r.GetBase(key); err != nil {
		return 0, notFound, err
	} else {
		if notFound {
			return 0, notFound, err
		} else {
			if val, err = cmd.Int(); err != nil {
				return 0, false, err
			} else {
				// found int value,
				return val, false, nil
			}
		}
	}
}

// GetInt64 gets int64 value from redis by key
func (r *Redis) GetInt64(key string) (val int64, notFound bool, err error) {
	if r == nil {
		return 0, false, errors.New("Redis GetInt64 Failed: Redis receiver is nil")
	}
	var cmd *redis.StringCmd

	if cmd, notFound, err = r.GetBase(key); err != nil {
		return 0, notFound, err
	} else {
		if notFound {
			return 0, notFound, err
		} else {
			if val, err = cmd.Int64(); err != nil {
				return 0, false, err
			} else {
				// found int value,
				return val, false, nil
			}
		}
	}
}

// GetFloat64 gets float64 value from redis by key
func (r *Redis) GetFloat64(key string) (val float64, notFound bool, err error) {
	if r == nil {
		return 0.0, false, errors.New("Redis GetFloat64 Failed: Redis receiver is nil")
	}
	var cmd *redis.StringCmd

	if cmd, notFound, err = r.GetBase(key); err != nil {
		return 0.00, notFound, err
	} else {
		if notFound {
			return 0.00, notFound, err
		} else {
			if val, err = cmd.Float64(); err != nil {
				return 0.00, false, err
			} else {
				// found int value,
				return val, false, nil
			}
		}
	}
}

// GetBytes gets []byte value from redis by key
func (r *Redis) GetBytes(key string) (val []byte, notFound bool, err error) {
	if r == nil {
		return nil, false, errors.New("Redis GetBytes Failed: Redis receiver is nil")
	}
	var cmd *redis.StringCmd

	if cmd, notFound, err = r.GetBase(key); err != nil {
		return []byte{}, notFound, err
	} else {
		if notFound {
			return []byte{}, notFound, err
		} else {
			if val, err = cmd.Bytes(); err != nil {
				return []byte{}, false, err
			} else {
				// found int value,
				return val, false, nil
			}
		}
	}
}

// GetJson gets Json object from redis by key (Json is stored as string in redis, unmarshaled to target object via get)
func (r *Redis) GetJson(key string, resultObjectPtr interface{}) (notFound bool, err error) {
	if r == nil {
		return false, errors.New("Redis GetJson Failed: Redis receiver is nil")
	}
	if resultObjectPtr == nil {
		return false, errors.New("Redis Get Failed: (GetJson) " + "JSON Result Pointer Object is Required")
	}

	var cmd *redis.StringCmd

	if cmd, notFound, err = r.GetBase(key); err != nil {
		return notFound, err
	} else {
		if notFound {
			return notFound, err
		} else {
			var valStr string

			if valStr, err = cmd.Result(); err != nil {
				return false, err
			} else {
				// found str value,
				// unmarshal to json
				if util.LenTrim(valStr) <= 0 {
					return false, nil
				} else {
					if err = util.UnmarshalJSON(valStr, resultObjectPtr); err != nil {
						// unmarshal error
						return false, err
					} else {
						// unmarshal success
						return false, nil
					}
				}
			}
		}
	}
}

// GetTime gets time.Time value from redis by key
func (r *Redis) GetTime(key string) (val time.Time, notFound bool, err error) {
	if r == nil {
		return time.Time{}, false, errors.New("Redis GetTime Failed: Redis receiver is nil")
	}
	var cmd *redis.StringCmd

	if cmd, notFound, err = r.GetBase(key); err != nil {
		return time.Time{}, notFound, err
	} else {
		if notFound {
			return time.Time{}, notFound, err
		} else {
			if val, err = cmd.Time(); err != nil {
				return time.Time{}, false, err
			} else {
				// found int value,
				return val, false, nil
			}
		}
	}
}

// ----------------------------------------------------------------------------------------------------------------
// GetSet function
// ----------------------------------------------------------------------------------------------------------------

// GetSet will get old string value from redis by key,
// and then set new string value into redis by the same key.
func (r *Redis) GetSet(key string, val string) (oldValue string, notFound bool, err error) {
	if r == nil {
		return "", false, errors.New("Redis GetSet Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// reg new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GetSet", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GetSet-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GetSet-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GetSet-Old_Value", oldValue))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GetSet-New-Value", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		oldValue, notFound, err = r.getSetInternal(snap, key, val)
		return oldValue, notFound, err
	} else {
		return r.getSetInternal(snap, key, val)
	}
}

// getSetInternal will get old string value from redis by key,
// and then set new string value into redis by the same key.
func (r *Redis) getSetInternal(snap redisConnSnapshot, key string, val string) (oldValue string, notFound bool, err error) {
	// validate
	if !snap.ready {
		return "", false, errors.New("Redis GetSet Failed: " + "Endpoint Connections Not Ready")
	}

	if util.LenTrim(key) <= 0 {
		return "", false, errors.New("Redis GetSet Failed: " + "Key is Required")
	}

	// persist value and get old value as return result
	cmd := snap.writer.GetSet(snap.writer.Context(), key, val)
	notFound, err = r.handleStringCmd(cmd, redisdatatype.String, &oldValue, "Redis GetSet Failed:  ")

	// return result
	return oldValue, notFound, err
}

// ----------------------------------------------------------------------------------------------------------------
// MSet, MSetNX and MGet functions
// ----------------------------------------------------------------------------------------------------------------

// MSet is helper to set multiple values into redis by keys,
// optional parameter setIfNotExists indicates if instead MSetNX is to be used
//
// notes
//
//	kvMap = map of key string, and interface{} value
func (r *Redis) MSet(kvMap map[string]interface{}, setIfNotExists ...bool) (err error) {
	if r == nil {
		return errors.New("Redis MSet Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-MSet", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-MSet-KeyValueMap", kvMap))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = r.msetInternal(snap, kvMap, setIfNotExists...)
		return err
	} else {
		return r.msetInternal(snap, kvMap, setIfNotExists...)
	}
}

// msetInternal is helper to set multiple values into redis by keys,
// optional parameter setIfNotExists indicates if instead MSetNX is to be used
//
// notes
//
//	kvMap = map of key string, and interface{} value
func (r *Redis) msetInternal(snap redisConnSnapshot, kvMap map[string]interface{}, setIfNotExists ...bool) error {
	// validate
	if len(kvMap) == 0 {
		return errors.New("Redis MSet Failed: " + "KVMap is Required")
	}

	if !snap.ready {
		return errors.New("Redis MSet Failed: " + "Endpoint Connections Not Ready")
	}

	// persist value to redis
	nx := false

	if len(setIfNotExists) > 0 {
		nx = setIfNotExists[0]
	}

	if !nx {
		// normal
		cmd := snap.writer.MSet(snap.writer.Context(), kvMap)
		return r.handleStatusCmd(cmd, "Redis MSet Failed: ")
	} else {
		// nx
		cmd := snap.writer.MSetNX(snap.writer.Context(), kvMap)
		if val, err := r.handleBoolCmd(cmd, "Redis MSetNX Failed: "); err != nil {
			return err
		} else {
			if val {
				// success
				return nil
			} else {
				// not success
				return errors.New("Redis MSetNX Failed: " + "Key Was Not Set")
			}
		}
	}
}

// MGet is a helper to get values from redis based on one or more keys specified
func (r *Redis) MGet(key ...string) (results []interface{}, notFound bool, err error) {
	if r == nil {
		return nil, false, errors.New("Redis MGet Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-MGet", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-MGet-Keys", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-MGet-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-MGet-Results", results))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		results, notFound, err = r.mgetInternal(snap, key...)
		return results, notFound, err
	} else {
		return r.mgetInternal(snap, key...)
	}
}

// mgetInternal is a helper to get values from redis based on one or more keys specified
func (r *Redis) mgetInternal(snap redisConnSnapshot, key ...string) (results []interface{}, notFound bool, err error) {
	// validate
	if len(key) <= 0 {
		return nil, false, errors.New("Redis MGet Failed: " + "Key is Required")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis MGet Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.MGet(snap.reader.Context(), key...)
	return r.handleSliceCmd(cmd, "Redis MGet Failed: ")
}

// ----------------------------------------------------------------------------------------------------------------
// SetRange and GetRange functions
// ----------------------------------------------------------------------------------------------------------------

// SetRange sets val into key's stored value in redis, offset by the offset number
//
// example:
//  1. "Hello World"
//  2. Offset 6 = W
//  3. Val "Xyz" replaces string from Offset Position 6
//  4. End Result String = "Hello Xyzld"
func (r *Redis) SetRange(key string, offset int64, val string) (err error) {
	if r == nil {
		return errors.New("Redis SetRange Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SetRange", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SetRange-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SetRange-Offset", offset))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SetRange-Value", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = r.setRangeInternal(snap, key, offset, val)
		return err
	} else {
		return r.setRangeInternal(snap, key, offset, val)
	}
}

// setRangeInternal sets val into key's stored value in redis, offset by the offset number
//
// example:
//  1. "Hello World"
//  2. Offset 6 = W
//  3. Val "Xyz" replaces string from Offset Position 6
//  4. End Result String = "Hello Xyzld"
func (r *Redis) setRangeInternal(snap redisConnSnapshot, key string, offset int64, val string) error {
	// validate
	if len(key) <= 0 {
		return errors.New("Redis SetRange Failed: " + "Key is Required")
	}

	if !snap.ready {
		return errors.New("Redis SetRange Failed: " + "Endpoint Connections Not Ready")
	}

	if offset < 0 {
		return errors.New("Redis SetRange Failed: " + "Offset Must Be 0 or Greater")
	}

	cmd := snap.writer.SetRange(snap.writer.Context(), key, offset, val)

	if _, _, err := r.handleIntCmd(cmd, "Redis SetRange Failed: "); err != nil {
		return err
	} else {
		return nil
	}
}

// GetRange gets val between start and end positions from string value stored by key in redis
func (r *Redis) GetRange(key string, start int64, end int64) (val string, notFound bool, err error) {
	if r == nil {
		return "", false, errors.New("Redis GetRange Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GetRange", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GetRange-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GetRange-Start-End", util.Int64ToString(start)+"-"+util.Int64ToString(end)))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GetRange-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GetRange-Value", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = r.getRangeInternal(snap, key, start, end)
		return val, notFound, err
	} else {
		return r.getRangeInternal(snap, key, start, end)
	}
}

// getRangeInternal gets val between start and end positions from string value stored by key in redis
func (r *Redis) getRangeInternal(snap redisConnSnapshot, key string, start int64, end int64) (val string, notFound bool, err error) {
	// validate
	if len(key) <= 0 {
		return "", false, errors.New("Redis GetRange Failed: " + "Key is Required")
	}

	if !snap.ready {
		return "", false, errors.New("Redis GetRange Failed: " + "Endpoint Connections Not Ready")
	}

	if start < 0 {
		return "", false, errors.New("Redis GetRange Failed: " + "Start Must Be 0 or Greater")
	}

	if end < start {
		return "", false, errors.New("Redis GetRange Failed: " + "End Must Equal or Be Greater Than Start")
	}

	cmd := snap.reader.GetRange(snap.reader.Context(), key, start, end)

	if notFound, err = r.handleStringCmd(cmd, redisdatatype.String, &val, "Redis GetRange Failed: "); err != nil {
		return "", false, err
	} else {
		return val, notFound, nil
	}
}

// ----------------------------------------------------------------------------------------------------------------
// Decr, DecrBy, Incr, IncrBy, and IncrByFloat functions
// ----------------------------------------------------------------------------------------------------------------

// Int64AddOrReduce will add or reduce int64 value against a key in redis,
// and return the new value if found and performed
func (r *Redis) Int64AddOrReduce(key string, val int64, isReduce ...bool) (newVal int64, notFound bool, err error) {
	if r == nil {
		return 0, false, errors.New("Redis Int64AddOrReduce Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Int64AddOrReduce", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Int64AddOrReduce-Key", key))

			if len(isReduce) > 0 {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Int64AddOrReduce-IsReduce", isReduce[0]))
			} else {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Int64AddOrReduce-IsReduce", "false"))
			}

			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Int64AddOrReduce-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Int64AddOrReduce-Old-Value", val))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Int64AddOrReduce-New-Value", newVal))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		newVal, notFound, err = r.int64AddOrReduceInternal(snap, key, val, isReduce...)
		return newVal, notFound, err
	} else {
		return r.int64AddOrReduceInternal(snap, key, val, isReduce...)
	}
}

// int64AddOrReduceInternal will add or reduce int64 value against a key in redis,
// and return the new value if found and performed
func (r *Redis) int64AddOrReduceInternal(snap redisConnSnapshot, key string, val int64, isReduce ...bool) (newVal int64, notFound bool, err error) {
	// get reduce bool
	reduce := false

	if len(isReduce) > 0 {
		reduce = isReduce[0]
	}

	methodName := ""

	if reduce {
		methodName = "Decr/DecrBy"
	} else {
		methodName = "Incr/IncrBy"
	}

	// validate
	if len(key) <= 0 {
		return 0, false, errors.New("Redis " + methodName + " Failed: " + "Key is Required")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis " + methodName + " Failed: " + "Endpoint Connections Not Ready")
	}

	if val <= 0 {
		return 0, false, errors.New("Redis " + methodName + " Failed: " + "Value Must Be Greater Than 0")
	}

	var cmd *redis.IntCmd

	if !reduce {
		// increment
		if val == 1 {
			cmd = snap.writer.Incr(snap.writer.Context(), key)
		} else {
			cmd = snap.writer.IncrBy(snap.writer.Context(), key, val)
		}
	} else {
		// decrement
		if val == 1 {
			cmd = snap.writer.Decr(snap.writer.Context(), key)
		} else {
			cmd = snap.writer.DecrBy(snap.writer.Context(), key, val)
		}
	}

	// evaluate cmd result
	return r.handleIntCmd(cmd, "Redis "+methodName+" Failed: ")
}

// Float64AddOrReduce will add or reduce float64 value against a key in redis,
// and return the new value if found and performed
func (r *Redis) Float64AddOrReduce(key string, val float64) (newVal float64, notFound bool, err error) {
	if r == nil {
		return 0.0, false, errors.New("Redis Float64AddOrReduce Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Float64AddOrReduce", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Float64AddOrReduce-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Float64AddOrReduce-Value", val))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Float64AddOrReduce-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Float64AddOrReduce-Result-NewValue", newVal))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		newVal, notFound, err = r.float64AddOrReduceInternal(snap, key, val)
		return newVal, notFound, err
	} else {
		return r.float64AddOrReduceInternal(snap, key, val)
	}
}

// float64AddOrReduceInternal will add or reduce float64 value against a key in redis,
// and return the new value if found and performed
func (r *Redis) float64AddOrReduceInternal(snap redisConnSnapshot, key string, val float64) (newVal float64, notFound bool, err error) {
	// validate
	if len(key) <= 0 {
		return 0.00, false, errors.New("Redis Float64AddOrReduce Failed: (IncrByFloat) " + "Key is Required")
	}

	if !snap.ready {
		return 0.00, false, errors.New("Redis Float64AddOrReduce Failed: (IncrByFloat) " + "Endpoint Connections Not Ready")
	}

	cmd := snap.writer.IncrByFloat(snap.writer.Context(), key, val)
	return r.handleFloatCmd(cmd, "Redis Float64AddOrReduce Failed: (IncrByFloat)")
}

// ----------------------------------------------------------------------------------------------------------------
// HyperLogLog functions
// ----------------------------------------------------------------------------------------------------------------

// PFAdd is a HyperLogLog function to uniquely accumulate the count of a specific value to redis,
// such as email hit count, user hit count, ip address hit count etc, that is based on the unique occurences of such value
func (r *Redis) PFAdd(key string, elements ...interface{}) (err error) {
	if r == nil {
		return errors.New("Redis PFAdd Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PFAdd", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PFAdd-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PFAdd-Elements", elements))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = r.pfAddInternal(snap, key, elements...)
		return err
	} else {
		return r.pfAddInternal(snap, key, elements...)
	}
}

// pfAddInternal is a HyperLogLog function to uniquely accumulate the count of a specific value to redis,
// such as email hit count, user hit count, ip address hit count etc, that is based on the unique occurences of such value
func (r *Redis) pfAddInternal(snap redisConnSnapshot, key string, elements ...interface{}) error {
	// validate
	if len(key) <= 0 {
		return errors.New("Redis PFAdd Failed: " + "Key is Required")
	}

	if !snap.ready {
		return errors.New("Redis PFAdd Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.writer.PFAdd(snap.writer.Context(), key, elements...)

	if _, _, err := r.handleIntCmd(cmd, "Redis PFAdd Failed: "); err != nil {
		return err
	} else {
		return nil
	}
}

// PFCount is a HyperLogLog function to retrieve the current count associated with the given unique value in redis,
// Specify one or more keys, if multiple keys used, the result count is the union of all keys' unique value counts
func (r *Redis) PFCount(key ...string) (val int64, notFound bool, err error) {
	if r == nil {
		return 0, false, errors.New("Redis PFCount Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PFCount", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PFCount-Keys", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PFCount-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PFCount-Result-Count", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = r.pfCountInternal(snap, key...)
		return val, notFound, err
	} else {
		return r.pfCountInternal(snap, key...)
	}
}

// pfCountInternal is a HyperLogLog function to retrieve the current count associated with the given unique value in redis,
// Specify one or more keys, if multiple keys used, the result count is the union of all keys' unique value counts
func (r *Redis) pfCountInternal(snap redisConnSnapshot, key ...string) (val int64, notFound bool, err error) {
	// validate
	if len(key) <= 0 {
		return 0, false, errors.New("Redis PFCount Failed: " + "Key is Required")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis PFCount Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.PFCount(snap.reader.Context(), key...)
	return r.handleIntCmd(cmd, "Redis PFCount Failed: ")
}

// PFMerge is a HyperLogLog function to merge two or more HyperLogLog as defined by keys together
func (r *Redis) PFMerge(destKey string, sourceKey ...string) (err error) {
	if r == nil {
		return errors.New("Redis PFMerge Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PFMerge", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PFMerge-DestKey", destKey))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PFMerge-SourceKeys", sourceKey))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = r.pfMergeInternal(snap, destKey, sourceKey...)
		return err
	} else {
		return r.pfMergeInternal(snap, destKey, sourceKey...)
	}
}

// pfMergeInternal is a HyperLogLog function to merge two or more HyperLogLog as defined by keys together
func (r *Redis) pfMergeInternal(snap redisConnSnapshot, destKey string, sourceKey ...string) error {
	// validate
	if len(destKey) <= 0 {
		return errors.New("Redis PFMerge Failed: " + "Destination Key is Required")
	}

	if len(sourceKey) <= 0 {
		return errors.New("Redis PFMerge Failed: " + "Source Key is Required")
	}

	if !snap.ready {
		return errors.New("Redis PFMerge Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.writer.PFMerge(snap.writer.Context(), destKey, sourceKey...)
	return r.handleStatusCmd(cmd, "Redis PFMerge Failed: ")
}

// ----------------------------------------------------------------------------------------------------------------
// Other functions
// ----------------------------------------------------------------------------------------------------------------

// Exists checks if one or more keys exists in redis
//
// foundCount = 0 indicates not found; > 0 indicates found count
func (r *Redis) Exists(key ...string) (foundCount int64, err error) {
	if r == nil {
		return 0, errors.New("Redis Exists Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Exists", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Exists-Keys", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Exists-Result-Count", foundCount))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		foundCount, err = r.existsInternal(snap, key...)
		return foundCount, err
	} else {
		return r.existsInternal(snap, key...)
	}
}

// existsInternal checks if one or more keys exists in redis
//
// foundCount = 0 indicates not found; > 0 indicates found count
func (r *Redis) existsInternal(snap redisConnSnapshot, key ...string) (foundCount int64, err error) {
	// validate
	if len(key) <= 0 {
		return 0, errors.New("Redis Exists Failed: " + "Key is Required")
	}

	if !snap.ready {
		return 0, errors.New("Redis Exists Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.Exists(snap.reader.Context(), key...)
	foundCount, _, err = r.handleIntCmd(cmd, "Redis Exists Failed: ")

	return foundCount, err
}

// StrLen gets the string length of the value stored by the key in redis
func (r *Redis) StrLen(key string) (length int64, notFound bool, err error) {
	if r == nil {
		return 0, false, errors.New("Redis StrLen Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-StrLen", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-StrLen-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-StrLen-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-StrLen-Result-Len", length))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		length, notFound, err = r.strLenInternal(snap, key)
		return length, notFound, err
	} else {
		return r.strLenInternal(snap, key)
	}
}

// strLenInternal gets the string length of the value stored by the key in redis
func (r *Redis) strLenInternal(snap redisConnSnapshot, key string) (length int64, notFound bool, err error) {
	// validate
	if len(key) <= 0 {
		return 0, false, errors.New("Redis StrLen Failed: " + "Key is Required")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis StrLen Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.StrLen(snap.reader.Context(), key)
	if length, _, err = r.handleIntCmd(cmd, "Redis StrLen Failed: "); err != nil {
		return 0, false, err
	}

	// Redis returns 0 for missing keys; detect that and surface notFound=true
	if length == 0 {
		if exists, _, existsErr := r.handleIntCmd(snap.reader.Exists(snap.reader.Context(), key), "Redis StrLen Failed: (Exists) "); existsErr != nil {
			return 0, false, existsErr
		} else if exists == 0 {
			return 0, true, nil
		}
	}

	return length, false, nil
}

// Append will append a value to the existing value under the given key in redis,
// if key does not exist, a new key based on the given key is created
func (r *Redis) Append(key string, valToAppend string) (err error) {
	if r == nil {
		return errors.New("Redis Append Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Append", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Append-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Append-Value", valToAppend))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = r.appendInternal(snap, key, valToAppend)
		return err
	} else {
		return r.appendInternal(snap, key, valToAppend)
	}
}

// appendInternal will append a value to the existing value under the given key in redis,
// if key does not exist, a new key based on the given key is created
func (r *Redis) appendInternal(snap redisConnSnapshot, key string, valToAppend string) error {
	// validate
	if len(key) <= 0 {
		return errors.New("Redis Append Failed: " + "Key is Required")
	}

	if !snap.ready {
		return errors.New("Redis Append Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.writer.Append(snap.writer.Context(), key, valToAppend)
	_, _, err := r.handleIntCmd(cmd)
	return err
}

// Del will delete one or more keys specified from redis
func (r *Redis) Del(key ...string) (deletedCount int64, err error) {
	if r == nil {
		return 0, errors.New("Redis Del Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Del", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Del-Keys", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Del-Result-Deleted-Count", deletedCount))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		deletedCount, err = r.delInternal(snap, key...)
		return deletedCount, err
	} else {
		return r.delInternal(snap, key...)
	}
}

// delInternal will delete one or more keys specified from redis
func (r *Redis) delInternal(snap redisConnSnapshot, key ...string) (deletedCount int64, err error) {
	// validate
	if len(key) <= 0 {
		return 0, errors.New("Redis Del Failed: " + "Key is Required")
	}

	if !snap.ready {
		return 0, errors.New("Redis Del Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.writer.Del(snap.writer.Context(), key...)
	deletedCount, _, err = r.handleIntCmd(cmd, "Redis Del Failed: ")
	return deletedCount, err
}

// Unlink is similar to Del where it removes one or more keys specified from redis,
// however, unlink performs the delete asynchronously and is faster than Del
func (r *Redis) Unlink(key ...string) (unlinkedCount int64, err error) {
	if r == nil {
		return 0, errors.New("Redis Unlink Failed: Redis receiver is nil")
	}
	snap := r.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Unlink", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Unlink-Keys", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Unlink-Result-Unlinked-Count", unlinkedCount))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		unlinkedCount, err = r.unlinkInternal(snap, key...)
		return unlinkedCount, err
	} else {
		return r.unlinkInternal(snap, key...)
	}
}

// unlinkInternal is similar to Del where it removes one or more keys specified from redis,
// however, unlink performs the delete asynchronously and is faster than Del
func (r *Redis) unlinkInternal(snap redisConnSnapshot, key ...string) (unlinkedCount int64, err error) {
	// validate
	if len(key) <= 0 {
		return 0, errors.New("Redis Unlink Failed: " + "Key is Required")
	}

	if !snap.ready {
		return 0, errors.New("Redis Unlink Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.writer.Unlink(snap.writer.Context(), key...)
	unlinkedCount, _, err = r.handleIntCmd(cmd, "Redis Unlink Failed: ")
	return unlinkedCount, err
}

// ----------------------------------------------------------------------------------------------------------------
// BIT functions
// ----------------------------------------------------------------------------------------------------------------

// SetBit will set or clear (1 or 0) the bit at offset in the string value stored by the key in redis,
// If the key doesn't exist, a new key with the key defined is created,
// The string holding bit value will grow as needed when offset exceeds the string, grown value defaults with bit 0
//
// bit range = left 0 -> right 8 = byte
func (b *BIT) SetBit(key string, offset int64, bitValue bool) (err error) {
	if b == nil {
		return errors.New("Redis BIT SetBit Failed: BIT receiver is nil")
	}
	if b.core == nil {
		return errors.New("Redis BIT SetBit Failed: Redis core is nil")
	}
	snap := b.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SetBit", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SetBit-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SetBit-Offset", offset))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SetBit-Bit-Value", bitValue))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = b.setBitInternal(snap, key, offset, bitValue)
		return err
	} else {
		return b.setBitInternal(snap, key, offset, bitValue)
	}
}

// setBitInternal will set or clear (1 or 0) the bit at offset in the string value stored by the key in redis,
// If the key doesn't exist, a new key with the key defined is created,
// The string holding bit value will grow as needed when offset exceeds the string, grown value defaults with bit 0
//
// bit range = left 0 -> right 8 = byte
func (b *BIT) setBitInternal(snap redisConnSnapshot, key string, offset int64, bitValue bool) error {
	// validate
	if b.core == nil {
		return errors.New("Redis SetBit Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis SetBit Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis SetBit Failed: " + "Key is Required")
	}

	if offset < 0 {
		return errors.New("Redis SetBit Failed: " + "Offset is 0 or Greater")
	}

	v := 0

	if bitValue {
		v = 1
	}

	cmd := snap.writer.SetBit(snap.writer.Context(), key, offset, v)
	_, _, err := b.core.handleIntCmd(cmd, "Redis SetBit Failed: ")
	return err
}

// GetBit will return the bit value (1 or 0) at offset position of the value for the key in redis
// If key is not found or offset is greater than key's value, then blank string is assumed and bit 0 is returned
func (b *BIT) GetBit(key string, offset int64) (val int, err error) {
	if b == nil {
		return 0, errors.New("Redis BIT GetBit Failed: BIT receiver is nil")
	}
	if b.core == nil {
		return 0, errors.New("Redis BIT GetBit Failed: Redis core is nil")
	}
	snap := b.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GetBit", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GetBit-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GetBit-Offset", offset))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GetBit-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, err = b.getBitInternal(snap, key, offset)
		return val, err
	} else {
		return b.getBitInternal(snap, key, offset)
	}
}

// getBitInternal will return the bit value (1 or 0) at offset position of the value for the key in redis
// If key is not found or offset is greater than key's value, then blank string is assumed and bit 0 is returned
func (b *BIT) getBitInternal(snap redisConnSnapshot, key string, offset int64) (val int, err error) {
	// validate
	if b.core == nil {
		return 0, errors.New("Redis GetBit Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, errors.New("Redis GetBit Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, errors.New("Redis GetBit Failed: " + "Key is Required")
	}

	if offset < 0 {
		return 0, errors.New("Redis GetBit Failed: " + "Offset is 0 or Greater")
	}

	cmd := snap.reader.GetBit(snap.reader.Context(), key, offset)
	v, _, e := b.core.handleIntCmd(cmd, "Redis GetBit Failed: ")
	val = int(v)
	return val, e
}

// BitCount counts the number of set bits (population counting of bits that are 1) in a string,
//
// offsetFrom = evaluate bitcount begin at offsetFrom position
// offsetTo = evaluate bitcount until offsetTo position
func (b *BIT) BitCount(key string, offsetFrom int64, offsetTo int64) (valCount int64, err error) {
	if b == nil {
		return 0, errors.New("Redis BIT BitCount Failed: BIT receiver is nil")
	}
	if b.core == nil {
		return 0, errors.New("Redis BIT BitCount Failed: Redis core is nil")
	}
	snap := b.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-BitCount", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitCount-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitCount-Offset-From", offsetFrom))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitCount-Offset-To", offsetTo))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitCount-Result-Count", valCount))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valCount, err = b.bitCountInternal(snap, key, offsetFrom, offsetTo)
		return valCount, err
	} else {
		return b.bitCountInternal(snap, key, offsetFrom, offsetTo)
	}
}

// bitCountInternal counts the number of set bits (population counting of bits that are 1) in a string,
//
// offsetFrom = evaluate bitcount begin at offsetFrom position
// offsetTo = evaluate bitcount until offsetTo position
func (b *BIT) bitCountInternal(snap redisConnSnapshot, key string, offsetFrom int64, offsetTo int64) (valCount int64, err error) {
	// validate
	if b.core == nil {
		return 0, errors.New("Redis BitCount Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, errors.New("Redis BitCount Failed: " + "Endpoint Connections Not Ready")
	}

	bc := new(redis.BitCount)

	bc.Start = offsetFrom
	bc.End = offsetTo

	cmd := snap.reader.BitCount(snap.reader.Context(), key, bc)
	valCount, _, err = b.core.handleIntCmd(cmd, "Redis BitCount Failed: ")
	return valCount, err
}

// BitField treats redis string as array of bits
// See detail at https://redis.io/commands/bitfield
//
// Supported Sub Commands:
//
//	GET <type> <offset> -- returns the specified bit field
//	SET <type> <offset> <value> -- sets the specified bit field and returns its old value
//	INCRBY <type> <offset> <increment> -- increments or decrements (if negative) the specified bit field and returns the new value
//
// Notes:
//
//	i = if integer type, i can be preceeded to indicate signed integer, such as i5 = signed integer 5
//	u = if integer type, u can be preceeded to indicate unsigned integer, such as u5 = unsigned integer 5
//	# = if offset is preceeded with #, the specified offset is multiplied by the type width, such as #0 = 0, #1 = 8 when type if 8-bit byte
func (b *BIT) BitField(key string, args ...interface{}) (valBits []int64, err error) {
	if b == nil {
		return nil, errors.New("Redis BIT BitField Failed: BIT receiver is nil")
	}
	if b.core == nil {
		return nil, errors.New("Redis BIT BitField Failed: Redis core is nil")
	}
	snap := b.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-BitField", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitField-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitField-Input-Args", args))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitField-Input-Result", valBits))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valBits, err = b.bitFieldInternal(snap, key, args...)
		return valBits, err
	} else {
		return b.bitFieldInternal(snap, key, args...)
	}
}

// bitFieldInternal treats redis string as array of bits
// See detail at https://redis.io/commands/bitfield
//
// Supported Sub Commands:
//
//	GET <type> <offset> -- returns the specified bit field
//	SET <type> <offset> <value> -- sets the specified bit field and returns its old value
//	INCRBY <type> <offset> <increment> -- increments or decrements (if negative) the specified bit field and returns the new value
//
// Notes:
//
//	i = if integer type, i can be preceeded to indicate signed integer, such as i5 = signed integer 5
//	u = if integer type, u can be preceeded to indicate unsigned integer, such as u5 = unsigned integer 5
//	# = if offset is preceeded with #, the specified offset is multiplied by the type width, such as #0 = 0, #1 = 8 when type if 8-bit byte
func (b *BIT) bitFieldInternal(snap redisConnSnapshot, key string, args ...interface{}) (valBits []int64, err error) {
	// validate
	if b.core == nil {
		return nil, errors.New("Redis BitField Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis BitField Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, errors.New("Redis BitField Failed: " + "Key is Required")
	}

	if len(args) <= 0 {
		return nil, errors.New("Redis BitField Failed: " + "Args is Required")
	}

	cmd := snap.writer.BitField(snap.writer.Context(), key, args...)
	valBits, _, err = b.core.handleIntSliceCmd(cmd, "Redis BitField Failed: ")
	return valBits, err
}

// BitOpAnd performs bitwise operation between multiple keys (containing string value),
// stores the result in the destination key,
// if operation failed, error is returned, if success, nil is returned
//
// Supported:
//
//	And, Or, XOr, Not
func (b *BIT) BitOp(keyDest string, bitOpType redisbitop.RedisBitop, keySource ...string) (err error) {
	if b == nil {
		return errors.New("Redis BIT BitOp Failed: BIT receiver is nil")
	}
	if b.core == nil {
		return errors.New("Redis BIT BitOp Failed: Redis core is nil")
	}
	snap := b.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-BitOp", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitOp-KeyDest", keyDest))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitOp-OpType", bitOpType))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitOp-KeySource", keySource))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = b.bitOpInternal(snap, keyDest, bitOpType, keySource...)
		return err
	} else {
		return b.bitOpInternal(snap, keyDest, bitOpType, keySource...)
	}
}

// bitOpInternal performs bitwise operation between multiple keys (containing string value),
// stores the result in the destination key,
// if operation failed, error is returned, if success, nil is returned
//
// Supported:
//
//	And, Or, XOr, Not
func (b *BIT) bitOpInternal(snap redisConnSnapshot, keyDest string, bitOpType redisbitop.RedisBitop, keySource ...string) error {
	// validate
	if b.core == nil {
		return errors.New("Redis BitOp Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis BitOp Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis BitOp Failed: " + "Key Destination is Required")
	}

	if !bitOpType.Valid() || bitOpType == redisbitop.UNKNOWN {
		return errors.New("Redis BitOp Failed: " + "BitOp Type Not Valid")
	}

	if bitOpType != redisbitop.NOT {
		if len(keySource) <= 1 {
			return errors.New("Redis BitOp Failed: " + "Key Source Must Be 2 Or More")
		}
	} else {
		if len(keySource) != 1 {
			return errors.New("Redis BitOp-Not Failed: " + "Key Source Must Be Singular")
		}
	}

	var cmd *redis.IntCmd

	switch bitOpType {
	case redisbitop.And:
		cmd = snap.writer.BitOpAnd(snap.writer.Context(), keyDest, keySource...)
	case redisbitop.Or:
		cmd = snap.writer.BitOpOr(snap.writer.Context(), keyDest, keySource...)
	case redisbitop.XOr:
		cmd = snap.writer.BitOpXor(snap.writer.Context(), keyDest, keySource...)
	case redisbitop.NOT:
		cmd = snap.writer.BitOpNot(snap.writer.Context(), keyDest, keySource[0])
	default:
		return errors.New("Redis BitOp Failed: " + "BitOp Type Not Expected")
	}

	_, _, err := b.core.handleIntCmd(cmd, "Redis BitOp Failed: ")
	return err
}

// BitPos returns the position of the first bit set to 1 or 0 (as requested via input query) in a string,
// position of bit is returned from left to right,
// first byte most significant bit is 0 on left most,
// second byte most significant bit is at position 8 (after the first byte right most bit 7), and so on
//
// bitValue = 1 or 0
// startPosition = bit pos start from this bit offset position
func (b *BIT) BitPos(key string, bitValue int64, startPosition ...int64) (valPosition int64, err error) {
	if b == nil {
		return 0, errors.New("Redis BIT BitPos Failed: BIT receiver is nil")
	}
	if b.core == nil {
		return 0, errors.New("Redis BIT BitPos Failed: Redis core is nil")
	}
	snap := b.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-BitPos", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitPos-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitPos-BitValue", bitValue))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitPos-Start-Position", startPosition))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-BitPos-Result-Position", valPosition))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valPosition, err = b.bitPosInternal(snap, key, bitValue, startPosition...)
		return valPosition, err
	} else {
		return b.bitPosInternal(snap, key, bitValue, startPosition...)
	}
}

// bitPosInternal returns the position of the first bit set to 1 or 0 (as requested via input query) in a string,
// position of bit is returned from left to right,
// first byte most significant bit is 0 on left most,
// second byte most significant bit is at position 8 (after the first byte right most bit 7), and so on
//
// bitValue = 1 or 0
// startPosition = bit pos start from this bit offset position
func (b *BIT) bitPosInternal(snap redisConnSnapshot, key string, bitValue int64, startPosition ...int64) (valPosition int64, err error) {
	// validate
	if b.core == nil {
		return 0, errors.New("Redis BitPos Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, errors.New("Redis BitPos Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, errors.New("Redis BitPos Failed: " + "Key is Required")
	}

	if bitValue != 0 && bitValue != 1 {
		return 0, errors.New("Redis BitPos Failed: " + "Bit Value Must Be 1 or 0")
	}

	cmd := snap.reader.BitPos(snap.reader.Context(), key, bitValue, startPosition...)
	valPosition, _, err = b.core.handleIntCmd(cmd, "Redis BitPos Failed: ")
	return valPosition, err
}

// ----------------------------------------------------------------------------------------------------------------
// LIST functions
// ----------------------------------------------------------------------------------------------------------------

// LSet will set element to the list index
func (l *LIST) LSet(key string, index int64, value interface{}) (err error) {
	if l == nil {
		return errors.New("Redis LIST LSet Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return errors.New("Redis LIST LSet Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LSet", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LSet-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LSet-Index", index))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LSet-value", value))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = l.lsetInternal(snap, key, index, value)
		return err
	} else {
		return l.lsetInternal(snap, key, index, value)
	}
}

// lsetInternal will set element to the list index
func (l *LIST) lsetInternal(snap redisConnSnapshot, key string, index int64, value interface{}) error {
	// validate
	if l.core == nil {
		return errors.New("Redis LSet Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis LSet Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis LSet Failed: " + "Key is Required")
	}

	if value == nil {
		return errors.New("Redis LSet Failed: " + "Value is Required")
	}

	cmd := snap.writer.LSet(snap.writer.Context(), key, index, value)
	return l.core.handleStatusCmd(cmd, "Redis LSet Failed: ")
}

// LInsert will insert a value either before or after the pivot element
func (l *LIST) LInsert(key string, bBefore bool, pivot interface{}, value interface{}) (err error) {
	if l == nil {
		return errors.New("Redis LIST LInsert Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return errors.New("Redis LIST LInsert Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LInsert", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LInsert-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LInsert-Insert-Before", bBefore))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LInsert-Pivot-Element", pivot))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LInsert-Insert-Value", value))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = l.linsertInternal(snap, key, bBefore, pivot, value)
		return err
	} else {
		return l.linsertInternal(snap, key, bBefore, pivot, value)
	}
}

// linsertInternal will insert a value either before or after the pivot element
func (l *LIST) linsertInternal(snap redisConnSnapshot, key string, bBefore bool, pivot interface{}, value interface{}) error {
	// validate
	if l.core == nil {
		return errors.New("Redis LInsert Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis LInsert Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis LInsert Failed: " + "Key is Required")
	}

	if pivot == nil {
		return errors.New("Redis LInsert Failed: " + "Pivot is Required")
	}

	if value == nil {
		return errors.New("Redis LInsert Failed: " + "Value is Required")
	}

	var cmd *redis.IntCmd

	if bBefore {
		cmd = snap.writer.LInsertBefore(snap.writer.Context(), key, pivot, value)
	} else {
		cmd = snap.writer.LInsertAfter(snap.writer.Context(), key, pivot, value)
	}

	_, _, err := l.core.handleIntCmd(cmd, "Redis LInsert Failed: ")
	return err
}

// LPush stores all the specified values at the head of the list as defined by the key,
// if key does not exist, then empty list is created before performing the push operation (unless keyMustExist bit is set)
//
// Elements are inserted one after the other to the head of the list, from the leftmost to the rightmost,
// for example, LPush mylist a b c will result in a list containing c as first element, b as second element, and a as third element
//
// error is returned if the key is not holding a value of type list
func (l *LIST) LPush(key string, keyMustExist bool, value ...interface{}) (err error) {
	if l == nil {
		return errors.New("Redis LIST LPush Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return errors.New("Redis LIST LPush Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LPush", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LPush-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LPush-Key-Must-Exist", keyMustExist))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LPush-Values", value))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = l.lpushInternal(snap, key, keyMustExist, value...)
		return err
	} else {
		return l.lpushInternal(snap, key, keyMustExist, value...)
	}
}

// lpushInternal stores all the specified values at the head of the list as defined by the key,
// if key does not exist, then empty list is created before performing the push operation (unless keyMustExist bit is set)
//
// Elements are inserted one after the other to the head of the list, from the leftmost to the rightmost,
// for example, LPush mylist a b c will result in a list containing c as first element, b as second element, and a as third element
//
// error is returned if the key is not holding a value of type list
func (l *LIST) lpushInternal(snap redisConnSnapshot, key string, keyMustExist bool, value ...interface{}) error {
	// validate
	if l.core == nil {
		return errors.New("Redis LPush Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis LPush Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis LPush Failed: " + "Key is Required")
	}

	if len(value) <= 0 {
		return errors.New("Redis LPush Failed: " + "At Least 1 Value is Required")
	}

	var cmd *redis.IntCmd

	if !keyMustExist {
		cmd = snap.writer.LPush(snap.writer.Context(), key, value...)
	} else {
		cmd = snap.writer.LPushX(snap.writer.Context(), key, value...)
	}

	return l.core.handleIntCmd2(cmd, "Redis LPush Failed: ")
}

// RPush stores all the specified values at the tail of the list as defined by the key,
// if key does not exist, then empty list is created before performing the push operation (unless keyMustExist bit is set)
//
// Elements are inserted one after the other to the tail of the list, from the leftmost to the rightmost,
// for example, RPush mylist a b c will result in a list containing a as first element, b as second element, and c as third element
//
// error is returned if the key is not holding a value of type list
func (l *LIST) RPush(key string, keyMustExist bool, value ...interface{}) (err error) {
	if l == nil {
		return errors.New("Redis LIST RPush Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return errors.New("Redis LIST RPush Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-RPush", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPush-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPush-Key-Must-Exist", keyMustExist))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPush-Values", value))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = l.rpushInternal(snap, key, keyMustExist, value...)
		return err
	} else {
		return l.rpushInternal(snap, key, keyMustExist, value...)
	}
}

// rpushInternal stores all the specified values at the tail of the list as defined by the key,
// if key does not exist, then empty list is created before performing the push operation (unless keyMustExist bit is set)
//
// Elements are inserted one after the other to the tail of the list, from the leftmost to the rightmost,
// for example, RPush mylist a b c will result in a list containing a as first element, b as second element, and c as third element
//
// error is returned if the key is not holding a value of type list
func (l *LIST) rpushInternal(snap redisConnSnapshot, key string, keyMustExist bool, value ...interface{}) error {
	// validate
	if l.core == nil {
		return errors.New("Redis RPush Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis RPush Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis RPush Failed: " + "Key is Required")
	}

	if len(value) <= 0 {
		return errors.New("Redis RPush Failed: " + "At Least 1 Value is Required")
	}

	var cmd *redis.IntCmd

	if !keyMustExist {
		cmd = snap.writer.RPush(snap.writer.Context(), key, value...)
	} else {
		cmd = snap.writer.RPushX(snap.writer.Context(), key, value...)
	}

	return l.core.handleIntCmd2(cmd, "Redis RPush Failed: ")
}

// LPop will remove and return the first element from the list stored at key
func (l *LIST) LPop(key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	if l == nil {
		return false, errors.New("Redis LIST LPop Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return false, errors.New("Redis LIST LPop Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LPop", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LPop-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LPop-Output-Data-Type", outputDataType))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LPop-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LPop-Output-Object", outputObjectPtr))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		notFound, err = l.lpopInternal(snap, key, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return l.lpopInternal(snap, key, outputDataType, outputObjectPtr)
	}
}

// lpopInternal will remove and return the first element from the list stored at key
func (l *LIST) lpopInternal(snap redisConnSnapshot, key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if l.core == nil {
		return false, errors.New("Redis LPop Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return false, errors.New("Redis LPop Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return false, errors.New("Redis LPop Failed: " + "Key is Required")
	}

	if !outputDataType.Valid() || outputDataType == redisdatatype.UNKNOWN {
		return false, errors.New("Redis LPop Failed: " + "Output Data Type is Required")
	}

	if outputObjectPtr == nil {
		return false, errors.New("Redis LPop Failed: " + "Output Object Pointer is Required")
	}

	cmd := snap.writer.LPop(snap.writer.Context(), key)
	return l.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis LPop Failed: ")
}

// RPop removes and returns the last element of the list stored at key
func (l *LIST) RPop(key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	if l == nil {
		return false, errors.New("Redis LIST RPop Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return false, errors.New("Redis LIST RPop Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-RPop", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPop-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPop-Output-Data-Type", outputDataType))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPop-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPop-Output-Object", outputObjectPtr))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		notFound, err = l.rpopInternal(snap, key, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return l.rpopInternal(snap, key, outputDataType, outputObjectPtr)
	}
}

// rpopInternal removes and returns the last element of the list stored at key
func (l *LIST) rpopInternal(snap redisConnSnapshot, key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if l.core == nil {
		return false, errors.New("Redis RPop Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return false, errors.New("Redis RPop Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return false, errors.New("Redis RPop Failed: " + "Key is Required")
	}

	if !outputDataType.Valid() || outputDataType == redisdatatype.UNKNOWN {
		return false, errors.New("Redis RPop Failed: " + "Output Data Type is Required")
	}

	if outputObjectPtr == nil {
		return false, errors.New("Redis RPop Failed: " + "Output Object Pointer is Required")
	}

	cmd := snap.writer.RPop(snap.writer.Context(), key)
	return l.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis RPop Failed: ")
}

// RPopLPush will atomically remove and return last element of the list stored at keySource,
// and then push the returned element at first element position (head) of the list stored at keyDest
func (l *LIST) RPopLPush(keySource string, keyDest string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	if l == nil {
		return false, errors.New("Redis LIST RPopLPush Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return false, errors.New("Redis LIST RPopLPush Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-RPopLPush", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPopLPush-KeySource", keySource))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPopLPush-KeyDest", keyDest))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPopLPush-Output-Data-Type", outputDataType))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPopLPush-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RPopLPush-Output-Object", outputObjectPtr))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		notFound, err = l.rpopLPushInternal(snap, keySource, keyDest, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return l.rpopLPushInternal(snap, keySource, keyDest, outputDataType, outputObjectPtr)
	}
}

// rpopLPushInternal will atomically remove and return last element of the list stored at keySource,
// and then push the returned element at first element position (head) of the list stored at keyDest
func (l *LIST) rpopLPushInternal(snap redisConnSnapshot, keySource string, keyDest string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if l.core == nil {
		return false, errors.New("Redis RPopLPush Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return false, errors.New("Redis RPopLPush Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keySource) <= 0 {
		return false, errors.New("Redis RPopLPush Failed: " + "Key Source is Required")
	}

	if len(keyDest) <= 0 {
		return false, errors.New("Redis RPopLPush Failed: " + "Key Destination is Required")
	}

	if !outputDataType.Valid() || outputDataType == redisdatatype.UNKNOWN {
		return false, errors.New("Redis RPopLPush Failed: " + "Output Data Type is Required")
	}

	if outputObjectPtr == nil {
		return false, errors.New("Redis RPopLPush Failed: " + "Output Object Pointer is Required")
	}

	cmd := snap.writer.RPopLPush(snap.writer.Context(), keySource, keyDest)
	return l.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis RPopLPush Failed: ")
}

// LIndex returns the element by list index position, for the value stored in list by key,
// Index is zero-based,
// Negative Index can be used to denote reverse order,
//
//	such as -1 = last element
//	such as -2 = second to last element, and so on
//
// Error is returned if value at key is not a list
func (l *LIST) LIndex(key string, index int64, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	if l == nil {
		return false, errors.New("Redis LIST LIndex Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return false, errors.New("Redis LIST LIndex Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LIndex", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LIndex-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LIndex-Index", index))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LIndex-Output-Data-Type", outputDataType))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LIndex-Output-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LIndex-Output-Object", outputObjectPtr))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		notFound, err = l.lindexInternal(snap, key, index, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return l.lindexInternal(snap, key, index, outputDataType, outputObjectPtr)
	}
}

// lindexInternal returns the element by list index position, for the value stored in list by key,
// Index is zero-based,
// Negative Index can be used to denote reverse order,
//
//	such as -1 = last element
//	such as -2 = second to last element, and so on
//
// Error is returned if value at key is not a list
func (l *LIST) lindexInternal(snap redisConnSnapshot, key string, index int64, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if l.core == nil {
		return false, errors.New("Redis LIndex Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return false, errors.New("Redis LIndex Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return false, errors.New("Redis LIndex Failed: " + "Key is Required")
	}

	cmd := snap.reader.LIndex(snap.reader.Context(), key, index)
	return l.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis LIndex Failed: ")
}

// LLen returns the length of the list stored at key,
// if key does not exist, it is treated as empty list and 0 is returned,
//
// Error is returned if value at key is not a list
func (l *LIST) LLen(key string) (val int64, notFound bool, err error) {
	if l == nil {
		return 0, false, errors.New("Redis LIST LLen Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return 0, false, errors.New("Redis LIST LLen Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LLen", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LLen-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LLen-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LLen-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = l.llenInternal(snap, key)
		return val, notFound, err
	} else {
		return l.llenInternal(snap, key)
	}
}

// llenInternal returns the length of the list stored at key,
// if key does not exist, it is treated as empty list and 0 is returned,
//
// Error is returned if value at key is not a list
func (l *LIST) llenInternal(snap redisConnSnapshot, key string) (val int64, notFound bool, err error) {
	// validate
	if l.core == nil {
		return 0, false, errors.New("Redis LLen Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis LLen Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis LLen Failed: " + "Key is Required")
	}

	cmd := snap.reader.LLen(snap.reader.Context(), key)
	return l.core.handleIntCmd(cmd, "Redis LLen Failed: ")
}

// LRange returns the specified elements of the list stored at key,
// Offsets start and stop are zero based indexes,
// Offsets can be negative, where -1 is the last element, while -2 is next to last element, and so on,
//
// Offsets start > stop, empty list is returned,
// Offsets stop > last element, stop uses last element instead
//
// Example:
//
//	start top = 0 - 10 = returns 11 elements (0 to 10 = 11)
func (l *LIST) LRange(key string, start int64, stop int64) (outputSlice []string, notFound bool, err error) {
	if l == nil {
		return nil, false, errors.New("Redis LIST LRange Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return nil, false, errors.New("Redis LIST LRange Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LRange", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LRange-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LRange-Start", start))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LRange-Stop", stop))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LRange-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LRange-Result", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = l.lrangeInternal(snap, key, start, stop)
		return outputSlice, notFound, err
	} else {
		return l.lrangeInternal(snap, key, start, stop)
	}
}

// lrangeInternal returns the specified elements of the list stored at key,
// Offsets start and stop are zero based indexes,
// Offsets can be negative, where -1 is the last element, while -2 is next to last element, and so on,
//
// Offsets start > stop, empty list is returned,
// Offsets stop > last element, stop uses last element instead
//
// Example:
//
//	start top = 0 - 10 = returns 11 elements (0 to 10 = 11)
func (l *LIST) lrangeInternal(snap redisConnSnapshot, key string, start int64, stop int64) (outputSlice []string, notFound bool, err error) {
	// validate
	if l.core == nil {
		return nil, false, errors.New("Redis LRange Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis LRange Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis LRange Failed: " + "Key is Required")
	}

	cmd := snap.reader.LRange(snap.reader.Context(), key, start, stop)
	return l.core.handleStringSliceCmd(cmd, "Redis LRange Failed: ")
}

// LRem removes the first count occurrences of elements equal to 'element value' from list stored at key
// count indicates number of occurrences
//
// count > 0 = removes elements equal to 'element value' moving from head to tail
// count < 0 = removes elements equal to 'element value' moving from tail to head
// count = 0 = removes all elements equal to 'element value'
//
// Example:
//
//	LREM list 02 "hello" = removes the last two occurrences of "hello" in the list stored at key named 'list'
func (l *LIST) LRem(key string, count int64, value interface{}) (err error) {
	if l == nil {
		return errors.New("Redis LIST LRem Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return errors.New("Redis LIST LRem Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LRem", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LRem-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LRem-Count", count))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LRem-Value", value))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = l.lremInternal(snap, key, count, value)
		return err
	} else {
		return l.lremInternal(snap, key, count, value)
	}
}

// lremInternal removes the first count occurrences of elements equal to 'element value' from list stored at key
// count indicates number of occurrences
//
// count > 0 = removes elements equal to 'element value' moving from head to tail
// count < 0 = removes elements equal to 'element value' moving from tail to head
// count = 0 = removes all elements equal to 'element value'
//
// Example:
//
//	LREM list 02 "hello" = removes the last two occurrences of "hello" in the list stored at key named 'list'
func (l *LIST) lremInternal(snap redisConnSnapshot, key string, count int64, value interface{}) error {
	// validate
	if l.core == nil {
		return errors.New("Redis LRem Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis LRem Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis LRem Failed: " + "Key is Required")
	}

	if value == nil {
		return errors.New("Redis LRem Failed: " + "Value is Required")
	}

	cmd := snap.writer.LRem(snap.writer.Context(), key, count, value)
	return l.core.handleIntCmd2(cmd, "Redis LRem Failed: ")
}

// LTrim will trim an existing list so that it will contian only the specified range of elements specified,
// Both start and stop are zero-based indexes,
// Both start and stop can be negative, where -1 is the last element, while -2 is the second to last element
//
// Example:
//
//	LTRIM foobar 0 2 = modifies the list store at key named 'foobar' so that only the first 3 elements of the list will remain
func (l *LIST) LTrim(key string, start int64, stop int64) (err error) {
	if l == nil {
		return errors.New("Redis LIST LTrim Failed: LIST receiver is nil")
	}
	if l.core == nil {
		return errors.New("Redis LIST LTrim Failed: Redis core is nil")
	}
	snap := l.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LTrim", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LTrim-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LTrim-Start", start))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LTrim-Stop", stop))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = l.ltrimInternal(snap, key, start, stop)
		return err
	} else {
		return l.ltrimInternal(snap, key, start, stop)
	}
}

// ltrimInternal will trim an existing list so that it will contian only the specified range of elements specified,
// Both start and stop are zero-based indexes,
// Both start and stop can be negative, where -1 is the last element, while -2 is the second to last element
//
// Example:
//
//	LTRIM foobar 0 2 = modifies the list store at key named 'foobar' so that only the first 3 elements of the list will remain
func (l *LIST) ltrimInternal(snap redisConnSnapshot, key string, start int64, stop int64) error {
	// validate
	if l.core == nil {
		return errors.New("Redis LTrim Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis LTrim Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis LTrim Failed: " + "Key is Required")
	}

	cmd := snap.writer.LTrim(snap.writer.Context(), key, start, stop)
	return l.core.handleStatusCmd(cmd, "Redis LTrim Failed: ")
}

// ----------------------------------------------------------------------------------------------------------------
// HASH functions
// ----------------------------------------------------------------------------------------------------------------

// HExists returns if field is an existing field in the hash stored at key
//
// 1 = exists; 0 = not exist or key not exist
func (h *HASH) HExists(key string, field string) (valExists bool, err error) {
	if h == nil {
		return false, errors.New("Redis HASH HExists Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return false, errors.New("Redis HASH HExists Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HExists", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HExists-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HExists-Field", field))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HExists-Result-Exists", valExists))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valExists, err = h.hexistsInternal(snap, key, field)
		return valExists, err
	} else {
		return h.hexistsInternal(snap, key, field)
	}
}

// hexistsInternal returns if field is an existing field in the hash stored at key
//
// 1 = exists; 0 = not exist or key not exist
func (h *HASH) hexistsInternal(snap redisConnSnapshot, key string, field string) (valExists bool, err error) {
	// validate
	if h.core == nil {
		return false, errors.New("Redis HExists Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return false, errors.New("Redis HExists Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return false, errors.New("Redis HExists Failed: " + "Key is Required")
	}

	if len(field) <= 0 {
		return false, errors.New("Redis HExists Failed: " + "Field is Required")
	}

	cmd := snap.reader.HExists(snap.reader.Context(), key, field)
	return h.core.handleBoolCmd(cmd, "Redis HExists Failed: ")
}

// HLen returns the number of fields contained in the hash stored at key
func (h *HASH) HLen(key string) (valLen int64, notFound bool, err error) {
	if h == nil {
		return 0, false, errors.New("Redis HASH HLen Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return 0, false, errors.New("Redis HASH HLen Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HLen", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HLen-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HLen-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HLen-Result-Length", valLen))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valLen, notFound, err = h.hlenInternal(snap, key)
		return valLen, notFound, err
	} else {
		return h.hlenInternal(snap, key)
	}
}

// hlenInternal returns the number of fields contained in the hash stored at key
func (h *HASH) hlenInternal(snap redisConnSnapshot, key string) (valLen int64, notFound bool, err error) {
	// validate
	if h.core == nil {
		return 0, false, errors.New("Redis HLen Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis HLen Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis HLen Failed: " + "Key is Required")
	}

	cmd := snap.reader.HLen(snap.reader.Context(), key)
	return h.core.handleIntCmd(cmd, "Redis HLen Failed: ")
}

// HSet will set 'field' in hash stored at key to 'value',
// if key does not exist, a new key holding a hash is created,
//
// if 'field' already exists in the hash, it will be overridden
// if 'field' does not exist, it will be added
func (h *HASH) HSet(key string, value ...interface{}) (err error) {
	if h == nil {
		return errors.New("Redis HASH HSet Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return errors.New("Redis HASH HSet Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HSet", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HSet-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HSet-Values", value))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = h.hsetInternal(snap, key, value...)
		return err
	} else {
		return h.hsetInternal(snap, key, value...)
	}
}

// hsetInternal will set 'field' in hash stored at key to 'value',
// if key does not exist, a new key holding a hash is created,
//
// if 'field' already exists in the hash, it will be overridden
// if 'field' does not exist, it will be added
func (h *HASH) hsetInternal(snap redisConnSnapshot, key string, value ...interface{}) error {
	// validate
	if h.core == nil {
		return errors.New("Redis HSet Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis HSet Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis HSet Failed: " + "Key is Required")
	}

	if len(value) <= 0 {
		return errors.New("Redis HSet Failed: " + "At Least 1 Value is Required")
	}

	cmd := snap.writer.HSet(snap.writer.Context(), key, value...)
	return h.core.handleIntCmd2(cmd, "Redis HSet Failed: ")
}

// HSetNX will set 'field' in hash stored at key to 'value',
// if 'field' does not currently existing in hash
//
// note:
//
//	'field' must not yet exist in hash, otherwise will not add
func (h *HASH) HSetNX(key string, field string, value interface{}) (err error) {
	if h == nil {
		return errors.New("Redis HASH HSetNX Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return errors.New("Redis HASH HSetNX Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HSetNX", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HSetNX-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HSetNX-Field", field))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HSetNX-Value", value))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = h.hsetNXInternal(snap, key, field, value)
		return err
	} else {
		return h.hsetNXInternal(snap, key, field, value)
	}
}

// hsetNXInternal will set 'field' in hash stored at key to 'value',
// if 'field' does not currently existing in hash
//
// note:
//
//	'field' must not yet exist in hash, otherwise will not add
func (h *HASH) hsetNXInternal(snap redisConnSnapshot, key string, field string, value interface{}) error {
	// validate
	if h.core == nil {
		return errors.New("Redis HSetNX Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis HSetNX Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis HSetNX Failed: " + "Key is Required")
	}

	if len(field) <= 0 {
		return errors.New("Redis HSetNX Failed: " + "Field is Required")
	}

	if value == nil {
		return errors.New("Redis HSetNX Failed: " + "Value is Required")
	}

	cmd := snap.writer.HSetNX(snap.writer.Context(), key, field, value)

	if val, err := h.core.handleBoolCmd(cmd, "Redis HSetNX Failed: "); err != nil {
		return err
	} else {
		if val {
			// success
			return nil
		} else {
			// error
			return errors.New("Redis HSetNX Failed: " + "Action Result Yielded False")
		}
	}
}

// HGet returns the value associated with 'field' in the hash stored at key
func (h *HASH) HGet(key string, field string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	if h == nil {
		return false, errors.New("Redis HASH HGet Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return false, errors.New("Redis HASH HGet Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HGet", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HGet-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HGet-Field", field))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HGet-Output-Data-Type", outputDataType))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HGet-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HGet-Output-Object", outputObjectPtr))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		notFound, err = h.hgetInternal(snap, key, field, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return h.hgetInternal(snap, key, field, outputDataType, outputObjectPtr)
	}
}

// hgetInternal returns the value associated with 'field' in the hash stored at key
func (h *HASH) hgetInternal(snap redisConnSnapshot, key string, field string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if h.core == nil {
		return false, errors.New("Redis HGet Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return false, errors.New("Redis HGet Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return false, errors.New("Redis HGet Failed: " + "Key is Required")
	}

	if len(field) <= 0 {
		return false, errors.New("Redis HGet Failed: " + "Field is Required")
	}

	if !outputDataType.Valid() || outputDataType == redisdatatype.UNKNOWN {
		return false, errors.New("Redis HGet Failed: " + "Output Data Type is Required")
	}

	if outputObjectPtr == nil {
		return false, errors.New("Redis HGet Failed: " + "Output Object Pointer is Required")
	}

	cmd := snap.reader.HGet(snap.reader.Context(), key, field)
	return h.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis HGet Failed: ")
}

// HGetAll returns all fields and values of the hash store at key,
// in the returned value, every field name is followed by its value, so the length of the reply is twice the size of the hash
func (h *HASH) HGetAll(key string) (outputMap map[string]string, notFound bool, err error) {
	if h == nil {
		return nil, false, errors.New("Redis HASH HGetAll Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return nil, false, errors.New("Redis HASH HGetAll Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HGetAll", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HGetAll-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HGetAll-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HGetAll-Result", outputMap))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputMap, notFound, err = h.hgetAllInternal(snap, key)
		return outputMap, notFound, err
	} else {
		return h.hgetAllInternal(snap, key)
	}
}

// hgetAllInternal returns all fields and values of the hash store at key,
// in the returned value, every field name is followed by its value, so the length of the reply is twice the size of the hash
func (h *HASH) hgetAllInternal(snap redisConnSnapshot, key string) (outputMap map[string]string, notFound bool, err error) {
	// validate
	if h.core == nil {
		return nil, false, errors.New("Redis HGetAll Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis HGetAll Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis HGetAll Failed: " + "Key is Required")
	}

	cmd := snap.reader.HGetAll(snap.reader.Context(), key)
	return h.core.handleStringStringMapCmd(cmd, "Redis HGetAll Failed: ")
}

// HMSet will set the specified 'fields' to their respective values in the hash stored by key,
// This command overrides any specified 'fields' already existing in the hash,
// If key does not exist, a new key holding a hash is created
func (h *HASH) HMSet(key string, value ...interface{}) (err error) {
	if h == nil {
		return errors.New("Redis HASH HMSet Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return errors.New("Redis HASH HMSet Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HMSet", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HMSet-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HMSet-Values", value))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = h.hmsetInternal(snap, key, value...)
		return err
	} else {
		return h.hmsetInternal(snap, key, value...)
	}
}

// hmsetInternal will set the specified 'fields' to their respective values in the hash stored by key,
// This command overrides any specified 'fields' already existing in the hash,
// If key does not exist, a new key holding a hash is created
func (h *HASH) hmsetInternal(snap redisConnSnapshot, key string, value ...interface{}) error {
	// validate
	if h.core == nil {
		return errors.New("Redis HMSet Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis HMSet Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis HMSet Failed: " + "Key is Required")
	}

	if len(value) <= 0 {
		return errors.New("Redis HMSet Failed: " + "At Least 1 Value is Required")
	}

	cmd := snap.writer.HMSet(snap.writer.Context(), key, value...)

	if val, err := h.core.handleBoolCmd(cmd, "Redis HMSet Failed: "); err != nil {
		return err
	} else {
		if val {
			// success
			return nil
		} else {
			// not success
			return errors.New("Redis HMSet Failed: " + "Action Result Yielded False")
		}
	}
}

// HMGet will return the values associated with the specified 'fields' in the hash stored at key,
// For every 'field' that does not exist in the hash, a nil value is returned,
// If key is not existent, then nil is returned for all values
func (h *HASH) HMGet(key string, field ...string) (outputSlice []interface{}, notFound bool, err error) {
	if h == nil {
		return nil, false, errors.New("Redis HASH HMGet Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return nil, false, errors.New("Redis HASH HMGet Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HMGet", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HMGet-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HMGet-Fields", field))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HMGet-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HMGet-Result", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = h.hmgetInternal(snap, key, field...)
		return outputSlice, notFound, err
	} else {
		return h.hmgetInternal(snap, key, field...)
	}
}

// hmgetInternal will return the values associated with the specified 'fields' in the hash stored at key,
// For every 'field' that does not exist in the hash, a nil value is returned,
// If key is not existent, then nil is returned for all values
func (h *HASH) hmgetInternal(snap redisConnSnapshot, key string, field ...string) (outputSlice []interface{}, notFound bool, err error) {
	// validate
	if h.core == nil {
		return nil, false, errors.New("Redis HMGet Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis HMGet Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis HMGet Failed: " + "Key is Required")
	}

	if len(field) <= 0 {
		return nil, false, errors.New("Redis HMGet Failed: " + "At Least 1 Field is Required")
	}

	cmd := snap.reader.HMGet(snap.reader.Context(), key, field...)
	return h.core.handleSliceCmd(cmd, "Redis HMGet Failed: ")
}

// HDel removes the specified 'fields' from the hash stored at key,
// any specified 'fields' that do not exist in the hash are ignored,
// if key does not exist, it is treated as an empty hash, and 0 is returned
func (h *HASH) HDel(key string, field ...string) (deletedCount int64, err error) {
	if h == nil {
		return 0, errors.New("Redis HASH HDel Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return 0, errors.New("Redis HASH HDel Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HDel", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HDel-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HDel-Fields", field))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HDel-Result-Deleted-Count", deletedCount))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		deletedCount, err = h.hdelInternal(snap, key, field...)
		return deletedCount, err
	} else {
		return h.hdelInternal(snap, key, field...)
	}
}

// hdelInternal removes the specified 'fields' from the hash stored at key,
// any specified 'fields' that do not exist in the hash are ignored,
// if key does not exist, it is treated as an empty hash, and 0 is returned
func (h *HASH) hdelInternal(snap redisConnSnapshot, key string, field ...string) (deletedCount int64, err error) {
	// validate
	if h.core == nil {
		return 0, errors.New("Redis HDel Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, errors.New("Redis HDel Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, errors.New("Redis HDel Failed: " + "Key is Required")
	}

	if len(field) <= 0 {
		return 0, errors.New("Redis HDel Failed: " + "At Least 1 Field is Required")
	}

	cmd := snap.writer.HDel(snap.writer.Context(), key, field...)
	deletedCount, _, err = h.core.handleIntCmd(cmd, "Redis HDel Failed: ")
	return deletedCount, err
}

// HKeys returns all field names in the hash stored at key,
// field names are the element keys
func (h *HASH) HKeys(key string) (outputSlice []string, notFound bool, err error) {
	if h == nil {
		return nil, false, errors.New("Redis HASH HKeys Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return nil, false, errors.New("Redis HASH HKeys Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HKeys", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HKeys-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HKeys-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HKeys-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = h.hkeysInternal(snap, key)
		return outputSlice, notFound, err
	} else {
		return h.hkeysInternal(snap, key)
	}
}

// hkeysInternal returns all field names in the hash stored at key,
// field names are the element keys
func (h *HASH) hkeysInternal(snap redisConnSnapshot, key string) (outputSlice []string, notFound bool, err error) {
	// validate
	if h.core == nil {
		return nil, false, errors.New("Redis HKeys Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis HKeys Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis HKeys Failed: " + "Key is Required")
	}

	cmd := snap.reader.HKeys(snap.reader.Context(), key)
	return h.core.handleStringSliceCmd(cmd, "Redis HKeys Failed: ")
}

// HVals returns all values in the hash stored at key
func (h *HASH) HVals(key string) (outputSlice []string, notFound bool, err error) {
	if h == nil {
		return nil, false, errors.New("Redis HASH HVals Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return nil, false, errors.New("Redis HASH HVals Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HVals", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HVals-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HVals-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HVals-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = h.hvalsInternal(snap, key)
		return outputSlice, notFound, err
	} else {
		return h.hvalsInternal(snap, key)
	}
}

// hvalsInternal returns all values in the hash stored at key
func (h *HASH) hvalsInternal(snap redisConnSnapshot, key string) (outputSlice []string, notFound bool, err error) {
	// validate
	if h.core == nil {
		return nil, false, errors.New("Redis HVals Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis HVals Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis HVals Failed: " + "Key is Required")
	}

	cmd := snap.reader.HVals(snap.reader.Context(), key)
	return h.core.handleStringSliceCmd(cmd, "Redis HVals Failed: ")
}

// HScan is used to incrementally iterate over a set of fields for hash stored at key，
// HScan is a cursor based iterator, at every call of the command, redis returns an updated cursor that client must use for next call to sort,
//
// start iteration = cursor set to 0
// stop iteration = when redis returns cursor value of 0
//
// match = filters elements based on match filter, for elements retrieved from redis before return to client
//
//	glob-style patterns:
//		1) h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//		2) h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//		3) h*llo = * represents any single or more char match (hllo, heeeelo match)
//		4) h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//		5) h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//		6) h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//		7) Use \ to escape special characters if needing to match verbatim
//
// count = hint to redis count of elements to retrieve in the call
func (h *HASH) HScan(key string, cursor uint64, match string, count int64) (outputKeys []string, outputCursor uint64, err error) {
	if h == nil {
		return nil, 0, errors.New("Redis HASH HScan Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return nil, 0, errors.New("Redis HASH HScan Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HScan", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HScan-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HScan-Cursor", cursor))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HScan-Match", match))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HScan-Count", count))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HScan-Result-Keys", outputKeys))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HScan-Result-Cursor", outputCursor))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputKeys, outputCursor, err = h.hscanInternal(snap, key, cursor, match, count)
		return outputKeys, outputCursor, err
	} else {
		return h.hscanInternal(snap, key, cursor, match, count)
	}
}

// hscanInternal is used to incrementally iterate over a set of fields for hash stored at key，
// HScan is a cursor based iterator, at every call of the command, redis returns an updated cursor that client must use for next call to sort,
//
// start iteration = cursor set to 0
// stop iteration = when redis returns cursor value of 0
//
// match = filters elements based on match filter, for elements retrieved from redis before return to client
//
//	glob-style patterns:
//		1) h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//		2) h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//		3) h*llo = * represents any single or more char match (hllo, heeeelo match)
//		4) h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//		5) h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//		6) h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//		7) Use \ to escape special characters if needing to match verbatim
//
// count = hint to redis count of elements to retrieve in the call
func (h *HASH) hscanInternal(snap redisConnSnapshot, key string, cursor uint64, match string, count int64) (outputKeys []string, outputCursor uint64, err error) {
	// validate
	if h.core == nil {
		return nil, 0, errors.New("Redis HScan Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, 0, errors.New("Redis HScan Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, 0, errors.New("Redis HScan Failed: " + "Key is Required")
	}

	if count < 0 {
		return nil, 0, errors.New("Redis HScan Failed: " + "Count Must Be Zero or Greater")
	}

	cmd := snap.reader.HScan(snap.reader.Context(), key, cursor, match, count)
	return h.core.handleScanCmd(cmd, "Redis HScan Failed: ")
}

// HIncrBy increments or decrements the number (int64) value at 'field' in the hash stored at key,
// if key does not exist, a new key holding a hash is created,
// if 'field' does not exist then the value is set to 0 before operation is performed
//
// this function supports both increment and decrement (although name of function is increment)
func (h *HASH) HIncrBy(key string, field string, incrValue int64) (err error) {
	if h == nil {
		return errors.New("Redis HASH HIncrBy Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return errors.New("Redis HASH HIncrBy Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HIncrBy", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HIncrBy-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HIncrBy-Field", field))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HIncrBy-Increment-Value", incrValue))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = h.hincrByInternal(snap, key, field, incrValue)
		return err
	} else {
		return h.hincrByInternal(snap, key, field, incrValue)
	}
}

// hincrByInternal increments or decrements the number (int64) value at 'field' in the hash stored at key,
// if key does not exist, a new key holding a hash is created,
// if 'field' does not exist then the value is set to 0 before operation is performed
//
// this function supports both increment and decrement (although name of function is increment)
func (h *HASH) hincrByInternal(snap redisConnSnapshot, key string, field string, incrValue int64) error {
	// validate
	if h.core == nil {
		return errors.New("Redis HIncrBy Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis HIncrBy Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis HIncrBy Failed: " + "Key is Required")
	}

	if len(field) <= 0 {
		return errors.New("Redis HIncrBy Failed: " + "Field is Required")
	}

	if incrValue == 0 {
		return errors.New("Redis HIncrBy Failed: " + "Increment Value Must Not Be Zero")
	}

	cmd := snap.writer.HIncrBy(snap.writer.Context(), key, field, incrValue)

	if _, _, err := h.core.handleIntCmd(cmd, "Redis HIncrBy Failed: "); err != nil {
		return err
	} else {
		return nil
	}
}

// HIncrByFloat increments or decrements the number (float64) value at 'field' in the hash stored at key,
// if key does not exist, a new key holding a hash is created,
// if 'field' does not exist then the value is set to 0 before operation is performed
//
// this function supports both increment and decrement (although name of function is increment)
func (h *HASH) HIncrByFloat(key string, field string, incrValue float64) (err error) {
	if h == nil {
		return errors.New("Redis HASH HIncrByFloat Failed: HASH receiver is nil")
	}
	if h.core == nil {
		return errors.New("Redis HASH HIncrByFloat Failed: Redis core is nil")
	}
	snap := h.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HIncrByFloat", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HIncrByFloat-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HIncrByFloat-Field", field))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-HIncrByFloat-Increment-Value", incrValue))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = h.hincrByFloatInternal(snap, key, field, incrValue)
		return err
	} else {
		return h.hincrByFloatInternal(snap, key, field, incrValue)
	}
}

// hincrByFloatInternal increments or decrements the number (float64) value at 'field' in the hash stored at key,
// if key does not exist, a new key holding a hash is created,
// if 'field' does not exist then the value is set to 0 before operation is performed
//
// this function supports both increment and decrement (although name of function is increment)
func (h *HASH) hincrByFloatInternal(snap redisConnSnapshot, key string, field string, incrValue float64) error {
	// validate
	if h.core == nil {
		return errors.New("Redis HIncrByFloat Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis HIncrByFloat Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis HIncrByFloat Failed: " + "Key is Required")
	}

	if len(field) <= 0 {
		return errors.New("Redis HIncrByFloat Failed: " + "Field is Required")
	}

	if incrValue == 0.00 {
		return errors.New("Redis HIncrByFloat Failed: " + "Increment Value Must Not Be Zero")
	}

	cmd := snap.writer.HIncrByFloat(snap.writer.Context(), key, field, incrValue)

	if _, _, err := h.core.handleFloatCmd(cmd, "Redis HIncrByFloat Failed: "); err != nil {
		return err
	} else {
		return nil
	}
}

// ----------------------------------------------------------------------------------------------------------------
// SET functions
// ----------------------------------------------------------------------------------------------------------------

// SAdd adds the specified members to the set stored at key,
// Specified members that are already a member of this set are ignored,
// If key does not exist, a new set is created before adding the specified members
//
// Error is returned when the value stored at key is not a set
func (s *SET) SAdd(key string, member ...interface{}) (err error) {
	if s == nil {
		return errors.New("Redis SET SAdd Failed: SET receiver is nil")
	}
	if s.core == nil {
		return errors.New("Redis SET SAdd Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SAdd", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SAdd-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SAdd-Members", member))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = s.saddInternal(snap, key, member...)
		return err
	} else {
		return s.saddInternal(snap, key, member...)
	}
}

// saddInternal adds the specified members to the set stored at key,
// Specified members that are already a member of this set are ignored,
// If key does not exist, a new set is created before adding the specified members
//
// Error is returned when the value stored at key is not a set
func (s *SET) saddInternal(snap redisConnSnapshot, key string, member ...interface{}) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SAdd Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis SAdd Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis SAdd Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return errors.New("Redis SAdd Failed: " + "At Least 1 Member is Required")
	}

	cmd := snap.writer.SAdd(snap.writer.Context(), key, member...)
	return s.core.handleIntCmd2(cmd, "Redis SAdd Failed: ")
}

// SCard returns the set cardinality (number of elements) of the set stored at key
func (s *SET) SCard(key string) (val int64, notFound bool, err error) {
	if s == nil {
		return 0, false, errors.New("Redis SET SCard Failed: SET receiver is nil")
	}
	if s.core == nil {
		return 0, false, errors.New("Redis SET SCard Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SCard", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SCard-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SCard-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SCard-Result-Count", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = s.scardInternal(snap, key)
		return val, notFound, err
	} else {
		return s.scardInternal(snap, key)
	}
}

// scardInternal returns the set cardinality (number of elements) of the set stored at key
func (s *SET) scardInternal(snap redisConnSnapshot, key string) (val int64, notFound bool, err error) {
	// validate
	if s.core == nil {
		return 0, false, errors.New("Redis SCard Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis SCard Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis SCard Failed: " + "Key is Required")
	}

	cmd := snap.reader.SCard(snap.reader.Context(), key)
	return s.core.handleIntCmd(cmd, "Redis SCard Failed: ")
}

// SDiff returns the members of the set resulting from the difference between the first set and all the successive sets
//
// Example:
//
//	key1 = { a, b, c, d }
//	key2 = { c }
//	key3 = { a, c, e }
//	SDIFF key1, key2, key3 = { b, d }
//		{ b, d } is returned because this is the difference delta
func (s *SET) SDiff(key ...string) (outputSlice []string, notFound bool, err error) {
	if s == nil {
		return nil, false, errors.New("Redis SET SDiff Failed: SET receiver is nil")
	}
	if s.core == nil {
		return nil, false, errors.New("Redis SET SDiff Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SDiff", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SDiff-Keys", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SDiff-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SDiff-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = s.sdiffInternal(snap, key...)
		return outputSlice, notFound, err
	} else {
		return s.sdiffInternal(snap, key...)
	}
}

// sdiffInternal returns the members of the set resulting from the difference between the first set and all the successive sets
//
// Example:
//
//	key1 = { a, b, c, d }
//	key2 = { c }
//	key3 = { a, c, e }
//	SDIFF key1, key2, key3 = { b, d }
//		{ b, d } is returned because this is the difference delta
func (s *SET) sdiffInternal(snap redisConnSnapshot, key ...string) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SDiff Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis SDiff Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 1 {
		return nil, false, errors.New("Redis SDiff Failed: " + "At Least 2 Keys Are Required")
	}

	cmd := snap.reader.SDiff(snap.reader.Context(), key...)
	return s.core.handleStringSliceCmd(cmd, "Redis SDiff Failed: ")
}

// SDiffStore will store the set differential to destination,
// if destination already exists, it is overwritten
//
// Example:
//
//	key1 = { a, b, c, d }
//	key2 = { c }
//	key3 = { a, c, e }
//	SDIFF key1, key2, key3 = { b, d }
//		{ b, d } is stored because this is the difference delta
func (s *SET) SDiffStore(keyDest string, keySource ...string) (err error) {
	if s == nil {
		return errors.New("Redis SET SDiffStore Failed: SET receiver is nil")
	}
	if s.core == nil {
		return errors.New("Redis SET SDiffStore Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SDiffStore", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SDiffStore-KeyDest", keyDest))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SDiffStore-KeySources", keySource))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = s.sdiffStoreInternal(snap, keyDest, keySource...)
		return err
	} else {
		return s.sdiffStoreInternal(snap, keyDest, keySource...)
	}
}

// sdiffStoreInternal will store the set differential to destination,
// if destination already exists, it is overwritten
//
// Example:
//
//	key1 = { a, b, c, d }
//	key2 = { c }
//	key3 = { a, c, e }
//	SDIFF key1, key2, key3 = { b, d }
//		{ b, d } is stored because this is the difference delta
func (s *SET) sdiffStoreInternal(snap redisConnSnapshot, keyDest string, keySource ...string) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SDiffStore Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis SDiffStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis SDiffStore Failed: " + "Key Destination is Required")
	}

	if len(keySource) <= 1 {
		return errors.New("Redis SDiffStore Failed: " + "At Least 2 Key Sources are Required")
	}

	cmd := snap.writer.SDiffStore(snap.writer.Context(), keyDest, keySource...)
	return s.core.handleIntCmd2(cmd, "Redis SDiffStore Failed: ")
}

// SInter returns the members of the set resulting from the intersection of all the given sets
//
// Example:
//
//	Key1 = { a, b, c, d }
//	Key2 = { c }
//	Key3 = { a, c, e }
//	SINTER key1 key2 key3 = { c }
//		{ c } is returned because this is the intersection on all keys (appearing in all keys)
func (s *SET) SInter(key ...string) (outputSlice []string, notFound bool, err error) {
	if s == nil {
		return nil, false, errors.New("Redis SET SInter Failed: SET receiver is nil")
	}
	if s.core == nil {
		return nil, false, errors.New("Redis SET SInter Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SInter", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SInter-Keys", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SInter-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SInter-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = s.sinterInternal(snap, key...)
		return outputSlice, notFound, err
	} else {
		return s.sinterInternal(snap, key...)
	}
}

// sinterInternal returns the members of the set resulting from the intersection of all the given sets
//
// Example:
//
//	Key1 = { a, b, c, d }
//	Key2 = { c }
//	Key3 = { a, c, e }
//	SINTER key1 key2 key3 = { c }
//		{ c } is returned because this is the intersection on all keys (appearing in all keys)
func (s *SET) sinterInternal(snap redisConnSnapshot, key ...string) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SInter Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis SInter Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 1 {
		return nil, false, errors.New("Redis SInter Failed: " + "At Least 2 Keys Are Required")
	}

	cmd := snap.reader.SInter(snap.reader.Context(), key...)
	return s.core.handleStringSliceCmd(cmd, "Redis SInter Failed: ")
}

// SInterStore stores the members of the set resulting from the intersection of all the given sets to destination,
// if destination already exists, it is overwritten
//
// Example:
//
//	Key1 = { a, b, c, d }
//	Key2 = { c }
//	Key3 = { a, c, e }
//	SINTER key1 key2 key3 = { c }
//		{ c } is stored because this is the intersection on all keys (appearing in all keys)
func (s *SET) SInterStore(keyDest string, keySource ...string) (err error) {
	if s == nil {
		return errors.New("Redis SET SInterStore Failed: SET receiver is nil")
	}
	if s.core == nil {
		return errors.New("Redis SET SInterStore Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SInterStore", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SInterStore-KeyDest", keyDest))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SInterStore-KeySources", keySource))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = s.sinterStoreInternal(snap, keyDest, keySource...)
		return err
	} else {
		return s.sinterStoreInternal(snap, keyDest, keySource...)
	}
}

// sinterStoreInternal stores the members of the set resulting from the intersection of all the given sets to destination,
// if destination already exists, it is overwritten
//
// Example:
//
//	Key1 = { a, b, c, d }
//	Key2 = { c }
//	Key3 = { a, c, e }
//	SINTER key1 key2 key3 = { c }
//		{ c } is stored because this is the intersection on all keys (appearing in all keys)
func (s *SET) sinterStoreInternal(snap redisConnSnapshot, keyDest string, keySource ...string) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SInterStore Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis SInterStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis SInterStore Failed: " + "Key Destination is Required")
	}

	if len(keySource) <= 1 {
		return errors.New("Redis SInterStore Failed: " + "At Least 2 Key Sources are Required")
	}

	cmd := snap.writer.SInterStore(snap.writer.Context(), keyDest, keySource...)
	return s.core.handleIntCmd2(cmd, "Redis SInterStore Failed: ")
}

// SIsMember returns status if 'member' is a member of the set stored at key
func (s *SET) SIsMember(key string, member interface{}) (val bool, err error) {
	if s == nil {
		return false, errors.New("Redis SET SIsMember Failed: SET receiver is nil")
	}
	if s.core == nil {
		return false, errors.New("Redis SET SIsMember Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SIsMember", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SIsMember-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SIsMember-Member", member))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SIsMember-Result-IsMember", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, err = s.sisMemberInternal(snap, key, member)
		return val, err
	} else {
		return s.sisMemberInternal(snap, key, member)
	}
}

// sisMemberInternal returns status if 'member' is a member of the set stored at key
func (s *SET) sisMemberInternal(snap redisConnSnapshot, key string, member interface{}) (val bool, err error) {
	// validate
	if s.core == nil {
		return false, errors.New("Redis SIsMember Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return false, errors.New("Redis SIsMember Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return false, errors.New("Redis SIsMember Failed: " + "Key is Required")
	}

	if member == nil {
		return false, errors.New("Redis SIsMember Failed: " + "Member is Required")
	}

	cmd := snap.reader.SIsMember(snap.reader.Context(), key, member)
	return s.core.handleBoolCmd(cmd, "Redis SIsMember Failed: ")
}

// SMembers returns all the members of the set value stored at key
func (s *SET) SMembers(key string) (outputSlice []string, notFound bool, err error) {
	if s == nil {
		return nil, false, errors.New("Redis SET SMembers Failed: SET receiver is nil")
	}
	if s.core == nil {
		return nil, false, errors.New("Redis SET SMembers Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SMembers", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SMember-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SMember-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SMember-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = s.smembersInternal(snap, key)
		return outputSlice, notFound, err
	} else {
		return s.smembersInternal(snap, key)
	}
}

// smembersInternal returns all the members of the set value stored at key
func (s *SET) smembersInternal(snap redisConnSnapshot, key string) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SMembers Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis SMembers Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis SMembers Failed: " + "Key is Required")
	}

	cmd := snap.reader.SMembers(snap.reader.Context(), key)
	return s.core.handleStringSliceCmd(cmd, "Redis SMember Failed: ")
}

// SMembersMap returns all the members of the set value stored at key, via map
func (s *SET) SMembersMap(key string) (outputMap map[string]struct{}, notFound bool, err error) {
	if s == nil {
		return nil, false, errors.New("Redis SET SMembersMap Failed: SET receiver is nil")
	}
	if s.core == nil {
		return nil, false, errors.New("Redis SET SMembersMap Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SMembersMap", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SMembersMap-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SMembersMap-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SMembersMap-Result", outputMap))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputMap, notFound, err = s.smembersMapInternal(snap, key)
		return outputMap, notFound, err
	} else {
		return s.smembersMapInternal(snap, key)
	}
}

// smembersMapInternal returns all the members of the set value stored at key, via map
func (s *SET) smembersMapInternal(snap redisConnSnapshot, key string) (outputMap map[string]struct{}, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SMembersMap Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis SMembersMap Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis SMembersMap Failed: " + "Key is Required")
	}

	cmd := snap.reader.SMembersMap(snap.reader.Context(), key)
	return s.core.handleStringStructMapCmd(cmd, "Redis SMembersMap Failed: ")
}

// SScan is used to incrementally iterate over a set of fields for set stored at key，
// SScan is a cursor based iterator, at every call of the command, redis returns an updated cursor that client must use for next call to sort,
//
// start iteration = cursor set to 0
// stop iteration = when redis returns cursor value of 0
//
// match = filters elements based on match filter, for elements retrieved from redis before return to client
//
//	glob-style patterns:
//		1) h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//		2) h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//		3) h*llo = * represents any single or more char match (hllo, heeeelo match)
//		4) h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//		5) h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//		6) h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//		7) Use \ to escape special characters if needing to match verbatim
//
// count = hint to redis count of elements to retrieve in the call
func (s *SET) SScan(key string, cursor uint64, match string, count int64) (outputKeys []string, outputCursor uint64, err error) {
	if s == nil {
		return nil, 0, errors.New("Redis SET SScan Failed: SET receiver is nil")
	}
	if s.core == nil {
		return nil, 0, errors.New("Redis SET SScan Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SScan", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SScan-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SScan-Cursor", cursor))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SScan-Match", match))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SScan-Count", count))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SScan-Result-Keys", outputKeys))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SScan-Result-Cursor", outputCursor))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputKeys, outputCursor, err = s.sscanInternal(snap, key, cursor, match, count)
		return outputKeys, outputCursor, err
	} else {
		return s.sscanInternal(snap, key, cursor, match, count)
	}
}

// sscanInternal is used to incrementally iterate over a set of fields for set stored at key，
// SScan is a cursor based iterator, at every call of the command, redis returns an updated cursor that client must use for next call to sort,
//
// start iteration = cursor set to 0
// stop iteration = when redis returns cursor value of 0
//
// match = filters elements based on match filter, for elements retrieved from redis before return to client
//
//	glob-style patterns:
//		1) h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//		2) h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//		3) h*llo = * represents any single or more char match (hllo, heeeelo match)
//		4) h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//		5) h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//		6) h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//		7) Use \ to escape special characters if needing to match verbatim
//
// count = hint to redis count of elements to retrieve in the call
func (s *SET) sscanInternal(snap redisConnSnapshot, key string, cursor uint64, match string, count int64) (outputKeys []string, outputCursor uint64, err error) {
	// validate
	if s.core == nil {
		return nil, 0, errors.New("Redis SScan Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, 0, errors.New("Redis SScan Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, 0, errors.New("Redis SScan Failed: " + "Key is Required")
	}

	if count < 0 {
		return nil, 0, errors.New("Redis SScan Failed: " + "Count Must Be 0 or Greater")
	}

	cmd := snap.reader.SScan(snap.reader.Context(), key, cursor, match, count)
	return s.core.handleScanCmd(cmd, "Redis SScan Failed: ")
}

// SRandMember returns a random element from the set value stored at key
func (s *SET) SRandMember(key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	if s == nil {
		return false, errors.New("Redis SET SRandMember Failed: SET receiver is nil")
	}
	if s.core == nil {
		return false, errors.New("Redis SET SRandMember Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SRandMember", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SRandMember-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SRandMember-Output-Data-Type", outputDataType))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SRandMember-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SRandMember-Output-Object", outputObjectPtr))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		notFound, err = s.srandMemberInternal(snap, key, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return s.srandMemberInternal(snap, key, outputDataType, outputObjectPtr)
	}
}

// srandMemberInternal returns a random element from the set value stored at key
func (s *SET) srandMemberInternal(snap redisConnSnapshot, key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if s.core == nil {
		return false, errors.New("Redis SRandMember Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return false, errors.New("Redis SRandMember Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return false, errors.New("Redis SRandMember Failed: " + "Key is Required")
	}

	if !outputDataType.Valid() || outputDataType == redisdatatype.UNKNOWN {
		return false, errors.New("Redis SRandMember Failed: " + "Output Data Type is Required")
	}

	if outputObjectPtr == nil {
		return false, errors.New("Redis SRandMember Failed: " + "Output Object Pointer is Required")
	}

	cmd := snap.reader.SRandMember(snap.reader.Context(), key)
	return s.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis SRandMember Failed: ")
}

// SRandMemberN returns one or more random elements from the set value stored at key, with count indicating return limit
//
// count > 0 = returns an array of count distinct elements (non-repeating), up to the set elements size
// count < 0 = returns an array of count elements (may be repeating), and up to the count size (selected members may still be part of the subsequent selection process)
func (s *SET) SRandMemberN(key string, count int64) (outputSlice []string, notFound bool, err error) {
	if s == nil {
		return nil, false, errors.New("Redis SET SRandMemberN Failed: SET receiver is nil")
	}
	if s.core == nil {
		return nil, false, errors.New("Redis SET SRandMemberN Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SRandMemberN", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SRandMemberN-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SRandMemberN-Count", count))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SRandMemberN-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SRandMemberN-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = s.srandMemberNInternal(snap, key, count)
		return outputSlice, notFound, err
	} else {
		return s.srandMemberNInternal(snap, key, count)
	}
}

// srandMemberNInternal returns one or more random elements from the set value stored at key, with count indicating return limit
//
// count > 0 = returns an array of count distinct elements (non-repeating), up to the set elements size
// count < 0 = returns an array of count elements (may be repeating), and up to the count size (selected members may still be part of the subsequent selection process)
func (s *SET) srandMemberNInternal(snap redisConnSnapshot, key string, count int64) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SRandMemberN Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis SRandMemberN Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis SRandMemberN Failed: " + "Key is Required")
	}

	if count == 0 {
		return nil, false, errors.New("Redis SRandMemberN Failed: " + "Count Must Not Be Zero")
	}

	cmd := snap.reader.SRandMemberN(snap.reader.Context(), key, count)
	return s.core.handleStringSliceCmd(cmd, "Redis SRandMemberN Failed: ")
}

// SRem removes the specified members from the set stored at key,
// Specified members that are not a member of this set are ignored,
// If key does not exist, it is treated as an empty set and this command returns 0
//
// Error is returned if the value stored at key is not a set
func (s *SET) SRem(key string, member ...interface{}) (err error) {
	if s == nil {
		return errors.New("Redis SET SRem Failed: SET receiver is nil")
	}
	if s.core == nil {
		return errors.New("Redis SET SRem Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SRem", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SRem-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SRem-Members", member))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = s.sremInternal(snap, key, member...)
		return err
	} else {
		return s.sremInternal(snap, key, member...)
	}
}

// sremInternal removes the specified members from the set stored at key,
// Specified members that are not a member of this set are ignored,
// If key does not exist, it is treated as an empty set and this command returns 0
//
// Error is returned if the value stored at key is not a set
func (s *SET) sremInternal(snap redisConnSnapshot, key string, member ...interface{}) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SRem Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis SRem Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis SRem Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return errors.New("Redis SRem Failed: " + "At Least 1 Member is Required")
	}

	cmd := snap.writer.SRem(snap.writer.Context(), key, member...)
	return s.core.handleIntCmd2(cmd, "Redis SRem Failed: ")
}

// SMove will move a member from the set at 'source' to the set at 'destination' atomically,
// The element will appear to be a member of source or destination for other clients
//
// If source set does not exist, or does not contain the specified element, no operation is performed and 0 is returned,
// Otherwise, the element is removed from the source set and added to the destination set
//
// # If the specified element already exist in the destination set, it is only removed from the source set
//
// Error is returned if the source or destination does not hold a set value
func (s *SET) SMove(keySource string, keyDest string, member interface{}) (err error) {
	if s == nil {
		return errors.New("Redis SET SMove Failed: SET receiver is nil")
	}
	if s.core == nil {
		return errors.New("Redis SET SMove Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SMove", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SMove-KeySource", keySource))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SMove-KeyDest", keyDest))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SMove-Member", member))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = s.smoveInternal(snap, keySource, keyDest, member)
		return err
	} else {
		return s.smoveInternal(snap, keySource, keyDest, member)
	}
}

// smoveInternal will move a member from the set at 'source' to the set at 'destination' atomically,
// The element will appear to be a member of source or destination for other clients
//
// If source set does not exist, or does not contain the specified element, no operation is performed and 0 is returned,
// Otherwise, the element is removed from the source set and added to the destination set
//
// # If the specified element already exist in the destination set, it is only removed from the source set
//
// Error is returned if the source or destination does not hold a set value
func (s *SET) smoveInternal(snap redisConnSnapshot, keySource string, keyDest string, member interface{}) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SMove Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis SMove Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keySource) <= 0 {
		return errors.New("Redis SMove Failed: " + "Key Source is Required")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis SMove Failed: " + "Key Destination is Required")
	}

	if member == nil {
		return errors.New("Redis SMove Failed: " + "Member is Required")
	}

	cmd := snap.writer.SMove(snap.writer.Context(), keySource, keyDest, member)

	if val, err := s.core.handleBoolCmd(cmd, "Redis SMove Failed: "); err != nil {
		return err
	} else {
		if val {
			// success
			return nil
		} else {
			// false
			return errors.New("Redis SMove Failed: " + "Action Result Yielded False")
		}
	}
}

// SPop removes and returns one random element from the set value stored at key
func (s *SET) SPop(key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	if s == nil {
		return false, errors.New("Redis SET SPop Failed: SET receiver is nil")
	}
	if s.core == nil {
		return false, errors.New("Redis SET SPop Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SPop", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SPop-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SPop-Output-Data-Type", outputDataType))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SPop-Output-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SPop-Output-Object", outputObjectPtr))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		notFound, err = s.spopInternal(snap, key, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return s.spopInternal(snap, key, outputDataType, outputObjectPtr)
	}
}

// spopInternal removes and returns one random element from the set value stored at key
func (s *SET) spopInternal(snap redisConnSnapshot, key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if s.core == nil {
		return false, errors.New("Redis SPop Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return false, errors.New("Redis SPop Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return false, errors.New("Redis SPop Failed: " + "Key is Required")
	}

	if !outputDataType.Valid() || outputDataType == redisdatatype.UNKNOWN {
		return false, errors.New("Redis SPop Failed: " + "Output Data Type is Required")
	}

	if outputObjectPtr == nil {
		return false, errors.New("Redis SPop Failed: " + "Output Object Pointer is Required")
	}

	cmd := snap.writer.SPop(snap.writer.Context(), key)
	return s.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis SPop Failed: ")
}

// SPopN removes and returns one or more random element from the set value stored at key
//
// count > 0 = returns an array of count distinct elements (non-repeating), up to the set elements size
// count < 0 = returns an array of count elements (may be repeating), and up to the count size (selected members may still be part of the subsequent selection process)
func (s *SET) SPopN(key string, count int64) (outputSlice []string, notFound bool, err error) {
	if s == nil {
		return nil, false, errors.New("Redis SET SPopN Failed: SET receiver is nil")
	}
	if s.core == nil {
		return nil, false, errors.New("Redis SET SPopN Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SPopN", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SPopN-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SPopN-Count", count))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SPopN-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SPopN-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = s.spopNInternal(snap, key, count)
		return outputSlice, notFound, err
	} else {
		return s.spopNInternal(snap, key, count)
	}
}

// spopNInternal removes and returns one or more random element from the set value stored at key
//
// count > 0 = returns an array of count distinct elements (non-repeating), up to the set elements size
// count < 0 = returns an array of count elements (may be repeating), and up to the count size (selected members may still be part of the subsequent selection process)
func (s *SET) spopNInternal(snap redisConnSnapshot, key string, count int64) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SPopN Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis SPopN Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis SPopN Failed: " + "Key is Required")
	}

	if count == 0 {
		return nil, false, errors.New("Redis SPopN Failed: " + "Count Must Not Be Zero")
	}

	cmd := snap.writer.SPopN(snap.writer.Context(), key, count)
	return s.core.handleStringSliceCmd(cmd, "Redis SPopN Failed: ")
}

// SUnion returns the members of the set resulting from the union of all the given sets,
// if a key is not existent, it is treated as a empty set
//
// Example:
//
//	key1 = { a, b, c }
//	key2 = { c }
//	key3 = { a, c, e }
//	SUNION key1 key2 key3 = { a, b, c, d, e }
func (s *SET) SUnion(key ...string) (outputSlice []string, notFound bool, err error) {
	if s == nil {
		return nil, false, errors.New("Redis SET SUnion Failed: SET receiver is nil")
	}
	if s.core == nil {
		return nil, false, errors.New("Redis SET SUnion Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SUnion", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SUnion-Keys", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SUnion-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SUnion-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = s.sunionInternal(snap, key...)
		return outputSlice, notFound, err
	} else {
		return s.sunionInternal(snap, key...)
	}
}

// sunionInternal returns the members of the set resulting from the union of all the given sets,
// if a key is not existent, it is treated as a empty set
//
// Example:
//
//	key1 = { a, b, c }
//	key2 = { c }
//	key3 = { a, c, e }
//	SUNION key1 key2 key3 = { a, b, c, d, e }
func (s *SET) sunionInternal(snap redisConnSnapshot, key ...string) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SUnion Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis SUnion Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 1 {
		return nil, false, errors.New("Redis SUnion Failed: " + "At Least 2 Keys Are Required")
	}

	cmd := snap.reader.SUnion(snap.reader.Context(), key...)
	return s.core.handleStringSliceCmd(cmd, "Redis SUnion Failed: ")
}

// SUnionStore will store the members of the set resulting from the union of all the given sets to 'destination'
// if a key is not existent, it is treated as a empty set,
// if 'destination' already exists, it is overwritten
//
// Example:
//
//	key1 = { a, b, c }
//	key2 = { c }
//	key3 = { a, c, e }
//	SUNION key1 key2 key3 = { a, b, c, d, e }
func (s *SET) SUnionStore(keyDest string, keySource ...string) (err error) {
	if s == nil {
		return errors.New("Redis SET SUnionStore Failed: SET receiver is nil")
	}
	if s.core == nil {
		return errors.New("Redis SET SUnionStore Failed: Redis core is nil")
	}
	snap := s.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SUnionStore", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SUnionStore-KeyDest", keyDest))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SUnionStore-KeySources", keySource))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = s.sunionStoreInternal(snap, keyDest, keySource...)
		return err
	} else {
		return s.sunionStoreInternal(snap, keyDest, keySource...)
	}
}

// sunionStoreInternal will store the members of the set resulting from the union of all the given sets to 'destination'
// if a key is not existent, it is treated as a empty set,
// if 'destination' already exists, it is overwritten
//
// Example:
//
//	key1 = { a, b, c }
//	key2 = { c }
//	key3 = { a, c, e }
//	SUNION key1 key2 key3 = { a, b, c, d, e }
func (s *SET) sunionStoreInternal(snap redisConnSnapshot, keyDest string, keySource ...string) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SUnionStore Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis SUnionStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis SUnionStore Failed: " + "Key Destination is Required")
	}

	if len(keySource) <= 1 {
		return errors.New("Redis SUnionStore Failed: " + "At Least 2 Key Sources are Required")
	}

	cmd := snap.writer.SUnionStore(snap.writer.Context(), keyDest, keySource...)
	return s.core.handleIntCmd2(cmd, "Redis SUnionStore Failed: ")
}

// ----------------------------------------------------------------------------------------------------------------
// SORTED_SET functions
// ----------------------------------------------------------------------------------------------------------------

// ZAdd will add all the specified members with the specified scores to the sorted set stored at key,
// If a specified member is already a member of the sorted set, the score is updated and the element reinserted at the right position to ensure the correct ordering
//
// # If key does not exist, a new sorted set with the specified members is created as if the set was empty
//
// # If value at key is not a sorted set, then error is returned
//
// Score Values:
//  1. Should be string representation of a double precision floating point number
//
// Other ZAdd Options:
//  1. ZAdd XX / XXCH = only update elements that already exists, never add elements
//  2. ZAdd NX / NXCH = don't update already existing elements, always add new elements
//  3. ZAdd CH = modify the return value from the number of new or updated elements added, CH = Changed
func (z *SORTED_SET) ZAdd(key string, setCondition redissetcondition.RedisSetCondition, getChanged bool, member ...*redis.Z) (err error) {
	if z == nil {
		return errors.New("Redis SORTED_SET ZAdd Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return errors.New("Redis SORTED_SET ZAdd Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZAdd", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZAdd-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZAdd-Condition", setCondition))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZAdd-Get-Changed", getChanged))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZAdd-Member", member))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = z.zaddInternal(snap, key, setCondition, getChanged, member...)
		return err
	} else {
		return z.zaddInternal(snap, key, setCondition, getChanged, member...)
	}
}

// zaddInternal will add all the specified members with the specified scores to the sorted set stored at key,
// If a specified member is already a member of the sorted set, the score is updated and the element reinserted at the right position to ensure the correct ordering
//
// # If key does not exist, a new sorted set with the specified members is created as if the set was empty
//
// # If value at key is not a sorted set, then error is returned
//
// Score Values:
//  1. Should be string representation of a double precision floating point number
//
// Other ZAdd Options:
//  1. ZAdd XX / XXCH = only update elements that already exists, never add elements
//  2. ZAdd NX / NXCH = don't update already existing elements, always add new elements
//  3. ZAdd CH = modify the return value from the number of new or updated elements added, CH = Changed
func (z *SORTED_SET) zaddInternal(snap redisConnSnapshot, key string, setCondition redissetcondition.RedisSetCondition, getChanged bool, member ...*redis.Z) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZAdd Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis ZAdd Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis ZAdd Failed: " + "Key is Required")
	}

	if !setCondition.Valid() || setCondition == redissetcondition.UNKNOWN {
		return errors.New("Redis ZAdd Failed: " + "Set Condition is Required")
	}

	if len(member) <= 0 {
		return errors.New("Redis ZAdd Failed: " + "At Least 1 Member is Required")
	}

	var cmd *redis.IntCmd

	switch setCondition {
	case redissetcondition.Normal:
		if !getChanged {
			cmd = snap.writer.ZAdd(snap.writer.Context(), key, member...)
		} else {
			cmd = snap.writer.ZAddCh(snap.writer.Context(), key, member...)
		}
	case redissetcondition.SetIfNotExists:
		if !getChanged {
			cmd = snap.writer.ZAddNX(snap.writer.Context(), key, member...)
		} else {
			cmd = snap.writer.ZAddNXCh(snap.writer.Context(), key, member...)
		}
	case redissetcondition.SetIfExists:
		if !getChanged {
			cmd = snap.writer.ZAddXX(snap.writer.Context(), key, member...)
		} else {
			cmd = snap.writer.ZAddXXCh(snap.writer.Context(), key, member...)
		}
	default:
		return errors.New("Redis ZAdd Failed: " + "Set Condition is Required")
	}

	return z.core.handleIntCmd2(cmd, "Redis ZAdd Failed: ")
}

// ZCard returns the sorted set cardinality (number of elements) of the sorted set stored at key
func (z *SORTED_SET) ZCard(key string) (val int64, notFound bool, err error) {
	if z == nil {
		return 0, false, errors.New("Redis SORTED_SET ZCard Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return 0, false, errors.New("Redis SORTED_SET ZCard Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZCard", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZCard-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZCard-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZCard-Not-Result-Count", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = z.zcardInternal(snap, key)
		return val, notFound, err
	} else {
		return z.zcardInternal(snap, key)
	}
}

// zcardInternal returns the sorted set cardinality (number of elements) of the sorted set stored at key
func (z *SORTED_SET) zcardInternal(snap redisConnSnapshot, key string) (val int64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZCard Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis ZCard Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ZCard Failed: " + "Key is Required")
	}

	cmd := snap.reader.ZCard(snap.reader.Context(), key)
	return z.core.handleIntCmd(cmd, "Redis ZCard Failed: ")
}

// ZCount returns the number of elements in the sorted set at key with a score between min and max
func (z *SORTED_SET) ZCount(key string, min string, max string) (val int64, notFound bool, err error) {
	if z == nil {
		return 0, false, errors.New("Redis SORTED_SET ZCount Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return 0, false, errors.New("Redis SORTED_SET ZCount Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZCount", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZCount-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZCount-Min", min))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZCount-Max", max))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZCount-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZCount-Result-Count", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = z.zcountInternal(snap, key, min, max)
		return val, notFound, err
	} else {
		return z.zcountInternal(snap, key, min, max)
	}
}

// zcountInternal returns the number of elements in the sorted set at key with a score between min and max
func (z *SORTED_SET) zcountInternal(snap redisConnSnapshot, key string, min string, max string) (val int64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZCount Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis ZCount Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ZCount Failed: " + "Key is Required")
	}

	if len(min) <= 0 {
		return 0, false, errors.New("Redis ZCount Failed: " + "Min is Required")
	}

	if len(max) <= 0 {
		return 0, false, errors.New("Redis ZCount Failed: " + "Max is Required")
	}

	cmd := snap.reader.ZCount(snap.reader.Context(), key, min, max)
	return z.core.handleIntCmd(cmd, "Redis ZCount Failed: ")
}

// ZIncr will increment the score of member in sorted set at key
//
// Also support for ZIncrXX (member must exist), ZIncrNX (member must not exist)
func (z *SORTED_SET) ZIncr(key string, setCondition redissetcondition.RedisSetCondition, member *redis.Z) (err error) {
	if z == nil {
		return errors.New("Redis SORTED_SET ZIncr Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return errors.New("Redis SORTED_SET ZIncr Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZIncr", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZIncr-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZIncr-Condition", setCondition))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZIncr-Member", member))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = z.zincrInternal(snap, key, setCondition, member)
		return err
	} else {
		return z.zincrInternal(snap, key, setCondition, member)
	}
}

// zincrInternal will increment the score of member in sorted set at key
//
// Also support for ZIncrXX (member must exist), ZIncrNX (member must not exist)
func (z *SORTED_SET) zincrInternal(snap redisConnSnapshot, key string, setCondition redissetcondition.RedisSetCondition, member *redis.Z) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZIncr Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis ZIncr Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis ZIncr Failed: " + "Key is Required")
	}

	if !setCondition.Valid() || setCondition == redissetcondition.UNKNOWN {
		return errors.New("Redis ZIncr Failed: " + "Set Condition is Required")
	}

	if member == nil {
		return errors.New("Redis ZIncr Failed: " + "Member is Required")
	}

	var cmd *redis.FloatCmd

	switch setCondition {
	case redissetcondition.Normal:
		cmd = snap.writer.ZIncr(snap.writer.Context(), key, member)
	case redissetcondition.SetIfNotExists:
		cmd = snap.writer.ZIncrNX(snap.writer.Context(), key, member)
	case redissetcondition.SetIfExists:
		cmd = snap.writer.ZIncrXX(snap.writer.Context(), key, member)
	default:
		return errors.New("Redis ZIncr Failed: " + "Set Condition is Required")
	}

	if _, _, err := z.core.handleFloatCmd(cmd, "Redis ZIncr Failed: "); err != nil {
		return err
	} else {
		return nil
	}
}

// ZIncrBy increments or decrements the score of member in the sorted set stored at key, with custom increment value,
// If member does not exist in the sorted set, it is added with increment value as its score,
// If key does not exist, a new sorted set with the specified member as its sole member is created
//
// # Error is returned if the value stored at key is not a sorted set
//
// Score should be string representation of a numeric value, and accepts double precision floating point numbers
// To decrement, use negative value
func (z *SORTED_SET) ZIncrBy(key string, increment float64, member string) (err error) {
	if z == nil {
		return errors.New("Redis SORTED_SET ZIncrBy Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return errors.New("Redis SORTED_SET ZIncrBy Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZIncrBy", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZIncrBy-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZIncrBy-Increment", increment))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZIncrBy-Member", member))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = z.zincrByInternal(snap, key, increment, member)
		return err
	} else {
		return z.zincrByInternal(snap, key, increment, member)
	}
}

// zincrByInternal increments or decrements the score of member in the sorted set stored at key, with custom increment value,
// If member does not exist in the sorted set, it is added with increment value as its score,
// If key does not exist, a new sorted set with the specified member as its sole member is created
//
// # Error is returned if the value stored at key is not a sorted set
//
// Score should be string representation of a numeric value, and accepts double precision floating point numbers
// To decrement, use negative value
func (z *SORTED_SET) zincrByInternal(snap redisConnSnapshot, key string, increment float64, member string) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZIncrBy Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis ZIncrBy Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis ZIncrBy Failed: " + "Key is Required")
	}

	if increment == 0.00 {
		return errors.New("Redis ZIncrBy Failed: " + "Increment is Required")
	}

	if len(member) <= 0 {
		return errors.New("Redis ZIncrBy Failed: " + "Member is Required")
	}

	cmd := snap.writer.ZIncrBy(snap.writer.Context(), key, increment, member)
	if _, _, err := z.core.handleFloatCmd(cmd, "Redis ZIncrBy Failed: "); err != nil {
		return err
	} else {
		return nil
	}
}

// ZInterStore computes the intersection of numKeys sorted set given by the specified keys,
// and stores the result in 'destination'
//
// numKeys (input keys) are required
//
// If 'destination' already exists, it is overwritten
//
// Default Logic:
//
//	Resulting score of an element is the sum of its scores in the sorted set where it exists
func (z *SORTED_SET) ZInterStore(keyDest string, store *redis.ZStore) (err error) {
	if z == nil {
		return errors.New("Redis SORTED_SET ZInterStore Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return errors.New("Redis SORTED_SET ZInterStore Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZInterStore", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZInterStore-KeyDest", keyDest))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZInterStore-Input-Args", store))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = z.zinterStoreInternal(snap, keyDest, store)
		return err
	} else {
		return z.zinterStoreInternal(snap, keyDest, store)
	}
}

// zinterStoreInternal computes the intersection of numKeys sorted set given by the specified keys,
// and stores the result in 'destination'
//
// numKeys (input keys) are required
//
// If 'destination' already exists, it is overwritten
//
// Default Logic:
//
//	Resulting score of an element is the sum of its scores in the sorted set where it exists
func (z *SORTED_SET) zinterStoreInternal(snap redisConnSnapshot, keyDest string, store *redis.ZStore) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZInterStore Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis ZInterStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis ZInterStore Failed: " + "Key Destination is Required")
	}

	if store == nil {
		return errors.New("Redis ZInterStore Failed: " + "Store is Required")
	}

	cmd := snap.writer.ZInterStore(snap.writer.Context(), keyDest, store)
	return z.core.handleIntCmd2(cmd, "Redis ZInterStore Failed: ")
}

// ZLexCount returns the number of elements in the sorted set at key, with a value between min and max
func (z *SORTED_SET) ZLexCount(key string, min string, max string) (val int64, notFound bool, err error) {
	if z == nil {
		return 0, false, errors.New("Redis SORTED_SET ZLexCount Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return 0, false, errors.New("Redis SORTED_SET ZLexCount Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZLexCount", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZLexCount-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZLexCount-Min", min))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZLexCount-Max", max))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZLexCount-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZLexCount-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = z.zlexCountInternal(snap, key, min, max)
		return val, notFound, err
	} else {
		return z.zlexCountInternal(snap, key, min, max)
	}
}

// zlexCountInternal returns the number of elements in the sorted set at key, with a value between min and max
func (z *SORTED_SET) zlexCountInternal(snap redisConnSnapshot, key string, min string, max string) (val int64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZLexCount Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis ZLexCount Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ZLexCount Failed: " + "Key is Required")
	}

	if len(min) <= 0 {
		return 0, false, errors.New("Redis ZLexCount Failed: " + "Min is Required")
	}

	if len(max) <= 0 {
		return 0, false, errors.New("Redis ZLexCount Failed: " + "Max is Required")
	}

	cmd := snap.reader.ZLexCount(snap.reader.Context(), key, min, max)
	return z.core.handleIntCmd(cmd, "Redis ZLexCount Failed: ")
}

// ZPopMax removes and returns up to the count of members with the highest scores in the sorted set stored at key,
// Specifying more count than members will not cause error, rather given back smaller result set,
// Returning elements ordered with highest score first, then subsequent and so on
func (z *SORTED_SET) ZPopMax(key string, count ...int64) (outputSlice []redis.Z, notFound bool, err error) {
	if z == nil {
		return nil, false, errors.New("Redis SORTED_SET ZPopMax Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return nil, false, errors.New("Redis SORTED_SET ZPopMax Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZPopMax", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZPopMax-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZPopMax-Count", count))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZPopMax-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZPopMax-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = z.zpopMaxInternal(snap, key, count...)
		return outputSlice, notFound, err
	} else {
		return z.zpopMaxInternal(snap, key, count...)
	}
}

// zpopMaxInternal removes and returns up to the count of members with the highest scores in the sorted set stored at key,
// Specifying more count than members will not cause error, rather given back smaller result set,
// Returning elements ordered with highest score first, then subsequent and so on
func (z *SORTED_SET) zpopMaxInternal(snap redisConnSnapshot, key string, count ...int64) (outputSlice []redis.Z, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZPopMax Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis ZPopMax Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZPopMax Failed: " + "Key is Required")
	}

	var cmd *redis.ZSliceCmd

	if len(count) <= 0 {
		cmd = snap.writer.ZPopMax(snap.writer.Context(), key)
	} else {
		cmd = snap.writer.ZPopMax(snap.writer.Context(), key, count...)
	}

	return z.core.handleZSliceCmd(cmd, "Redis ZPopMax Failed: ")
}

// ZPopMin removes and returns up to the count of members with the lowest scores in the sorted set stored at key,
// Specifying more count than members will not cause error, rather given back smaller result set,
// Returning elements ordered with lowest score first, then subsequently higher score, and so on
func (z *SORTED_SET) ZPopMin(key string, count ...int64) (outputSlice []redis.Z, notFound bool, err error) {
	if z == nil {
		return nil, false, errors.New("Redis SORTED_SET ZPopMin Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return nil, false, errors.New("Redis SORTED_SET ZPopMin Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZPopMin", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZPopMin-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZPopMin-Count", count))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZPopMin-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZPopMin-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = z.zpopMinInternal(snap, key, count...)
		return outputSlice, notFound, err
	} else {
		return z.zpopMinInternal(snap, key, count...)
	}
}

// zpopMinInternal removes and returns up to the count of members with the lowest scores in the sorted set stored at key,
// Specifying more count than members will not cause error, rather given back smaller result set,
// Returning elements ordered with lowest score first, then subsequently higher score, and so on
func (z *SORTED_SET) zpopMinInternal(snap redisConnSnapshot, key string, count ...int64) (outputSlice []redis.Z, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZPopMin Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis ZPopMin Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZPopMin Failed: " + "Key is Required")
	}

	var cmd *redis.ZSliceCmd

	if len(count) <= 0 {
		cmd = snap.writer.ZPopMin(snap.writer.Context(), key)
	} else {
		cmd = snap.writer.ZPopMin(snap.writer.Context(), key, count...)
	}

	return z.core.handleZSliceCmd(cmd, "Redis ZPopMin Failed: ")
}

// ZRange returns the specified range of elements in the sorted set stored at key,
// The elements are considered to be ordered form lowest to the highest score,
// Lexicographical order is used for elements with equal score
//
// start and stop are both zero-based indexes,
// start and stop may be negative, where -1 is the last index, and -2 is the second to the last index,
// start and stop are inclusive range
func (z *SORTED_SET) ZRange(key string, start int64, stop int64) (outputSlice []string, notFound bool, err error) {
	if z == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRange Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRange Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRange", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRange-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRange-Start", start))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRange-Stop", stop))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRange-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRange-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = z.zrangeInternal(snap, key, start, stop)
		return outputSlice, notFound, err
	} else {
		return z.zrangeInternal(snap, key, start, stop)
	}
}

// zrangeInternal returns the specified range of elements in the sorted set stored at key,
// The elements are considered to be ordered form lowest to the highest score,
// Lexicographical order is used for elements with equal score
//
// start and stop are both zero-based indexes,
// start and stop may be negative, where -1 is the last index, and -2 is the second to the last index,
// start and stop are inclusive range
func (z *SORTED_SET) zrangeInternal(snap redisConnSnapshot, key string, start int64, stop int64) (outputSlice []string, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRange Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis ZRange Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRange Failed: " + "Key is Required")
	}

	cmd := snap.reader.ZRange(snap.reader.Context(), key, start, stop)
	return z.core.handleStringSliceCmd(cmd, "Redis ZRange Failed: ")
}

// ZRangeByLex returns all the elements in the sorted set at key with a value between min and max
func (z *SORTED_SET) ZRangeByLex(key string, opt *redis.ZRangeBy) (outputSlice []string, notFound bool, err error) {
	if z == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRangeByLex Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRangeByLex Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRangeByLex", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByLex-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByLex-Input-Args", opt))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByLex-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByLex-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = z.zrangeByLexInternal(snap, key, opt)
		return outputSlice, notFound, err
	} else {
		return z.zrangeByLexInternal(snap, key, opt)
	}
}

// zrangeByLexInternal returns all the elements in the sorted set at key with a value between min and max
func (z *SORTED_SET) zrangeByLexInternal(snap redisConnSnapshot, key string, opt *redis.ZRangeBy) (outputSlice []string, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Key is Required")
	}

	if opt == nil {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Opt is Required")
	}

	cmd := snap.reader.ZRangeByLex(snap.reader.Context(), key, opt)
	return z.core.handleStringSliceCmd(cmd, "Redis ZRangeByLex Failed: ")
}

// ZRangeByScore returns all the elements in the sorted set at key with a score between min and max,
// Elements are considered to be ordered from low to high scores
func (z *SORTED_SET) ZRangeByScore(key string, opt *redis.ZRangeBy) (outputSlice []string, notFound bool, err error) {
	if z == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRangeByScore Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRangeByScore Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRangeByScore", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByScore-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByScore-Input-Args", opt))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByScore-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByScore-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = z.zrangeByScoreInternal(snap, key, opt)
		return outputSlice, notFound, err
	} else {
		return z.zrangeByScoreInternal(snap, key, opt)
	}
}

// zrangeByScoreInternal returns all the elements in the sorted set at key with a score between min and max,
// Elements are considered to be ordered from low to high scores
func (z *SORTED_SET) zrangeByScoreInternal(snap redisConnSnapshot, key string, opt *redis.ZRangeBy) (outputSlice []string, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRangeByScore Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis ZRangeByScore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRangeByScore Failed: " + "Key is Required")
	}

	if opt == nil {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Opt is Required")
	}

	cmd := snap.reader.ZRangeByScore(snap.reader.Context(), key, opt)
	return z.core.handleStringSliceCmd(cmd, "Redis ZRangeByScore Failed: ")
}

// ZRangeByScoreWithScores returns all the elements in the sorted set at key with a score between min and max,
// Elements are considered to be ordered from low to high scores
func (z *SORTED_SET) ZRangeByScoreWithScores(key string, opt *redis.ZRangeBy) (outputSlice []redis.Z, notFound bool, err error) {
	if z == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRangeByScoreWithScores Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRangeByScoreWithScores Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRangeByScoreWithScores", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByScoreWithScores-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByScoreWithScores-Input-Args", opt))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByScoreWithScores-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRangeByScoreWithScores-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = z.zrangeByScoreWithScoresInternal(snap, key, opt)
		return outputSlice, notFound, err
	} else {
		return z.zrangeByScoreWithScoresInternal(snap, key, opt)
	}
}

// zrangeByScoreWithScoresInternal returns all the elements in the sorted set at key with a score between min and max,
// Elements are considered to be ordered from low to high scores
func (z *SORTED_SET) zrangeByScoreWithScoresInternal(snap redisConnSnapshot, key string, opt *redis.ZRangeBy) (outputSlice []redis.Z, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRangeByScoreWithScores Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis ZRangeByScoreWithScores Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRangeByScoreWithScores Failed: " + "Key is Required")
	}

	if opt == nil {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Opt is Required")
	}

	cmd := snap.reader.ZRangeByScoreWithScores(snap.reader.Context(), key, opt)
	return z.core.handleZSliceCmd(cmd, "ZRangeByLex")
}

// ZRank returns the rank of member in the sorted set stored at key, with the scores ordered from low to high,
// The rank (or index) is zero-based, where lowest member is index 0 (or rank 0)
func (z *SORTED_SET) ZRank(key string, member string) (val int64, notFound bool, err error) {
	if z == nil {
		return 0, false, errors.New("Redis SORTED_SET ZRank Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return 0, false, errors.New("Redis SORTED_SET ZRank Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRank", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRank-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRank-Member", member))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRank-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRank-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = z.zrankInternal(snap, key, member)
		return val, notFound, err
	} else {
		return z.zrankInternal(snap, key, member)
	}
}

// zrankInternal returns the rank of member in the sorted set stored at key, with the scores ordered from low to high,
// The rank (or index) is zero-based, where lowest member is index 0 (or rank 0)
func (z *SORTED_SET) zrankInternal(snap redisConnSnapshot, key string, member string) (val int64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZRank Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis ZRank Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ZRank Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return 0, false, errors.New("Redis ZRank Failed: " + "Member is Required")
	}

	cmd := snap.reader.ZRank(snap.reader.Context(), key, member)
	return z.core.handleIntCmd(cmd, "Redis ZRank Failed: ")
}

// ZRem removes the specified members from the stored set stored at key,
// Non-existing members are ignored
//
// Error is returned if the value at key is not a sorted set
func (z *SORTED_SET) ZRem(key string, member ...interface{}) (err error) {
	if z == nil {
		return errors.New("Redis SORTED_SET ZRem Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return errors.New("Redis SORTED_SET ZRem Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRem", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRem-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRem-Members", member))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = z.zremInternal(snap, key, member...)
		return err
	} else {
		return z.zremInternal(snap, key, member...)
	}
}

// zremInternal removes the specified members from the stored set stored at key,
// Non-existing members are ignored
//
// Error is returned if the value at key is not a sorted set
func (z *SORTED_SET) zremInternal(snap redisConnSnapshot, key string, member ...interface{}) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZRem Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis ZRem Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis ZRem Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return errors.New("Redis ZRem Failed: " + "Member is Required")
	}

	cmd := snap.writer.ZRem(snap.writer.Context(), key, member...)
	return z.core.handleIntCmd2(cmd, "Redis ZRem Failed: ")
}

// ZRemRangeByLex removes all elements in the sorted set stored at key, between the lexicographical range specified by min and max
func (z *SORTED_SET) ZRemRangeByLex(key string, min string, max string) (err error) {
	if z == nil {
		return errors.New("Redis SORTED_SET ZRemRangeByLex Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return errors.New("Redis SORTED_SET ZRemRangeByLex Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRemRangeByLex", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRemRangeByLex-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRemRangeByLex-Min", min))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRemRangeByLex-Max", max))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = z.zremRangeByLexInternal(snap, key, min, max)
		return err
	} else {
		return z.zremRangeByLexInternal(snap, key, min, max)
	}
}

// zremRangeByLexInternal removes all elements in the sorted set stored at key, between the lexicographical range specified by min and max
func (z *SORTED_SET) zremRangeByLexInternal(snap redisConnSnapshot, key string, min string, max string) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZRemRangeByLex Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis ZRemRangeByLex Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis ZRemRangeByLex Failed: " + "Key is Required")
	}

	if len(min) <= 0 {
		return errors.New("Redis ZRemRangeByLex Failed: " + "Min is Required")
	}

	if len(max) <= 0 {
		return errors.New("Redis ZRemRangeByLex Failed: " + "Max is Required")
	}

	cmd := snap.writer.ZRemRangeByLex(snap.writer.Context(), key, min, max)
	return z.core.handleIntCmd2(cmd, "Redis ZRemRangeByLex Failed: ")
}

// ZRemRangeByScore removes all elements in the sorted set stored at key, with a score between min and max (inclusive)
func (z *SORTED_SET) ZRemRangeByScore(key string, min string, max string) (err error) {
	if z == nil {
		return errors.New("Redis SORTED_SET ZRemRangeByScore Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return errors.New("Redis SORTED_SET ZRemRangeByScore Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRemRangeByScore", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRemRangeByScore-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRemRangeByScore-Min", min))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRemRangeByScore-Max", max))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = z.zremRangeByScoreInternal(snap, key, min, max)
		return err
	} else {
		return z.zremRangeByScoreInternal(snap, key, min, max)
	}
}

// zremRangeByScoreInternal removes all elements in the sorted set stored at key, with a score between min and max (inclusive)
func (z *SORTED_SET) zremRangeByScoreInternal(snap redisConnSnapshot, key string, min string, max string) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZRemRangeByScore Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis ZRemRangeByScore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis ZRemRangeByScore Failed: " + "Key is Required")
	}

	if len(min) <= 0 {
		return errors.New("Redis ZRemRangeByScore Failed: " + "Min is Required")
	}

	if len(max) <= 0 {
		return errors.New("Redis ZRemRangeByScore Failed: " + "Max is Required")
	}

	cmd := snap.writer.ZRemRangeByScore(snap.writer.Context(), key, min, max)
	return z.core.handleIntCmd2(cmd, "Redis ZRemRangeByScore Failed: ")
}

// ZRemRangeByRank removes all elements in the sorted set stored at key, with rank between start and stop
//
// Both start and stop are zero-based,
// Both start and stop can be negative, where -1 is the element with highest score, -2 is the element with next to highest score, and so on
func (z *SORTED_SET) ZRemRangeByRank(key string, start int64, stop int64) (err error) {
	if z == nil {
		return errors.New("Redis SORTED_SET ZRemRangeByRank Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return errors.New("Redis SORTED_SET ZRemRangeByRank Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRemRangeByRank", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRemRangeByRank-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRemRangeByRank-Start", start))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRemRangeByRank-Stop", stop))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = z.zremRangeByRankInternal(snap, key, start, stop)
		return err
	} else {
		return z.zremRangeByRankInternal(snap, key, start, stop)
	}
}

// zremRangeByRankInternal removes all elements in the sorted set stored at key, with rank between start and stop
//
// Both start and stop are zero-based,
// Both start and stop can be negative, where -1 is the element with highest score, -2 is the element with next to highest score, and so on
func (z *SORTED_SET) zremRangeByRankInternal(snap redisConnSnapshot, key string, start int64, stop int64) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZRemRangeByRank Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis ZRemRangeByRank Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis ZRemRangeByRank Failed: " + "Key is Required")
	}

	cmd := snap.writer.ZRemRangeByRank(snap.writer.Context(), key, start, stop)
	return z.core.handleIntCmd2(cmd, "Redis ZRemRangeByRank Failed: ")
}

// ZRevRange returns the specified range of elements in the sorted set stored at key,
// With elements ordered from highest to the lowest score,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) ZRevRange(key string, start int64, stop int64) (outputSlice []string, notFound bool, err error) {
	if z == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRevRange Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRevRange Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRevRange", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRange-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRange-Start", start))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRange-Stop", stop))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRange-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRange-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = z.zrevRangeInternal(snap, key, start, stop)
		return outputSlice, notFound, err
	} else {
		return z.zrevRangeInternal(snap, key, start, stop)
	}
}

// zrevRangeInternal returns the specified range of elements in the sorted set stored at key,
// With elements ordered from highest to the lowest score,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) zrevRangeInternal(snap redisConnSnapshot, key string, start int64, stop int64) (outputSlice []string, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRevRange Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis ZRevRange Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRevRange Failed: " + "Key is Required")
	}

	cmd := snap.reader.ZRevRange(snap.reader.Context(), key, start, stop)
	return z.core.handleStringSliceCmd(cmd, "Redis ZRevRange Failed: ")
}

// ZRevRangeWithScores returns the specified range of elements (with scores) in the sorted set stored at key,
// With elements ordered from highest to the lowest score,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) ZRevRangeWithScores(key string, start int64, stop int64) (outputSlice []redis.Z, notFound bool, err error) {
	if z == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRevRangeWithScores Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRevRangeWithScores Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRevRangeWithScores", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRangeWithScores-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRangeWithScores-Start", start))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRangeWithScores-Stop", stop))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRangeWithScores-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRangeWithScores-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = z.zrevRangeWithScoresInternal(snap, key, start, stop)
		return outputSlice, notFound, err
	} else {
		return z.zrevRangeWithScoresInternal(snap, key, start, stop)
	}
}

// zrevRangeWithScoresInternal returns the specified range of elements (with scores) in the sorted set stored at key,
// With elements ordered from highest to the lowest score,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) zrevRangeWithScoresInternal(snap redisConnSnapshot, key string, start int64, stop int64) (outputSlice []redis.Z, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRevRangeWithScores Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis ZRevRangeWithScores Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRevRangeWithScores Failed: " + "Key is Required")
	}

	cmd := snap.reader.ZRevRangeWithScores(snap.reader.Context(), key, start, stop)
	return z.core.handleZSliceCmd(cmd, "Redis ZRevRangeWithScores Failed: ")
}

// ZRevRangeByScoreWithScores returns all the elements (with scores) in the sorted set at key, with a score between max and min (inclusive),
// With elements ordered from highest to lowest scores,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) ZRevRangeByScoreWithScores(key string, opt *redis.ZRangeBy) (outputSlice []redis.Z, notFound bool, err error) {
	if z == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRevRangeByScoreWithScores Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return nil, false, errors.New("Redis SORTED_SET ZRevRangeByScoreWithScores Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRevRangeByScoreWithScores", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRangeByScoreWithScores-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRangeByScoreWithScores-Input-Args", opt))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRangeByScoreWithScores-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRangeByScoreWithScores-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = z.zrevRangeByScoreWithScoresInternal(snap, key, opt)
		return outputSlice, notFound, err
	} else {
		return z.zrevRangeByScoreWithScoresInternal(snap, key, opt)
	}
}

// zrevRangeByScoreWithScoresInternal returns all the elements (with scores) in the sorted set at key, with a score between max and min (inclusive),
// With elements ordered from highest to lowest scores,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) zrevRangeByScoreWithScoresInternal(snap redisConnSnapshot, key string, opt *redis.ZRangeBy) (outputSlice []redis.Z, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRevRangeByScoreWithScores Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis ZRevRangeByScoreWithScores Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRevRangeByScoreWithScores Failed: " + "Key is Required")
	}

	if opt == nil {
		return nil, false, errors.New("Redis ZRevRangeByScoreWithScores Failed: " + "Opt is Required")
	}

	cmd := snap.reader.ZRevRangeByScoreWithScores(snap.reader.Context(), key, opt)
	return z.core.handleZSliceCmd(cmd, "Redis ZRevRangeByScoreWithScores Failed: ")
}

// ZRevRank returns the rank of member in the sorted set stored at key, with the scores ordered from high to low,
// Rank (index) is ordered from high to low, and is zero-based, where 0 is the highest rank (index)
// ZRevRank is opposite of ZRank
func (z *SORTED_SET) ZRevRank(key string, member string) (val int64, notFound bool, err error) {
	if z == nil {
		return 0, false, errors.New("Redis SORTED_SET ZRevRank Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return 0, false, errors.New("Redis SORTED_SET ZRevRank Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRevRank", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRank-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRank-Member", member))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRank-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZRevRank-Result-Rank", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = z.zrevRankInternal(snap, key, member)
		return val, notFound, err
	} else {
		return z.zrevRankInternal(snap, key, member)
	}
}

// zrevRankInternal returns the rank of member in the sorted set stored at key, with the scores ordered from high to low,
// Rank (index) is ordered from high to low, and is zero-based, where 0 is the highest rank (index)
// ZRevRank is opposite of ZRank
func (z *SORTED_SET) zrevRankInternal(snap redisConnSnapshot, key string, member string) (val int64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZRevRank Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis ZRevRank Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ZRevRank Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return 0, false, errors.New("Redis ZRevRank Failed: " + "Member is Required")
	}

	cmd := snap.reader.ZRevRank(snap.reader.Context(), key, member)
	return z.core.handleIntCmd(cmd, "Redis ZRevRank Failed: ")
}

// ZScan is used to incrementally iterate over a sorted set of fields stored at key，
// ZScan is a cursor based iterator, at every call of the command, redis returns an updated cursor that client must use for next call to sort,
//
// start iteration = cursor set to 0
// stop iteration = when redis returns cursor value of 0
//
// match = filters elements based on match filter, for elements retrieved from redis before return to client
//
//	glob-style patterns:
//		1) h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//		2) h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//		3) h*llo = * represents any single or more char match (hllo, heeeelo match)
//		4) h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//		5) h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//		6) h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//		7) Use \ to escape special characters if needing to match verbatim
//
// count = hint to redis count of elements to retrieve in the call
func (z *SORTED_SET) ZScan(key string, cursor uint64, match string, count int64) (outputKeys []string, outputCursor uint64, err error) {
	if z == nil {
		return nil, 0, errors.New("Redis SORTED_SET ZScan Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return nil, 0, errors.New("Redis SORTED_SET ZScan Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZScan", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZScan-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZScan-Cursor", cursor))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZScan-Match", match))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZScan-Count", count))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZScan-Result-Keys", outputKeys))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZScan-Result-Cursor", outputCursor))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputKeys, outputCursor, err = z.zscanInternal(snap, key, cursor, match, count)
		return outputKeys, outputCursor, err
	} else {
		return z.zscanInternal(snap, key, cursor, match, count)
	}
}

// zscanInternal is used to incrementally iterate over a sorted set of fields stored at key，
// ZScan is a cursor based iterator, at every call of the command, redis returns an updated cursor that client must use for next call to sort,
//
// start iteration = cursor set to 0
// stop iteration = when redis returns cursor value of 0
//
// match = filters elements based on match filter, for elements retrieved from redis before return to client
//
//	glob-style patterns:
//		1) h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//		2) h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//		3) h*llo = * represents any single or more char match (hllo, heeeelo match)
//		4) h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//		5) h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//		6) h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//		7) Use \ to escape special characters if needing to match verbatim
//
// count = hint to redis count of elements to retrieve in the call
func (z *SORTED_SET) zscanInternal(snap redisConnSnapshot, key string, cursor uint64, match string, count int64) (outputKeys []string, outputCursor uint64, err error) {
	// validate
	if z.core == nil {
		return nil, 0, errors.New("Redis ZScan Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, 0, errors.New("Redis ZScan Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, 0, errors.New("Redis ZScan Failed: " + "Key is Required")
	}

	if count < 0 {
		return nil, 0, errors.New("Redis ZScan Failed: " + "Count Must Be Zero or Greater")
	}

	cmd := snap.reader.ZScan(snap.reader.Context(), key, cursor, match, count)
	return z.core.handleScanCmd(cmd, "Redis ZScan Failed: ")
}

// ZScore returns the score of member in the sorted set at key,
// if member is not existent in the sorted set, or key does not exist, nil is returned
func (z *SORTED_SET) ZScore(key string, member string) (val float64, notFound bool, err error) {
	if z == nil {
		return 0.0, false, errors.New("Redis SORTED_SET ZScore Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return 0.0, false, errors.New("Redis SORTED_SET ZScore Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZScore", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZScore-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZScore-Member", member))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZScore-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZScore-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = z.zscoreInternal(snap, key, member)
		return val, notFound, err
	} else {
		return z.zscoreInternal(snap, key, member)
	}
}

// zscoreInternal returns the score of member in the sorted set at key,
// if member is not existent in the sorted set, or key does not exist, nil is returned
func (z *SORTED_SET) zscoreInternal(snap redisConnSnapshot, key string, member string) (val float64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZScore Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis ZScore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ZScore Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return 0, false, errors.New("Redis ZScore Failed: " + "Member is Required")
	}

	cmd := snap.reader.ZScore(snap.reader.Context(), key, member)
	return z.core.handleFloatCmd(cmd, "Redis ZScore Failed: ")
}

// ZUnionStore computes the union of numKeys sorted set given by the specified keys,
// and stores the result in 'destination'
//
// numKeys (input keys) are required
func (z *SORTED_SET) ZUnionStore(keyDest string, store *redis.ZStore) (err error) {
	if z == nil {
		return errors.New("Redis SORTED_SET ZUnionStore Failed: SORTED_SET receiver is nil")
	}
	if z.core == nil {
		return errors.New("Redis SORTED_SET ZUnionStore Failed: Redis core is nil")
	}
	snap := z.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZUnionStore", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZUnionStore-KeyDest", keyDest))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ZUnionStore-Input-Keys", store))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = z.zunionStoreInternal(snap, keyDest, store)
		return err
	} else {
		return z.zunionStoreInternal(snap, keyDest, store)
	}
}

// zunionStoreInternal computes the union of numKeys sorted set given by the specified keys,
// and stores the result in 'destination'
//
// numKeys (input keys) are required
func (z *SORTED_SET) zunionStoreInternal(snap redisConnSnapshot, keyDest string, store *redis.ZStore) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZUnionStore Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis ZUnionStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis ZUnionStore Failed: " + "Key Destination is Required")
	}

	if store == nil {
		return errors.New("Redis ZUnionStore Failed: " + "Store is Required")
	}

	cmd := snap.writer.ZUnionStore(snap.writer.Context(), keyDest, store)
	return z.core.handleIntCmd2(cmd, "Redis ZUnionStore Failed: ")
}

// ----------------------------------------------------------------------------------------------------------------
// GEO functions
// ----------------------------------------------------------------------------------------------------------------

// GeoAdd will add geospatial info (lat, lon, name) to the specified key,
// data is stored into the key as a sorted set,
// supports later query by radius with GeoRadius or GeoRadiusByMember commands
//
// valid longitude = -180 to 180 degrees
// valid latitude = -85.05112878 to 85.05112878 degrees
//
// Use ZREM to remove Geo Key (since there is no GEODEL Command)
func (g *GEO) GeoAdd(key string, geoLocation *redis.GeoLocation) (err error) {
	if g == nil {
		return errors.New("Redis GEO GeoAdd Failed: GEO receiver is nil")
	}
	if g.core == nil {
		return errors.New("Redis GEO GeoAdd Failed: Redis core is nil")
	}
	snap := g.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoAdd", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoAdd-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoAdd-Location", geoLocation))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = g.geoAddInternal(snap, key, geoLocation)
		return err
	} else {
		return g.geoAddInternal(snap, key, geoLocation)
	}
}

// geoAddInternal will add geospatial info (lat, lon, name) to the specified key,
// data is stored into the key as a sorted set,
// supports later query by radius with GeoRadius or GeoRadiusByMember commands
//
// valid longitude = -180 to 180 degrees
// valid latitude = -85.05112878 to 85.05112878 degrees
//
// Use ZREM to remove Geo Key (since there is no GEODEL Command)
func (g *GEO) geoAddInternal(snap redisConnSnapshot, key string, geoLocation *redis.GeoLocation) error {
	// validate
	if g.core == nil {
		return errors.New("Redis GeoAdd Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis GeoAdd Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis GeoAdd Failed: " + "Key is Required")
	}

	if geoLocation == nil {
		return errors.New("Redis GeoAdd Failed: " + "Geo Location is Required")
	}

	cmd := snap.writer.GeoAdd(snap.writer.Context(), key, geoLocation)
	_, _, err := g.core.handleIntCmd(cmd, "Redis GeoAdd Failed: ")
	return err
}

// GeoDist returns the distance between two members in the geospatial index represented by the sorted set
//
// unit = m (meters), km (kilometers), mi (miles), ft (feet)
func (g *GEO) GeoDist(key string, member1 string, member2 string, unit redisradiusunit.RedisRadiusUnit) (valDist float64, notFound bool, err error) {
	if g == nil {
		return 0.0, false, errors.New("Redis GEO GeoDist Failed: GEO receiver is nil")
	}
	if g.core == nil {
		return 0.0, false, errors.New("Redis GEO GeoDist Failed: Redis core is nil")
	}
	snap := g.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoDist", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoDist-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoDist-Member1", member1))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoDist-Member2", member2))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoDist-Radius-Unit", unit))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoDist-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoDist-Result-Distance", valDist))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valDist, notFound, err = g.geoDistInternal(snap, key, member1, member2, unit)
		return valDist, notFound, err
	} else {
		return g.geoDistInternal(snap, key, member1, member2, unit)
	}
}

// geoDistInternal returns the distance between two members in the geospatial index represented by the sorted set
//
// unit = m (meters), km (kilometers), mi (miles), ft (feet)
func (g *GEO) geoDistInternal(snap redisConnSnapshot, key string, member1 string, member2 string, unit redisradiusunit.RedisRadiusUnit) (valDist float64, notFound bool, err error) {
	// validate
	if g.core == nil {
		return 0.00, false, errors.New("Redis GeoDist Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0.00, false, errors.New("Redis GeoDist Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0.00, false, errors.New("Redis GeoDist Failed: " + "Key is Required")
	}

	if len(member1) <= 0 {
		return 0.00, false, errors.New("Redis GeoDist Failed: " + "Member 1 is Required")
	}

	if len(member2) <= 0 {
		return 0.00, false, errors.New("Redis GeoDist Failed: " + "Member 2 is Required")
	}

	if !unit.Valid() || unit == redisradiusunit.UNKNOWN {
		return 0.00, false, errors.New("Radius GeoDist Failed: " + "Radius Unit is Required")
	}

	cmd := snap.reader.GeoDist(snap.reader.Context(), key, member1, member2, unit.Key())
	return g.core.handleFloatCmd(cmd, "Redis GeoDist Failed: ")
}

// GeoHash returns valid GeoHash string representing the position of one or more elements in a sorted set (added by GeoAdd)
// This function returns a STANDARD GEOHASH as described on geohash.org site
func (g *GEO) GeoHash(key string, member ...string) (geoHashSlice []string, notFound bool, err error) {
	if g == nil {
		return nil, false, errors.New("Redis GEO GeoHash Failed: GEO receiver is nil")
	}
	if g.core == nil {
		return nil, false, errors.New("Redis GEO GeoHash Failed: Redis core is nil")
	}
	snap := g.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoHash", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoHash-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoHash-Members", member))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoHash-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoHash-Result-Positions", geoHashSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		geoHashSlice, notFound, err = g.geoHashInternal(snap, key, member...)
		return geoHashSlice, notFound, err
	} else {
		return g.geoHashInternal(snap, key, member...)
	}
}

// geoHashInternal returns valid GeoHash string representing the position of one or more elements in a sorted set (added by GeoAdd)
// This function returns a STANDARD GEOHASH as described on geohash.org site
func (g *GEO) geoHashInternal(snap redisConnSnapshot, key string, member ...string) (geoHashSlice []string, notFound bool, err error) {
	// validate
	if g.core == nil {
		return nil, false, errors.New("Redis GeoHash Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis GeoHash Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis GeoHash Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return nil, false, errors.New("Redis GeoHash Failed: " + "At Least 1 Member is Required")
	}

	cmd := snap.reader.GeoHash(snap.reader.Context(), key, member...)
	return g.core.handleStringSliceCmd(cmd, "Redis GeoHash Failed: ")
}

// GeoPos returns the position (longitude and latitude) of all the specified members of the geospatial index represented by the sorted set at key
func (g *GEO) GeoPos(key string, member ...string) (cmd *redis.GeoPosCmd, err error) {
	if g == nil {
		return nil, errors.New("Redis GEO GeoPos Failed: GEO receiver is nil")
	}
	if g.core == nil {
		return nil, errors.New("Redis GEO GeoPos Failed: Redis core is nil")
	}
	snap := g.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoPos", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoPos-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoPos-Members", member))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoPos-Result-Position", cmd))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		cmd, err = g.geoPosInternal(snap, key, member...)
		return cmd, err
	} else {
		return g.geoPosInternal(snap, key, member...)
	}
}

// geoPosInternal returns the position (longitude and latitude) of all the specified members of the geospatial index represented by the sorted set at key
func (g *GEO) geoPosInternal(snap redisConnSnapshot, key string, member ...string) (*redis.GeoPosCmd, error) {
	// validate
	if g.core == nil {
		return nil, errors.New("Redis GeoPos Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis GeoPos Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, errors.New("Redis GeoPos Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return nil, errors.New("Redis GeoPos Failed: " + "At Least 1 Member is Required")
	}

	cmd := snap.reader.GeoPos(snap.reader.Context(), key, member...)
	if cmd.Err() != nil && cmd.Err() != redis.Nil {
		return cmd, cmd.Err()
	}
	return cmd, nil
}

// GeoRadius returns the members of a sorted set populated with geospatial information using GeoAdd,
// which are within the borders of the area specified with the center location and the maximum distance from the center (the radius)
//
// radius = units are: m (meters), km (kilometers), mi (miles), ft (feet)
// withDist = return the distance of returned items from the specified center (using same unit as units specified in the radius)
// withCoord = return the longitude and latitude coordinates of the matching items
//
// asc = sort returned items from the nearest to the farthest, relative to the center
// desc = sort returned items from the farthest to the nearest, relative to the center
//
// count = optional limit of return items; default is return all items found, use count to limit the list
//
// store = store the items in a sorted set populated with their geospatial information
// storeDist = store the items in a sorted set populated with their distance from the center as a floating point number, in the same unit specified in the radius
func (g *GEO) GeoRadius(key string, longitude float64, latitude float64, query *redis.GeoRadiusQuery) (cmd *redis.GeoLocationCmd, err error) {
	if g == nil {
		return nil, errors.New("Redis GEO GeoRadius Failed: GEO receiver is nil")
	}
	if g.core == nil {
		return nil, errors.New("Redis GEO GeoRadius Failed: Redis core is nil")
	}
	snap := g.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoRadius", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadius-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadius-Longitude", longitude))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadius-Latitude", latitude))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadius-Query", query))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadius-Result-Location", cmd))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		cmd, err = g.geoRadiusInternal(snap, key, longitude, latitude, query)
		return cmd, err
	} else {
		return g.geoRadiusInternal(snap, key, longitude, latitude, query)
	}
}

// helper to defensively copy and normalize GeoRadiusQuery to avoid mutating caller input
// now also validates radius > 0 and STORE / STOREDIST compatibility
func validateGeoRadiusQuery(query *redis.GeoRadiusQuery, allowStore bool) (*redis.GeoRadiusQuery, error) {
	if query == nil {
		return nil, errors.New("Redis GeoRadius Failed: Query is Required")
	}

	// shallow copy is enough because fields are value types / slices
	q := *query

	// sanitize Sort
	if util.LenTrim(q.Sort) > 0 {
		switch strings.ToUpper(q.Sort) {
		case "ASC", "DESC":
			q.Sort = strings.ToUpper(q.Sort)
		default:
			q.Sort = ""
		}
	}

	// sanitize Unit
	if util.LenTrim(q.Unit) > 0 {
		switch strings.ToUpper(q.Unit) {
		case "M", "KM", "MI", "FT":
			q.Unit = strings.ToLower(q.Unit)
		default:
			return nil, errors.New("Redis GeoRadius Failed: Unit must be one of m|km|mi|ft")
		}
	} else {
		q.Unit = "m"
	}

	// validate radius
	if q.Radius <= 0 {
		return nil, errors.New("Redis GeoRadius Failed: Radius must be greater than 0")
	}

	// drop store options when not allowed (read-only helpers)
	if !allowStore {
		q.Store = ""
		q.StoreDist = ""
	} else if util.LenTrim(q.Store) > 0 && util.LenTrim(q.StoreDist) > 0 {
		return nil, errors.New("Redis GeoRadius Failed: STORE and STOREDIST cannot both be set")
	}

	return &q, nil
}

// geoRadiusInternal returns the members of a sorted set populated with geospatial information using GeoAdd,
// which are within the borders of the area specified with the center location and the maximum distance from the center (the radius)
//
// radius = units are: m (meters), km (kilometers), mi (miles), ft (feet)
// withDist = return the distance of returned items from the specified center (using same unit as units specified in the radius)
// withCoord = return the longitude and latitude coordinates of the matching items
//
// asc = sort returned items from the nearest to the farthest, relative to the center
// desc = sort returned items from the farthest to the nearest, relative to the center
//
// count = optional limit of return items; default is return all items found, use count to limit the list
//
// store = store the items in a sorted set populated with their geospatial information
// storeDist = store the items in a sorted set populated with their distance from the center as a floating point number, in the same unit specified in the radius
func (g *GEO) geoRadiusInternal(snap redisConnSnapshot, key string, longitude float64, latitude float64, query *redis.GeoRadiusQuery) (*redis.GeoLocationCmd, error) {
	// validate
	if g.core == nil {
		return nil, errors.New("Redis GeoRadius Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis GeoRadius Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, errors.New("Redis GeoRadius Failed: " + "Key is Required")
	}

	q, err := validateGeoRadiusQuery(query, false)
	if err != nil {
		return nil, err
	}

	cmd := snap.reader.GeoRadius(snap.reader.Context(), key, longitude, latitude, q)
	if cmd.Err() != nil && cmd.Err() != redis.Nil {
		return cmd, cmd.Err()
	}
	return cmd, nil
}

// GeoRadiusStore will store the members of a sorted set populated with geospatial information using GeoAdd,
// which are within the borders of the area specified with the center location and the maximum distance from the center (the radius)
//
// radius = units are: m (meters), km (kilometers), mi (miles), ft (feet)
// withDist = return the distance of returned items from the specified center (using same unit as units specified in the radius)
// withCoord = return the longitude and latitude coordinates of the matching items
//
// asc = sort returned items from the nearest to the farthest, relative to the center
// desc = sort returned items from the farthest to the nearest, relative to the center
//
// count = optional limit of return items; default is return all items found, use count to limit the list
//
// store = store the items in a sorted set populated with their geospatial information
// storeDist = store the items in a sorted set populated with their distance from the center as a floating point number, in the same unit specified in the radius
func (g *GEO) GeoRadiusStore(key string, longitude float64, latitude float64, query *redis.GeoRadiusQuery) (err error) {
	if g == nil {
		return errors.New("Redis GEO GeoRadiusStore Failed: GEO receiver is nil")
	}
	if g.core == nil {
		return errors.New("Redis GEO GeoRadiusStore Failed: Redis core is nil")
	}
	snap := g.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoRadiusStore", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadiusStore-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadiusStore-Longitude", longitude))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadiusStore-Latitude", latitude))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadiusStore-Query", query))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = g.geoRadiusStoreInternal(snap, key, longitude, latitude, query)
		return err
	} else {
		return g.geoRadiusStoreInternal(snap, key, longitude, latitude, query)
	}
}

// geoRadiusStoreInternal will store the members of a sorted set populated with geospatial information using GeoAdd,
// which are within the borders of the area specified with the center location and the maximum distance from the center (the radius)
//
// radius = units are: m (meters), km (kilometers), mi (miles), ft (feet)
// withDist = return the distance of returned items from the specified center (using same unit as units specified in the radius)
// withCoord = return the longitude and latitude coordinates of the matching items
//
// asc = sort returned items from the nearest to the farthest, relative to the center
// desc = sort returned items from the farthest to the nearest, relative to the center
//
// count = optional limit of return items; default is return all items found, use count to limit the list
//
// store = store the items in a sorted set populated with their geospatial information
// storeDist = store the items in a sorted set populated with their distance from the center as a floating point number, in the same unit specified in the radius
func (g *GEO) geoRadiusStoreInternal(snap redisConnSnapshot, key string, longitude float64, latitude float64, query *redis.GeoRadiusQuery) error {
	// validate
	if g.core == nil {
		return errors.New("Redis GeoRadiusStore Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis GeoRadiusStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis GeoRadiusStore Failed: " + "Key is Required")
	}

	q, err := validateGeoRadiusQuery(query, true) // <-- validation added
	if err != nil {
		return err
	}

	cmd := snap.writer.GeoRadiusStore(snap.writer.Context(), key, longitude, latitude, q)
	_, _, err = g.core.handleIntCmd(cmd, "Redis GeoRadiusStore Failed: ")
	return err
}

// GeoRadiusByMember is same as GeoRadius, except instead of taking as the center of the area to query (long lat),
// this takes the name of a member already existing inside the geospatial index represented by the sorted set
//
// # The position of the specified member is used as the center of the query
//
// radius = units are: m (meters), km (kilometers), mi (miles), ft (feet)
// withDist = return the distance of returned items from the specified center (using same unit as units specified in the radius)
// withCoord = return the longitude and latitude coordinates of the matching items
//
// asc = sort returned items from the nearest to the farthest, relative to the center
// desc = sort returned items from the farthest to the nearest, relative to the center
//
// count = optional limit of return items; default is return all items found, use count to limit the list
//
// store = store the items in a sorted set populated with their geospatial information
// storeDist = store the items in a sorted set populated with their distance from the center as a floating point number, in the same unit specified in the radius
func (g *GEO) GeoRadiusByMember(key string, member string, query *redis.GeoRadiusQuery) (cmd *redis.GeoLocationCmd, err error) {
	if g == nil {
		return nil, errors.New("Redis GEO GeoRadiusByMember Failed: GEO receiver is nil")
	}
	if g.core == nil {
		return nil, errors.New("Redis GEO GeoRadiusByMember Failed: Redis core is nil")
	}
	snap := g.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoRadiusByMember", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadiusByMember-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadiusByMember-Member", member))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadiusByMember-Query", query))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadiusByMember-Result-Location", cmd))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		cmd, err = g.geoRadiusByMemberInternal(snap, key, member, query)
		return cmd, err
	} else {
		return g.geoRadiusByMemberInternal(snap, key, member, query)
	}
}

// geoRadiusByMemberInternal is same as GeoRadius, except instead of taking as the center of the area to query (long lat),
// this takes the name of a member already existing inside the geospatial index represented by the sorted set
//
// # The position of the specified member is used as the center of the query
//
// radius = units are: m (meters), km (kilometers), mi (miles), ft (feet)
// withDist = return the distance of returned items from the specified center (using same unit as units specified in the radius)
// withCoord = return the longitude and latitude coordinates of the matching items
//
// asc = sort returned items from the nearest to the farthest, relative to the center
// desc = sort returned items from the farthest to the nearest, relative to the center
//
// count = optional limit of return items; default is return all items found, use count to limit the list
//
// store = store the items in a sorted set populated with their geospatial information
// storeDist = store the items in a sorted set populated with their distance from the center as a floating point number, in the same unit specified in the radius
func (g *GEO) geoRadiusByMemberInternal(snap redisConnSnapshot, key string, member string, query *redis.GeoRadiusQuery) (*redis.GeoLocationCmd, error) {
	// validate
	if g.core == nil {
		return nil, errors.New("Redis GeoRadiusByMember Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis GeoRadiusByMember Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, errors.New("Redis GeoRadiusByMember Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return nil, errors.New("Redis GeoRadiusByMember Failed: " + "Member is Required")
	}

	q, err := validateGeoRadiusQuery(query, false) // <-- validation added
	if err != nil {
		return nil, err
	}

	cmd := snap.reader.GeoRadiusByMember(snap.reader.Context(), key, member, q)
	if cmd.Err() != nil && cmd.Err() != redis.Nil {
		return cmd, cmd.Err()
	}
	return cmd, nil
}

// GeoRadiusByMemberStore is same as GeoRadiusStore, except instead of taking as the center of the area to query (long lat),
// this takes the name of a member already existing inside the geospatial index represented by the sorted set
//
// # The position of the specified member is used as the center of the query
//
// radius = units are: m (meters), km (kilometers), mi (miles), ft (feet)
// withDist = return the distance of returned items from the specified center (using same unit as units specified in the radius)
// withCoord = return the longitude and latitude coordinates of the matching items
//
// asc = sort returned items from the nearest to the farthest, relative to the center
// desc = sort returned items from the farthest to the nearest, relative to the center
//
// count = optional limit of return items; default is return all items found, use count to limit the list
//
// store = store the items in a sorted set populated with their geospatial information
// storeDist = store the items in a sorted set populated with their distance from the center as a floating point number, in the same unit specified in the radius
func (g *GEO) GeoRadiusByMemberStore(key string, member string, query *redis.GeoRadiusQuery) (err error) {
	if g == nil {
		return errors.New("Redis GEO GeoRadiusByMemberStore Failed: GEO receiver is nil")
	}
	if g.core == nil {
		return errors.New("Redis GEO GeoRadiusByMemberStore Failed: Redis core is nil")
	}
	snap := g.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoRadiusByMemberStore", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadiusByMemberStore-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadiusByMemberStore-Member", member))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-GeoRadiusByMemberStore-Query", query))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = g.geoRadiusByMemberStoreInternal(snap, key, member, query)
		return err
	} else {
		return g.geoRadiusByMemberStoreInternal(snap, key, member, query)
	}
}

// geoRadiusByMemberStoreInternal is same as GeoRadiusStore, except instead of taking as the center of the area to query (long lat),
// this takes the name of a member already existing inside the geospatial index represented by the sorted set
//
// # The position of the specified member is used as the center of the query
//
// radius = units are: m (meters), km (kilometers), mi (miles), ft (feet)
// withDist = return the distance of returned items from the specified center (using same unit as units specified in the radius)
// withCoord = return the longitude and latitude coordinates of the matching items
//
// asc = sort returned items from the nearest to the farthest, relative to the center
// desc = sort returned items from the farthest to the nearest, relative to the center
//
// count = optional limit of return items; default is return all items found, use count to limit the list
//
// store = store the items in a sorted set populated with their geospatial information
// storeDist = store the items in a sorted set populated with their distance from the center as a floating point number, in the same unit specified in the radius
func (g *GEO) geoRadiusByMemberStoreInternal(snap redisConnSnapshot, key string, member string, query *redis.GeoRadiusQuery) error {
	// validate
	if g.core == nil {
		return errors.New("Redis GeoRadiusByMemberStore Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis GeoRadiusByMemberStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis GeoRadiusByMemberStore Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return errors.New("Redis GeoRadiusByMemberStore Failed: " + "Member is Required")
	}

	q, err := validateGeoRadiusQuery(query, true) // <-- validation added
	if err != nil {
		return err
	}

	cmd := snap.writer.GeoRadiusByMemberStore(snap.writer.Context(), key, member, q)
	_, _, err = g.core.handleIntCmd(cmd, "Redis GeoRadiusByMemberStore Failed: ")
	return err
}

// ----------------------------------------------------------------------------------------------------------------
// STREAM functions
// ----------------------------------------------------------------------------------------------------------------

//
// *** REDIS STREAM INTRODUCTION = https://redis.io/topics/streams-intro ***
//

// XAck removes one or multiple messages from the 'pending entries list (PEL)' of a stream consumer group
//
// # A message is pending, and as such stored inside the PEL, when it was delivered to some consumer
//
// Once a consumer successfully processes a message, it should call XAck to remove the message so it does not get processed again (and releases message from memory in redis)
func (x *STREAM) XAck(stream string, group string, id ...string) (err error) {
	if x == nil {
		return errors.New("Redis STREAM XAck Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return errors.New("Redis STREAM XAck Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XAck", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XAck-Stream", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XAck-Group", group))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XAck-IDs", id))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = x.xackInternal(snap, stream, group, id...)
		return err
	} else {
		return x.xackInternal(snap, stream, group, id...)
	}
}

// xackInternal removes one or multiple messages from the 'pending entries list (PEL)' of a stream consumer group
//
// # A message is pending, and as such stored inside the PEL, when it was delivered to some consumer
//
// Once a consumer successfully processes a message, it should call XAck to remove the message so it does not get processed again (and releases message from memory in redis)
func (x *STREAM) xackInternal(snap redisConnSnapshot, stream string, group string, id ...string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XAck Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis XAck Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return errors.New("Redis XAck Failed: " + "Stream is Required")
	}

	if len(group) <= 0 {
		return errors.New("Redis XAck Failed: " + "Group is Required")
	}

	if len(id) <= 0 {
		return errors.New("Redis XAck Failed: " + "At Least 1 ID is Required")
	}

	cmd := snap.writer.XAck(snap.writer.Context(), stream, group, id...)
	return x.core.handleIntCmd2(cmd, "Redis XAck Failed: ")
}

// XAdd appends the specified stream entry to the stream at the specified key,
// If the key does not exist, as a side effect of running this command the key is created with a stream value
func (x *STREAM) XAdd(addArgs *redis.XAddArgs) (err error) {
	if x == nil {
		return errors.New("Redis STREAM XAdd Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return errors.New("Redis STREAM XAdd Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XAdd", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XAdd-Input-Args", addArgs))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = x.xaddInternal(snap, addArgs)
		return err
	} else {
		return x.xaddInternal(snap, addArgs)
	}
}

// xaddInternal appends the specified stream entry to the stream at the specified key,
// If the key does not exist, as a side effect of running this command the key is created with a stream value
func (x *STREAM) xaddInternal(snap redisConnSnapshot, addArgs *redis.XAddArgs) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XAdd Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis XAdd Failed: " + "Endpoint Connections Not Ready")
	}

	if addArgs == nil {
		return errors.New("Redis XAdd Failed: " + "AddArgs is Required")
	}

	cmd := snap.writer.XAdd(snap.writer.Context(), addArgs)

	if _, _, err := x.core.handleStringCmd2(cmd, "Redis XAdd Failed: "); err != nil {
		return err
	} else {
		return nil
	}
}

// XClaim in the context of stream consumer group, this function changes the ownership of a pending message,
// so that the new owner is the consumer specified as the command argument
func (x *STREAM) XClaim(claimArgs *redis.XClaimArgs) (valMessages []redis.XMessage, notFound bool, err error) {
	if x == nil {
		return nil, false, errors.New("Redis STREAM XClaim Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return nil, false, errors.New("Redis STREAM XClaim Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XClaim", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XClaim-Input-Args", claimArgs))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XClaim-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XClaim-Results", valMessages))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valMessages, notFound, err = x.xclaimInternal(snap, claimArgs)
		return valMessages, notFound, err
	} else {
		return x.xclaimInternal(snap, claimArgs)
	}
}

// xclaimInternal in the context of stream consumer group, this function changes the ownership of a pending message,
// so that the new owner is the consumer specified as the command argument
func (x *STREAM) xclaimInternal(snap redisConnSnapshot, claimArgs *redis.XClaimArgs) (valMessages []redis.XMessage, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XClaim Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis XClaim Failed: " + "Endpoint Connections Not Ready")
	}

	if claimArgs == nil {
		return nil, false, errors.New("Redis XClaim Failed: " + "ClaimArgs is Required")
	}

	cmd := snap.writer.XClaim(snap.writer.Context(), claimArgs)
	return x.core.handleXMessageSliceCmd(cmd, "Redis XClaim Failed: ")
}

// XClaimJustID in the context of stream consumer group, this function changes the ownership of a pending message,
// so that the new owner is the consumer specified as the command argument
func (x *STREAM) XClaimJustID(claimArgs *redis.XClaimArgs) (outputSlice []string, notFound bool, err error) {
	if x == nil {
		return nil, false, errors.New("Redis STREAM XClaimJustID Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return nil, false, errors.New("Redis STREAM XClaimJustID Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XClaimJustID", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XClaimJustID-Input-Args", claimArgs))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XClaimJustID-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XClaimJustID-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = x.xclaimJustIDInternal(snap, claimArgs)
		return outputSlice, notFound, err
	} else {
		return x.xclaimJustIDInternal(snap, claimArgs)
	}
}

// xclaimJustIDInternal in the context of stream consumer group, this function changes the ownership of a pending message,
// so that the new owner is the consumer specified as the command argument
func (x *STREAM) xclaimJustIDInternal(snap redisConnSnapshot, claimArgs *redis.XClaimArgs) (outputSlice []string, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XClaim Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis XClaim Failed: " + "Endpoint Connections Not Ready")
	}

	if claimArgs == nil {
		return nil, false, errors.New("Redis XClaim Failed: " + "ClaimArgs is Required")
	}

	cmd := snap.writer.XClaimJustID(snap.writer.Context(), claimArgs)
	return x.core.handleStringSliceCmd(cmd, "Redis XClaim Failed: ")
}

// XDel removes the specified entries from a stream
func (x *STREAM) XDel(stream string, id ...string) (err error) {
	if x == nil {
		return errors.New("Redis STREAM XDel Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return errors.New("Redis STREAM XDel Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XDel", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XDel-Stream", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XDel-IDs", id))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = x.xdelInternal(snap, stream, id...)
		return err
	} else {
		return x.xdelInternal(snap, stream, id...)
	}
}

// xdelInternal removes the specified entries from a stream
func (x *STREAM) xdelInternal(snap redisConnSnapshot, stream string, id ...string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XDel Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis XDel Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return errors.New("Redis XDel Failed: " + "Stream is Required")
	}

	if len(id) <= 0 {
		return errors.New("Redis XDel Failed: " + "At Least 1 ID is Required")
	}

	cmd := snap.writer.XDel(snap.writer.Context(), stream, id...)
	return x.core.handleIntCmd2(cmd, "Redis XDel Failed: ")
}

// XGroupCreate will create a new consumer group associated with a stream
func (x *STREAM) XGroupCreate(stream string, group string, start string) (err error) {
	if x == nil {
		return errors.New("Redis STREAM XGroupCreate Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return errors.New("Redis STREAM XGroupCreate Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XGroupCreate", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupCreate-Stream", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupCreate-Group", group))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupCreate-Start", start))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = x.xgroupCreateInternal(snap, stream, group, start)
		return err
	} else {
		return x.xgroupCreateInternal(snap, stream, group, start)
	}
}

// xgroupCreateInternal will create a new consumer group associated with a stream
func (x *STREAM) xgroupCreateInternal(snap redisConnSnapshot, stream string, group string, start string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XGroupCreate Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis XGroupCreate Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return errors.New("Redis XGroupCreate Failed: " + "Stream is Required")
	}

	if len(group) <= 0 {
		return errors.New("Redis XGroupCreate Failed: " + "Group is Required")
	}

	if len(start) <= 0 {
		return errors.New("Redis XGroupCreate Failed: " + "Start is Required")
	}

	cmd := snap.writer.XGroupCreate(snap.writer.Context(), stream, group, start)
	return x.core.handleStatusCmd(cmd, "Redis XGroupCreate Failed: ")
}

// XGroupCreateMkStream will create a new consumer group, and create a stream if stream doesn't exist
func (x *STREAM) XGroupCreateMkStream(stream string, group string, start string) (err error) {
	if x == nil {
		return errors.New("Redis STREAM XGroupCreateMkStream Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return errors.New("Redis STREAM XGroupCreateMkStream Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XGroupCreateMkStream", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupCreateMkStream-Stream", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupCreateMkStream-Group", group))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupCreateMkStream-Start", start))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = x.xgroupCreateMkStreamInternal(snap, stream, group, start)
		return err
	} else {
		return x.xgroupCreateMkStreamInternal(snap, stream, group, start)
	}
}

// xgroupCreateMkStreamInternal will create a new consumer group, and create a stream if stream doesn't exist
func (x *STREAM) xgroupCreateMkStreamInternal(snap redisConnSnapshot, stream string, group string, start string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XGroupCreateMkStream Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis XGroupCreateMkStream Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return errors.New("Redis XGroupCreateMkStream Failed: " + "Stream is Required")
	}

	if len(group) <= 0 {
		return errors.New("Redis XGroupCreateMkStream Failed: " + "Group is Required")
	}

	if len(start) <= 0 {
		return errors.New("Redis XGroupCreateMkStream Failed: " + "Start is Required")
	}

	cmd := snap.writer.XGroupCreateMkStream(snap.writer.Context(), stream, group, start)
	return x.core.handleStatusCmd(cmd, "Redis XGroupCreateMkStream Failed: ")
}

// XGroupDelConsumer removes a given consumer from a consumer group
func (x *STREAM) XGroupDelConsumer(stream string, group string, consumer string) (err error) {
	if x == nil {
		return errors.New("Redis STREAM XGroupDelConsumer Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return errors.New("Redis STREAM XGroupDelConsumer Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XGroupDelConsumer", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupDelConsumer-Stream", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupDelConsumer-Group", group))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupDelConsumer-Consumer", consumer))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = x.xgroupDelConsumerInternal(snap, stream, group, consumer)
		return err
	} else {
		return x.xgroupDelConsumerInternal(snap, stream, group, consumer)
	}
}

// xgroupDelConsumerInternal removes a given consumer from a consumer group
func (x *STREAM) xgroupDelConsumerInternal(snap redisConnSnapshot, stream string, group string, consumer string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XGroupDelConsumer Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis XGroupDelConsumer Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return errors.New("Redis XGroupDelConsumer Failed: " + "Stream is Required")
	}

	if len(group) <= 0 {
		return errors.New("Redis XGroupDelConsumer Failed: " + "Group is Required")
	}

	if len(consumer) <= 0 {
		return errors.New("Redis XGroupDelConsumer Failed: " + "Consumer is Required")
	}

	cmd := snap.writer.XGroupDelConsumer(snap.writer.Context(), stream, group, consumer)
	return x.core.handleIntCmd2(cmd, "Redis XGroupDelConsumer Failed: ")
}

// XGroupDestroy will destroy a consumer group even if there are active consumers and pending messages
func (x *STREAM) XGroupDestroy(stream string, group string) (err error) {
	if x == nil {
		return errors.New("Redis STREAM XGroupDestroy Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return errors.New("Redis STREAM XGroupDestroy Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XGroupDestroy", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupDestroy-Stream", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupDestroy-Group", group))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = x.xgroupDestroyInternal(snap, stream, group)
		return err
	} else {
		return x.xgroupDestroyInternal(snap, stream, group)
	}
}

// xgroupDestroyInternal will destroy a consumer group even if there are active consumers and pending messages
func (x *STREAM) xgroupDestroyInternal(snap redisConnSnapshot, stream string, group string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XGroupDestroy Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis XGroupDestroy Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return errors.New("Redis XGroupDestroy Failed: " + "Stream is Required")
	}

	if len(group) <= 0 {
		return errors.New("Redis XGroupDestroy Failed: " + "Group is Required")
	}

	cmd := snap.writer.XGroupDestroy(snap.writer.Context(), stream, group)
	return x.core.handleIntCmd2(cmd, "Redis XGroupDestroy Failed: ")
}

// XGroupSetID will set the next message to deliver,
// Normally the next ID is set when the consumer is created, as the last argument to XGroupCreate,
// However, using XGroupSetID resets the next message ID in case prior message needs to be reprocessed
func (x *STREAM) XGroupSetID(stream string, group string, start string) (err error) {
	if x == nil {
		return errors.New("Redis STREAM XGroupSetID Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return errors.New("Redis STREAM XGroupSetID Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XGroupSetID", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupSetID-Stream", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupSetID-Group", group))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XGroupSetID-Start", start))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = x.xgroupSetIDInternal(snap, stream, group, start)
		return err
	} else {
		return x.xgroupSetIDInternal(snap, stream, group, start)
	}
}

// xgroupSetIDInternal will set the next message to deliver,
// Normally the next ID is set when the consumer is created, as the last argument to XGroupCreate,
// However, using XGroupSetID resets the next message ID in case prior message needs to be reprocessed
func (x *STREAM) xgroupSetIDInternal(snap redisConnSnapshot, stream string, group string, start string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XGroupSetID Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis XGroupSetID Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return errors.New("Redis XGroupSetID Failed: " + "Stream is Required")
	}

	if len(group) <= 0 {
		return errors.New("Redis XGroupSetID Failed: " + "Group is Required")
	}

	if len(start) <= 0 {
		return errors.New("Redis XGroupSetID Failed: " + "Start is Required")
	}

	cmd := snap.writer.XGroupSetID(snap.writer.Context(), stream, group, start)
	return x.core.handleStatusCmd(cmd, "Redis XGroupSetID Failed: ")
}

// XInfoGroups retrieves different information about the streams, and associated consumer groups
func (x *STREAM) XInfoGroups(key string) (outputSlice []redis.XInfoGroup, notFound bool, err error) {
	if x == nil {
		return nil, false, errors.New("Redis STREAM XInfoGroups Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return nil, false, errors.New("Redis STREAM XInfoGroups Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XInfoGroups", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XInfoGroups-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XInfoGroups-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XInfoGroups-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = x.xinfoGroupsInternal(snap, key)
		return outputSlice, notFound, err
	} else {
		return x.xinfoGroupsInternal(snap, key)
	}
}

// xinfoGroupsInternal retrieves different information about the streams, and associated consumer groups
func (x *STREAM) xinfoGroupsInternal(snap redisConnSnapshot, key string) (outputSlice []redis.XInfoGroup, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XInfoGroups Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis XInfoGroups Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis XInfoGroups Failed: " + "Key is Required")
	}

	cmd := snap.reader.XInfoGroups(snap.reader.Context(), key)
	return x.core.handleXInfoGroupsCmd(cmd, "Redis XInfoGroups Failed: ")
}

// XLen returns the number of entries inside a stream
func (x *STREAM) XLen(stream string) (val int64, notFound bool, err error) {
	if x == nil {
		return 0, false, errors.New("Redis STREAM XLen Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return 0, false, errors.New("Redis STREAM XLen Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XLen", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XLen-Stream", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XLen-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XLen-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = x.xlenInternal(snap, stream)
		return val, notFound, err
	} else {
		return x.xlenInternal(snap, stream)
	}
}

// xlenInternal returns the number of entries inside a stream
func (x *STREAM) xlenInternal(snap redisConnSnapshot, stream string) (val int64, notFound bool, err error) {
	// validate
	if x.core == nil {
		return 0, false, errors.New("Redis XLen Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis XLen Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return 0, false, errors.New("Redis XLen Failed: " + "Stream is Required")
	}

	cmd := snap.reader.XLen(snap.reader.Context(), stream)
	return x.core.handleIntCmd(cmd, "Redis XLen Failed: ")
}

// XPending fetches data from a stream via a consumer group, and not acknowledging such data, its like creating pending entries
func (x *STREAM) XPending(stream string, group string) (val *redis.XPending, notFound bool, err error) {
	if x == nil {
		return nil, false, errors.New("Redis STREAM XPending Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return nil, false, errors.New("Redis STREAM XPending Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XPending", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XPending-Stream", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XPending-Group", group))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XPending-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XPending-Results", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = x.xpendingInternal(snap, stream, group)
		return val, notFound, err
	} else {
		return x.xpendingInternal(snap, stream, group)
	}
}

// xpendingInternal fetches data from a stream via a consumer group, and not acknowledging such data, its like creating pending entries
func (x *STREAM) xpendingInternal(snap redisConnSnapshot, stream string, group string) (val *redis.XPending, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XPending Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis XPending Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return nil, false, errors.New("Redis XPending Failed: " + "Stream is Required")
	}

	if len(group) <= 0 {
		return nil, false, errors.New("Redis XPending Failed: " + "Group is Required")
	}

	cmd := snap.reader.XPending(snap.reader.Context(), stream, group)
	return x.core.handleXPendingCmd(cmd, "Redis XPending Failed: ")
}

// XPendingExt fetches data from a stream via a consumer group, and not acknowledging such data, its like creating pending entries
func (x *STREAM) XPendingExt(pendingArgs *redis.XPendingExtArgs) (outputSlice []redis.XPendingExt, notFound bool, err error) {
	if x == nil {
		return nil, false, errors.New("Redis STREAM XPendingExt Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return nil, false, errors.New("Redis STREAM XPendingExt Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XPendingExt", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XPendingExt-Input-Args", pendingArgs))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XPendingExt-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XPendingExt-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = x.xpendingExtInternal(snap, pendingArgs)
		return outputSlice, notFound, err
	} else {
		return x.xpendingExtInternal(snap, pendingArgs)
	}
}

// xpendingExtInternal fetches data from a stream via a consumer group, and not acknowledging such data, its like creating pending entries
func (x *STREAM) xpendingExtInternal(snap redisConnSnapshot, pendingArgs *redis.XPendingExtArgs) (outputSlice []redis.XPendingExt, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XPendingExt Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis XPendingExt Failed: " + "Endpoint Connections Not Ready")
	}

	if pendingArgs == nil {
		return nil, false, errors.New("Redis XPendingExt Failed: " + "PendingArgs is Required")
	}

	cmd := snap.reader.XPendingExt(snap.reader.Context(), pendingArgs)
	return x.core.handleXPendingExtCmd(cmd, "Redis XPendingExt Failed: ")
}

// XRange returns the stream entries matching a given range of IDs,
// Range is specified by a minimum and maximum ID,
// Ordering is lowest to highest
func (x *STREAM) XRange(stream string, start string, stop string, count ...int64) (outputSlice []redis.XMessage, notFound bool, err error) {
	if x == nil {
		return nil, false, errors.New("Redis STREAM XRange Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return nil, false, errors.New("Redis STREAM XRange Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XRange", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRange-Stream", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRange-Start", start))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRange-Stop", stop))

			if len(count) > 0 {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRange-Limit-Count", count[0]))
			} else {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRange-Limit-Count", "None"))
			}

			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRange-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRange-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = x.xrangeInternal(snap, stream, start, stop, count...)
		return outputSlice, notFound, err
	} else {
		return x.xrangeInternal(snap, stream, start, stop, count...)
	}
}

// xrangeInternal returns the stream entries matching a given range of IDs,
// Range is specified by a minimum and maximum ID,
// Ordering is lowest to highest
func (x *STREAM) xrangeInternal(snap redisConnSnapshot, stream string, start string, stop string, count ...int64) (outputSlice []redis.XMessage, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XRange Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis XRange Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return nil, false, errors.New("Redis XRange Failed: " + "Stream is Required")
	}

	if len(start) <= 0 {
		return nil, false, errors.New("Redis XRange Failed: " + "Start is Required")
	}

	if len(stop) <= 0 {
		return nil, false, errors.New("Redis XRange Failed: " + "Stop is Required")
	}

	var cmd *redis.XMessageSliceCmd

	if len(count) <= 0 {
		cmd = snap.reader.XRange(snap.reader.Context(), stream, start, stop)
	} else {
		cmd = snap.reader.XRangeN(snap.reader.Context(), stream, start, stop, count[0])
	}

	return x.core.handleXMessageSliceCmd(cmd, "Redis XRange Failed: ")
}

// XRevRange returns the stream entries matching a given range of IDs,
// Range is specified by a maximum and minimum ID,
// Ordering is highest to lowest
func (x *STREAM) XRevRange(stream string, start string, stop string, count ...int64) (outputSlice []redis.XMessage, notFound bool, err error) {
	if x == nil {
		return nil, false, errors.New("Redis STREAM XRevRange Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return nil, false, errors.New("Redis STREAM XRevRange Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XRevRange", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRevRange-Stream", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRevRange-Start", start))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRevRange-Stop", stop))

			if len(count) > 0 {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRevRange-Limit-Count", count[0]))
			} else {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRevRange-Limit-Count", "None"))
			}

			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRevRange-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRevRange-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = x.xrevRangeInternal(snap, stream, start, stop, count...)
		return outputSlice, notFound, err
	} else {
		return x.xrevRangeInternal(snap, stream, start, stop, count...)
	}
}

// xrevRangeInternal returns the stream entries matching a given range of IDs,
// Range is specified by a maximum and minimum ID,
// Ordering is highest to lowest
func (x *STREAM) xrevRangeInternal(snap redisConnSnapshot, stream string, start string, stop string, count ...int64) (outputSlice []redis.XMessage, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XRevRange Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis XRevRange Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return nil, false, errors.New("Redis XRevRange Failed: " + "Stream is Required")
	}

	if len(start) <= 0 {
		return nil, false, errors.New("Redis XRevRange Failed: " + "Start is Required")
	}

	if len(stop) <= 0 {
		return nil, false, errors.New("Redis XRevRange Failed: " + "Stop is Required")
	}

	var cmd *redis.XMessageSliceCmd

	if len(count) <= 0 {
		cmd = snap.reader.XRevRange(snap.reader.Context(), stream, start, stop)
	} else {
		cmd = snap.reader.XRevRangeN(snap.reader.Context(), stream, start, stop, count[0])
	}

	return x.core.handleXMessageSliceCmd(cmd, "Redis XRevRange Failed: ")
}

// XRead will read data from one or multiple streams,
// only returning entries with an ID greater than the last received ID reported by the caller
func (x *STREAM) XRead(readArgs *redis.XReadArgs) (outputSlice []redis.XStream, notFound bool, err error) {
	if x == nil {
		return nil, false, errors.New("Redis STREAM XRead Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return nil, false, errors.New("Redis STREAM XRead Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XRead", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRead-Input-Args", readArgs))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRead-Input-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XRead-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = x.xreadInternal(snap, readArgs)
		return outputSlice, notFound, err
	} else {
		return x.xreadInternal(snap, readArgs)
	}
}

// xreadInternal will read data from one or multiple streams,
// only returning entries with an ID greater than the last received ID reported by the caller
func (x *STREAM) xreadInternal(snap redisConnSnapshot, readArgs *redis.XReadArgs) (outputSlice []redis.XStream, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XRead Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis XRead Failed: " + "Endpoint Connections Not Ready")
	}

	if readArgs == nil {
		return nil, false, errors.New("Redis XRead Failed: " + "ReadArgs is Required")
	}

	cmd := snap.reader.XRead(snap.reader.Context(), readArgs)
	return x.core.handleXStreamSliceCmd(cmd, "Redis XRead Failed: ")
}

// XReadStreams is a special version of XRead command for streams
func (x *STREAM) XReadStreams(stream ...string) (outputSlice []redis.XStream, notFound bool, err error) {
	if x == nil {
		return nil, false, errors.New("Redis STREAM XReadStreams Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return nil, false, errors.New("Redis STREAM XReadStreams Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XReadStreams", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XReadStreams-Streams", stream))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XReadStreams-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XReadStreams-Results", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = x.xreadStreamsInternal(snap, stream...)
		return outputSlice, notFound, err
	} else {
		return x.xreadStreamsInternal(snap, stream...)
	}
}

// xreadStreamsInternal is a special version of XRead command for streams
func (x *STREAM) xreadStreamsInternal(snap redisConnSnapshot, stream ...string) (outputSlice []redis.XStream, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XReadStreams Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis XReadStreams Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return nil, false, errors.New("Redis XReadStreams Failed: " + "At Least 1 Stream is Required")
	}

	cmd := snap.reader.XReadStreams(snap.reader.Context(), stream...)
	return x.core.handleXStreamSliceCmd(cmd, "Redis XReadStream Failed: ")
}

// XReadGroup is a special version of XRead command with support for consumer groups
func (x *STREAM) XReadGroup(readGroupArgs *redis.XReadGroupArgs) (outputSlice []redis.XStream, notFound bool, err error) {
	if x == nil {
		return nil, false, errors.New("Redis STREAM XReadGroup Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return nil, false, errors.New("Redis STREAM XReadGroup Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XReadGroup", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XReadGroup-ReadGroup", readGroupArgs))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XReadGroup-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XReadGroup-Result", outputSlice))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		outputSlice, notFound, err = x.xreadGroupInternal(snap, readGroupArgs)
		return outputSlice, notFound, err
	} else {
		return x.xreadGroupInternal(snap, readGroupArgs)
	}
}

// xreadGroupInternal is a special version of XRead command with support for consumer groups
func (x *STREAM) xreadGroupInternal(snap redisConnSnapshot, readGroupArgs *redis.XReadGroupArgs) (outputSlice []redis.XStream, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XReadGroup Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis XReadGroup Failed: " + "Endpoint Connections Not Ready")
	}

	if readGroupArgs == nil {
		return nil, false, errors.New("Redis XReadGroup Failed: " + "ReadGroupArgs is Required")
	}

	cmd := snap.writer.XReadGroup(snap.writer.Context(), readGroupArgs)
	return x.core.handleXStreamSliceCmd(cmd, "Redis XReadGroup Failed: ")
}

// XTrim trims the stream to a given number of items, evicting older items (items with lower IDs) if needed
func (x *STREAM) XTrim(key string, maxLen int64) (err error) {
	if x == nil {
		return errors.New("Redis STREAM XTrim Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return errors.New("Redis STREAM XTrim Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XTrim", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XTrim-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XTrim-MaxLen", maxLen))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = x.xtrimInternal(snap, key, maxLen)
		return err
	} else {
		return x.xtrimInternal(snap, key, maxLen)
	}
}

// xtrimInternal trims the stream to a given number of items, evicting older items (items with lower IDs) if needed
func (x *STREAM) xtrimInternal(snap redisConnSnapshot, key string, maxLen int64) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XTrim Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis XTrim Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis XTrim Failed: " + "Key is Required")
	}

	if maxLen < 0 {
		return errors.New("Redis XTrim Failed: " + "MaxLen Must Not Be Negative")
	}

	cmd := snap.writer.XTrim(snap.writer.Context(), key, maxLen)
	_, _, err := x.core.handleIntCmd(cmd, "Redis XTrim Failed: ")
	return err
}

// XTrimApprox trims the stream to a given number of items, evicting older items (items with lower IDs) if needed
func (x *STREAM) XTrimApprox(key string, maxLen int64) (err error) {
	if x == nil {
		return errors.New("Redis STREAM XTrimApprox Failed: STREAM receiver is nil")
	}
	if x.core == nil {
		return errors.New("Redis STREAM XTrimApprox Failed: Redis core is nil")
	}
	snap := x.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XTrimApprox", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XTrimApprox-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-XTrimApprox-MaxLen", maxLen))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = x.xtrimApproxInternal(snap, key, maxLen)
		return err
	} else {
		return x.xtrimApproxInternal(snap, key, maxLen)
	}
}

// xtrimApproxInternal trims the stream to a given number of items, evicting older items (items with lower IDs) if needed
func (x *STREAM) xtrimApproxInternal(snap redisConnSnapshot, key string, maxLen int64) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XTrimApprox Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis XTrimApprox Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis XTrimApprox Failed: " + "Key is Required")
	}

	if maxLen < 0 {
		return errors.New("Redis XTrimApprox Failed: " + "MaxLen Must Not Be Negative")
	}

	cmd := snap.writer.XTrimApprox(snap.writer.Context(), key, maxLen)
	_, _, err := x.core.handleIntCmd(cmd, "Redis XTrimApprox Failed: ")
	return err
}

// ----------------------------------------------------------------------------------------------------------------
// PUBSUB functions
// ----------------------------------------------------------------------------------------------------------------

//
// *** REDIS PUB/SUB INTRODUCTION = https://redis.io/topics/pubsub ***
//

// PSubscribe (Pattern Subscribe) will subscribe client to the given pattern channels (glob-style),
// a pointer to redis PubSub object is returned upon successful subscribe
//
// Once client is subscribed, do not call other redis actions, other than Subscribe, PSubscribe, Ping, Unsubscribe, PUnsubscribe, and Quit (Per Redis Doc)
//
// glob-style patterns:
//  1. h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//  2. h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//  3. h*llo = * represents any single or more char match (hllo, heeeelo match)
//  4. h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//  5. h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//  6. h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//  7. Use \ to escape special characters if needing to match verbatim
func (ps *PUBSUB) PSubscribe(channel ...string) (psObj *redis.PubSub, err error) {
	if ps == nil {
		return nil, errors.New("Redis PUBSUB PSubscribe Failed: PUBSUB receiver is nil")
	}
	if ps.core == nil {
		return nil, errors.New("Redis PUBSUB PSubscribe Failed: Redis core is nil")
	}
	snap := ps.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PSubscribe", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PSubscribe-Channels", channel))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		psObj, err = ps.psubscribeInternal(snap, channel...)
		return psObj, err
	} else {
		return ps.psubscribeInternal(snap, channel...)
	}
}

// psubscribeInternal (Pattern Subscribe) will subscribe client to the given pattern channels (glob-style),
// a pointer to redis PubSub object is returned upon successful subscribe
//
// Once client is subscribed, do not call other redis actions, other than Subscribe, PSubscribe, Ping, Unsubscribe, PUnsubscribe, and Quit (Per Redis Doc)
//
// glob-style patterns:
//  1. h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//  2. h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//  3. h*llo = * represents any single or more char match (hllo, heeeelo match)
//  4. h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//  5. h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//  6. h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//  7. Use \ to escape special characters if needing to match verbatim
func (ps *PUBSUB) psubscribeInternal(snap redisConnSnapshot, channel ...string) (*redis.PubSub, error) {
	// validate
	if ps.core == nil {
		return nil, errors.New("Redis PSubscribe Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis PSubscribe Failed: " + "Endpoint Connections Not Ready")
	}

	if len(channel) <= 0 {
		return nil, errors.New("Redis PSubscribe Failed: " + "At Least 1 Channel is Required")
	}

	return snap.writer.PSubscribe(snap.writer.Context(), channel...), nil
}

// Subscribe (Non-Pattern Subscribe) will subscribe client to the given channels,
// a pointer to redis PubSub object is returned upon successful subscribe
//
// Once client is subscribed, do not call other redis actions, other than Subscribe, PSubscribe, Ping, Unsubscribe, PUnsubscribe, and Quit (Per Redis Doc)
func (ps *PUBSUB) Subscribe(channel ...string) (psObj *redis.PubSub, err error) {
	if ps == nil {
		return nil, errors.New("Redis PUBSUB Subscribe Failed: PUBSUB receiver is nil")
	}
	if ps.core == nil {
		return nil, errors.New("Redis PUBSUB Subscribe Failed: Redis core is nil")
	}
	snap := ps.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Subscribe", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Subscribe-Channels", channel))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		psObj, err = ps.subscribeInternal(snap, channel...)
		return psObj, err
	} else {
		return ps.subscribeInternal(snap, channel...)
	}
}

// subscribeInternal (Non-Pattern Subscribe) will subscribe client to the given channels,
// a pointer to redis PubSub object is returned upon successful subscribe
//
// Once client is subscribed, do not call other redis actions, other than Subscribe, PSubscribe, Ping, Unsubscribe, PUnsubscribe, and Quit (Per Redis Doc)
func (ps *PUBSUB) subscribeInternal(snap redisConnSnapshot, channel ...string) (*redis.PubSub, error) {
	// validate
	if ps.core == nil {
		return nil, errors.New("Redis Subscribe Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis Subscribe Failed: " + "Endpoint Connections Not Ready")
	}

	if len(channel) <= 0 {
		return nil, errors.New("Redis Subscribe Failed: " + "At Least 1 Channel is Required")
	}

	return snap.writer.Subscribe(snap.writer.Context(), channel...), nil
}

// Publish will post a message to a given channel,
// returns number of clients that received the message
func (ps *PUBSUB) Publish(channel string, message interface{}) (valReceived int64, err error) {
	if ps == nil {
		return 0, errors.New("Redis PUBSUB Publish Failed: PUBSUB receiver is nil")
	}
	if ps.core == nil {
		return 0, errors.New("Redis PUBSUB Publish Failed: Redis core is nil")
	}
	snap := ps.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Publish", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Publish-Channel", channel))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Publish-Message", message))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Publish-Received-Clients-Count", valReceived))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valReceived, err = ps.publishInternal(snap, channel, message)
		return valReceived, err
	} else {
		return ps.publishInternal(snap, channel, message)
	}
}

// publishInternal will post a message to a given channel,
// returns number of clients that received the message
func (ps *PUBSUB) publishInternal(snap redisConnSnapshot, channel string, message interface{}) (valReceived int64, err error) {
	// validate
	if ps.core == nil {
		return 0, errors.New("Redis Publish Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, errors.New("Redis Publish Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.writer.Publish(snap.writer.Context(), channel, message)
	valReceived, _, err = ps.core.handleIntCmd(cmd, "Redis Publish Failed: ")
	return valReceived, err
}

// PubSubChannels lists currently active channels,
// active channel = pub/sub channel with one or more subscribers (excluding clients subscribed to patterns),
// pattern = optional, channels matching specific glob-style pattern are listed; otherwise, all channels listed
//
// glob-style patterns:
//  1. h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//  2. h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//  3. h*llo = * represents any single or more char match (hllo, heeeelo match)
//  4. h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//  5. h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//  6. h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//  7. Use \ to escape special characters if needing to match verbatim
func (ps *PUBSUB) PubSubChannels(pattern string) (valChannels []string, err error) {
	if ps == nil {
		return nil, errors.New("Redis PUBSUB PubSubChannels Failed: PUBSUB receiver is nil")
	}
	if ps.core == nil {
		return nil, errors.New("Redis PUBSUB PubSubChannels Failed: Redis core is nil")
	}
	snap := ps.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PubSubChannels", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PubSubChannels-Pattern", pattern))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PubSubChannels-Result", valChannels))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valChannels, err = ps.pubSubChannelsInternal(snap, pattern)
		return valChannels, err
	} else {
		return ps.pubSubChannelsInternal(snap, pattern)
	}
}

// pubSubChannelsInternal lists currently active channels,
// active channel = pub/sub channel with one or more subscribers (excluding clients subscribed to patterns),
// pattern = optional, channels matching specific glob-style pattern are listed; otherwise, all channels listed
//
// glob-style patterns:
//  1. h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//  2. h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//  3. h*llo = * represents any single or more char match (hllo, heeeelo match)
//  4. h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//  5. h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//  6. h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//  7. Use \ to escape special characters if needing to match verbatim
func (ps *PUBSUB) pubSubChannelsInternal(snap redisConnSnapshot, pattern string) (valChannels []string, err error) {
	// validate
	if ps.core == nil {
		return nil, errors.New("Redis PubSubChannels Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis PubSubChannels Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.PubSubChannels(snap.reader.Context(), pattern)
	valChannels, _, err = ps.core.handleStringSliceCmd(cmd, "Redis PubSubChannels Failed: ")
	return valChannels, err
}

// PubSubNumPat (Pub/Sub Number of Patterns) returns the number of subscriptions to patterns (that were using PSubscribe Command),
// This counts both clients subscribed to patterns, and also total number of patterns all the clients are subscribed to
func (ps *PUBSUB) PubSubNumPat() (valPatterns int64, err error) {
	if ps == nil {
		return 0, errors.New("Redis PUBSUB PubSubNumPat Failed: PUBSUB receiver is nil")
	}
	if ps.core == nil {
		return 0, errors.New("Redis PUBSUB PubSubNumPat Failed: Redis core is nil")
	}
	snap := ps.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PubSubNumPat", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PubSubNumPat-Result", valPatterns))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valPatterns, err = ps.pubSubNumPatInternal(snap)
		return valPatterns, err
	} else {
		return ps.pubSubNumPatInternal(snap)
	}
}

// pubSubNumPatInternal (Pub/Sub Number of Patterns) returns the number of subscriptions to patterns (that were using PSubscribe Command),
// This counts both clients subscribed to patterns, and also total number of patterns all the clients are subscribed to
func (ps *PUBSUB) pubSubNumPatInternal(snap redisConnSnapshot) (valPatterns int64, err error) {
	// validate
	if ps.core == nil {
		return 0, errors.New("Redis PubSubNumPat Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, errors.New("Redis PubSubNumPat Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.PubSubNumPat(snap.reader.Context())
	valPatterns, _, err = ps.core.handleIntCmd(cmd, "Redis PubSubNumPat Failed: ")
	return valPatterns, err
}

// PubSubNumSub (Pub/Sub Number of Subscribers) returns number of subscribers (not counting clients subscribed to patterns) for the specific channels
func (ps *PUBSUB) PubSubNumSub(channel ...string) (val map[string]int64, err error) {
	if ps == nil {
		return nil, errors.New("Redis PUBSUB PubSubNumSub Failed: PUBSUB receiver is nil")
	}
	if ps.core == nil {
		return nil, errors.New("Redis PUBSUB PubSubNumSub Failed: Redis core is nil")
	}
	snap := ps.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PubSubNumSub", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PubSubNumSub-Channels", channel))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-PubSubNumSub-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, err = ps.pubSubNumSubInternal(snap, channel...)
		return val, err
	} else {
		return ps.pubSubNumSubInternal(snap, channel...)
	}
}

// pubSubNumSubInternal (Pub/Sub Number of Subscribers) returns number of subscribers (not counting clients subscribed to patterns) for the specific channels
func (ps *PUBSUB) pubSubNumSubInternal(snap redisConnSnapshot, channel ...string) (val map[string]int64, err error) {
	// validate
	if ps.core == nil {
		return nil, errors.New("Redis PubSubNumSub Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis PubSubNumSub Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.PubSubNumSub(snap.reader.Context(), channel...)
	val, _, err = ps.core.handleStringIntMapCmd(cmd, "Redis PubSubNumSub Failed: ")
	return val, err
}

// ----------------------------------------------------------------------------------------------------------------
// PIPELINE functions
// ----------------------------------------------------------------------------------------------------------------

//
// *** REDIS PIPELINING INTRODUCTION = https://redis.io/topics/pipelining ***
//

// Pipeline allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) Pipeline() (result redis.Pipeliner, err error) {
	if p == nil {
		return nil, errors.New("Redis PIPELINE Pipeline Failed: PIPELINE receiver is nil")
	}
	if p.core == nil {
		return nil, errors.New("Redis PIPELINE Pipeline Failed: Redis core is nil")
	}
	snap := p.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Pipeline", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Pipeline-Result", result))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		result, err = p.pipelineInternal(snap)
		return result, err
	} else {
		return p.pipelineInternal(snap)
	}
}

// pipelineInternal allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) pipelineInternal(snap redisConnSnapshot) (redis.Pipeliner, error) {
	// validate
	if p.core == nil {
		return nil, errors.New("Redis Pipeline Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis Pipeline Failed: " + "Endpoint Connections Not Ready")
	}

	return snap.writer.Pipeline(), nil
}

// Pipelined allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) Pipelined(fn func(redis.Pipeliner) error) (result []redis.Cmder, err error) {
	if p == nil {
		return nil, errors.New("Redis PIPELINE Pipelined Failed: PIPELINE receiver is nil")
	}
	if p.core == nil {
		return nil, errors.New("Redis PIPELINE Pipelined Failed: Redis core is nil")
	}
	snap := p.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Pipelined", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Pipelined-Result", result))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		result, err = p.pipelinedInternal(snap, fn)
		return result, err
	} else {
		return p.pipelinedInternal(snap, fn)
	}
}

// pipelinedInternal allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) pipelinedInternal(snap redisConnSnapshot, fn func(redis.Pipeliner) error) ([]redis.Cmder, error) {
	// validate
	if p.core == nil {
		return nil, errors.New("Redis Pipelined Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis Pipelined Failed: " + "Endpoint Connections Not Ready")
	}

	return snap.writer.Pipelined(snap.writer.Context(), fn)
}

// TxPipeline allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) TxPipeline() (result redis.Pipeliner, err error) {
	if p == nil {
		return nil, errors.New("Redis PIPELINE TxPipeline Failed: PIPELINE receiver is nil")
	}
	if p.core == nil {
		return nil, errors.New("Redis PIPELINE TxPipeline Failed: Redis core is nil")
	}
	snap := p.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-TxPipeline", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-TxPipeline-Result", result))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		result, err = p.txPipelineInternal(snap)
		return result, err
	} else {
		return p.txPipelineInternal(snap)
	}
}

// txPipelineInternal allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) txPipelineInternal(snap redisConnSnapshot) (redis.Pipeliner, error) {
	// validate
	if p.core == nil {
		return nil, errors.New("Redis TxPipeline Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis TxPipeline Failed: " + "Endpoint Connections Not Ready")
	}

	return snap.writer.TxPipeline(), nil
}

// TxPipelined allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) TxPipelined(fn func(redis.Pipeliner) error) (result []redis.Cmder, err error) {
	if p == nil {
		return nil, errors.New("Redis PIPELINE TxPipelined Failed: PIPELINE receiver is nil")
	}
	if p.core == nil {
		return nil, errors.New("Redis PIPELINE TxPipelined Failed: Redis core is nil")
	}
	snap := p.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-TxPipelined", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-TxPipelined-Result", result))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		result, err = p.txPipelinedInternal(snap, fn)
		return result, err
	} else {
		return p.txPipelinedInternal(snap, fn)
	}
}

// txPipelinedInternal allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) txPipelinedInternal(snap redisConnSnapshot, fn func(redis.Pipeliner) error) ([]redis.Cmder, error) {
	// validate
	if p.core == nil {
		return nil, errors.New("Redis TxPipelined Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, errors.New("Redis TxPipelined Failed: " + "Endpoint Connections Not Ready")
	}

	return snap.writer.TxPipelined(snap.writer.Context(), fn)
}

// ----------------------------------------------------------------------------------------------------------------
// TTL functions
// ----------------------------------------------------------------------------------------------------------------

// TTL returns the remainder time to live in seconds or milliseconds, for key that has a TTL set,
// returns -1 if no TTL applicable (forever living)
func (t *TTL) TTL(key string, bGetMilliseconds bool) (valTTL int64, notFound bool, err error) {
	if t == nil {
		return 0, false, errors.New("Redis TTL TTL Failed: TTL receiver is nil")
	}
	if t.core == nil {
		return 0, false, errors.New("Redis TTL TTL Failed: Redis core is nil")
	}
	snap := t.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-TTL", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-TTL-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-TTL-Not-Found", notFound))

			if bGetMilliseconds {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-TTL-Remainder-Milliseconds", valTTL))
			} else {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-TTL-Remainder-Seconds", valTTL))
			}

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valTTL, notFound, err = t.ttlInternal(snap, key, bGetMilliseconds)
		return valTTL, notFound, err
	} else {
		return t.ttlInternal(snap, key, bGetMilliseconds)
	}
}

// ttlInternal returns the remainder time to live in seconds or milliseconds, for key that has a TTL set,
// returns -1 if no TTL applicable (forever living)
func (t *TTL) ttlInternal(snap redisConnSnapshot, key string, bGetMilliseconds bool) (valTTL int64, notFound bool, err error) {
	// validate
	if t.core == nil {
		return 0, false, errors.New("Redis TTL Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis TTL Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis TTL Failed: " + "Key is Required")
	}

	var cmd *redis.DurationCmd

	if bGetMilliseconds {
		cmd = snap.reader.PTTL(snap.reader.Context(), key)
	} else {
		cmd = snap.reader.TTL(snap.reader.Context(), key)
	}

	var d time.Duration

	d, notFound, err = t.core.handleDurationCmd(cmd, "Redis TTL Failed: ")

	if err != nil {
		return 0, false, err
	}

	if notFound || d == -2 {
		// not found
		return 0, true, nil
	}
	if d == -1 {
		// forever living
		return -1, false, nil
	}

	if bGetMilliseconds {
		valTTL = d.Milliseconds()
	} else {
		valTTL = int64(d.Seconds())
	}

	return valTTL, false, err
}

// Expire sets a timeout on key (seconds or milliseconds based on input parameter)
//
// expireValue = in seconds or milliseconds
func (t *TTL) Expire(key string, bSetMilliseconds bool, expireValue time.Duration) (err error) {
	if t == nil {
		return errors.New("Redis TTL Expire Failed: TTL receiver is nil")
	}
	if t.core == nil {
		return errors.New("Redis TTL Expire Failed: Redis core is nil")
	}
	snap := t.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Expire", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Expire-Key", key))

			if bSetMilliseconds {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Expire-Milliseconds", expireValue.Milliseconds()))
			} else {
				xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Expire-Seconds", expireValue.Seconds()))
			}

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = t.expireInternal(snap, key, bSetMilliseconds, expireValue)
		return err
	} else {
		return t.expireInternal(snap, key, bSetMilliseconds, expireValue)
	}
}

// expireInternal sets a timeout on key (seconds or milliseconds based on input parameter)
//
// expireValue = in seconds or milliseconds
func (t *TTL) expireInternal(snap redisConnSnapshot, key string, bSetMilliseconds bool, expireValue time.Duration) error {
	// validate
	if t.core == nil {
		return errors.New("Redis Expire Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis Expire Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis Expire Failed: " + "Key is Required")
	}

	if expireValue < 0 {
		return errors.New("Redis Expire Failed: " + "Expire Value Must Be 0 or Greater")
	}

	var cmd *redis.BoolCmd

	if bSetMilliseconds {
		cmd = snap.writer.PExpire(snap.writer.Context(), key, expireValue)
	} else {
		cmd = snap.writer.Expire(snap.writer.Context(), key, expireValue)
	}

	if val, err := t.core.handleBoolCmd(cmd, "Redis Expire Failed: "); err != nil {
		return err
	} else {
		if val {
			// success
			return nil
		} else {
			// key not exist
			return errors.New("Redis Expire Failed: " + "Key Was Not Found")
		}
	}
}

// ExpireAt sets the hard expiration date time based on unix timestamp for a given key
//
// Setting expireTime to the past immediately deletes the key
func (t *TTL) ExpireAt(key string, expireTime time.Time) (err error) {
	if t == nil {
		return errors.New("Redis TTL ExpireAt Failed: TTL receiver is nil")
	}
	if t.core == nil {
		return errors.New("Redis TTL ExpireAt Failed: Redis core is nil")
	}
	snap := t.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ExpireAt", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ExpireAt-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ExpireAt-Expire-Time", expireTime))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = t.expireAtInternal(snap, key, expireTime)
		return err
	} else {
		return t.expireAtInternal(snap, key, expireTime)
	}
}

// expireAtInternal sets the hard expiration date time based on unix timestamp for a given key
//
// Setting expireTime to the past immediately deletes the key
func (t *TTL) expireAtInternal(snap redisConnSnapshot, key string, expireTime time.Time) error {
	// validate
	if t.core == nil {
		return errors.New("Redis ExpireAt Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis ExpireAt Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis ExpireAt Failed: " + "Key is Required")
	}

	if expireTime.IsZero() {
		return errors.New("Redis ExpireAt Failed: " + "Expire Time is Required")
	}

	cmd := snap.writer.ExpireAt(snap.writer.Context(), key, expireTime)

	if val, err := t.core.handleBoolCmd(cmd, "Redis ExpireAt Failed: "); err != nil {
		return err
	} else {
		if val {
			// success
			return nil
		} else {
			// fail
			return errors.New("Redis ExpireAt Failed: " + "Key Was Not Found")
		}
	}
}

// Touch alters the last access time of a key or keys,
// if key doesn't exist, it is ignored
func (t *TTL) Touch(key ...string) (err error) {
	if t == nil {
		return errors.New("Redis TTL Touch Failed: TTL receiver is nil")
	}
	if t.core == nil {
		return errors.New("Redis TTL Touch Failed: Redis core is nil")
	}
	snap := t.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Touch", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Touch-Keys", key))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = t.touchInternal(snap, key...)
		return err
	} else {
		return t.touchInternal(snap, key...)
	}
}

// touchInternal alters the last access time of a key or keys,
// if key doesn't exist, it is ignored
func (t *TTL) touchInternal(snap redisConnSnapshot, key ...string) error {
	// validate
	if t.core == nil {
		return errors.New("Redis Touch Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis Touch Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis Touch Failed: " + "At Least 1 Key is Required")
	}

	cmd := snap.writer.Touch(snap.writer.Context(), key...)

	if val, _, err := t.core.handleIntCmd(cmd, "Redis Touch Failed: "); err != nil {
		return err
	} else {
		if val > 0 {
			// success
			return nil
		} else {
			// fail
			return errors.New("Redis Touch Failed: " + "All Keys in Param Not Found")
		}
	}
}

// Persist removes existing timeout TTL of a key so it lives forever
func (t *TTL) Persist(key string) (err error) {
	if t == nil {
		return errors.New("Redis TTL Persist Failed: TTL receiver is nil")
	}
	if t.core == nil {
		return errors.New("Redis TTL Persist Failed: Redis core is nil")
	}
	snap := t.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Persist", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Persist-Key", key))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = t.persistInternal(snap, key)
		return err
	} else {
		return t.persistInternal(snap, key)
	}
}

// persistInternal removes existing timeout TTL of a key so it lives forever
func (t *TTL) persistInternal(snap redisConnSnapshot, key string) error {
	// validate
	if t.core == nil {
		return errors.New("Redis Persist Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis Persist Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis Persist Failed: " + "Key is Required")
	}

	cmd := snap.writer.Persist(snap.writer.Context(), key)

	if val, err := t.core.handleBoolCmd(cmd, "Redis Persist Failed: "); err != nil {
		return err
	} else {
		if val {
			// success
			return nil
		} else {
			// fail
			return errors.New("Redis Persist Failed: " + "Key Was Not Found")
		}
	}
}

// ----------------------------------------------------------------------------------------------------------------
// UTILS functions
// ----------------------------------------------------------------------------------------------------------------

// Ping will ping the redis server to see if its up
//
// result nil = success; otherwise error info is returned via error object
func (u *UTILS) Ping() (err error) {
	if u == nil {
		return errors.New("Redis UTILS Ping Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return errors.New("Redis UTILS Ping Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Ping", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = u.pingInternal(snap)
		return err
	} else {
		return u.pingInternal(snap)
	}
}

// pingInternal will ping the redis server to see if its up
//
// result nil = success; otherwise error info is returned via error object
func (u *UTILS) pingInternal(snap redisConnSnapshot) error {
	// validate
	if u.core == nil {
		return errors.New("Redis Ping Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis Ping Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.Ping(snap.reader.Context())
	return u.core.handleStatusCmd(cmd, "Redis Ping Failed: ")
}

// DBSize returns number of keys in the redis database
func (u *UTILS) DBSize() (val int64, err error) {
	if u == nil {
		return 0, errors.New("Redis UTILS DBSize Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return 0, errors.New("Redis UTILS DBSize Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-DBSize", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-DBSize-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, err = u.dbSizeInternal(snap)
		return val, err
	} else {
		return u.dbSizeInternal(snap)
	}
}

// dbSizeInternal returns number of keys in the redis database
func (u *UTILS) dbSizeInternal(snap redisConnSnapshot) (val int64, err error) {
	// validate
	if u.core == nil {
		return 0, errors.New("Redis DBSize Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, errors.New("Redis DBSize Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.DBSize(snap.reader.Context())
	val, _, err = u.core.handleIntCmd(cmd, "Redis DBSize Failed: ")
	return val, err
}

// Time returns the redis server time
func (u *UTILS) Time() (val time.Time, err error) {
	if u == nil {
		return time.Time{}, errors.New("Redis UTILS Time Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return time.Time{}, errors.New("Redis UTILS Time Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Time", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Time-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, err = u.timeInternal(snap)
		return val, err
	} else {
		return u.timeInternal(snap)
	}
}

// timeInternal returns the redis server time
func (u *UTILS) timeInternal(snap redisConnSnapshot) (val time.Time, err error) {
	// validate
	if u.core == nil {
		return time.Time{}, errors.New("Redis Time Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return time.Time{}, errors.New("Redis Time Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.Time(snap.reader.Context())
	val, _, err = u.core.handleTimeCmd(cmd, "Redis Time Failed: ")
	return val, err
}

// LastSave checks if last db save action was successful
func (u *UTILS) LastSave() (val time.Time, err error) {
	if u == nil {
		return time.Time{}, errors.New("Redis UTILS LastSave Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return time.Time{}, errors.New("Redis UTILS LastSave Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LastSave", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-LastSave-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, err = u.lastSaveInternal(snap)
		return val, err
	} else {
		return u.lastSaveInternal(snap)
	}
}

// lastSaveInternal checks if last db save action was successful
func (u *UTILS) lastSaveInternal(snap redisConnSnapshot) (val time.Time, err error) {
	// validate
	if u.core == nil {
		return time.Time{}, errors.New("Redis LastSave Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return time.Time{}, errors.New("Redis LastSave Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.LastSave(snap.reader.Context())
	v, _, e := u.core.handleIntCmd(cmd, "Redis LastSave Failed: ")

	if e != nil {
		return time.Time{}, e
	} else {
		return time.Unix(v, 0), nil
	}
}

// Type returns the redis key's value type stored
// expected result in string = list, set, zset, hash, and stream
func (u *UTILS) Type(key string) (val rediskeytype.RedisKeyType, err error) {
	if u == nil {
		return rediskeytype.UNKNOWN, errors.New("Redis UTILS Type Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return rediskeytype.UNKNOWN, errors.New("Redis UTILS Type Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Type", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Type-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Type-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, err = u.typeInternal(snap, key)
		return val, err
	} else {
		return u.typeInternal(snap, key)
	}
}

// typeInternal returns the redis key's value type stored
// expected result in string = list, set, zset, hash, and stream
func (u *UTILS) typeInternal(snap redisConnSnapshot, key string) (val rediskeytype.RedisKeyType, err error) {
	// validate
	if u.core == nil {
		return rediskeytype.UNKNOWN, errors.New("Redis Type Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return rediskeytype.UNKNOWN, errors.New("Redis Type Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return rediskeytype.UNKNOWN, errors.New("Redis Type Failed: " + "Key is Required")
	}

	cmd := snap.reader.Type(snap.reader.Context(), key)

	if v, _, e := u.core.handleStringStatusCmd(cmd, "Redis Type Failed: "); e != nil {
		return rediskeytype.UNKNOWN, e
	} else {
		switch strings.ToUpper(v) {
		case "STRING":
			return rediskeytype.String, nil
		case "LIST":
			return rediskeytype.List, nil
		case "SET":
			return rediskeytype.Set, nil
		case "ZSET":
			return rediskeytype.ZSet, nil
		case "HASH":
			return rediskeytype.Hash, nil
		case "STREAM":
			return rediskeytype.Stream, nil
		case "NONE":
			return rediskeytype.UNKNOWN, nil
		default:
			return rediskeytype.UNKNOWN, errors.New("Redis Type Failed: " + "Type '" + v + "' Not Expected Value")
		}
	}
}

// ObjectEncoding returns the internal representation used in order to store the value associated with a key
func (u *UTILS) ObjectEncoding(key string) (val string, notFound bool, err error) {
	if u == nil {
		return "", false, errors.New("Redis UTILS ObjectEncoding Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return "", false, errors.New("Redis UTILS ObjectEncoding Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ObjectEncoding", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ObjectEncoding-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ObjectEncoding-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ObjectEncoding-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = u.objectEncodingInternal(snap, key)
		return val, notFound, err
	} else {
		return u.objectEncodingInternal(snap, key)
	}
}

// objectEncodingInternal returns the internal representation used in order to store the value associated with a key
func (u *UTILS) objectEncodingInternal(snap redisConnSnapshot, key string) (val string, notFound bool, err error) {
	// validate
	if u.core == nil {
		return "", false, errors.New("Redis ObjectEncoding Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return "", false, errors.New("Redis ObjectEncoding Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return "", false, errors.New("Redis ObjectEncoding Failed: " + "Key is Required")
	}

	cmd := snap.reader.ObjectEncoding(snap.reader.Context(), key)
	return u.core.handleStringCmd2(cmd, "Redis ObjectEncoding Failed: ")
}

// ObjectIdleTime returns the number of seconds since the object stored at the specified key is idle (not requested by read or write operations)
func (u *UTILS) ObjectIdleTime(key string) (val time.Duration, notFound bool, err error) {
	if u == nil {
		return 0, false, errors.New("Redis UTILS ObjectIdleTime Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return 0, false, errors.New("Redis UTILS ObjectIdleTime Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ObjectIdleTime", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ObjectIdleTime-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ObjectIdleTime-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ObjectIdleTime-Result-Seconds", val.Seconds()))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = u.objectIdleTimeInternal(snap, key)
		return val, notFound, err
	} else {
		return u.objectIdleTimeInternal(snap, key)
	}
}

// objectIdleTimeInternal returns the number of seconds since the object stored at the specified key is idle (not requested by read or write operations)
func (u *UTILS) objectIdleTimeInternal(snap redisConnSnapshot, key string) (val time.Duration, notFound bool, err error) {
	// validate
	if u.core == nil {
		return 0, false, errors.New("Redis ObjectIdleTime Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis ObjectIdleTime Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ObjectIdleTime Failed: " + "Key is Required")
	}

	cmd := snap.reader.ObjectIdleTime(snap.reader.Context(), key)
	return u.core.handleDurationCmd(cmd, "Redis ObjectIdleTime Failed: ")
}

// ObjectRefCount returns the number of references of the value associated with the specified key
func (u *UTILS) ObjectRefCount(key string) (val int64, notFound bool, err error) {
	if u == nil {
		return 0, false, errors.New("Redis UTILS ObjectRefCount Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return 0, false, errors.New("Redis UTILS ObjectRefCount Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ObjectRefCount", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ObjectRefCount-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ObjectRefCount-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-ObjectRefCount-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = u.objectRefCountInternal(snap, key)
		return val, notFound, err
	} else {
		return u.objectRefCountInternal(snap, key)
	}
}

// objectRefCountInternal returns the number of references of the value associated with the specified key
func (u *UTILS) objectRefCountInternal(snap redisConnSnapshot, key string) (val int64, notFound bool, err error) {
	// validate
	if u.core == nil {
		return 0, false, errors.New("Redis ObjectRefCount Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return 0, false, errors.New("Redis ObjectRefCount Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ObjectRefCount Failed: " + "Key is Required")
	}

	cmd := snap.reader.ObjectRefCount(snap.reader.Context(), key)
	return u.core.handleIntCmd(cmd, "Redis ObjectRefCount: ")
}

// Scan is used to incrementally iterate over a set of keys，
// Scan is a cursor based iterator, at every call of the command, redis returns an updated cursor that client must use for next call to sort,
//
// start iteration = cursor set to 0
// stop iteration = when redis returns cursor value of 0
//
// match = filters elements based on match filter, for elements retrieved from redis before return to client
//
//	glob-style patterns:
//		1) h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//		2) h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//		3) h*llo = * represents any single or more char match (hllo, heeeelo match)
//		4) h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//		5) h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//		6) h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//		7) Use \ to escape special characters if needing to match verbatim
//
// count = hint to redis count of elements to retrieve in the call
func (u *UTILS) Scan(cursor uint64, match string, count int64) (keys []string, resultCursor uint64, err error) {
	if u == nil {
		return nil, 0, errors.New("Redis UTILS Scan Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return nil, 0, errors.New("Redis UTILS Scan Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Scan", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Scan-Match", match))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Scan-Scan-Cursor", cursor))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Scan-Scan-Count", count))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Scan-Result-Cursor", resultCursor))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Scan-Keys-Found", keys))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		keys, resultCursor, err = u.scanInternal(snap, cursor, match, count)
		return keys, resultCursor, err
	} else {
		return u.scanInternal(snap, cursor, match, count)
	}
}

// scanInternal is used to incrementally iterate over a set of keys，
// Scan is a cursor based iterator, at every call of the command, redis returns an updated cursor that client must use for next call to sort,
//
// start iteration = cursor set to 0
// stop iteration = when redis returns cursor value of 0
//
// match = filters elements based on match filter, for elements retrieved from redis before return to client
//
//	glob-style patterns:
//		1) h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//		2) h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//		3) h*llo = * represents any single or more char match (hllo, heeeelo match)
//		4) h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//		5) h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//		6) h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//		7) Use \ to escape special characters if needing to match verbatim
//
// count = hint to redis count of elements to retrieve in the call
func (u *UTILS) scanInternal(snap redisConnSnapshot, cursor uint64, match string, count int64) (keys []string, resultCursor uint64, err error) {
	// validate
	if u.core == nil {
		return nil, 0, errors.New("Redis Scan Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, 0, errors.New("Redis Scan Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.Scan(snap.reader.Context(), cursor, match, count)
	return u.core.handleScanCmd(cmd, "Redis Scan Failed: ")
}

// Deprecated: Keys uses the Redis KEYS command which performs an O(N) scan of the entire keyspace,
// blocking the Redis event loop for the duration. On a production instance with millions of keys
// this can cause multi-second latency spikes for all clients.
// Use [UTILS.ScanKeys] (full collection) or [UTILS.Scan] (cursor-based iteration) instead,
// both of which use the incremental SCAN command that yields the event loop between batches.
//
// Keys returns all keys matching given pattern (use with extreme care, use for debugging only, may slow production system down significantly)
//
// start iteration = cursor set to 0
// stop iteration = when redis returns cursor value of 0
//
// match = filters elements based on match filter, for elements retrieved from redis before return to client
//
//	glob-style patterns:
//		1) h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//		2) h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//		3) h*llo = * represents any single or more char match (hllo, heeeelo match)
//		4) h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//		5) h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//		6) h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//		7) Use \ to escape special characters if needing to match verbatim
func (u *UTILS) Keys(match string) (valKeys []string, notFound bool, err error) {
	// Emit a one-time deprecation warning so operators can detect usage in production logs.
	keysDeprecationOnce.Do(func() {
		if KeysDeprecationHook != nil {
			KeysDeprecationHook()
		}
	})

	if u == nil {
		return nil, false, errors.New("Redis UTILS Keys Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return nil, false, errors.New("Redis UTILS Keys Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Keys", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Keys-Match", match))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Keys-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Keys-Keys-Found", valKeys))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		valKeys, notFound, err = u.keysInternal(snap, match)
		return valKeys, notFound, err
	} else {
		return u.keysInternal(snap, match)
	}
}

// keysInternal returns all keys matching given pattern (use with extreme care, use for debugging only, may slow production system down significantly)
//
// start iteration = cursor set to 0
// stop iteration = when redis returns cursor value of 0
//
// match = filters elements based on match filter, for elements retrieved from redis before return to client
//
//	glob-style patterns:
//		1) h?llo = ? represents any single char match (hello, hallo, hxllo match, but heello not match)
//		2) h??llo = ?? represents any two char match (heello, haello, hxyllo match, but heeello not match)
//		3) h*llo = * represents any single or more char match (hllo, heeeelo match)
//		4) h[ae]llo = [ae] represents char inside [ ] that are to match (hello, hallo match, but hillo not match)
//		5) h[^e]llo = [^e] represents any char other than e to match (hallo, hbllo match, but hello not match)
//		6) h[a-b]llo = [a-b] represents any char match between the a-b range (hallo, hbllo match, but hcllo not match)
//		7) Use \ to escape special characters if needing to match verbatim
func (u *UTILS) keysInternal(snap redisConnSnapshot, match string) (valKeys []string, notFound bool, err error) {
	// validate
	if u.core == nil {
		return nil, false, errors.New("Redis Keys Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis Keys Failed: " + "Endpoint Connections Not Ready")
	}

	if len(match) <= 0 {
		return nil, false, errors.New("Redis Keys Failed: " + "Match is Required")
	}

	cmd := snap.reader.Keys(snap.reader.Context(), match)
	return u.core.handleStringSliceCmd(cmd, "Redis Keys Failed: ")
}

// ScanKeys returns all keys matching the given pattern using the incremental SCAN command.
// Unlike [UTILS.Keys], ScanKeys does not block the Redis event loop because SCAN yields
// between batches. This makes it safe to use in production.
//
// countHint suggests how many keys Redis should return per SCAN iteration (default 100 if <= 0).
// The actual number returned per iteration may vary; Redis treats count as a hint, not a hard limit.
//
// match uses the same glob-style patterns as [UTILS.Keys]:
//
//	1) h?llo = ? represents any single char match
//	2) h*llo = * represents any single or more char match
//	3) h[ae]llo = [ae] represents char inside [ ] that are to match
//	4) h[^e]llo = [^e] represents any char other than e to match
//	5) h[a-b]llo = [a-b] represents any char match between the a-b range
//	6) Use \ to escape special characters if needing to match verbatim
func (u *UTILS) ScanKeys(match string, countHint ...int64) (allKeys []string, err error) {
	if u == nil {
		return nil, errors.New("Redis UTILS ScanKeys Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return nil, errors.New("Redis UTILS ScanKeys Failed: Redis core is nil")
	}
	if len(match) == 0 {
		return nil, errors.New("Redis UTILS ScanKeys Failed: Match is Required")
	}

	var count int64 = 100
	if len(countHint) > 0 && countHint[0] > 0 {
		count = countHint[0]
	}

	var cursor uint64
	for {
		var keys []string
		keys, cursor, err = u.Scan(cursor, match, count)
		if err != nil {
			return nil, err
		}
		allKeys = append(allKeys, keys...)
		if cursor == 0 {
			break
		}
	}

	if len(allKeys) == 0 {
		return nil, nil
	}
	return allKeys, nil
}

// RandomKey returns a random key from redis
func (u *UTILS) RandomKey() (val string, err error) {
	if u == nil {
		return "", errors.New("Redis UTILS RandomKey Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return "", errors.New("Redis UTILS RandomKey Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-RandomKey", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RandomKey-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, err = u.randomKeyInternal(snap)
		return val, err
	} else {
		return u.randomKeyInternal(snap)
	}
}

// randomKeyInternal returns a random key from redis
func (u *UTILS) randomKeyInternal(snap redisConnSnapshot) (val string, err error) {
	// validate
	if u.core == nil {
		return "", errors.New("Redis RandomKey Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return "", errors.New("Redis RandomKey Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := snap.reader.RandomKey(snap.reader.Context())
	val, _, err = u.core.handleStringCmd2(cmd, "Redis RandomKey Failed: ")
	return val, err
}

// Rename will rename the keyOriginal to be keyNew in redis,
// if keyNew already exist in redis, then Rename will override existing keyNew with keyOriginal
// if keyOriginal is not in redis, error is returned
func (u *UTILS) Rename(keyOriginal string, keyNew string) (err error) {
	if u == nil {
		return errors.New("Redis UTILS Rename Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return errors.New("Redis UTILS Rename Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Rename", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Rename-OriginalKey", keyOriginal))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Rename-NewKey", keyNew))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = u.renameInternal(snap, keyOriginal, keyNew)
		return err
	} else {
		return u.renameInternal(snap, keyOriginal, keyNew)
	}
}

// renameInternal will rename the keyOriginal to be keyNew in redis,
// if keyNew already exist in redis, then Rename will override existing keyNew with keyOriginal
// if keyOriginal is not in redis, error is returned
func (u *UTILS) renameInternal(snap redisConnSnapshot, keyOriginal string, keyNew string) error {
	// validate
	if u.core == nil {
		return errors.New("Redis Rename Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis Rename Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyOriginal) <= 0 {
		return errors.New("Redis Rename Failed: " + "Key Original is Required")
	}

	if len(keyNew) <= 0 {
		return errors.New("Redis Rename Failed: " + "Key New is Required")
	}

	cmd := snap.writer.Rename(snap.writer.Context(), keyOriginal, keyNew)
	return u.core.handleStatusCmd(cmd, "Redis Rename Failed: ")
}

// RenameNX will rename the keyOriginal to be keyNew IF keyNew does not yet exist in redis
// if RenameNX fails due to keyNew already exist, or other errors, the error is returned
func (u *UTILS) RenameNX(keyOriginal string, keyNew string) (err error) {
	if u == nil {
		return errors.New("Redis UTILS RenameNX Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return errors.New("Redis UTILS RenameNX Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-RenameNX", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RenameNX-OriginalKey", keyOriginal))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-RenameNX-NewKey", keyNew))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = u.renameNXInternal(snap, keyOriginal, keyNew)
		return err
	} else {
		return u.renameNXInternal(snap, keyOriginal, keyNew)
	}
}

// renameNXInternal will rename the keyOriginal to be keyNew IF keyNew does not yet exist in redis
// if RenameNX fails due to keyNew already exist, or other errors, the error is returned
func (u *UTILS) renameNXInternal(snap redisConnSnapshot, keyOriginal string, keyNew string) error {
	// validate
	if u.core == nil {
		return errors.New("Redis RenameNX Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis RenameNX Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyOriginal) <= 0 {
		return errors.New("Redis RenameNX Failed: " + "Key Original is Required")
	}

	if len(keyNew) <= 0 {
		return errors.New("Redis RenameNX Failed: " + "Key New is Required")
	}

	cmd := snap.writer.RenameNX(snap.writer.Context(), keyOriginal, keyNew)
	if val, err := u.core.handleBoolCmd(cmd, "Redis RenameNX Failed: "); err != nil {
		return err
	} else {
		if val {
			// success
			return nil
		} else {
			// not success
			return errors.New("Redis RenameNX Failed: " + "Key Was Not Renamed")
		}
	}
}

// Sort will sort values as defined by keyToSort, along with sortPattern, and then return the sorted data via string slice
// sort is applicable to list, set, or sorted set as defined by key
//
// sortPattern = defines the sort conditions (see redis sort documentation for details)
func (u *UTILS) Sort(key string, sortPattern *redis.Sort) (val []string, notFound bool, err error) {
	if u == nil {
		return nil, false, errors.New("Redis UTILS Sort Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return nil, false, errors.New("Redis UTILS Sort Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Sort", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Sort-Key", key))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Sort-SortPattern", sortPattern))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Sort-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-Sort-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = u.sortInternal(snap, key, sortPattern)
		return val, notFound, err
	} else {
		return u.sortInternal(snap, key, sortPattern)
	}
}

// sortInternal will sort values as defined by keyToSort, along with sortPattern, and then return the sorted data via string slice
// sort is applicable to list, set, or sorted set as defined by key
//
// sortPattern = defines the sort conditions (see redis sort documentation for details)
func (u *UTILS) sortInternal(snap redisConnSnapshot, key string, sortPattern *redis.Sort) (val []string, notFound bool, err error) {
	// validate
	if u.core == nil {
		return nil, false, errors.New("Redis Sort Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis Sort Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis Sort Failed: " + "Key is Required")
	}

	cmd := snap.reader.Sort(snap.reader.Context(), key, sortPattern)
	return u.core.handleStringSliceCmd(cmd, "Redis Sort Failed: ")
}

// SortInterfaces will sort values as defined by keyToSort, along with sortPattern, and then return the sorted data via []interface{}
// sort is applicable to list, set, or sorted set as defined by key
//
// sortPattern = defines the sort conditions (see redis sort documentation for details)
func (u *UTILS) SortInterfaces(keyToSort string, sortPattern *redis.Sort) (val []interface{}, notFound bool, err error) {
	if u == nil {
		return nil, false, errors.New("Redis UTILS SortInterfaces Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return nil, false, errors.New("Redis UTILS SortInterfaces Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SortInterfaces", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SortInterfaces-SortKey", keyToSort))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SortInterfaces-SortPattern", sortPattern))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SortInterfaces-Not-Found", notFound))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SortInterfaces-Result", val))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		val, notFound, err = u.sortInterfacesInternal(snap, keyToSort, sortPattern)
		return val, notFound, err
	} else {
		return u.sortInterfacesInternal(snap, keyToSort, sortPattern)
	}
}

// sortInterfacesInternal will sort values as defined by keyToSort, along with sortPattern, and then return the sorted data via []interface{}
// sort is applicable to list, set, or sorted set as defined by key
//
// sortPattern = defines the sort conditions (see redis sort documentation for details)
func (u *UTILS) sortInterfacesInternal(snap redisConnSnapshot, keyToSort string, sortPattern *redis.Sort) (val []interface{}, notFound bool, err error) {
	// validate
	if u.core == nil {
		return nil, false, errors.New("Redis SortInterfaces Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return nil, false, errors.New("Redis SortInterfaces Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyToSort) <= 0 {
		return nil, false, errors.New("Redis SortInterfaces Failed: " + "KeyToSort is Required")
	}

	cmd := snap.reader.SortInterfaces(snap.reader.Context(), keyToSort, sortPattern)
	return u.core.handleSliceCmd(cmd, "Redis SortInterfaces Failed: ")
}

// SortStore will sort values defined by keyToSort, and sort according to sortPattern, and set sorted results into keyToStore in redis
// sort is applicable to list, set, or sorted set as defined by key
//
// sortPattern = defines the sort conditions (see redis sort documentation for details)
func (u *UTILS) SortStore(keyToSort string, keyToStore string, sortPattern *redis.Sort) (err error) {
	if u == nil {
		return errors.New("Redis UTILS SortStore Failed: UTILS receiver is nil")
	}
	if u.core == nil {
		return errors.New("Redis UTILS SortStore Failed: Redis core is nil")
	}
	snap := u.core.connSnapshot()
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SortStore", snap.parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SortStore-SortKey", keyToSort))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SortStore-SortPattern", sortPattern))
			xray.LogXrayAddFailure("Redis", seg.SafeAddMetadata("Redis-SortStore-StoreKey", keyToStore))

			if err != nil {
				xray.LogXrayAddFailure("Redis", seg.SafeAddError(err))
			}
		}()

		err = u.sortStoreInternal(snap, keyToSort, keyToStore, sortPattern)
		return err
	} else {
		return u.sortStoreInternal(snap, keyToSort, keyToStore, sortPattern)
	}
}

// sortStoreInternal will sort values defined by keyToSort, and sort according to sortPattern, and set sorted results into keyToStore in redis
// sort is applicable to list, set, or sorted set as defined by key
//
// sortPattern = defines the sort conditions (see redis sort documentation for details)
func (u *UTILS) sortStoreInternal(snap redisConnSnapshot, keyToSort string, keyToStore string, sortPattern *redis.Sort) error {
	// validate
	if u.core == nil {
		return errors.New("Redis SortStore Failed: " + "Base is Nil")
	}

	if !snap.ready {
		return errors.New("Redis SortStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyToSort) <= 0 {
		return errors.New("Redis SortStore Failed: " + "KeyToSort is Required")
	}

	if len(keyToStore) <= 0 {
		return errors.New("Redis SortStore Failed: " + "KeyToStore is Required")
	}

	cmd := snap.writer.SortStore(snap.writer.Context(), keyToSort, keyToStore, sortPattern)
	_, _, err := u.core.handleIntCmd(cmd, "Redis SortStore Failed: ")
	return err
}
