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
	"crypto/tls"
	"strings"

	"github.com/aldelo/common/wrapper/redis/redisbitop"
	"github.com/aldelo/common/wrapper/redis/redisdatatype"
	"github.com/aldelo/common/wrapper/redis/rediskeytype"
	"github.com/aldelo/common/wrapper/redis/redisradiusunit"
	"github.com/aldelo/common/wrapper/redis/redissetcondition"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/go-redis/redis/v8"

	"errors"
	"time"

	util "github.com/aldelo/common"
)

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
	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			r._parentSegment = parentSegment[0]
		}

		seg := xray.NewSegment("Redis-Connect", r._parentSegment)
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Writer-Endpoint", r.AwsRedisWriterEndpoint)
			_ = seg.Seg.AddMetadata("Redis-Reader-Endpoint", r.AwsRedisReaderEndpoint)

			if err != nil {
				_ = seg.Seg.AddError(err)
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
	// clean up prior cn reference
	if r.BIT != nil {
		r.BIT.core = nil
		r.BIT = nil
	}

	if r.LIST != nil {
		r.LIST.core = nil
		r.LIST = nil
	}

	if r.HASH != nil {
		r.HASH.core = nil
		r.HASH = nil
	}

	if r.SET != nil {
		r.SET.core = nil
		r.SET = nil
	}

	if r.SORTED_SET != nil {
		r.SORTED_SET.core = nil
		r.SORTED_SET = nil
	}

	if r.GEO != nil {
		r.GEO.core = nil
		r.GEO = nil
	}

	if r.STREAM != nil {
		r.STREAM.core = nil
		r.STREAM = nil
	}

	if r.PUBSUB != nil {
		r.PUBSUB.core = nil
		r.PUBSUB = nil
	}

	if r.PIPELINE != nil {
		r.PIPELINE.core = nil
		r.PIPELINE = nil
	}

	if r.TTL != nil {
		r.TTL.core = nil
		r.TTL = nil
	}

	if r.UTILS != nil {
		r.UTILS.core = nil
		r.UTILS = nil
	}

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
	optWriter := &redis.Options{
		Addr:         r.AwsRedisWriterEndpoint, // redis endpoint url and port
		Password:     "",                       // no password set
		DB:           0,                        // use default DB
		ReadTimeout:  3 * time.Second,          // time after read operation timeout
		WriteTimeout: 3 * time.Second,          // time after write operation timeout
		PoolSize:     10,                       // 10 connections per every cpu
		MinIdleConns: 3,                        // minimum number of idle connections to keep
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
		ReadTimeout:  3 * time.Second,          // time after read operation timeout
		WriteTimeout: 3 * time.Second,          // time after write operation timeout
		PoolSize:     10,                       // 10 connections per every cpu
		MinIdleConns: 3,                        // minimum number of idle connections to keep
	}
	if r.EnableTLS {
		optReader.TLSConfig = &tls.Config{InsecureSkipVerify: false}
	}
	r.cnReader = redis.NewClient(optReader)

	if r.cnReader == nil {
		return errors.New("Connect To Redis Failed: (Reader Endpoint) " + "Obtain Client Yielded Nil")
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
	// clean up prior cn reference
	if r.BIT != nil {
		r.BIT.core = nil
		r.BIT = nil
	}

	if r.LIST != nil {
		r.LIST.core = nil
		r.LIST = nil
	}

	if r.HASH != nil {
		r.HASH.core = nil
		r.HASH = nil
	}

	if r.SET != nil {
		r.SET.core = nil
		r.SET = nil
	}

	if r.SORTED_SET != nil {
		r.SORTED_SET.core = nil
		r.SORTED_SET = nil
	}

	if r.GEO != nil {
		r.GEO.core = nil
		r.GEO = nil
	}

	if r.STREAM != nil {
		r.STREAM.core = nil
		r.STREAM = nil
	}

	if r.PUBSUB != nil {
		r.PUBSUB.core = nil
		r.PUBSUB = nil
	}

	if r.PIPELINE != nil {
		r.PIPELINE.core = nil
		r.PIPELINE = nil
	}

	if r.TTL != nil {
		r.TTL.core = nil
		r.TTL = nil
	}

	if r.UTILS != nil {
		r.UTILS.core = nil
		r.UTILS = nil
	}

	if r.cnWriter != nil {
		_ = r.cnWriter.Close()
		r.cnWriter = nil
	}

	if r.cnReader != nil {
		_ = r.cnReader.Close()
		r.cnReader = nil
	}

	r.cnAreReady = false
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (r *Redis) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	r._parentSegment = parentSegment
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
						return true, nil
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
				if len(outputSlice) > 0 {
					return outputSlice, false, nil
				} else {
					return nil, true, nil
				}
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
				if len(outputSlice) > 0 {
					return outputSlice, false, nil
				} else {
					return nil, true, nil
				}
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
				if len(outputSlice) > 0 {
					return outputSlice, false, nil
				} else {
					return nil, true, nil
				}
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Set", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Set-Key", key)
			_ = seg.Seg.AddMetadata("Redis-Set-Value", val)
			_ = seg.Seg.AddMetadata("Redis-Set-Condition", setCondition.Caption())

			if len(expires) > 0 {
				_ = seg.Seg.AddMetadata("Redis-Set-Expire-Seconds", expires[0].Seconds())
			} else {
				_ = seg.Seg.AddMetadata("Redis-Set-Expire-Seconds", "Not Defined")
			}

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = r.setBaseInternal(key, val, setCondition, expires...)
		return err
	} else {
		return r.setBaseInternal(key, val, setCondition, expires...)
	}
}

// setBaseInternal is helper to set value into redis by key
//
// notes
//
//	setCondition = support for SetNX and SetXX
func (r *Redis) setBaseInternal(key string, val interface{}, setCondition redissetcondition.RedisSetCondition, expires ...time.Duration) error {
	// validate
	if !r.cnAreReady {
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
		cmd := r.cnWriter.Set(r.cnWriter.Context(), key, val, expireDuration)
		return r.handleStatusCmd(cmd, "Redis Set Failed: (Set Method) ")

	case redissetcondition.SetIfExists:
		cmd := r.cnWriter.SetXX(r.cnWriter.Context(), key, val, expireDuration)
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
		cmd := r.cnWriter.SetNX(r.cnWriter.Context(), key, val, expireDuration)
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
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetBool sets boolean value into redis by key
func (r *Redis) SetBool(key string, val bool, expires ...time.Duration) error {
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetInt sets int value into redis by key
func (r *Redis) SetInt(key string, val int, expires ...time.Duration) error {
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetInt64 sets int64 value into redis by key
func (r *Redis) SetInt64(key string, val int64, expires ...time.Duration) error {
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetFloat64 sets float64 value into redis by key
func (r *Redis) SetFloat64(key string, val float64, expires ...time.Duration) error {
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetBytes sets []byte value into redis by key
func (r *Redis) SetBytes(key string, val []byte, expires ...time.Duration) error {
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// SetJson sets Json object into redis by key (Json object is marshaled into string and then saved to redis)
func (r *Redis) SetJson(key string, jsonObject interface{}, expires ...time.Duration) error {
	if val, err := util.MarshalJSONCompact(jsonObject); err != nil {
		return errors.New("Redis Set Failed: (Marshal Json) " + err.Error())
	} else {
		return r.SetBase(key, val, redissetcondition.Normal, expires...)
	}
}

// SetTime sets time.Time value into redis by key
func (r *Redis) SetTime(key string, val time.Time, expires ...time.Duration) error {
	return r.SetBase(key, val, redissetcondition.Normal, expires...)
}

// GetBase is internal helper to get value from redis.
func (r *Redis) GetBase(key string) (cmd *redis.StringCmd, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Get", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Get-Key", key)
			_ = seg.Seg.AddMetadata("Redis-Get-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-Get-Value-Cmd", cmd)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		cmd, notFound, err = r.getBaseInternal(key)
		return cmd, notFound, err
	} else {
		return r.getBaseInternal(key)
	}
}

// getBaseInternal is internal helper to get value from redis.
func (r *Redis) getBaseInternal(key string) (cmd *redis.StringCmd, notFound bool, err error) {
	// validate
	if !r.cnAreReady {
		return nil, false, errors.New("Redis Get Failed: " + "Endpoint Connections Not Ready")
	}

	if util.LenTrim(key) <= 0 {
		return nil, false, errors.New("Redis Get Failed: " + "Key is Required")
	}

	// get value from redis
	cmd = r.cnReader.Get(r.cnReader.Context(), key)

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
					return true, nil
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
	// reg new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GetSet", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-GetSet-Key", key)
			_ = seg.Seg.AddMetadata("Redis-GetSet-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-GetSet-Old_Value", oldValue)
			_ = seg.Seg.AddMetadata("Redis-GetSet-New-Value", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		oldValue, notFound, err = r.getSetInternal(key, val)
		return oldValue, notFound, err
	} else {
		return r.getSetInternal(key, val)
	}
}

// getSetInternal will get old string value from redis by key,
// and then set new string value into redis by the same key.
func (r *Redis) getSetInternal(key string, val string) (oldValue string, notFound bool, err error) {
	// validate
	if !r.cnAreReady {
		return "", false, errors.New("Redis GetSet Failed: " + "Endpoint Connections Not Ready")
	}

	if util.LenTrim(key) <= 0 {
		return "", false, errors.New("Redis GetSet Failed: " + "Key is Required")
	}

	// persist value and get old value as return result
	cmd := r.cnWriter.GetSet(r.cnWriter.Context(), key, val)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-MSet", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-MSet-KeyValueMap", kvMap)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = r.msetInternal(kvMap, setIfNotExists...)
		return err
	} else {
		return r.msetInternal(kvMap, setIfNotExists...)
	}
}

// msetInternal is helper to set multiple values into redis by keys,
// optional parameter setIfNotExists indicates if instead MSetNX is to be used
//
// notes
//
//	kvMap = map of key string, and interface{} value
func (r *Redis) msetInternal(kvMap map[string]interface{}, setIfNotExists ...bool) error {
	// validate
	if kvMap == nil {
		return errors.New("Redis MSet Failed: " + "KVMap is Required")
	}

	if !r.cnAreReady {
		return errors.New("Redis MSet Failed: " + "Endpoint Connections Not Ready")
	}

	// persist value to redis
	nx := false

	if len(setIfNotExists) > 0 {
		nx = setIfNotExists[0]
	}

	if !nx {
		// normal
		cmd := r.cnWriter.MSet(r.cnWriter.Context(), kvMap)
		return r.handleStatusCmd(cmd, "Redis MSet Failed: ")
	} else {
		// nx
		cmd := r.cnWriter.MSetNX(r.cnWriter.Context(), kvMap)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-MGet", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-MGet-Keys", key)
			_ = seg.Seg.AddMetadata("Redis-MGet-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-MGet-Results", results)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		results, notFound, err = r.mgetInternal(key...)
		return results, notFound, err
	} else {
		return r.mgetInternal(key...)
	}
}

// mgetInternal is a helper to get values from redis based on one or more keys specified
func (r *Redis) mgetInternal(key ...string) (results []interface{}, notFound bool, err error) {
	// validate
	if len(key) <= 0 {
		return nil, false, errors.New("Redis MGet Failed: " + "Key is Required")
	}

	if !r.cnAreReady {
		return nil, false, errors.New("Redis MGet Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := r.cnReader.MGet(r.cnReader.Context(), key...)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SetRange", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SetRange-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SetRange-Offset", offset)
			_ = seg.Seg.AddMetadata("Redis-SetRange-Value", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = r.setRangeInternal(key, offset, val)
		return err
	} else {
		return r.setRangeInternal(key, offset, val)
	}
}

// setRangeInternal sets val into key's stored value in redis, offset by the offset number
//
// example:
//  1. "Hello World"
//  2. Offset 6 = W
//  3. Val "Xyz" replaces string from Offset Position 6
//  4. End Result String = "Hello Xyzld"
func (r *Redis) setRangeInternal(key string, offset int64, val string) error {
	// validate
	if len(key) <= 0 {
		return errors.New("Redis SetRange Failed: " + "Key is Required")
	}

	if !r.cnAreReady {
		return errors.New("Redis SetRange Failed: " + "Endpoint Connections Not Ready")
	}

	if offset < 0 {
		return errors.New("Redis SetRange Failed: " + "Offset Must Be 0 or Greater")
	}

	cmd := r.cnWriter.SetRange(r.cnWriter.Context(), key, offset, val)

	if _, _, err := r.handleIntCmd(cmd, "Redis SetRange Failed: "); err != nil {
		return err
	} else {
		return nil
	}
}

// GetRange gets val between start and end positions from string value stored by key in redis
func (r *Redis) GetRange(key string, start int64, end int64) (val string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GetRange", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-GetRange-Key", key)
			_ = seg.Seg.AddMetadata("Redis-GetRange-Start-End", util.Int64ToString(start)+"-"+util.Int64ToString(end))
			_ = seg.Seg.AddMetadata("Redis-GetRange-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-GetRange-Value", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = r.getRangeInternal(key, start, end)
		return val, notFound, err
	} else {
		return r.getRangeInternal(key, start, end)
	}
}

// getRangeInternal gets val between start and end positions from string value stored by key in redis
func (r *Redis) getRangeInternal(key string, start int64, end int64) (val string, notFound bool, err error) {
	// validate
	if len(key) <= 0 {
		return "", false, errors.New("Redis GetRange Failed: " + "Key is Required")
	}

	if !r.cnAreReady {
		return "", false, errors.New("Redis GetRange Failed: " + "Endpoint Connections Not Ready")
	}

	if start < 0 {
		return "", false, errors.New("Redis GetRange Failed: " + "Start Must Be 0 or Greater")
	}

	if end < start {
		return "", false, errors.New("Redis GetRange Failed: " + "End Must Equal or Be Greater Than Start")
	}

	cmd := r.cnReader.GetRange(r.cnReader.Context(), key, start, end)

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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Int64AddOrReduce", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Int64AddOrReduce-Key", key)

			if len(isReduce) > 0 {
				_ = seg.Seg.AddMetadata("Redis-Int64AddOrReduce-IsReduce", isReduce[0])
			} else {
				_ = seg.Seg.AddMetadata("Redis-Int64AddOrReduce-IsReduce", "false")
			}

			_ = seg.Seg.AddMetadata("Redis-Int64AddOrReduce-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-Int64AddOrReduce-Old-Value", val)
			_ = seg.Seg.AddMetadata("Redis-Int64AddOrReduce-New-Value", newVal)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		newVal, notFound, err = r.int64AddOrReduceInternal(key, val, isReduce...)
		return newVal, notFound, err
	} else {
		return r.int64AddOrReduceInternal(key, val, isReduce...)
	}
}

// int64AddOrReduceInternal will add or reduce int64 value against a key in redis,
// and return the new value if found and performed
func (r *Redis) int64AddOrReduceInternal(key string, val int64, isReduce ...bool) (newVal int64, notFound bool, err error) {
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

	if !r.cnAreReady {
		return 0, false, errors.New("Redis " + methodName + " Failed: " + "Endpoint Connections Not Ready")
	}

	if val <= 0 {
		return 0, false, errors.New("Redis " + methodName + " Failed: " + "Value Must Be Greater Than 0")
	}

	var cmd *redis.IntCmd

	if !reduce {
		// increment
		if val == 1 {
			cmd = r.cnWriter.Incr(r.cnWriter.Context(), key)
		} else {
			cmd = r.cnWriter.IncrBy(r.cnWriter.Context(), key, val)
		}
	} else {
		// decrement
		if val == 1 {
			cmd = r.cnWriter.Decr(r.cnWriter.Context(), key)
		} else {
			cmd = r.cnWriter.DecrBy(r.cnWriter.Context(), key, val)
		}
	}

	// evaluate cmd result
	return r.handleIntCmd(cmd, "Redis "+methodName+" Failed: ")
}

// Float64AddOrReduce will add or reduce float64 value against a key in redis,
// and return the new value if found and performed
func (r *Redis) Float64AddOrReduce(key string, val float64) (newVal float64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Float64AddOrReduce", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Float64AddOrReduce-Key", key)
			_ = seg.Seg.AddMetadata("Redis-Float64AddOrReduce-Value", val)
			_ = seg.Seg.AddMetadata("Redis-Float64AddOrReduce-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-Float64AddOrReduce-Result-NewValue", newVal)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		newVal, notFound, err = r.float64AddOrReduceInternal(key, val)
		return newVal, notFound, err
	} else {
		return r.float64AddOrReduceInternal(key, val)
	}
}

// float64AddOrReduceInternal will add or reduce float64 value against a key in redis,
// and return the new value if found and performed
func (r *Redis) float64AddOrReduceInternal(key string, val float64) (newVal float64, notFound bool, err error) {
	// validate
	if len(key) <= 0 {
		return 0.00, false, errors.New("Redis Float64AddOrReduce Failed: (IncrByFloat) " + "Key is Required")
	}

	if !r.cnAreReady {
		return 0.00, false, errors.New("Redis Float64AddOrReduce Failed: (IncrByFloat) " + "Endpoint Connections Not Ready")
	}

	cmd := r.cnWriter.IncrByFloat(r.cnWriter.Context(), key, val)
	return r.handleFloatCmd(cmd, "Redis Float64AddOrReduce Failed: (IncrByFloat)")
}

// ----------------------------------------------------------------------------------------------------------------
// HyperLogLog functions
// ----------------------------------------------------------------------------------------------------------------

// PFAdd is a HyperLogLog function to uniquely accumulate the count of a specific value to redis,
// such as email hit count, user hit count, ip address hit count etc, that is based on the unique occurences of such value
func (r *Redis) PFAdd(key string, elements ...interface{}) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PFAdd", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-PFAdd-Key", key)
			_ = seg.Seg.AddMetadata("Redis-PFAdd-Elements", elements)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = r.pfAddInternal(key, elements...)
		return err
	} else {
		return r.pfAddInternal(key, elements...)
	}
}

// pfAddInternal is a HyperLogLog function to uniquely accumulate the count of a specific value to redis,
// such as email hit count, user hit count, ip address hit count etc, that is based on the unique occurences of such value
func (r *Redis) pfAddInternal(key string, elements ...interface{}) error {
	// validate
	if len(key) <= 0 {
		return errors.New("Redis PFAdd Failed: " + "Key is Required")
	}

	if !r.cnAreReady {
		return errors.New("Redis PFAdd Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := r.cnWriter.PFAdd(r.cnWriter.Context(), key, elements...)

	if _, _, err := r.handleIntCmd(cmd, "Redis PFAdd Failed: "); err != nil {
		return err
	} else {
		return nil
	}
}

// PFCount is a HyperLogLog function to retrieve the current count associated with the given unique value in redis,
// Specify one or more keys, if multiple keys used, the result count is the union of all keys' unique value counts
func (r *Redis) PFCount(key ...string) (val int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PFCount", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-PFCount-Keys", key)
			_ = seg.Seg.AddMetadata("Redis-PFCount-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-PFCount-Result-Count", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = r.pfCountInternal(key...)
		return val, notFound, err
	} else {
		return r.pfCountInternal(key...)
	}
}

// pfCountInternal is a HyperLogLog function to retrieve the current count associated with the given unique value in redis,
// Specify one or more keys, if multiple keys used, the result count is the union of all keys' unique value counts
func (r *Redis) pfCountInternal(key ...string) (val int64, notFound bool, err error) {
	// validate
	if len(key) <= 0 {
		return 0, false, errors.New("Redis PFCount Failed: " + "Key is Required")
	}

	if !r.cnAreReady {
		return 0, false, errors.New("Redis PFCount Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := r.cnReader.PFCount(r.cnReader.Context(), key...)
	return r.handleIntCmd(cmd, "Redis PFCount Failed: ")
}

// PFMerge is a HyperLogLog function to merge two or more HyperLogLog as defined by keys together
func (r *Redis) PFMerge(destKey string, sourceKey ...string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PFMerge", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-PFMerge-DestKey", destKey)
			_ = seg.Seg.AddMetadata("Redis-PFMerge-SourceKeys", sourceKey)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = r.pfMergeInternal(destKey, sourceKey...)
		return err
	} else {
		return r.pfMergeInternal(destKey, sourceKey...)
	}
}

// pfMergeInternal is a HyperLogLog function to merge two or more HyperLogLog as defined by keys together
func (r *Redis) pfMergeInternal(destKey string, sourceKey ...string) error {
	// validate
	if len(destKey) <= 0 {
		return errors.New("Redis PFMerge Failed: " + "Destination Key is Required")
	}

	if len(sourceKey) <= 0 {
		return errors.New("Redis PFMerge Failed: " + "Source Key is Required")
	}

	if !r.cnAreReady {
		return errors.New("Redis PFMerge Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := r.cnWriter.PFMerge(r.cnWriter.Context(), destKey, sourceKey...)
	return r.handleStatusCmd(cmd, "Redis PFMerge Failed: ")
}

// ----------------------------------------------------------------------------------------------------------------
// Other functions
// ----------------------------------------------------------------------------------------------------------------

// Exists checks if one or more keys exists in redis
//
// foundCount = 0 indicates not found; > 0 indicates found count
func (r *Redis) Exists(key ...string) (foundCount int64, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Exists", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Exists-Keys", key)
			_ = seg.Seg.AddMetadata("Redis-Exists-Result-Count", foundCount)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		foundCount, err = r.existsInternal(key...)
		return foundCount, err
	} else {
		return r.existsInternal(key...)
	}
}

// existsInternal checks if one or more keys exists in redis
//
// foundCount = 0 indicates not found; > 0 indicates found count
func (r *Redis) existsInternal(key ...string) (foundCount int64, err error) {
	// validate
	if len(key) <= 0 {
		return 0, errors.New("Redis Exists Failed: " + "Key is Required")
	}

	if !r.cnAreReady {
		return 0, errors.New("Redis Exists Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := r.cnReader.Exists(r.cnReader.Context(), key...)
	foundCount, _, err = r.handleIntCmd(cmd, "Redis Exists Failed: ")

	return foundCount, err
}

// StrLen gets the string length of the value stored by the key in redis
func (r *Redis) StrLen(key string) (length int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-StrLen", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-StrLen-Key", key)
			_ = seg.Seg.AddMetadata("Redis-StrLen-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-StrLen-Result-Len", length)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		length, notFound, err = r.strLenInternal(key)
		return length, notFound, err
	} else {
		return r.strLenInternal(key)
	}
}

// strLenInternal gets the string length of the value stored by the key in redis
func (r *Redis) strLenInternal(key string) (length int64, notFound bool, err error) {
	// validate
	if len(key) <= 0 {
		return 0, false, errors.New("Redis StrLen Failed: " + "Key is Required")
	}

	if !r.cnAreReady {
		return 0, false, errors.New("Redis StrLen Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := r.cnReader.StrLen(r.cnReader.Context(), key)
	return r.handleIntCmd(cmd, "Redis StrLen Failed: ")
}

// Append will append a value to the existing value under the given key in redis,
// if key does not exist, a new key based on the given key is created
func (r *Redis) Append(key string, valToAppend string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Append", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Append-Key", key)
			_ = seg.Seg.AddMetadata("Redis-Append-Value", valToAppend)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = r.appendInternal(key, valToAppend)
		return err
	} else {
		return r.appendInternal(key, valToAppend)
	}
}

// appendInternal will append a value to the existing value under the given key in redis,
// if key does not exist, a new key based on the given key is created
func (r *Redis) appendInternal(key string, valToAppend string) error {
	// validate
	if len(key) <= 0 {
		return errors.New("Redis Append Failed: " + "Key is Required")
	}

	if !r.cnAreReady {
		return errors.New("Redis Append Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := r.cnWriter.Append(r.cnWriter.Context(), key, valToAppend)
	_, _, err := r.handleIntCmd(cmd)
	return err
}

// Del will delete one or more keys specified from redis
func (r *Redis) Del(key ...string) (deletedCount int64, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Del", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Del-Keys", key)
			_ = seg.Seg.AddMetadata("Redis-Del-Result-Deleted-Count", deletedCount)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		deletedCount, err = r.delInternal(key...)
		return deletedCount, err
	} else {
		return r.delInternal(key...)
	}
}

// delInternal will delete one or more keys specified from redis
func (r *Redis) delInternal(key ...string) (deletedCount int64, err error) {
	// validate
	if len(key) <= 0 {
		return 0, errors.New("Redis Del Failed: " + "Key is Required")
	}

	if !r.cnAreReady {
		return 0, errors.New("Redis Del Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := r.cnWriter.Del(r.cnWriter.Context(), key...)
	deletedCount, _, err = r.handleIntCmd(cmd, "Redis Del Failed: ")
	return deletedCount, err
}

// Unlink is similar to Del where it removes one or more keys specified from redis,
// however, unlink performs the delete asynchronously and is faster than Del
func (r *Redis) Unlink(key ...string) (unlinkedCount int64, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Unlink", r._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Unlink-Keys", key)
			_ = seg.Seg.AddMetadata("Redis-Unlink-Result-Unlinked-Count", unlinkedCount)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		unlinkedCount, err = r.unlinkInternal(key...)
		return unlinkedCount, err
	} else {
		return r.unlinkInternal(key...)
	}
}

// unlinkInternal is similar to Del where it removes one or more keys specified from redis,
// however, unlink performs the delete asynchronously and is faster than Del
func (r *Redis) unlinkInternal(key ...string) (unlinkedCount int64, err error) {
	// validate
	if len(key) <= 0 {
		return 0, errors.New("Redis Unlink Failed: " + "Key is Required")
	}

	if !r.cnAreReady {
		return 0, errors.New("Redis Unlink Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := r.cnWriter.Unlink(r.cnWriter.Context(), key...)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SetBit", b.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SetBit-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SetBit-Offset", offset)
			_ = seg.Seg.AddMetadata("Redis-SetBit-Bit-Value", bitValue)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = b.setBitInternal(key, offset, bitValue)
		return err
	} else {
		return b.setBitInternal(key, offset, bitValue)
	}
}

// setBitInternal will set or clear (1 or 0) the bit at offset in the string value stored by the key in redis,
// If the key doesn't exist, a new key with the key defined is created,
// The string holding bit value will grow as needed when offset exceeds the string, grown value defaults with bit 0
//
// bit range = left 0 -> right 8 = byte
func (b *BIT) setBitInternal(key string, offset int64, bitValue bool) error {
	// validate
	if b.core == nil {
		return errors.New("Redis SetBit Failed: " + "Base is Nil")
	}

	if !b.core.cnAreReady {
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

	cmd := b.core.cnWriter.SetBit(b.core.cnWriter.Context(), key, offset, v)
	_, _, err := b.core.handleIntCmd(cmd, "Redis SetBit Failed: ")
	return err
}

// GetBit will return the bit value (1 or 0) at offset position of the value for the key in redis
// If key is not found or offset is greater than key's value, then blank string is assumed and bit 0 is returned
func (b *BIT) GetBit(key string, offset int64) (val int, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GetBit", b.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-GetBit-Key", key)
			_ = seg.Seg.AddMetadata("Redis-GetBit-Offset", offset)
			_ = seg.Seg.AddMetadata("Redis-GetBit-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, err = b.getBitInternal(key, offset)
		return val, err
	} else {
		return b.getBitInternal(key, offset)
	}
}

// getBitInternal will return the bit value (1 or 0) at offset position of the value for the key in redis
// If key is not found or offset is greater than key's value, then blank string is assumed and bit 0 is returned
func (b *BIT) getBitInternal(key string, offset int64) (val int, err error) {
	// validate
	if b.core == nil {
		return 0, errors.New("Redis GetBit Failed: " + "Base is Nil")
	}

	if !b.core.cnAreReady {
		return 0, errors.New("Redis GetBit Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, errors.New("Redis GetBit Failed: " + "Key is Required")
	}

	if offset < 0 {
		return 0, errors.New("Redis GetBit Failed: " + "Offset is 0 or Greater")
	}

	cmd := b.core.cnReader.GetBit(b.core.cnReader.Context(), key, offset)
	v, _, e := b.core.handleIntCmd(cmd, "Redis GetBit Failed: ")
	val = int(v)
	return val, e
}

// BitCount counts the number of set bits (population counting of bits that are 1) in a string,
//
// offsetFrom = evaluate bitcount begin at offsetFrom position
// offsetTo = evaluate bitcount until offsetTo position
func (b *BIT) BitCount(key string, offsetFrom int64, offsetTo int64) (valCount int64, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-BitCount", b.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-BitCount-Key", key)
			_ = seg.Seg.AddMetadata("Redis-BitCount-Offset-From", offsetFrom)
			_ = seg.Seg.AddMetadata("Redis-BitCount-Offset-To", offsetTo)
			_ = seg.Seg.AddMetadata("Redis-BitCount-Result-Count", valCount)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valCount, err = b.bitCountInternal(key, offsetFrom, offsetTo)
		return valCount, err
	} else {
		return b.bitCountInternal(key, offsetFrom, offsetTo)
	}
}

// bitCountInternal counts the number of set bits (population counting of bits that are 1) in a string,
//
// offsetFrom = evaluate bitcount begin at offsetFrom position
// offsetTo = evaluate bitcount until offsetTo position
func (b *BIT) bitCountInternal(key string, offsetFrom int64, offsetTo int64) (valCount int64, err error) {
	// validate
	if b.core == nil {
		return 0, errors.New("Redis BitCount Failed: " + "Base is Nil")
	}

	if !b.core.cnAreReady {
		return 0, errors.New("Redis BitCount Failed: " + "Endpoint Connections Not Ready")
	}

	bc := new(redis.BitCount)

	bc.Start = offsetFrom
	bc.End = offsetTo

	cmd := b.core.cnReader.BitCount(b.core.cnReader.Context(), key, bc)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-BitField", b.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-BitField-Key", key)
			_ = seg.Seg.AddMetadata("Redis-BitField-Input-Args", args)
			_ = seg.Seg.AddMetadata("Redis-BitField-Input-Result", valBits)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valBits, err = b.bitFieldInternal(key, args...)
		return valBits, err
	} else {
		return b.bitFieldInternal(key, args...)
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
func (b *BIT) bitFieldInternal(key string, args ...interface{}) (valBits []int64, err error) {
	// validate
	if b.core == nil {
		return nil, errors.New("Redis BitField Failed: " + "Base is Nil")
	}

	if !b.core.cnAreReady {
		return nil, errors.New("Redis BitField Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, errors.New("Redis BitField Failed: " + "Key is Required")
	}

	if len(args) <= 0 {
		return nil, errors.New("Redis BitField Failed: " + "Args is Required")
	}

	cmd := b.core.cnWriter.BitField(b.core.cnWriter.Context(), key, args...)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-BitOp", b.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-BitOp-KeyDest", keyDest)
			_ = seg.Seg.AddMetadata("Redis-BitOp-OpType", bitOpType)
			_ = seg.Seg.AddMetadata("Redis-BitOp-KeySource", keySource)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = b.bitOpInternal(keyDest, bitOpType, keySource...)
		return err
	} else {
		return b.bitOpInternal(keyDest, bitOpType, keySource...)
	}
}

// bitOpInternal performs bitwise operation between multiple keys (containing string value),
// stores the result in the destination key,
// if operation failed, error is returned, if success, nil is returned
//
// Supported:
//
//	And, Or, XOr, Not
func (b *BIT) bitOpInternal(keyDest string, bitOpType redisbitop.RedisBitop, keySource ...string) error {
	// validate
	if b.core == nil {
		return errors.New("Redis BitOp Failed: " + "Base is Nil")
	}

	if !b.core.cnAreReady {
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
		cmd = b.core.cnWriter.BitOpAnd(b.core.cnWriter.Context(), keyDest, keySource...)
	case redisbitop.Or:
		cmd = b.core.cnWriter.BitOpOr(b.core.cnWriter.Context(), keyDest, keySource...)
	case redisbitop.XOr:
		cmd = b.core.cnWriter.BitOpXor(b.core.cnWriter.Context(), keyDest, keySource...)
	case redisbitop.NOT:
		cmd = b.core.cnWriter.BitOpNot(b.core.cnWriter.Context(), keyDest, keySource[0])
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-BitPos", b.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-BitPos-Key", key)
			_ = seg.Seg.AddMetadata("Redis-BitPos-BitValue", bitValue)
			_ = seg.Seg.AddMetadata("Redis-BitPos-Start-Position", startPosition)
			_ = seg.Seg.AddMetadata("Redis-BitPos-Result-Position", valPosition)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valPosition, err = b.bitPosInternal(key, bitValue, startPosition...)
		return valPosition, err
	} else {
		return b.bitPosInternal(key, bitValue, startPosition...)
	}
}

// bitPosInternal returns the position of the first bit set to 1 or 0 (as requested via input query) in a string,
// position of bit is returned from left to right,
// first byte most significant bit is 0 on left most,
// second byte most significant bit is at position 8 (after the first byte right most bit 7), and so on
//
// bitValue = 1 or 0
// startPosition = bit pos start from this bit offset position
func (b *BIT) bitPosInternal(key string, bitValue int64, startPosition ...int64) (valPosition int64, err error) {
	// validate
	if b.core == nil {
		return 0, errors.New("Redis BitPos Failed: " + "Base is Nil")
	}

	if !b.core.cnAreReady {
		return 0, errors.New("Redis BitPos Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, errors.New("Redis BitPos Failed: " + "Key is Required")
	}

	if bitValue != 0 && bitValue != 1 {
		return 0, errors.New("Redis BitPos Failed: " + "Bit Value Must Be 1 or 0")
	}

	cmd := b.core.cnReader.BitPos(b.core.cnReader.Context(), key, bitValue, startPosition...)
	valPosition, _, err = b.core.handleIntCmd(cmd, "Redis BitPos Failed: ")
	return valPosition, err
}

// ----------------------------------------------------------------------------------------------------------------
// LIST functions
// ----------------------------------------------------------------------------------------------------------------

// LSet will set element to the list index
func (l *LIST) LSet(key string, index int64, value interface{}) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LSet", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-LSet-Key", key)
			_ = seg.Seg.AddMetadata("Redis-LSet-Index", index)
			_ = seg.Seg.AddMetadata("Redis-LSet-value", value)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = l.lsetInternal(key, index, value)
		return err
	} else {
		return l.lsetInternal(key, index, value)
	}
}

// lsetInternal will set element to the list index
func (l *LIST) lsetInternal(key string, index int64, value interface{}) error {
	// validate
	if l.core == nil {
		return errors.New("Redis LSet Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
		return errors.New("Redis LSet Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis LSet Failed: " + "Key is Required")
	}

	if value == nil {
		return errors.New("Redis LSet Failed: " + "Value is Required")
	}

	cmd := l.core.cnWriter.LSet(l.core.cnWriter.Context(), key, index, value)
	return l.core.handleStatusCmd(cmd, "Redis LSet Failed: ")
}

// LInsert will insert a value either before or after the pivot element
func (l *LIST) LInsert(key string, bBefore bool, pivot interface{}, value interface{}) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LInsert", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-LInsert-Key", key)
			_ = seg.Seg.AddMetadata("Redis-LInsert-Insert-Before", bBefore)
			_ = seg.Seg.AddMetadata("Redis-LInsert-Pivot-Element", pivot)
			_ = seg.Seg.AddMetadata("Redis-LInsert-Insert-Value", value)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = l.linsertInternal(key, bBefore, pivot, value)
		return err
	} else {
		return l.linsertInternal(key, bBefore, pivot, value)
	}
}

// linsertInternal will insert a value either before or after the pivot element
func (l *LIST) linsertInternal(key string, bBefore bool, pivot interface{}, value interface{}) error {
	// validate
	if l.core == nil {
		return errors.New("Redis LInsert Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
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
		cmd = l.core.cnWriter.LInsertBefore(l.core.cnWriter.Context(), key, pivot, value)
	} else {
		cmd = l.core.cnWriter.LInsertAfter(l.core.cnWriter.Context(), key, pivot, value)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LPush", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-LPush-Key", key)
			_ = seg.Seg.AddMetadata("Redis-LPush-Key-Must-Exist", keyMustExist)
			_ = seg.Seg.AddMetadata("Redis-LPush-Values", value)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = l.lpushInternal(key, keyMustExist, value...)
		return err
	} else {
		return l.lpushInternal(key, keyMustExist, value...)
	}
}

// lpushInternal stores all the specified values at the head of the list as defined by the key,
// if key does not exist, then empty list is created before performing the push operation (unless keyMustExist bit is set)
//
// Elements are inserted one after the other to the head of the list, from the leftmost to the rightmost,
// for example, LPush mylist a b c will result in a list containing c as first element, b as second element, and a as third element
//
// error is returned if the key is not holding a value of type list
func (l *LIST) lpushInternal(key string, keyMustExist bool, value ...interface{}) error {
	// validate
	if l.core == nil {
		return errors.New("Redis LPush Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
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
		cmd = l.core.cnWriter.LPush(l.core.cnWriter.Context(), key, value...)
	} else {
		cmd = l.core.cnWriter.LPushX(l.core.cnWriter.Context(), key, value...)
	}

	_, _, err := l.core.handleIntCmd(cmd, "Redis LPush Failed: ")
	return err
}

// RPush stores all the specified values at the tail of the list as defined by the key,
// if key does not exist, then empty list is created before performing the push operation (unless keyMustExist bit is set)
//
// Elements are inserted one after the other to the tail of the list, from the leftmost to the rightmost,
// for example, RPush mylist a b c will result in a list containing a as first element, b as second element, and c as third element
//
// error is returned if the key is not holding a value of type list
func (l *LIST) RPush(key string, keyMustExist bool, value ...interface{}) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-RPush", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-RPush-Key", key)
			_ = seg.Seg.AddMetadata("Redis-RPush-Key-Must-Exist", keyMustExist)
			_ = seg.Seg.AddMetadata("Redis-RPush-Values", value)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = l.rpushInternal(key, keyMustExist, value...)
		return err
	} else {
		return l.rpushInternal(key, keyMustExist, value...)
	}
}

// rpushInternal stores all the specified values at the tail of the list as defined by the key,
// if key does not exist, then empty list is created before performing the push operation (unless keyMustExist bit is set)
//
// Elements are inserted one after the other to the tail of the list, from the leftmost to the rightmost,
// for example, RPush mylist a b c will result in a list containing a as first element, b as second element, and c as third element
//
// error is returned if the key is not holding a value of type list
func (l *LIST) rpushInternal(key string, keyMustExist bool, value ...interface{}) error {
	// validate
	if l.core == nil {
		return errors.New("Redis RPush Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
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
		cmd = l.core.cnWriter.RPush(l.core.cnWriter.Context(), key, value...)
	} else {
		cmd = l.core.cnWriter.RPushX(l.core.cnWriter.Context(), key, value...)
	}

	_, _, err := l.core.handleIntCmd(cmd, "Redis RPush Failed: ")
	return err
}

// LPop will remove and return the first element from the list stored at key
func (l *LIST) LPop(key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LPop", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-LPop-Key", key)
			_ = seg.Seg.AddMetadata("Redis-LPop-Output-Data-Type", outputDataType)
			_ = seg.Seg.AddMetadata("Redis-LPop-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-LPop-Output-Object", outputObjectPtr)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		notFound, err = l.lpopInternal(key, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return l.lpopInternal(key, outputDataType, outputObjectPtr)
	}
}

// lpopInternal will remove and return the first element from the list stored at key
func (l *LIST) lpopInternal(key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if l.core == nil {
		return false, errors.New("Redis LPop Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
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

	cmd := l.core.cnWriter.LPop(l.core.cnWriter.Context(), key)
	return l.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis LPop Failed: ")
}

// RPop removes and returns the last element of the list stored at key
func (l *LIST) RPop(key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-RPop", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-RPop-Key", key)
			_ = seg.Seg.AddMetadata("Redis-RPop-Output-Data-Type", outputDataType)
			_ = seg.Seg.AddMetadata("Redis-RPop-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-RPop-Output-Object", outputObjectPtr)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		notFound, err = l.rpopInternal(key, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return l.rpopInternal(key, outputDataType, outputObjectPtr)
	}
}

// rpopInternal removes and returns the last element of the list stored at key
func (l *LIST) rpopInternal(key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if l.core == nil {
		return false, errors.New("Redis RPop Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
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

	cmd := l.core.cnWriter.RPop(l.core.cnWriter.Context(), key)
	return l.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis RPop Failed: ")
}

// RPopLPush will atomically remove and return last element of the list stored at keySource,
// and then push the returned element at first element position (head) of the list stored at keyDest
func (l *LIST) RPopLPush(keySource string, keyDest string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-RPopLPush", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-RPopLPush-KeySource", keySource)
			_ = seg.Seg.AddMetadata("Redis-RPopLPush-KeyDest", keyDest)
			_ = seg.Seg.AddMetadata("Redis-RPopLPush-Output-Data-Type", outputDataType)
			_ = seg.Seg.AddMetadata("Redis-RPopLPush-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-RPopLPush-Output-Object", outputObjectPtr)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		notFound, err = l.rpopLPushInternal(keySource, keyDest, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return l.rpopLPushInternal(keySource, keyDest, outputDataType, outputObjectPtr)
	}
}

// rpopLPushInternal will atomically remove and return last element of the list stored at keySource,
// and then push the returned element at first element position (head) of the list stored at keyDest
func (l *LIST) rpopLPushInternal(keySource string, keyDest string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if l.core == nil {
		return false, errors.New("Redis RPopLPush Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
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

	cmd := l.core.cnWriter.RPopLPush(l.core.cnWriter.Context(), keySource, keyDest)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LIndex", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-LIndex-Key", key)
			_ = seg.Seg.AddMetadata("Redis-LIndex-Index", index)
			_ = seg.Seg.AddMetadata("Redis-LIndex-Output-Data-Type", outputDataType)
			_ = seg.Seg.AddMetadata("Redis-LIndex-Output-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-LIndex-Output-Object", outputObjectPtr)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		notFound, err = l.lindexInternal(key, index, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return l.lindexInternal(key, index, outputDataType, outputObjectPtr)
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
func (l *LIST) lindexInternal(key string, index int64, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if l.core == nil {
		return false, errors.New("Redis LIndex Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
		return false, errors.New("Redis LIndex Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return false, errors.New("Redis LIndex Failed: " + "Key is Required")
	}

	cmd := l.core.cnReader.LIndex(l.core.cnReader.Context(), key, index)
	return l.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis LIndex Failed: ")
}

// LLen returns the length of the list stored at key,
// if key does not exist, it is treated as empty list and 0 is returned,
//
// Error is returned if value at key is not a list
func (l *LIST) LLen(key string) (val int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LLen", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-LLen-Key", key)
			_ = seg.Seg.AddMetadata("Redis-LLen-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-LLen-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = l.llenInternal(key)
		return val, notFound, err
	} else {
		return l.llenInternal(key)
	}
}

// llenInternal returns the length of the list stored at key,
// if key does not exist, it is treated as empty list and 0 is returned,
//
// Error is returned if value at key is not a list
func (l *LIST) llenInternal(key string) (val int64, notFound bool, err error) {
	// validate
	if l.core == nil {
		return 0, false, errors.New("Redis LLen Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
		return 0, false, errors.New("Redis LLen Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis LLen Failed: " + "Key is Required")
	}

	cmd := l.core.cnReader.LLen(l.core.cnReader.Context(), key)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LRange", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-LRange-Key", key)
			_ = seg.Seg.AddMetadata("Redis-LRange-Start", start)
			_ = seg.Seg.AddMetadata("Redis-LRange-Stop", stop)
			_ = seg.Seg.AddMetadata("Redis-LRange-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-LRange-Result", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = l.lrangeInternal(key, start, stop)
		return outputSlice, notFound, err
	} else {
		return l.lrangeInternal(key, start, stop)
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
func (l *LIST) lrangeInternal(key string, start int64, stop int64) (outputSlice []string, notFound bool, err error) {
	// validate
	if l.core == nil {
		return nil, false, errors.New("Redis LRange Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
		return nil, false, errors.New("Redis LRange Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis LRange Failed: " + "Key is Required")
	}

	cmd := l.core.cnReader.LRange(l.core.cnReader.Context(), key, start, stop)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LRem", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-LRem-Key", key)
			_ = seg.Seg.AddMetadata("Redis-LRem-Count", count)
			_ = seg.Seg.AddMetadata("Redis-LRem-Value", value)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = l.lremInternal(key, count, value)
		return err
	} else {
		return l.lremInternal(key, count, value)
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
func (l *LIST) lremInternal(key string, count int64, value interface{}) error {
	// validate
	if l.core == nil {
		return errors.New("Redis LRem Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
		return errors.New("Redis LRem Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis LRem Failed: " + "Key is Required")
	}

	if value == nil {
		return errors.New("Redis LRem Failed: " + "Value is Required")
	}

	cmd := l.core.cnWriter.LRem(l.core.cnWriter.Context(), key, count, value)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LTrim", l.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-LTrim-Key", key)
			_ = seg.Seg.AddMetadata("Redis-LTrim-Start", start)
			_ = seg.Seg.AddMetadata("Redis-LTrim-Stop", stop)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = l.ltrimInternal(key, start, stop)
		return err
	} else {
		return l.ltrimInternal(key, start, stop)
	}
}

// ltrimInternal will trim an existing list so that it will contian only the specified range of elements specified,
// Both start and stop are zero-based indexes,
// Both start and stop can be negative, where -1 is the last element, while -2 is the second to last element
//
// Example:
//
//	LTRIM foobar 0 2 = modifies the list store at key named 'foobar' so that only the first 3 elements of the list will remain
func (l *LIST) ltrimInternal(key string, start int64, stop int64) error {
	// validate
	if l.core == nil {
		return errors.New("Redis LTrim Failed: " + "Base is Nil")
	}

	if !l.core.cnAreReady {
		return errors.New("Redis LTrim Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis LTrim Failed: " + "Key is Required")
	}

	cmd := l.core.cnWriter.LTrim(l.core.cnWriter.Context(), key, start, stop)
	return l.core.handleStatusCmd(cmd, "Redis LTrim Failed: ")
}

// ----------------------------------------------------------------------------------------------------------------
// HASH functions
// ----------------------------------------------------------------------------------------------------------------

// HExists returns if field is an existing field in the hash stored at key
//
// 1 = exists; 0 = not exist or key not exist
func (h *HASH) HExists(key string, field string) (valExists bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HExists", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HExists-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HExists-Field", field)
			_ = seg.Seg.AddMetadata("Redis-HExists-Result-Exists", valExists)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valExists, err = h.hexistsInternal(key, field)
		return valExists, err
	} else {
		return h.hexistsInternal(key, field)
	}
}

// hexistsInternal returns if field is an existing field in the hash stored at key
//
// 1 = exists; 0 = not exist or key not exist
func (h *HASH) hexistsInternal(key string, field string) (valExists bool, err error) {
	// validate
	if h.core == nil {
		return false, errors.New("Redis HExists Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
		return false, errors.New("Redis HExists Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return false, errors.New("Redis HExists Failed: " + "Key is Required")
	}

	if len(field) <= 0 {
		return false, errors.New("Redis HExists Failed: " + "Field is Required")
	}

	cmd := h.core.cnReader.HExists(h.core.cnReader.Context(), key, field)
	return h.core.handleBoolCmd(cmd, "Redis HExists Failed: ")
}

// HLen returns the number of fields contained in the hash stored at key
func (h *HASH) HLen(key string) (valLen int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HLen", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HLen-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HLen-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-HLen-Result-Length", valLen)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valLen, notFound, err = h.hlenInternal(key)
		return valLen, notFound, err
	} else {
		return h.hlenInternal(key)
	}
}

// hlenInternal returns the number of fields contained in the hash stored at key
func (h *HASH) hlenInternal(key string) (valLen int64, notFound bool, err error) {
	// validate
	if h.core == nil {
		return 0, false, errors.New("Redis HLen Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
		return 0, false, errors.New("Redis HLen Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis HLen Failed: " + "Key is Required")
	}

	cmd := h.core.cnReader.HLen(h.core.cnReader.Context(), key)
	return h.core.handleIntCmd(cmd, "Redis HLen Failed: ")
}

// HSet will set 'field' in hash stored at key to 'value',
// if key does not exist, a new key holding a hash is created,
//
// if 'field' already exists in the hash, it will be overridden
// if 'field' does not exist, it will be added
func (h *HASH) HSet(key string, value ...interface{}) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HSet", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HSet-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HSet-Values", value)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = h.hsetInternal(key, value...)
		return err
	} else {
		return h.hsetInternal(key, value...)
	}
}

// hsetInternal will set 'field' in hash stored at key to 'value',
// if key does not exist, a new key holding a hash is created,
//
// if 'field' already exists in the hash, it will be overridden
// if 'field' does not exist, it will be added
func (h *HASH) hsetInternal(key string, value ...interface{}) error {
	// validate
	if h.core == nil {
		return errors.New("Redis HSet Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
		return errors.New("Redis HSet Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis HSet Failed: " + "Key is Required")
	}

	if len(value) <= 0 {
		return errors.New("Redis HSet Failed: " + "At Least 1 Value is Required")
	}

	cmd := h.core.cnWriter.HSet(h.core.cnWriter.Context(), key, value...)
	return h.core.handleIntCmd2(cmd, "Redis HSet Failed: ")
}

// HSetNX will set 'field' in hash stored at key to 'value',
// if 'field' does not currently existing in hash
//
// note:
//
//	'field' must not yet exist in hash, otherwise will not add
func (h *HASH) HSetNX(key string, field string, value interface{}) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HSetNX", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HSetNX-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HSetNX-Field", field)
			_ = seg.Seg.AddMetadata("Redis-HSetNX-Value", value)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = h.hsetNXInternal(key, field, value)
		return err
	} else {
		return h.hsetNXInternal(key, field, value)
	}
}

// hsetNXInternal will set 'field' in hash stored at key to 'value',
// if 'field' does not currently existing in hash
//
// note:
//
//	'field' must not yet exist in hash, otherwise will not add
func (h *HASH) hsetNXInternal(key string, field string, value interface{}) error {
	// validate
	if h.core == nil {
		return errors.New("Redis HSetNX Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
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

	cmd := h.core.cnWriter.HSetNX(h.core.cnWriter.Context(), key, field, value)

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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HGet", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HGet-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HGet-Field", field)
			_ = seg.Seg.AddMetadata("Redis-HGet-Output-Data-Type", outputDataType)
			_ = seg.Seg.AddMetadata("Redis-HGet-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-HGet-Output-Object", outputObjectPtr)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		notFound, err = h.hgetInternal(key, field, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return h.hgetInternal(key, field, outputDataType, outputObjectPtr)
	}
}

// hgetInternal returns the value associated with 'field' in the hash stored at key
func (h *HASH) hgetInternal(key string, field string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if h.core == nil {
		return false, errors.New("Redis HGet Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
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

	cmd := h.core.cnReader.HGet(h.core.cnReader.Context(), key, field)
	return h.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis HGet Failed: ")
}

// HGetAll returns all fields and values of the hash store at key,
// in the returned value, every field name is followed by its value, so the length of the reply is twice the size of the hash
func (h *HASH) HGetAll(key string) (outputMap map[string]string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HGetAll", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HGetAll-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HGetAll-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-HGetAll-Result", outputMap)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputMap, notFound, err = h.hgetAllInternal(key)
		return outputMap, notFound, err
	} else {
		return h.hgetAllInternal(key)
	}
}

// hgetAllInternal returns all fields and values of the hash store at key,
// in the returned value, every field name is followed by its value, so the length of the reply is twice the size of the hash
func (h *HASH) hgetAllInternal(key string) (outputMap map[string]string, notFound bool, err error) {
	// validate
	if h.core == nil {
		return nil, false, errors.New("Redis HGetAll Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
		return nil, false, errors.New("Redis HGetAll Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis HGetAll Failed: " + "Key is Required")
	}

	cmd := h.core.cnReader.HGetAll(h.core.cnReader.Context(), key)
	return h.core.handleStringStringMapCmd(cmd, "Redis HGetAll Failed: ")
}

// HMSet will set the specified 'fields' to their respective values in the hash stored by key,
// This command overrides any specified 'fields' already existing in the hash,
// If key does not exist, a new key holding a hash is created
func (h *HASH) HMSet(key string, value ...interface{}) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HMSet", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HMSet-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HMSet-Values", value)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = h.hmsetInternal(key, value...)
		return err
	} else {
		return h.hmsetInternal(key, value...)
	}
}

// hmsetInternal will set the specified 'fields' to their respective values in the hash stored by key,
// This command overrides any specified 'fields' already existing in the hash,
// If key does not exist, a new key holding a hash is created
func (h *HASH) hmsetInternal(key string, value ...interface{}) error {
	// validate
	if h.core == nil {
		return errors.New("Redis HMSet Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
		return errors.New("Redis HMSet Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis HMSet Failed: " + "Key is Required")
	}

	if len(value) <= 0 {
		return errors.New("Redis HMSet Failed: " + "At Least 1 Value is Required")
	}

	cmd := h.core.cnWriter.HMSet(h.core.cnWriter.Context(), key, value...)

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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HMGet", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HMGet-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HMGet-Fields", field)
			_ = seg.Seg.AddMetadata("Redis-HMGet-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-HMGet-Result", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = h.hmgetInternal(key, field...)
		return outputSlice, notFound, err
	} else {
		return h.hmgetInternal(key, field...)
	}
}

// hmgetInternal will return the values associated with the specified 'fields' in the hash stored at key,
// For every 'field' that does not exist in the hash, a nil value is returned,
// If key is not existent, then nil is returned for all values
func (h *HASH) hmgetInternal(key string, field ...string) (outputSlice []interface{}, notFound bool, err error) {
	// validate
	if h.core == nil {
		return nil, false, errors.New("Redis HMGet Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
		return nil, false, errors.New("Redis HMGet Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis HMGet Failed: " + "Key is Required")
	}

	if len(field) <= 0 {
		return nil, false, errors.New("Redis HMGet Failed: " + "At Least 1 Field is Required")
	}

	cmd := h.core.cnReader.HMGet(h.core.cnReader.Context(), key, field...)
	return h.core.handleSliceCmd(cmd, "Redis HMGet Failed: ")
}

// HDel removes the specified 'fields' from the hash stored at key,
// any specified 'fields' that do not exist in the hash are ignored,
// if key does not exist, it is treated as an empty hash, and 0 is returned
func (h *HASH) HDel(key string, field ...string) (deletedCount int64, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HDel", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HDel-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HDel-Fields", field)
			_ = seg.Seg.AddMetadata("Redis-HDel-Result-Deleted-Count", deletedCount)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		deletedCount, err = h.hdelInternal(key, field...)
		return deletedCount, err
	} else {
		return h.hdelInternal(key, field...)
	}
}

// hdelInternal removes the specified 'fields' from the hash stored at key,
// any specified 'fields' that do not exist in the hash are ignored,
// if key does not exist, it is treated as an empty hash, and 0 is returned
func (h *HASH) hdelInternal(key string, field ...string) (deletedCount int64, err error) {
	// validate
	if h.core == nil {
		return 0, errors.New("Redis HDel Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
		return 0, errors.New("Redis HDel Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, errors.New("Redis HDel Failed: " + "Key is Required")
	}

	if len(field) <= 0 {
		return 0, errors.New("Redis HDel Failed: " + "At Least 1 Field is Required")
	}

	cmd := h.core.cnWriter.HDel(h.core.cnWriter.Context(), key, field...)
	deletedCount, _, err = h.core.handleIntCmd(cmd, "Redis HDel Failed: ")
	return deletedCount, err
}

// HKeys returns all field names in the hash stored at key,
// field names are the element keys
func (h *HASH) HKeys(key string) (outputSlice []string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HKeys", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HKeys-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HKeys-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-HKeys-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = h.hkeysInternal(key)
		return outputSlice, notFound, err
	} else {
		return h.hkeysInternal(key)
	}
}

// hkeysInternal returns all field names in the hash stored at key,
// field names are the element keys
func (h *HASH) hkeysInternal(key string) (outputSlice []string, notFound bool, err error) {
	// validate
	if h.core == nil {
		return nil, false, errors.New("Redis HKeys Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
		return nil, false, errors.New("Redis HKeys Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis HKeys Failed: " + "Key is Required")
	}

	cmd := h.core.cnReader.HKeys(h.core.cnReader.Context(), key)
	return h.core.handleStringSliceCmd(cmd, "Redis HKeys Failed: ")
}

// HVals returns all values in the hash stored at key
func (h *HASH) HVals(key string) (outputSlice []string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HVals", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HVals-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HVals-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-HVals-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = h.hvalsInternal(key)
		return outputSlice, notFound, err
	} else {
		return h.hvalsInternal(key)
	}
}

// hvalsInternal returns all values in the hash stored at key
func (h *HASH) hvalsInternal(key string) (outputSlice []string, notFound bool, err error) {
	// validate
	if h.core == nil {
		return nil, false, errors.New("Redis HVals Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
		return nil, false, errors.New("Redis HVals Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis HVals Failed: " + "Key is Required")
	}

	cmd := h.core.cnReader.HVals(h.core.cnReader.Context(), key)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HScan", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HScan-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HScan-Cursor", cursor)
			_ = seg.Seg.AddMetadata("Redis-HScan-Match", match)
			_ = seg.Seg.AddMetadata("Redis-HScan-Count", count)
			_ = seg.Seg.AddMetadata("Redis-HScan-Result-Keys", outputKeys)
			_ = seg.Seg.AddMetadata("Redis-HScan-Result-Cursor", outputCursor)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputKeys, outputCursor, err = h.hscanInternal(key, cursor, match, count)
		return outputKeys, outputCursor, err
	} else {
		return h.hscanInternal(key, cursor, match, count)
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
func (h *HASH) hscanInternal(key string, cursor uint64, match string, count int64) (outputKeys []string, outputCursor uint64, err error) {
	// validate
	if h.core == nil {
		return nil, 0, errors.New("Redis HScan Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
		return nil, 0, errors.New("Redis HScan Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, 0, errors.New("Redis HScan Failed: " + "Key is Required")
	}

	if len(match) <= 0 {
		return nil, 0, errors.New("Redis HScan Failed: " + "Match is Required")
	}

	if count < 0 {
		return nil, 0, errors.New("Redis HScan Failed: " + "Count Must Be Zero or Greater")
	}

	cmd := h.core.cnReader.HScan(h.core.cnReader.Context(), key, cursor, match, count)
	return h.core.handleScanCmd(cmd, "Redis HScan Failed: ")
}

// HIncrBy increments or decrements the number (int64) value at 'field' in the hash stored at key,
// if key does not exist, a new key holding a hash is created,
// if 'field' does not exist then the value is set to 0 before operation is performed
//
// this function supports both increment and decrement (although name of function is increment)
func (h *HASH) HIncrBy(key string, field string, incrValue int64) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HIncrBy", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HIncrBy-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HIncrBy-Field", field)
			_ = seg.Seg.AddMetadata("Redis-HIncrBy-Increment-Value", incrValue)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = h.hincrByInternal(key, field, incrValue)
		return err
	} else {
		return h.hincrByInternal(key, field, incrValue)
	}
}

// hincrByInternal increments or decrements the number (int64) value at 'field' in the hash stored at key,
// if key does not exist, a new key holding a hash is created,
// if 'field' does not exist then the value is set to 0 before operation is performed
//
// this function supports both increment and decrement (although name of function is increment)
func (h *HASH) hincrByInternal(key string, field string, incrValue int64) error {
	// validate
	if h.core == nil {
		return errors.New("Redis HIncrBy Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
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

	cmd := h.core.cnWriter.HIncrBy(h.core.cnWriter.Context(), key, field, incrValue)

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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-HIncrByFloat", h.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-HIncrByFloat-Key", key)
			_ = seg.Seg.AddMetadata("Redis-HIncrByFloat-Field", field)
			_ = seg.Seg.AddMetadata("Redis-HIncrByFloat-Increment-Value", incrValue)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = h.hincrByFloatInternal(key, field, incrValue)
		return err
	} else {
		return h.hincrByFloatInternal(key, field, incrValue)
	}
}

// hincrByFloatInternal increments or decrements the number (float64) value at 'field' in the hash stored at key,
// if key does not exist, a new key holding a hash is created,
// if 'field' does not exist then the value is set to 0 before operation is performed
//
// this function supports both increment and decrement (although name of function is increment)
func (h *HASH) hincrByFloatInternal(key string, field string, incrValue float64) error {
	// validate
	if h.core == nil {
		return errors.New("Redis HIncrByFloat Failed: " + "Base is Nil")
	}

	if !h.core.cnAreReady {
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

	cmd := h.core.cnWriter.HIncrByFloat(h.core.cnWriter.Context(), key, field, incrValue)

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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SAdd", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SAdd-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SAdd-Members", member)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = s.saddInternal(key, member...)
		return err
	} else {
		return s.saddInternal(key, member...)
	}
}

// saddInternal adds the specified members to the set stored at key,
// Specified members that are already a member of this set are ignored,
// If key does not exist, a new set is created before adding the specified members
//
// Error is returned when the value stored at key is not a set
func (s *SET) saddInternal(key string, member ...interface{}) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SAdd Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return errors.New("Redis SAdd Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis SAdd Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return errors.New("Redis SAdd Failed: " + "At Least 1 Member is Required")
	}

	cmd := s.core.cnWriter.SAdd(s.core.cnWriter.Context(), key, member...)
	return s.core.handleIntCmd2(cmd, "Redis SAdd Failed: ")
}

// SCard returns the set cardinality (number of elements) of the set stored at key
func (s *SET) SCard(key string) (val int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SCard", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SCard-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SCard-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-SCard-Result-Count", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = s.scardInternal(key)
		return val, notFound, err
	} else {
		return s.scardInternal(key)
	}
}

// scardInternal returns the set cardinality (number of elements) of the set stored at key
func (s *SET) scardInternal(key string) (val int64, notFound bool, err error) {
	// validate
	if s.core == nil {
		return 0, false, errors.New("Redis SCard Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return 0, false, errors.New("Redis SCard Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis SCard Failed: " + "Key is Required")
	}

	cmd := s.core.cnReader.SCard(s.core.cnReader.Context(), key)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SDiff", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SDiff-Keys", key)
			_ = seg.Seg.AddMetadata("Redis-SDiff-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-SDiff-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = s.sdiffInternal(key...)
		return outputSlice, notFound, err
	} else {
		return s.sdiffInternal(key...)
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
func (s *SET) sdiffInternal(key ...string) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SDiff Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return nil, false, errors.New("Redis SDiff Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 1 {
		return nil, false, errors.New("Redis SDiff Failed: " + "At Least 2 Keys Are Required")
	}

	cmd := s.core.cnReader.SDiff(s.core.cnReader.Context(), key...)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SDiffStore", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SDiffStore-KeyDest", keyDest)
			_ = seg.Seg.AddMetadata("Redis-SDiffStore-KeySources", keySource)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = s.sdiffStoreInternal(keyDest, keySource...)
		return err
	} else {
		return s.sdiffStoreInternal(keyDest, keySource...)
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
func (s *SET) sdiffStoreInternal(keyDest string, keySource ...string) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SDiffStore Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return errors.New("Redis SDiffStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis SDiffStore Failed: " + "Key Destination is Required")
	}

	if len(keySource) <= 1 {
		return errors.New("Redis SDiffStore Failed: " + "At Least 2 Key Sources are Required")
	}

	cmd := s.core.cnWriter.SDiffStore(s.core.cnWriter.Context(), keyDest, keySource...)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SInter", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SInter-Keys", key)
			_ = seg.Seg.AddMetadata("Redis-SInter-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-SInter-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = s.sinterInternal(key...)
		return outputSlice, notFound, err
	} else {
		return s.sinterInternal(key...)
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
func (s *SET) sinterInternal(key ...string) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SInter Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return nil, false, errors.New("Redis SInter Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 1 {
		return nil, false, errors.New("Redis SInter Failed: " + "At Least 2 Keys Are Required")
	}

	cmd := s.core.cnReader.SInter(s.core.cnReader.Context(), key...)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SInterStore", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SInterStore-KeyDest", keyDest)
			_ = seg.Seg.AddMetadata("Redis-SInterStore-KeySources", keySource)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = s.sinterStoreInternal(keyDest, keySource...)
		return err
	} else {
		return s.sinterStoreInternal(keyDest, keySource...)
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
func (s *SET) sinterStoreInternal(keyDest string, keySource ...string) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SInterStore Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return errors.New("Redis SInterStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis SInterStore Failed: " + "Key Destination is Required")
	}

	if len(keySource) <= 1 {
		return errors.New("Redis SInterStore Failed: " + "At Least 2 Key Sources are Required")
	}

	cmd := s.core.cnWriter.SInterStore(s.core.cnWriter.Context(), keyDest, keySource...)
	return s.core.handleIntCmd2(cmd, "Redis SInterStore Failed: ")
}

// SIsMember returns status if 'member' is a member of the set stored at key
func (s *SET) SIsMember(key string, member interface{}) (val bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SIsMember", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SIsMember-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SIsMember-Member", member)
			_ = seg.Seg.AddMetadata("Redis-SIsMember-Result-IsMember", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, err = s.sisMemberInternal(key, member)
		return val, err
	} else {
		return s.sisMemberInternal(key, member)
	}
}

// sisMemberInternal returns status if 'member' is a member of the set stored at key
func (s *SET) sisMemberInternal(key string, member interface{}) (val bool, err error) {
	// validate
	if s.core == nil {
		return false, errors.New("Redis SIsMember Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return false, errors.New("Redis SIsMember Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return false, errors.New("Redis SIsMember Failed: " + "Key is Required")
	}

	if member == nil {
		return false, errors.New("Redis SIsMember Failed: " + "Member is Required")
	}

	cmd := s.core.cnReader.SIsMember(s.core.cnReader.Context(), key, member)
	return s.core.handleBoolCmd(cmd, "Redis SIsMember Failed: ")
}

// SMembers returns all the members of the set value stored at key
func (s *SET) SMembers(key string) (outputSlice []string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SMembers", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SMember-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SMember-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-SMember-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = s.smembersInternal(key)
		return outputSlice, notFound, err
	} else {
		return s.smembersInternal(key)
	}
}

// smembersInternal returns all the members of the set value stored at key
func (s *SET) smembersInternal(key string) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SMembers Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return nil, false, errors.New("Redis SMembers Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis SMembers Failed: " + "Key is Required")
	}

	cmd := s.core.cnReader.SMembers(s.core.cnReader.Context(), key)
	return s.core.handleStringSliceCmd(cmd, "Redis SMember Failed: ")
}

// SMembersMap returns all the members of the set value stored at key, via map
func (s *SET) SMembersMap(key string) (outputMap map[string]struct{}, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SMembersMap", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SMembersMap-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SMembersMap-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-SMembersMap-Result", outputMap)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputMap, notFound, err = s.smembersMapInternal(key)
		return outputMap, notFound, err
	} else {
		return s.smembersMapInternal(key)
	}
}

// smembersMapInternal returns all the members of the set value stored at key, via map
func (s *SET) smembersMapInternal(key string) (outputMap map[string]struct{}, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SMembersMap Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return nil, false, errors.New("Redis SMembersMap Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis SMembersMap Failed: " + "Key is Required")
	}

	cmd := s.core.cnReader.SMembersMap(s.core.cnReader.Context(), key)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SScan", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SScan-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SScan-Cursor", cursor)
			_ = seg.Seg.AddMetadata("Redis-SScan-Match", match)
			_ = seg.Seg.AddMetadata("Redis-SScan-Count", count)
			_ = seg.Seg.AddMetadata("Redis-SScan-Result-Keys", outputKeys)
			_ = seg.Seg.AddMetadata("Redis-SScan-Result-Cursor", outputCursor)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputKeys, outputCursor, err = s.sscanInternal(key, cursor, match, count)
		return outputKeys, outputCursor, err
	} else {
		return s.sscanInternal(key, cursor, match, count)
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
func (s *SET) sscanInternal(key string, cursor uint64, match string, count int64) (outputKeys []string, outputCursor uint64, err error) {
	// validate
	if s.core == nil {
		return nil, 0, errors.New("Redis SScan Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return nil, 0, errors.New("Redis SScan Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, 0, errors.New("Redis SScan Failed: " + "Key is Required")
	}

	if len(match) <= 0 {
		return nil, 0, errors.New("Redis SScan Failed: " + "Match is Required")
	}

	if count < 0 {
		return nil, 0, errors.New("Redis SScan Failed: " + "Count Must Be 0 or Greater")
	}

	cmd := s.core.cnReader.SScan(s.core.cnReader.Context(), key, cursor, match, count)
	return s.core.handleScanCmd(cmd, "Redis SScan Failed: ")
}

// SRandMember returns a random element from the set value stored at key
func (s *SET) SRandMember(key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SRandMember", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SRandMember-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SRandMember-Output-Data-Type", outputDataType)
			_ = seg.Seg.AddMetadata("Redis-SRandMember-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-SRandMember-Output-Object", outputObjectPtr)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		notFound, err = s.srandMemberInternal(key, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return s.srandMemberInternal(key, outputDataType, outputObjectPtr)
	}
}

// srandMemberInternal returns a random element from the set value stored at key
func (s *SET) srandMemberInternal(key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if s.core == nil {
		return false, errors.New("Redis SRandMember Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
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

	cmd := s.core.cnReader.SRandMember(s.core.cnReader.Context(), key)
	return s.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis SRandMember Failed: ")
}

// SRandMemberN returns one or more random elements from the set value stored at key, with count indicating return limit
//
// count > 0 = returns an array of count distinct elements (non-repeating), up to the set elements size
// count < 0 = returns an array of count elements (may be repeating), and up to the count size (selected members may still be part of the subsequent selection process)
func (s *SET) SRandMemberN(key string, count int64) (outputSlice []string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SRandMemberN", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SRandMemberN-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SRandMemberN-Count", count)
			_ = seg.Seg.AddMetadata("Redis-SRandMemberN-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-SRandMemberN-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = s.srandMemberNInternal(key, count)
		return outputSlice, notFound, err
	} else {
		return s.srandMemberNInternal(key, count)
	}
}

// srandMemberNInternal returns one or more random elements from the set value stored at key, with count indicating return limit
//
// count > 0 = returns an array of count distinct elements (non-repeating), up to the set elements size
// count < 0 = returns an array of count elements (may be repeating), and up to the count size (selected members may still be part of the subsequent selection process)
func (s *SET) srandMemberNInternal(key string, count int64) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SRandMemberN Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return nil, false, errors.New("Redis SRandMemberN Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis SRandMemberN Failed: " + "Key is Required")
	}

	if count == 0 {
		return nil, false, errors.New("Redis SRandMemberN Failed: " + "Count Must Not Be Zero")
	}

	cmd := s.core.cnReader.SRandMemberN(s.core.cnReader.Context(), key, count)
	return s.core.handleStringSliceCmd(cmd, "Redis SRandMemberN Failed: ")
}

// SRem removes the specified members from the set stored at key,
// Specified members that are not a member of this set are ignored,
// If key does not exist, it is treated as an empty set and this command returns 0
//
// Error is returned if the value stored at key is not a set
func (s *SET) SRem(key string, member ...interface{}) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SRem", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SRem-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SRem-Members", member)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = s.sremInternal(key, member...)
		return err
	} else {
		return s.sremInternal(key, member...)
	}
}

// sremInternal removes the specified members from the set stored at key,
// Specified members that are not a member of this set are ignored,
// If key does not exist, it is treated as an empty set and this command returns 0
//
// Error is returned if the value stored at key is not a set
func (s *SET) sremInternal(key string, member ...interface{}) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SRem Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return errors.New("Redis SRem Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis SRem Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return errors.New("Redis SRem Failed: " + "At Least 1 Member is Required")
	}

	cmd := s.core.cnWriter.SRem(s.core.cnWriter.Context(), key, member...)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SMove", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SMove-KeySource", keySource)
			_ = seg.Seg.AddMetadata("Redis-SMove-KeyDest", keyDest)
			_ = seg.Seg.AddMetadata("Redis-SMove-Member", member)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = s.smoveInternal(keySource, keyDest, member)
		return err
	} else {
		return s.smoveInternal(keySource, keyDest, member)
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
func (s *SET) smoveInternal(keySource string, keyDest string, member interface{}) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SMove Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
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

	cmd := s.core.cnWriter.SMove(s.core.cnWriter.Context(), keySource, keyDest, member)

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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SPop", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SPop-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SPop-Output-Data-Type", outputDataType)
			_ = seg.Seg.AddMetadata("Redis-SPop-Output-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-SPop-Output-Object", outputObjectPtr)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		notFound, err = s.spopInternal(key, outputDataType, outputObjectPtr)
		return notFound, err
	} else {
		return s.spopInternal(key, outputDataType, outputObjectPtr)
	}
}

// spopInternal removes and returns one random element from the set value stored at key
func (s *SET) spopInternal(key string, outputDataType redisdatatype.RedisDataType, outputObjectPtr interface{}) (notFound bool, err error) {
	// validate
	if s.core == nil {
		return false, errors.New("Redis SPop Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
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

	cmd := s.core.cnWriter.SPop(s.core.cnWriter.Context(), key)
	return s.core.handleStringCmd(cmd, outputDataType, outputObjectPtr, "Redis SPop Failed: ")
}

// SPopN removes and returns one or more random element from the set value stored at key
//
// count > 0 = returns an array of count distinct elements (non-repeating), up to the set elements size
// count < 0 = returns an array of count elements (may be repeating), and up to the count size (selected members may still be part of the subsequent selection process)
func (s *SET) SPopN(key string, count int64) (outputSlice []string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SPopN", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SPopN-Key", key)
			_ = seg.Seg.AddMetadata("Redis-SPopN-Count", count)
			_ = seg.Seg.AddMetadata("Redis-SPopN-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-SPopN-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = s.spopNInternal(key, count)
		return outputSlice, notFound, err
	} else {
		return s.spopNInternal(key, count)
	}
}

// spopNInternal removes and returns one or more random element from the set value stored at key
//
// count > 0 = returns an array of count distinct elements (non-repeating), up to the set elements size
// count < 0 = returns an array of count elements (may be repeating), and up to the count size (selected members may still be part of the subsequent selection process)
func (s *SET) spopNInternal(key string, count int64) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SPopN Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return nil, false, errors.New("Redis SPopN Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis SPopN Failed: " + "Key is Required")
	}

	if count == 0 {
		return nil, false, errors.New("Redis SPopN Failed: " + "Count Must Not Be Zero")
	}

	cmd := s.core.cnWriter.SPopN(s.core.cnWriter.Context(), key, count)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SUnion", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SUnion-Keys", key)
			_ = seg.Seg.AddMetadata("Redis-SUnion-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-SUnion-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = s.sunionInternal(key...)
		return outputSlice, notFound, err
	} else {
		return s.sunionInternal(key...)
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
func (s *SET) sunionInternal(key ...string) (outputSlice []string, notFound bool, err error) {
	// validate
	if s.core == nil {
		return nil, false, errors.New("Redis SUnion Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return nil, false, errors.New("Redis SUnion Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 1 {
		return nil, false, errors.New("Redis SUnion Failed: " + "At Least 2 Keys Are Required")
	}

	cmd := s.core.cnReader.SUnion(s.core.cnReader.Context(), key...)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SUnionStore", s.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SUnionStore-KeyDest", keyDest)
			_ = seg.Seg.AddMetadata("Redis-SUnionStore-KeySources", keySource)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = s.sunionStoreInternal(keyDest, keySource...)
		return err
	} else {
		return s.sunionStoreInternal(keyDest, keySource...)
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
func (s *SET) sunionStoreInternal(keyDest string, keySource ...string) error {
	// validate
	if s.core == nil {
		return errors.New("Redis SUnionStore Failed: " + "Base is Nil")
	}

	if !s.core.cnAreReady {
		return errors.New("Redis SUnionStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis SUnionStore Failed: " + "Key Destination is Required")
	}

	if len(keySource) <= 1 {
		return errors.New("Redis SUnionStore Failed: " + "At Least 2 Key Sources are Required")
	}

	cmd := s.core.cnWriter.SUnionStore(s.core.cnWriter.Context(), keyDest, keySource...)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZAdd", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZAdd-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZAdd-Condition", setCondition)
			_ = seg.Seg.AddMetadata("Redis-ZAdd-Get-Changed", getChanged)
			_ = seg.Seg.AddMetadata("Redis-ZAdd-Member", member)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = z.zaddInternal(key, setCondition, getChanged, member...)
		return err
	} else {
		return z.zaddInternal(key, setCondition, getChanged, member...)
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
func (z *SORTED_SET) zaddInternal(key string, setCondition redissetcondition.RedisSetCondition, getChanged bool, member ...*redis.Z) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZAdd Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
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
			cmd = z.core.cnWriter.ZAdd(z.core.cnWriter.Context(), key, member...)
		} else {
			cmd = z.core.cnWriter.ZAddCh(z.core.cnWriter.Context(), key, member...)
		}
	case redissetcondition.SetIfNotExists:
		if !getChanged {
			cmd = z.core.cnWriter.ZAddNX(z.core.cnWriter.Context(), key, member...)
		} else {
			cmd = z.core.cnWriter.ZAddNXCh(z.core.cnWriter.Context(), key, member...)
		}
	case redissetcondition.SetIfExists:
		if !getChanged {
			cmd = z.core.cnWriter.ZAddXX(z.core.cnWriter.Context(), key, member...)
		} else {
			cmd = z.core.cnWriter.ZAddXXCh(z.core.cnWriter.Context(), key, member...)
		}
	default:
		return errors.New("Redis ZAdd Failed: " + "Set Condition is Required")
	}

	return z.core.handleIntCmd2(cmd, "Redis ZAdd Failed: ")
}

// ZCard returns the sorted set cardinality (number of elements) of the sorted set stored at key
func (z *SORTED_SET) ZCard(key string) (val int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZCard", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZCard-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZCard-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZCard-Not-Result-Count", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = z.zcardInternal(key)
		return val, notFound, err
	} else {
		return z.zcardInternal(key)
	}
}

// zcardInternal returns the sorted set cardinality (number of elements) of the sorted set stored at key
func (z *SORTED_SET) zcardInternal(key string) (val int64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZCard Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return 0, false, errors.New("Redis ZCard Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ZCard Failed: " + "Key is Required")
	}

	cmd := z.core.cnReader.ZCard(z.core.cnReader.Context(), key)
	return z.core.handleIntCmd(cmd, "Redis ZCard Failed: ")
}

// ZCount returns the number of elements in the sorted set at key with a score between min and max
func (z *SORTED_SET) ZCount(key string, min string, max string) (val int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZCount", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZCount-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZCount-Min", min)
			_ = seg.Seg.AddMetadata("Redis-ZCount-Max", max)
			_ = seg.Seg.AddMetadata("Redis-ZCount-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZCount-Result-Count", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = z.zcountInternal(key, min, max)
		return val, notFound, err
	} else {
		return z.zcountInternal(key, min, max)
	}
}

// zcountInternal returns the number of elements in the sorted set at key with a score between min and max
func (z *SORTED_SET) zcountInternal(key string, min string, max string) (val int64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZCount Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
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

	cmd := z.core.cnReader.ZCount(z.core.cnReader.Context(), key, min, max)
	return z.core.handleIntCmd(cmd, "Redis ZCount Failed: ")
}

// ZIncr will increment the score of member in sorted set at key
//
// Also support for ZIncrXX (member must exist), ZIncrNX (member must not exist)
func (z *SORTED_SET) ZIncr(key string, setCondition redissetcondition.RedisSetCondition, member *redis.Z) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZIncr", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZIncr-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZIncr-Condition", setCondition)
			_ = seg.Seg.AddMetadata("Redis-ZIncr-Member", member)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = z.zincrInternal(key, setCondition, member)
		return err
	} else {
		return z.zincrInternal(key, setCondition, member)
	}
}

// zincrInternal will increment the score of member in sorted set at key
//
// Also support for ZIncrXX (member must exist), ZIncrNX (member must not exist)
func (z *SORTED_SET) zincrInternal(key string, setCondition redissetcondition.RedisSetCondition, member *redis.Z) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZIncr Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
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
		cmd = z.core.cnWriter.ZIncr(z.core.cnWriter.Context(), key, member)
	case redissetcondition.SetIfNotExists:
		cmd = z.core.cnWriter.ZIncrNX(z.core.cnWriter.Context(), key, member)
	case redissetcondition.SetIfExists:
		cmd = z.core.cnWriter.ZIncrXX(z.core.cnWriter.Context(), key, member)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZIncrBy", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZIncrBy-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZIncrBy-Increment", increment)
			_ = seg.Seg.AddMetadata("Redis-ZIncrBy-Member", member)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = z.zincrByInternal(key, increment, member)
		return err
	} else {
		return z.zincrByInternal(key, increment, member)
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
func (z *SORTED_SET) zincrByInternal(key string, increment float64, member string) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZIncrBy Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
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

	cmd := z.core.cnWriter.ZIncrBy(z.core.cnWriter.Context(), key, increment, member)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZInterStore", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZInterStore-KeyDest", keyDest)
			_ = seg.Seg.AddMetadata("Redis-ZInterStore-Input-Args", store)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = z.zinterStoreInternal(keyDest, store)
		return err
	} else {
		return z.zinterStoreInternal(keyDest, store)
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
func (z *SORTED_SET) zinterStoreInternal(keyDest string, store *redis.ZStore) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZInterStore Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return errors.New("Redis ZInterStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis ZInterStore Failed: " + "Key Destination is Required")
	}

	if store == nil {
		return errors.New("Redis ZInterStore Failed: " + "Store is Required")
	}

	cmd := z.core.cnWriter.ZInterStore(z.core.cnWriter.Context(), keyDest, store)
	return z.core.handleIntCmd2(cmd, "Redis ZInterStore Failed: ")
}

// ZLexCount returns the number of elements in the sorted set at key, with a value between min and max
func (z *SORTED_SET) ZLexCount(key string, min string, max string) (val int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZLexCount", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZLexCount-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZLexCount-Min", min)
			_ = seg.Seg.AddMetadata("Redis-ZLexCount-Max", max)
			_ = seg.Seg.AddMetadata("Redis-ZLexCount-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZLexCount-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = z.zlexCountInternal(key, min, max)
		return val, notFound, err
	} else {
		return z.zlexCountInternal(key, min, max)
	}
}

// zlexCountInternal returns the number of elements in the sorted set at key, with a value between min and max
func (z *SORTED_SET) zlexCountInternal(key string, min string, max string) (val int64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZLexCount Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
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

	cmd := z.core.cnReader.ZLexCount(z.core.cnReader.Context(), key, min, max)
	return z.core.handleIntCmd(cmd, "Redis ZLexCount Failed: ")
}

// ZPopMax removes and returns up to the count of members with the highest scores in the sorted set stored at key,
// Specifying more count than members will not cause error, rather given back smaller result set,
// Returning elements ordered with highest score first, then subsequent and so on
func (z *SORTED_SET) ZPopMax(key string, count ...int64) (outputSlice []redis.Z, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZPopMax", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZPopMax-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZPopMax-Count", count)
			_ = seg.Seg.AddMetadata("Redis-ZPopMax-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZPopMax-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = z.zpopMaxInternal(key, count...)
		return outputSlice, notFound, err
	} else {
		return z.zpopMaxInternal(key, count...)
	}
}

// zpopMaxInternal removes and returns up to the count of members with the highest scores in the sorted set stored at key,
// Specifying more count than members will not cause error, rather given back smaller result set,
// Returning elements ordered with highest score first, then subsequent and so on
func (z *SORTED_SET) zpopMaxInternal(key string, count ...int64) (outputSlice []redis.Z, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZPopMax Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return nil, false, errors.New("Redis ZPopMax Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZPopMax Failed: " + "Key is Required")
	}

	var cmd *redis.ZSliceCmd

	if len(count) <= 0 {
		cmd = z.core.cnWriter.ZPopMax(z.core.cnWriter.Context(), key)
	} else {
		cmd = z.core.cnWriter.ZPopMax(z.core.cnWriter.Context(), key, count...)
	}

	return z.core.handleZSliceCmd(cmd, "Redis ZPopMax Failed: ")
}

// ZPopMin removes and returns up to the count of members with the lowest scores in the sorted set stored at key,
// Specifying more count than members will not cause error, rather given back smaller result set,
// Returning elements ordered with lowest score first, then subsequently higher score, and so on
func (z *SORTED_SET) ZPopMin(key string, count ...int64) (outputSlice []redis.Z, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZPopMin", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZPopMin-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZPopMin-Count", count)
			_ = seg.Seg.AddMetadata("Redis-ZPopMin-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZPopMin-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = z.zpopMinInternal(key, count...)
		return outputSlice, notFound, err
	} else {
		return z.zpopMinInternal(key, count...)
	}
}

// zpopMinInternal removes and returns up to the count of members with the lowest scores in the sorted set stored at key,
// Specifying more count than members will not cause error, rather given back smaller result set,
// Returning elements ordered with lowest score first, then subsequently higher score, and so on
func (z *SORTED_SET) zpopMinInternal(key string, count ...int64) (outputSlice []redis.Z, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZPopMin Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return nil, false, errors.New("Redis ZPopMin Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZPopMin Failed: " + "Key is Required")
	}

	var cmd *redis.ZSliceCmd

	if len(count) <= 0 {
		cmd = z.core.cnWriter.ZPopMin(z.core.cnWriter.Context(), key)
	} else {
		cmd = z.core.cnWriter.ZPopMin(z.core.cnWriter.Context(), key, count...)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRange", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRange-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRange-Start", start)
			_ = seg.Seg.AddMetadata("Redis-ZRange-Stop", stop)
			_ = seg.Seg.AddMetadata("Redis-ZRange-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZRange-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = z.zrangeInternal(key, start, stop)
		return outputSlice, notFound, err
	} else {
		return z.zrangeInternal(key, start, stop)
	}
}

// zrangeInternal returns the specified range of elements in the sorted set stored at key,
// The elements are considered to be ordered form lowest to the highest score,
// Lexicographical order is used for elements with equal score
//
// start and stop are both zero-based indexes,
// start and stop may be negative, where -1 is the last index, and -2 is the second to the last index,
// start and stop are inclusive range
func (z *SORTED_SET) zrangeInternal(key string, start int64, stop int64) (outputSlice []string, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRange Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return nil, false, errors.New("Redis ZRange Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRange Failed: " + "Key is Required")
	}

	cmd := z.core.cnReader.ZRange(z.core.cnReader.Context(), key, start, stop)
	return z.core.handleStringSliceCmd(cmd, "Redis ZRange Failed: ")
}

// ZRangeByLex returns all the elements in the sorted set at key with a value between min and max
func (z *SORTED_SET) ZRangeByLex(key string, opt *redis.ZRangeBy) (outputSlice []string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRangeByLex", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRangeByLex-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRangeByLex-Input-Args", opt)
			_ = seg.Seg.AddMetadata("Redis-ZRangeByLex-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZRangeByLex-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = z.zrangeByLexInternal(key, opt)
		return outputSlice, notFound, err
	} else {
		return z.zrangeByLexInternal(key, opt)
	}
}

// zrangeByLexInternal returns all the elements in the sorted set at key with a value between min and max
func (z *SORTED_SET) zrangeByLexInternal(key string, opt *redis.ZRangeBy) (outputSlice []string, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Key is Required")
	}

	if opt == nil {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Opt is Required")
	}

	cmd := z.core.cnReader.ZRangeByLex(z.core.cnReader.Context(), key, opt)
	return z.core.handleStringSliceCmd(cmd, "Redis ZRangeByLex Failed: ")
}

// ZRangeByScore returns all the elements in the sorted set at key with a score between min and max,
// Elements are considered to be ordered from low to high scores
func (z *SORTED_SET) ZRangeByScore(key string, opt *redis.ZRangeBy) (outputSlice []string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRangeByScore", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRangeByScore-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRangeByScore-Input-Args", opt)
			_ = seg.Seg.AddMetadata("Redis-ZRangeByScore-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZRangeByScore-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = z.zrangeByScoreInternal(key, opt)
		return outputSlice, notFound, err
	} else {
		return z.zrangeByScoreInternal(key, opt)
	}
}

// zrangeByScoreInternal returns all the elements in the sorted set at key with a score between min and max,
// Elements are considered to be ordered from low to high scores
func (z *SORTED_SET) zrangeByScoreInternal(key string, opt *redis.ZRangeBy) (outputSlice []string, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRangeByScore Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return nil, false, errors.New("Redis ZRangeByScore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRangeByScore Failed: " + "Key is Required")
	}

	if opt == nil {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Opt is Required")
	}

	cmd := z.core.cnReader.ZRangeByScore(z.core.cnReader.Context(), key, opt)
	return z.core.handleStringSliceCmd(cmd, "Redis ZRangeByScore Failed: ")
}

// ZRangeByScoreWithScores returns all the elements in the sorted set at key with a score between min and max,
// Elements are considered to be ordered from low to high scores
func (z *SORTED_SET) ZRangeByScoreWithScores(key string, opt *redis.ZRangeBy) (outputSlice []redis.Z, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRangeByScoreWithScores", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRangeByScoreWithScores-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRangeByScoreWithScores-Input-Args", opt)
			_ = seg.Seg.AddMetadata("Redis-ZRangeByScoreWithScores-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZRangeByScoreWithScores-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = z.zrangeByScoreWithScoresInternal(key, opt)
		return outputSlice, notFound, err
	} else {
		return z.zrangeByScoreWithScoresInternal(key, opt)
	}
}

// zrangeByScoreWithScoresInternal returns all the elements in the sorted set at key with a score between min and max,
// Elements are considered to be ordered from low to high scores
func (z *SORTED_SET) zrangeByScoreWithScoresInternal(key string, opt *redis.ZRangeBy) (outputSlice []redis.Z, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRangeByScoreWithScores Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return nil, false, errors.New("Redis ZRangeByScoreWithScores Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRangeByScoreWithScores Failed: " + "Key is Required")
	}

	if opt == nil {
		return nil, false, errors.New("Redis ZRangeByLex Failed: " + "Opt is Required")
	}

	cmd := z.core.cnReader.ZRangeByScoreWithScores(z.core.cnReader.Context(), key, opt)
	return z.core.handleZSliceCmd(cmd, "ZRangeByLex")
}

// ZRank returns the rank of member in the sorted set stored at key, with the scores ordered from low to high,
// The rank (or index) is zero-based, where lowest member is index 0 (or rank 0)
func (z *SORTED_SET) ZRank(key string, member string) (val int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRank", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRank-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRank-Member", member)
			_ = seg.Seg.AddMetadata("Redis-ZRank-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZRank-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = z.zrankInternal(key, member)
		return val, notFound, err
	} else {
		return z.zrankInternal(key, member)
	}
}

// zrankInternal returns the rank of member in the sorted set stored at key, with the scores ordered from low to high,
// The rank (or index) is zero-based, where lowest member is index 0 (or rank 0)
func (z *SORTED_SET) zrankInternal(key string, member string) (val int64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZRank Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return 0, false, errors.New("Redis ZRank Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ZRank Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return 0, false, errors.New("Redis ZRank Failed: " + "Member is Required")
	}

	cmd := z.core.cnReader.ZRank(z.core.cnReader.Context(), key, member)
	return z.core.handleIntCmd(cmd, "Redis ZRank Failed: ")
}

// ZRem removes the specified members from the stored set stored at key,
// Non-existing members are ignored
//
// Error is returned if the value at key is not a sorted set
func (z *SORTED_SET) ZRem(key string, member ...interface{}) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRem", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRem-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRem-Members", member)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = z.zremInternal(key, member...)
		return err
	} else {
		return z.zremInternal(key, member...)
	}
}

// zremInternal removes the specified members from the stored set stored at key,
// Non-existing members are ignored
//
// Error is returned if the value at key is not a sorted set
func (z *SORTED_SET) zremInternal(key string, member ...interface{}) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZRem Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return errors.New("Redis ZRem Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis ZRem Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return errors.New("Redis ZRem Failed: " + "Member is Required")
	}

	cmd := z.core.cnWriter.ZRem(z.core.cnWriter.Context(), key, member...)
	return z.core.handleIntCmd2(cmd, "Redis ZRem Failed: ")
}

// ZRemRangeByLex removes all elements in the sorted set stored at key, between the lexicographical range specified by min and max
func (z *SORTED_SET) ZRemRangeByLex(key string, min string, max string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRemRangeByLex", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRemRangeByLex-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRemRangeByLex-Min", min)
			_ = seg.Seg.AddMetadata("Redis-ZRemRangeByLex-Max", max)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = z.zremRangeByLexInternal(key, min, max)
		return err
	} else {
		return z.zremRangeByLexInternal(key, min, max)
	}
}

// zremRangeByLexInternal removes all elements in the sorted set stored at key, between the lexicographical range specified by min and max
func (z *SORTED_SET) zremRangeByLexInternal(key string, min string, max string) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZRemRangeByLex Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
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

	cmd := z.core.cnWriter.ZRemRangeByLex(z.core.cnWriter.Context(), key, min, max)
	return z.core.handleIntCmd2(cmd, "Redis ZRemRangeByLex Failed: ")
}

// ZRemRangeByScore removes all elements in the sorted set stored at key, with a score between min and max (inclusive)
func (z *SORTED_SET) ZRemRangeByScore(key string, min string, max string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRemRangeByScore", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRemRangeByScore-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRemRangeByScore-Min", min)
			_ = seg.Seg.AddMetadata("Redis-ZRemRangeByScore-Max", max)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = z.zremRangeByScoreInternal(key, min, max)
		return err
	} else {
		return z.zremRangeByScoreInternal(key, min, max)
	}
}

// zremRangeByScoreInternal removes all elements in the sorted set stored at key, with a score between min and max (inclusive)
func (z *SORTED_SET) zremRangeByScoreInternal(key string, min string, max string) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZRemRangeByScore Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
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

	cmd := z.core.cnWriter.ZRemRangeByScore(z.core.cnWriter.Context(), key, min, max)
	return z.core.handleIntCmd2(cmd, "Redis ZRemRangeByScore Failed: ")
}

// ZRemRangeByRank removes all elements in the sorted set stored at key, with rank between start and stop
//
// Both start and stop are zero-based,
// Both start and stop can be negative, where -1 is the element with highest score, -2 is the element with next to highest score, and so on
func (z *SORTED_SET) ZRemRangeByRank(key string, start int64, stop int64) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRemRangeByRank", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRemRangeByRank-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRemRangeByRank-Start", start)
			_ = seg.Seg.AddMetadata("Redis-ZRemRangeByRank-Stop", stop)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = z.zremRangeByRankInternal(key, start, stop)
		return err
	} else {
		return z.zremRangeByRankInternal(key, start, stop)
	}
}

// zremRangeByRankInternal removes all elements in the sorted set stored at key, with rank between start and stop
//
// Both start and stop are zero-based,
// Both start and stop can be negative, where -1 is the element with highest score, -2 is the element with next to highest score, and so on
func (z *SORTED_SET) zremRangeByRankInternal(key string, start int64, stop int64) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZRemRangeByRank Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return errors.New("Redis ZRemRangeByRank Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis ZRemRangeByRank Failed: " + "Key is Required")
	}

	cmd := z.core.cnWriter.ZRemRangeByRank(z.core.cnWriter.Context(), key, start, stop)
	return z.core.handleIntCmd2(cmd, "Redis ZRemRangeByRank Failed: ")
}

// ZRevRange returns the specified range of elements in the sorted set stored at key,
// With elements ordered from highest to the lowest score,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) ZRevRange(key string, start int64, stop int64) (outputSlice []string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRevRange", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRevRange-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRevRange-Start", start)
			_ = seg.Seg.AddMetadata("Redis-ZRevRange-Stop", stop)
			_ = seg.Seg.AddMetadata("Redis-ZRevRange-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZRevRange-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = z.zrevRangeInternal(key, start, stop)
		return outputSlice, notFound, err
	} else {
		return z.zrevRangeInternal(key, start, stop)
	}
}

// zrevRangeInternal returns the specified range of elements in the sorted set stored at key,
// With elements ordered from highest to the lowest score,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) zrevRangeInternal(key string, start int64, stop int64) (outputSlice []string, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRevRange Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return nil, false, errors.New("Redis ZRevRange Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRevRange Failed: " + "Key is Required")
	}

	cmd := z.core.cnReader.ZRevRange(z.core.cnReader.Context(), key, start, stop)
	return z.core.handleStringSliceCmd(cmd, "Redis ZRevRange Failed: ")
}

// ZRevRangeWithScores returns the specified range of elements (with scores) in the sorted set stored at key,
// With elements ordered from highest to the lowest score,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) ZRevRangeWithScores(key string, start int64, stop int64) (outputSlice []redis.Z, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRevRangeWithScores", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRevRangeWithScores-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRevRangeWithScores-Start", start)
			_ = seg.Seg.AddMetadata("Redis-ZRevRangeWithScores-Stop", stop)
			_ = seg.Seg.AddMetadata("Redis-ZRevRangeWithScores-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZRevRangeWithScores-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = z.zrevRangeWithScoresInternal(key, start, stop)
		return outputSlice, notFound, err
	} else {
		return z.zrevRangeWithScoresInternal(key, start, stop)
	}
}

// zrevRangeWithScoresInternal returns the specified range of elements (with scores) in the sorted set stored at key,
// With elements ordered from highest to the lowest score,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) zrevRangeWithScoresInternal(key string, start int64, stop int64) (outputSlice []redis.Z, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRevRangeWithScores Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return nil, false, errors.New("Redis ZRevRangeWithScores Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRevRangeWithScores Failed: " + "Key is Required")
	}

	cmd := z.core.cnReader.ZRevRangeWithScores(z.core.cnReader.Context(), key, start, stop)
	return z.core.handleZSliceCmd(cmd, "Redis ZRevRangeWithScores Failed: ")
}

// ZRevRangeByScoreWithScores returns all the elements (with scores) in the sorted set at key, with a score between max and min (inclusive),
// With elements ordered from highest to lowest scores,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) ZRevRangeByScoreWithScores(key string, opt *redis.ZRangeBy) (outputSlice []redis.Z, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRevRangeByScoreWithScores", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRevRangeByScoreWithScores-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRevRangeByScoreWithScores-Input-Args", opt)
			_ = seg.Seg.AddMetadata("Redis-ZRevRangeByScoreWithScores-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZRevRangeByScoreWithScores-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = z.zrevRangeByScoreWithScoresInternal(key, opt)
		return outputSlice, notFound, err
	} else {
		return z.zrevRangeByScoreWithScoresInternal(key, opt)
	}
}

// zrevRangeByScoreWithScoresInternal returns all the elements (with scores) in the sorted set at key, with a score between max and min (inclusive),
// With elements ordered from highest to lowest scores,
// Descending lexicographical order is used for elements with equal score
func (z *SORTED_SET) zrevRangeByScoreWithScoresInternal(key string, opt *redis.ZRangeBy) (outputSlice []redis.Z, notFound bool, err error) {
	// validate
	if z.core == nil {
		return nil, false, errors.New("Redis ZRevRangeByScoreWithScores Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return nil, false, errors.New("Redis ZRevRangeByScoreWithScores Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis ZRevRangeByScoreWithScores Failed: " + "Key is Required")
	}

	if opt == nil {
		return nil, false, errors.New("Redis ZRevRangeByScoreWithScores Failed: " + "Opt is Required")
	}

	cmd := z.core.cnReader.ZRevRangeByScoreWithScores(z.core.cnReader.Context(), key, opt)
	return z.core.handleZSliceCmd(cmd, "Redis ZRevRangeByScoreWithScores Failed: ")
}

// ZRevRank returns the rank of member in the sorted set stored at key, with the scores ordered from high to low,
// Rank (index) is ordered from high to low, and is zero-based, where 0 is the highest rank (index)
// ZRevRank is opposite of ZRank
func (z *SORTED_SET) ZRevRank(key string, member string) (val int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZRevRank", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZRevRank-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZRevRank-Member", member)
			_ = seg.Seg.AddMetadata("Redis-ZRevRank-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZRevRank-Result-Rank", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = z.zrevRankInternal(key, member)
		return val, notFound, err
	} else {
		return z.zrevRankInternal(key, member)
	}
}

// zrevRankInternal returns the rank of member in the sorted set stored at key, with the scores ordered from high to low,
// Rank (index) is ordered from high to low, and is zero-based, where 0 is the highest rank (index)
// ZRevRank is opposite of ZRank
func (z *SORTED_SET) zrevRankInternal(key string, member string) (val int64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZRevRank Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return 0, false, errors.New("Redis ZRevRank Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ZRevRank Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return 0, false, errors.New("Redis ZRevRank Failed: " + "Member is Required")
	}

	cmd := z.core.cnReader.ZRevRank(z.core.cnReader.Context(), key, member)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZScan", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZScan-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZScan-Cursor", cursor)
			_ = seg.Seg.AddMetadata("Redis-ZScan-Match", match)
			_ = seg.Seg.AddMetadata("Redis-ZScan-Count", count)
			_ = seg.Seg.AddMetadata("Redis-ZScan-Result-Keys", outputKeys)
			_ = seg.Seg.AddMetadata("Redis-ZScan-Result-Cursor", outputCursor)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputKeys, outputCursor, err = z.zscanInternal(key, cursor, match, count)
		return outputKeys, outputCursor, err
	} else {
		return z.zscanInternal(key, cursor, match, count)
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
func (z *SORTED_SET) zscanInternal(key string, cursor uint64, match string, count int64) (outputKeys []string, outputCursor uint64, err error) {
	// validate
	if z.core == nil {
		return nil, 0, errors.New("Redis ZScan Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return nil, 0, errors.New("Redis ZScan Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, 0, errors.New("Redis ZScan Failed: " + "Key is Required")
	}

	if len(match) <= 0 {
		return nil, 0, errors.New("Redis ZScan Failed: " + "Match is Required")
	}

	if count < 0 {
		return nil, 0, errors.New("Redis ZScan Failed: " + "Count Must Be Zero or Greater")
	}

	cmd := z.core.cnReader.ZScan(z.core.cnReader.Context(), key, cursor, match, count)
	return z.core.handleScanCmd(cmd, "Redis ZScan Failed: ")
}

// ZScore returns the score of member in the sorted set at key,
// if member is not existent in the sorted set, or key does not exist, nil is returned
func (z *SORTED_SET) ZScore(key string, member string) (val float64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZScore", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZScore-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ZScore-Member", member)
			_ = seg.Seg.AddMetadata("Redis-ZScore-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ZScore-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = z.zscoreInternal(key, member)
		return val, notFound, err
	} else {
		return z.zscoreInternal(key, member)
	}
}

// zscoreInternal returns the score of member in the sorted set at key,
// if member is not existent in the sorted set, or key does not exist, nil is returned
func (z *SORTED_SET) zscoreInternal(key string, member string) (val float64, notFound bool, err error) {
	// validate
	if z.core == nil {
		return 0, false, errors.New("Redis ZScore Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return 0, false, errors.New("Redis ZScore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ZScore Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return 0, false, errors.New("Redis ZScore Failed: " + "Member is Required")
	}

	cmd := z.core.cnReader.ZScore(z.core.cnReader.Context(), key, member)
	return z.core.handleFloatCmd(cmd, "Redis ZScore Failed: ")
}

// ZUnionStore computes the union of numKeys sorted set given by the specified keys,
// and stores the result in 'destination'
//
// numKeys (input keys) are required
func (z *SORTED_SET) ZUnionStore(keyDest string, store *redis.ZStore) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ZUnionStore", z.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ZUnionStore-KeyDest", keyDest)
			_ = seg.Seg.AddMetadata("Redis-ZUnionStore-Input-Keys", store)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = z.zunionStoreInternal(keyDest, store)
		return err
	} else {
		return z.zunionStoreInternal(keyDest, store)
	}
}

// zunionStoreInternal computes the union of numKeys sorted set given by the specified keys,
// and stores the result in 'destination'
//
// numKeys (input keys) are required
func (z *SORTED_SET) zunionStoreInternal(keyDest string, store *redis.ZStore) error {
	// validate
	if z.core == nil {
		return errors.New("Redis ZUnionStore Failed: " + "Base is Nil")
	}

	if !z.core.cnAreReady {
		return errors.New("Redis ZUnionStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyDest) <= 0 {
		return errors.New("Redis ZUnionStore Failed: " + "Key Destination is Required")
	}

	if store == nil {
		return errors.New("Redis ZUnionStore Failed: " + "Store is Required")
	}

	cmd := z.core.cnWriter.ZUnionStore(z.core.cnWriter.Context(), keyDest, store)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoAdd", g.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-GeoAdd-Key", key)
			_ = seg.Seg.AddMetadata("Redis-GeoAdd-Location", geoLocation)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = g.geoAddInternal(key, geoLocation)
		return err
	} else {
		return g.geoAddInternal(key, geoLocation)
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
func (g *GEO) geoAddInternal(key string, geoLocation *redis.GeoLocation) error {
	// validate
	if g.core == nil {
		return errors.New("Redis GeoAdd Failed: " + "Base is Nil")
	}

	if !g.core.cnAreReady {
		return errors.New("Redis GeoAdd Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis GeoAdd Failed: " + "Key is Required")
	}

	if geoLocation == nil {
		return errors.New("Redis GeoAdd Failed: " + "Geo Location is Required")
	}

	cmd := g.core.cnWriter.GeoAdd(g.core.cnWriter.Context(), key, geoLocation)
	_, _, err := g.core.handleIntCmd(cmd, "Redis GeoAdd Failed: ")
	return err
}

// GeoDist returns the distance between two members in the geospatial index represented by the sorted set
//
// unit = m (meters), km (kilometers), mi (miles), ft (feet)
func (g *GEO) GeoDist(key string, member1 string, member2 string, unit redisradiusunit.RedisRadiusUnit) (valDist float64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoDist", g.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-GeoDist-Key", key)
			_ = seg.Seg.AddMetadata("Redis-GeoDist-Member1", member1)
			_ = seg.Seg.AddMetadata("Redis-GeoDist-Member2", member2)
			_ = seg.Seg.AddMetadata("Redis-GeoDist-Radius-Unit", unit)
			_ = seg.Seg.AddMetadata("Redis-GeoDist-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-GeoDist-Result-Distance", valDist)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valDist, notFound, err = g.geoDistInternal(key, member1, member2, unit)
		return valDist, notFound, err
	} else {
		return g.geoDistInternal(key, member1, member2, unit)
	}
}

// geoDistInternal returns the distance between two members in the geospatial index represented by the sorted set
//
// unit = m (meters), km (kilometers), mi (miles), ft (feet)
func (g *GEO) geoDistInternal(key string, member1 string, member2 string, unit redisradiusunit.RedisRadiusUnit) (valDist float64, notFound bool, err error) {
	// validate
	if g.core == nil {
		return 0.00, false, errors.New("Redis GeoDist Failed: " + "Base is Nil")
	}

	if !g.core.cnAreReady {
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

	cmd := g.core.cnReader.GeoDist(g.core.cnReader.Context(), key, member1, member2, unit.Key())
	return g.core.handleFloatCmd(cmd, "Redis GeoDist Failed: ")
}

// GeoHash returns valid GeoHash string representing the position of one or more elements in a sorted set (added by GeoAdd)
// This function returns a STANDARD GEOHASH as described on geohash.org site
func (g *GEO) GeoHash(key string, member ...string) (geoHashSlice []string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoHash", g.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-GeoHash-Key", key)
			_ = seg.Seg.AddMetadata("Redis-GeoHash-Members", member)
			_ = seg.Seg.AddMetadata("Redis-GeoHash-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-GeoHash-Result-Positions", geoHashSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		geoHashSlice, notFound, err = g.geoHashInternal(key, member...)
		return geoHashSlice, notFound, err
	} else {
		return g.geoHashInternal(key, member...)
	}
}

// geoHashInternal returns valid GeoHash string representing the position of one or more elements in a sorted set (added by GeoAdd)
// This function returns a STANDARD GEOHASH as described on geohash.org site
func (g *GEO) geoHashInternal(key string, member ...string) (geoHashSlice []string, notFound bool, err error) {
	// validate
	if g.core == nil {
		return nil, false, errors.New("Redis GeoHash Failed: " + "Base is Nil")
	}

	if !g.core.cnAreReady {
		return nil, false, errors.New("Redis GeoHash Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis GeoHash Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return nil, false, errors.New("Redis GeoHash Failed: " + "At Least 1 Member is Required")
	}

	cmd := g.core.cnReader.GeoHash(g.core.cnReader.Context(), key, member...)
	return g.core.handleStringSliceCmd(cmd, "Redis GeoHash Failed: ")
}

// GeoPos returns the position (longitude and latitude) of all the specified members of the geospatial index represented by the sorted set at key
func (g *GEO) GeoPos(key string, member ...string) (cmd *redis.GeoPosCmd, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoPos", g.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-GeoPos-Key", key)
			_ = seg.Seg.AddMetadata("Redis-GeoPos-Members", member)
			_ = seg.Seg.AddMetadata("Redis-GeoPos-Result-Position", cmd)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		cmd, err = g.geoPosInternal(key, member...)
		return cmd, err
	} else {
		return g.geoPosInternal(key, member...)
	}
}

// geoPosInternal returns the position (longitude and latitude) of all the specified members of the geospatial index represented by the sorted set at key
func (g *GEO) geoPosInternal(key string, member ...string) (*redis.GeoPosCmd, error) {
	// validate
	if g.core == nil {
		return nil, errors.New("Redis GeoPos Failed: " + "Base is Nil")
	}

	if !g.core.cnAreReady {
		return nil, errors.New("Redis GeoPos Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, errors.New("Redis GeoPos Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return nil, errors.New("Redis GeoPos Failed: " + "At Least 1 Member is Required")
	}

	return g.core.cnReader.GeoPos(g.core.cnReader.Context(), key, member...), nil
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoRadius", g.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-GeoRadius-Key", key)
			_ = seg.Seg.AddMetadata("Redis-GeoRadius-Longitude", longitude)
			_ = seg.Seg.AddMetadata("Redis-GeoRadius-Latitude", latitude)
			_ = seg.Seg.AddMetadata("Redis-GeoRadius-Query", query)
			_ = seg.Seg.AddMetadata("Redis-GeoRadius-Result-Location", cmd)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		cmd, err = g.geoRadiusInternal(key, longitude, latitude, query)
		return cmd, err
	} else {
		return g.geoRadiusInternal(key, longitude, latitude, query)
	}
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
func (g *GEO) geoRadiusInternal(key string, longitude float64, latitude float64, query *redis.GeoRadiusQuery) (*redis.GeoLocationCmd, error) {
	// validate
	if g.core == nil {
		return nil, errors.New("Redis GeoRadius Failed: " + "Base is Nil")
	}

	if !g.core.cnAreReady {
		return nil, errors.New("Redis GeoRadius Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, errors.New("Redis GeoRadius Failed: " + "Key is Required")
	}

	if query == nil {
		return nil, errors.New("Redis GeoRadius Failed: " + "Query is Required")
	}

	// remove invalid query fields
	if util.LenTrim(query.Sort) > 0 {
		switch strings.ToUpper(query.Sort) {
		case "ASC":
			// valid
		case "DESC":
			// valid
		default:
			// not valid
			query.Sort = ""
		}
	}

	if util.LenTrim(query.Store) > 0 {
		// this function is read only
		query.Store = ""
	}

	if util.LenTrim(query.StoreDist) > 0 {
		// this function is read only
		query.StoreDist = ""
	}

	if util.LenTrim(query.Unit) > 0 {
		switch strings.ToUpper(query.Unit) {
		case "M":
		case "KM":
		case "MI":
		case "FT":
			// valid
		default:
			// not valid
			query.Unit = "mi"
		}
	}

	return g.core.cnReader.GeoRadius(g.core.cnReader.Context(), key, longitude, latitude, query), nil
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoRadiusStore", g.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-GeoRadiusStore-Key", key)
			_ = seg.Seg.AddMetadata("Redis-GeoRadiusStore-Longitude", longitude)
			_ = seg.Seg.AddMetadata("Redis-GeoRadiusStore-Latitude", latitude)
			_ = seg.Seg.AddMetadata("Redis-GeoRadiusStore-Query", query)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = g.geoRadiusStoreInternal(key, longitude, latitude, query)
		return err
	} else {
		return g.geoRadiusStoreInternal(key, longitude, latitude, query)
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
func (g *GEO) geoRadiusStoreInternal(key string, longitude float64, latitude float64, query *redis.GeoRadiusQuery) error {
	// validate
	if g.core == nil {
		return errors.New("Redis GeoRadiusStore Failed: " + "Base is Nil")
	}

	if !g.core.cnAreReady {
		return errors.New("Redis GeoRadiusStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis GeoRadiusStore Failed: " + "Key is Required")
	}

	if query == nil {
		return errors.New("Redis GeoRadiusStore Failed: " + "Query is Required")
	}

	// remove invalid query fields
	if util.LenTrim(query.Sort) > 0 {
		switch strings.ToUpper(query.Sort) {
		case "ASC":
			// valid
		case "DESC":
			// valid
		default:
			// not valid
			query.Sort = ""
		}
	}

	if util.LenTrim(query.Unit) > 0 {
		switch strings.ToUpper(query.Unit) {
		case "M":
		case "KM":
		case "MI":
		case "FT":
			// valid
		default:
			// not valid
			query.Unit = "mi"
		}
	}

	cmd := g.core.cnWriter.GeoRadiusStore(g.core.cnWriter.Context(), key, longitude, latitude, query)
	_, _, err := g.core.handleIntCmd(cmd, "Redis GeoRadiusStore Failed: ")
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoRadiusByMember", g.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-GeoRadiusByMember-Key", key)
			_ = seg.Seg.AddMetadata("Redis-GeoRadiusByMember-Member", member)
			_ = seg.Seg.AddMetadata("Redis-GeoRadiusByMember-Query", query)
			_ = seg.Seg.AddMetadata("Redis-GeoRadiusByMember-Result-Location", cmd)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		cmd, err = g.geoRadiusByMemberInternal(key, member, query)
		return cmd, err
	} else {
		return g.geoRadiusByMemberInternal(key, member, query)
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
func (g *GEO) geoRadiusByMemberInternal(key string, member string, query *redis.GeoRadiusQuery) (*redis.GeoLocationCmd, error) {
	// validate
	if g.core == nil {
		return nil, errors.New("Redis GeoRadiusByMember Failed: " + "Base is Nil")
	}

	if !g.core.cnAreReady {
		return nil, errors.New("Redis GeoRadiusByMember Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, errors.New("Redis GeoRadiusByMember Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return nil, errors.New("Redis GeoRadiusByMember Failed: " + "Member is Required")
	}

	if query == nil {
		return nil, errors.New("Redis GeoRadiusByMember Failed: " + "Query is Required")
	}

	// remove invalid query fields
	if util.LenTrim(query.Sort) > 0 {
		switch strings.ToUpper(query.Sort) {
		case "ASC":
			// valid
		case "DESC":
			// valid
		default:
			// not valid
			query.Sort = ""
		}
	}

	if util.LenTrim(query.Store) > 0 {
		// this function is read only
		query.Store = ""
	}

	if util.LenTrim(query.StoreDist) > 0 {
		// this function is read only
		query.StoreDist = ""
	}

	if util.LenTrim(query.Unit) > 0 {
		switch strings.ToUpper(query.Unit) {
		case "M":
		case "KM":
		case "MI":
		case "FT":
			// valid
		default:
			// not valid
			query.Unit = "mi"
		}
	}

	return g.core.cnReader.GeoRadiusByMember(g.core.cnReader.Context(), key, member, query), nil
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-GeoRadiusByMemberStore", g.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-GeoRadiusByMemberStore-Key", key)
			_ = seg.Seg.AddMetadata("Redis-GeoRadiusByMemberStore-Member", member)
			_ = seg.Seg.AddMetadata("Redis-GeoRadiusByMemberStore-Query", query)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = g.geoRadiusByMemberStoreInternal(key, member, query)
		return err
	} else {
		return g.geoRadiusByMemberStoreInternal(key, member, query)
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
func (g *GEO) geoRadiusByMemberStoreInternal(key string, member string, query *redis.GeoRadiusQuery) error {
	// validate
	if g.core == nil {
		return errors.New("Redis GeoRadiusByMemberStore Failed: " + "Base is Nil")
	}

	if !g.core.cnAreReady {
		return errors.New("Redis GeoRadiusByMemberStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis GeoRadiusByMemberStore Failed: " + "Key is Required")
	}

	if len(member) <= 0 {
		return errors.New("Redis GeoRadiusByMemberStore Failed: " + "Member is Required")
	}

	if query == nil {
		return errors.New("Redis GeoRadiusByMemberStore Failed: " + "Query is Required")
	}

	// remove invalid query fields
	if util.LenTrim(query.Sort) > 0 {
		switch strings.ToUpper(query.Sort) {
		case "ASC":
			// valid
		case "DESC":
			// valid
		default:
			// not valid
			query.Sort = ""
		}
	}

	if util.LenTrim(query.Unit) > 0 {
		switch strings.ToUpper(query.Unit) {
		case "M":
		case "KM":
		case "MI":
		case "FT":
			// valid
		default:
			// not valid
			query.Unit = "mi"
		}
	}

	cmd := g.core.cnWriter.GeoRadiusByMemberStore(g.core.cnWriter.Context(), key, member, query)
	_, _, err := g.core.handleIntCmd(cmd, "Redis GeoRadiusByMemberStore Failed: ")
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XAck", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XAck-Stream", stream)
			_ = seg.Seg.AddMetadata("Redis-XAck-Group", group)
			_ = seg.Seg.AddMetadata("Redis-XAck-IDs", id)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = x.xackInternal(stream, group, id...)
		return err
	} else {
		return x.xackInternal(stream, group, id...)
	}
}

// xackInternal removes one or multiple messages from the 'pending entries list (PEL)' of a stream consumer group
//
// # A message is pending, and as such stored inside the PEL, when it was delivered to some consumer
//
// Once a consumer successfully processes a message, it should call XAck to remove the message so it does not get processed again (and releases message from memory in redis)
func (x *STREAM) xackInternal(stream string, group string, id ...string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XAck Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
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

	cmd := x.core.cnWriter.XAck(x.core.cnWriter.Context(), stream, group, id...)
	return x.core.handleIntCmd2(cmd, "Redis XAck Failed: ")
}

// XAdd appends the specified stream entry to the stream at the specified key,
// If the key does not exist, as a side effect of running this command the key is created with a stream value
func (x *STREAM) XAdd(addArgs *redis.XAddArgs) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XAdd", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XAdd-Input-Args", addArgs)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = x.xaddInternal(addArgs)
		return err
	} else {
		return x.xaddInternal(addArgs)
	}
}

// xaddInternal appends the specified stream entry to the stream at the specified key,
// If the key does not exist, as a side effect of running this command the key is created with a stream value
func (x *STREAM) xaddInternal(addArgs *redis.XAddArgs) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XAdd Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return errors.New("Redis XAdd Failed: " + "Endpoint Connections Not Ready")
	}

	if addArgs == nil {
		return errors.New("Redis XAdd Failed: " + "AddArgs is Required")
	}

	cmd := x.core.cnWriter.XAdd(x.core.cnWriter.Context(), addArgs)

	if _, _, err := x.core.handleStringCmd2(cmd, "Redis XAdd Failed: "); err != nil {
		return err
	} else {
		return nil
	}
}

// XClaim in the context of stream consumer group, this function changes the ownership of a pending message,
// so that the new owner is the consumer specified as the command argument
func (x *STREAM) XClaim(claimArgs *redis.XClaimArgs) (valMessages []redis.XMessage, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XClaim", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XClaim-Input-Args", claimArgs)
			_ = seg.Seg.AddMetadata("Redis-XClaim-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-XClaim-Results", valMessages)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valMessages, notFound, err = x.xclaimInternal(claimArgs)
		return valMessages, notFound, err
	} else {
		return x.xclaimInternal(claimArgs)
	}
}

// xclaimInternal in the context of stream consumer group, this function changes the ownership of a pending message,
// so that the new owner is the consumer specified as the command argument
func (x *STREAM) xclaimInternal(claimArgs *redis.XClaimArgs) (valMessages []redis.XMessage, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XClaim Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return nil, false, errors.New("Redis XClaim Failed: " + "Endpoint Connections Not Ready")
	}

	if claimArgs == nil {
		return nil, false, errors.New("Redis XClaim Failed: " + "ClaimArgs is Required")
	}

	cmd := x.core.cnWriter.XClaim(x.core.cnWriter.Context(), claimArgs)
	return x.core.handleXMessageSliceCmd(cmd, "Redis XClaim Failed: ")
}

// XClaimJustID in the context of stream consumer group, this function changes the ownership of a pending message,
// so that the new owner is the consumer specified as the command argument
func (x *STREAM) XClaimJustID(claimArgs *redis.XClaimArgs) (outputSlice []string, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XClaimJustID", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XClaimJustID-Input-Args", claimArgs)
			_ = seg.Seg.AddMetadata("Redis-XClaimJustID-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-XClaimJustID-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = x.xclaimJustIDInternal(claimArgs)
		return outputSlice, notFound, err
	} else {
		return x.xclaimJustIDInternal(claimArgs)
	}
}

// xclaimJustIDInternal in the context of stream consumer group, this function changes the ownership of a pending message,
// so that the new owner is the consumer specified as the command argument
func (x *STREAM) xclaimJustIDInternal(claimArgs *redis.XClaimArgs) (outputSlice []string, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XClaim Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return nil, false, errors.New("Redis XClaim Failed: " + "Endpoint Connections Not Ready")
	}

	if claimArgs == nil {
		return nil, false, errors.New("Redis XClaim Failed: " + "ClaimArgs is Required")
	}

	cmd := x.core.cnWriter.XClaimJustID(x.core.cnWriter.Context(), claimArgs)
	return x.core.handleStringSliceCmd(cmd, "Redis XClaim Failed: ")
}

// XDel removes the specified entries from a stream
func (x *STREAM) XDel(stream string, id ...string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XDel", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XDel-Stream", stream)
			_ = seg.Seg.AddMetadata("Redis-XDel-IDs", id)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = x.xdelInternal(stream, id...)
		return err
	} else {
		return x.xdelInternal(stream, id...)
	}
}

// xdelInternal removes the specified entries from a stream
func (x *STREAM) xdelInternal(stream string, id ...string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XDel Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return errors.New("Redis XDel Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return errors.New("Redis XDel Failed: " + "Stream is Required")
	}

	if len(id) <= 0 {
		return errors.New("Redis XDel Failed: " + "At Least 1 ID is Required")
	}

	cmd := x.core.cnWriter.XDel(x.core.cnWriter.Context(), stream, id...)
	return x.core.handleIntCmd2(cmd, "Redis XDel Failed: ")
}

// XGroupCreate will create a new consumer group associated with a stream
func (x *STREAM) XGroupCreate(stream string, group string, start string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XGroupCreate", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XGroupCreate-Stream", stream)
			_ = seg.Seg.AddMetadata("Redis-XGroupCreate-Group", group)
			_ = seg.Seg.AddMetadata("Redis-XGroupCreate-Start", start)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = x.xgroupCreateInternal(stream, group, start)
		return err
	} else {
		return x.xgroupCreateInternal(stream, group, start)
	}
}

// xgroupCreateInternal will create a new consumer group associated with a stream
func (x *STREAM) xgroupCreateInternal(stream string, group string, start string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XGroupCreate Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
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

	cmd := x.core.cnWriter.XGroupCreate(x.core.cnWriter.Context(), stream, group, start)
	return x.core.handleStatusCmd(cmd, "Redis XGroupCreate Failed: ")
}

// XGroupCreateMkStream will create a new consumer group, and create a stream if stream doesn't exist
func (x *STREAM) XGroupCreateMkStream(stream string, group string, start string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XGroupCreateMkStream", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XGroupCreateMkStream-Stream", stream)
			_ = seg.Seg.AddMetadata("Redis-XGroupCreateMkStream-Group", group)
			_ = seg.Seg.AddMetadata("Redis-XGroupCreateMkStream-Start", start)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = x.xgroupCreateMkStreamInternal(stream, group, start)
		return err
	} else {
		return x.xgroupCreateMkStreamInternal(stream, group, start)
	}
}

// xgroupCreateMkStreamInternal will create a new consumer group, and create a stream if stream doesn't exist
func (x *STREAM) xgroupCreateMkStreamInternal(stream string, group string, start string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XGroupCreateMkStream Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
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

	cmd := x.core.cnWriter.XGroupCreateMkStream(x.core.cnWriter.Context(), stream, group, start)
	return x.core.handleStatusCmd(cmd, "Redis XGroupCreateMkStream Failed: ")
}

// XGroupDelConsumer removes a given consumer from a consumer group
func (x *STREAM) XGroupDelConsumer(stream string, group string, consumer string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XGroupDelConsumer", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XGroupDelConsumer-Stream", stream)
			_ = seg.Seg.AddMetadata("Redis-XGroupDelConsumer-Group", group)
			_ = seg.Seg.AddMetadata("Redis-XGroupDelConsumer-Consumer", consumer)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = x.xgroupDelConsumerInternal(stream, group, consumer)
		return err
	} else {
		return x.xgroupDelConsumerInternal(stream, group, consumer)
	}
}

// xgroupDelConsumerInternal removes a given consumer from a consumer group
func (x *STREAM) xgroupDelConsumerInternal(stream string, group string, consumer string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XGroupDelConsumer Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
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

	cmd := x.core.cnWriter.XGroupDelConsumer(x.core.cnWriter.Context(), stream, group, consumer)
	return x.core.handleIntCmd2(cmd, "Redis XGroupDelConsumer Failed: ")
}

// XGroupDestroy will destroy a consumer group even if there are active consumers and pending messages
func (x *STREAM) XGroupDestroy(stream string, group string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XGroupDestroy", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XGroupDestroy-Stream", stream)
			_ = seg.Seg.AddMetadata("Redis-XGroupDestroy-Group", group)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = x.xgroupDestroyInternal(stream, group)
		return err
	} else {
		return x.xgroupDestroyInternal(stream, group)
	}
}

// xgroupDestroyInternal will destroy a consumer group even if there are active consumers and pending messages
func (x *STREAM) xgroupDestroyInternal(stream string, group string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XGroupDestroy Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return errors.New("Redis XGroupDestroy Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return errors.New("Redis XGroupDestroy Failed: " + "Stream is Required")
	}

	if len(group) <= 0 {
		return errors.New("Redis XGroupDestroy Failed: " + "Group is Required")
	}

	cmd := x.core.cnWriter.XGroupDestroy(x.core.cnWriter.Context(), stream, group)
	return x.core.handleIntCmd2(cmd, "Redis XGroupDestroy Failed: ")
}

// XGroupSetID will set the next message to deliver,
// Normally the next ID is set when the consumer is created, as the last argument to XGroupCreate,
// However, using XGroupSetID resets the next message ID in case prior message needs to be reprocessed
func (x *STREAM) XGroupSetID(stream string, group string, start string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XGroupSetID", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XGroupSetID-Stream", stream)
			_ = seg.Seg.AddMetadata("Redis-XGroupSetID-Group", group)
			_ = seg.Seg.AddMetadata("Redis-XGroupSetID-Start", start)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = x.xgroupSetIDInternal(stream, group, start)
		return err
	} else {
		return x.xgroupSetIDInternal(stream, group, start)
	}
}

// xgroupSetIDInternal will set the next message to deliver,
// Normally the next ID is set when the consumer is created, as the last argument to XGroupCreate,
// However, using XGroupSetID resets the next message ID in case prior message needs to be reprocessed
func (x *STREAM) xgroupSetIDInternal(stream string, group string, start string) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XGroupSetID Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
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

	cmd := x.core.cnWriter.XGroupSetID(x.core.cnWriter.Context(), stream, group, start)
	return x.core.handleStatusCmd(cmd, "Redis XGroupSetID Failed: ")
}

// XInfoGroups retrieves different information about the streams, and associated consumer groups
func (x *STREAM) XInfoGroups(key string) (outputSlice []redis.XInfoGroup, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XInfoGroups", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XInfoGroups-Key", key)
			_ = seg.Seg.AddMetadata("Redis-XInfoGroups-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-XInfoGroups-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = x.xinfoGroupsInternal(key)
		return outputSlice, notFound, err
	} else {
		return x.xinfoGroupsInternal(key)
	}
}

// xinfoGroupsInternal retrieves different information about the streams, and associated consumer groups
func (x *STREAM) xinfoGroupsInternal(key string) (outputSlice []redis.XInfoGroup, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XInfoGroups Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return nil, false, errors.New("Redis XInfoGroups Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis XInfoGroups Failed: " + "Key is Required")
	}

	cmd := x.core.cnReader.XInfoGroups(x.core.cnReader.Context(), key)
	return x.core.handleXInfoGroupsCmd(cmd, "Redis XInfoGroups Failed: ")
}

// XLen returns the number of entries inside a stream
func (x *STREAM) XLen(stream string) (val int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XLen", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XLen-Stream", stream)
			_ = seg.Seg.AddMetadata("Redis-XLen-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-XLen-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = x.xlenInternal(stream)
		return val, notFound, err
	} else {
		return x.xlenInternal(stream)
	}
}

// xlenInternal returns the number of entries inside a stream
func (x *STREAM) xlenInternal(stream string) (val int64, notFound bool, err error) {
	// validate
	if x.core == nil {
		return 0, false, errors.New("Redis XLen Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return 0, false, errors.New("Redis XLen Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return 0, false, errors.New("Redis XLen Failed: " + "Stream is Required")
	}

	cmd := x.core.cnReader.XLen(x.core.cnReader.Context(), stream)
	return x.core.handleIntCmd(cmd, "Redis XLen Failed: ")
}

// XPending fetches data from a stream via a consumer group, and not acknowledging such data, its like creating pending entries
func (x *STREAM) XPending(stream string, group string) (val *redis.XPending, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XPending", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XPending-Stream", stream)
			_ = seg.Seg.AddMetadata("Redis-XPending-Group", group)
			_ = seg.Seg.AddMetadata("Redis-XPending-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-XPending-Results", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = x.xpendingInternal(stream, group)
		return val, notFound, err
	} else {
		return x.xpendingInternal(stream, group)
	}
}

// xpendingInternal fetches data from a stream via a consumer group, and not acknowledging such data, its like creating pending entries
func (x *STREAM) xpendingInternal(stream string, group string) (val *redis.XPending, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XPending Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return nil, false, errors.New("Redis XPending Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return nil, false, errors.New("Redis XPending Failed: " + "Stream is Required")
	}

	if len(group) <= 0 {
		return nil, false, errors.New("Redis XPending Failed: " + "Group is Required")
	}

	cmd := x.core.cnWriter.XPending(x.core.cnWriter.Context(), stream, group)
	return x.core.handleXPendingCmd(cmd, "Redis XPending Failed: ")
}

// XPendingExt fetches data from a stream via a consumer group, and not acknowledging such data, its like creating pending entries
func (x *STREAM) XPendingExt(pendingArgs *redis.XPendingExtArgs) (outputSlice []redis.XPendingExt, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XPendingExt", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XPendingExt-Input-Args", pendingArgs)
			_ = seg.Seg.AddMetadata("Redis-XPendingExt-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-XPendingExt-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = x.xpendingExtInternal(pendingArgs)
		return outputSlice, notFound, err
	} else {
		return x.xpendingExtInternal(pendingArgs)
	}
}

// xpendingExtInternal fetches data from a stream via a consumer group, and not acknowledging such data, its like creating pending entries
func (x *STREAM) xpendingExtInternal(pendingArgs *redis.XPendingExtArgs) (outputSlice []redis.XPendingExt, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XPendingExt Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return nil, false, errors.New("Redis XPendingExt Failed: " + "Endpoint Connections Not Ready")
	}

	if pendingArgs == nil {
		return nil, false, errors.New("Redis XPendingExt Failed: " + "PendingArgs is Required")
	}

	cmd := x.core.cnWriter.XPendingExt(x.core.cnWriter.Context(), pendingArgs)
	return x.core.handleXPendingExtCmd(cmd, "Redis XPendingExt Failed: ")
}

// XRange returns the stream entries matching a given range of IDs,
// Range is specified by a minimum and maximum ID,
// Ordering is lowest to highest
func (x *STREAM) XRange(stream string, start string, stop string, count ...int64) (outputSlice []redis.XMessage, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XRange", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XRange-Stream", stream)
			_ = seg.Seg.AddMetadata("Redis-XRange-Start", start)
			_ = seg.Seg.AddMetadata("Redis-XRange-Stop", stop)

			if len(count) > 0 {
				_ = seg.Seg.AddMetadata("Redis-XRange-Limit-Count", count[0])
			} else {
				_ = seg.Seg.AddMetadata("Redis-XRange-Limit-Count", "None")
			}

			_ = seg.Seg.AddMetadata("Redis-XRange-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-XRange-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = x.xrangeInternal(stream, start, stop, count...)
		return outputSlice, notFound, err
	} else {
		return x.xrangeInternal(stream, start, stop, count...)
	}
}

// xrangeInternal returns the stream entries matching a given range of IDs,
// Range is specified by a minimum and maximum ID,
// Ordering is lowest to highest
func (x *STREAM) xrangeInternal(stream string, start string, stop string, count ...int64) (outputSlice []redis.XMessage, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XRange Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
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
		cmd = x.core.cnReader.XRange(x.core.cnReader.Context(), stream, start, stop)
	} else {
		cmd = x.core.cnReader.XRangeN(x.core.cnReader.Context(), stream, start, stop, count[0])
	}

	return x.core.handleXMessageSliceCmd(cmd, "Redis XRange Failed: ")
}

// XRevRange returns the stream entries matching a given range of IDs,
// Range is specified by a maximum and minimum ID,
// Ordering is highest to lowest
func (x *STREAM) XRevRange(stream string, start string, stop string, count ...int64) (outputSlice []redis.XMessage, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XRevRange", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XRevRange-Stream", stream)
			_ = seg.Seg.AddMetadata("Redis-XRevRange-Start", start)
			_ = seg.Seg.AddMetadata("Redis-XRevRange-Stop", stop)

			if len(count) > 0 {
				_ = seg.Seg.AddMetadata("Redis-XRevRange-Limit-Count", count[0])
			} else {
				_ = seg.Seg.AddMetadata("Redis-XRevRange-Limit-Count", "None")
			}

			_ = seg.Seg.AddMetadata("Redis-XRevRange-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-XRevRange-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = x.xrevRangeInternal(stream, start, stop, count...)
		return outputSlice, notFound, err
	} else {
		return x.xrevRangeInternal(stream, start, stop, count...)
	}
}

// xrevRangeInternal returns the stream entries matching a given range of IDs,
// Range is specified by a maximum and minimum ID,
// Ordering is highest to lowest
func (x *STREAM) xrevRangeInternal(stream string, start string, stop string, count ...int64) (outputSlice []redis.XMessage, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XRevRange Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
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
		cmd = x.core.cnReader.XRevRange(x.core.cnReader.Context(), stream, start, stop)
	} else {
		cmd = x.core.cnReader.XRevRangeN(x.core.cnReader.Context(), stream, start, stop, count[0])
	}

	return x.core.handleXMessageSliceCmd(cmd, "Redis XRevRange Failed: ")
}

// XRead will read data from one or multiple streams,
// only returning entries with an ID greater than the last received ID reported by the caller
func (x *STREAM) XRead(readArgs *redis.XReadArgs) (outputSlice []redis.XStream, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XRead", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XRead-Input-Args", readArgs)
			_ = seg.Seg.AddMetadata("Redis-XRead-Input-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-XRead-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = x.xreadInternal(readArgs)
		return outputSlice, notFound, err
	} else {
		return x.xreadInternal(readArgs)
	}
}

// xreadInternal will read data from one or multiple streams,
// only returning entries with an ID greater than the last received ID reported by the caller
func (x *STREAM) xreadInternal(readArgs *redis.XReadArgs) (outputSlice []redis.XStream, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XRead Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return nil, false, errors.New("Redis XRead Failed: " + "Endpoint Connections Not Ready")
	}

	if readArgs == nil {
		return nil, false, errors.New("Redis XRead Failed: " + "ReadArgs is Required")
	}

	cmd := x.core.cnReader.XRead(x.core.cnReader.Context(), readArgs)
	return x.core.handleXStreamSliceCmd(cmd, "Redis XRead Failed: ")
}

// XReadStreams is a special version of XRead command for streams
func (x *STREAM) XReadStreams(stream ...string) (outputSlice []redis.XStream, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XReadStreams", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XReadStreams-Streams", stream)
			_ = seg.Seg.AddMetadata("Redis-XReadStreams-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-XReadStreams-Results", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = x.xreadStreamsInternal(stream...)
		return outputSlice, notFound, err
	} else {
		return x.xreadStreamsInternal(stream...)
	}
}

// xreadStreamsInternal is a special version of XRead command for streams
func (x *STREAM) xreadStreamsInternal(stream ...string) (outputSlice []redis.XStream, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XReadStreams Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return nil, false, errors.New("Redis XReadStreams Failed: " + "Endpoint Connections Not Ready")
	}

	if len(stream) <= 0 {
		return nil, false, errors.New("Redis XReadStreams Failed: " + "At Least 1 Stream is Required")
	}

	cmd := x.core.cnReader.XReadStreams(x.core.cnReader.Context(), stream...)
	return x.core.handleXStreamSliceCmd(cmd, "Redis XReadStream Failed: ")
}

// XReadGroup is a special version of XRead command with support for consumer groups
func (x *STREAM) XReadGroup(readGroupArgs *redis.XReadGroupArgs) (outputSlice []redis.XStream, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XReadGroup", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XReadGroup-ReadGroup", readGroupArgs)
			_ = seg.Seg.AddMetadata("Redis-XReadGroup-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-XReadGroup-Result", outputSlice)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		outputSlice, notFound, err = x.xreadGroupInternal(readGroupArgs)
		return outputSlice, notFound, err
	} else {
		return x.xreadGroupInternal(readGroupArgs)
	}
}

// xreadGroupInternal is a special version of XRead command with support for consumer groups
func (x *STREAM) xreadGroupInternal(readGroupArgs *redis.XReadGroupArgs) (outputSlice []redis.XStream, notFound bool, err error) {
	// validate
	if x.core == nil {
		return nil, false, errors.New("Redis XReadGroup Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return nil, false, errors.New("Redis XReadGroup Failed: " + "Endpoint Connections Not Ready")
	}

	if readGroupArgs == nil {
		return nil, false, errors.New("Redis XReadGroup Failed: " + "ReadGroupArgs is Required")
	}

	cmd := x.core.cnReader.XReadGroup(x.core.cnReader.Context(), readGroupArgs)
	return x.core.handleXStreamSliceCmd(cmd, "Redis XReadGroup Failed: ")
}

// XTrim trims the stream to a given number of items, evicting older items (items with lower IDs) if needed
func (x *STREAM) XTrim(key string, maxLen int64) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XTrim", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XTrim-Key", key)
			_ = seg.Seg.AddMetadata("Redis-XTrim-MaxLen", maxLen)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = x.xtrimInternal(key, maxLen)
		return err
	} else {
		return x.xtrimInternal(key, maxLen)
	}
}

// xtrimInternal trims the stream to a given number of items, evicting older items (items with lower IDs) if needed
func (x *STREAM) xtrimInternal(key string, maxLen int64) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XTrim Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return errors.New("Redis XTrim Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis XTrim Failed: " + "Key is Required")
	}

	if maxLen < 0 {
		return errors.New("Redis XTrim Failed: " + "MaxLen Must Not Be Negative")
	}

	cmd := x.core.cnWriter.XTrim(x.core.cnWriter.Context(), key, maxLen)
	return x.core.handleIntCmd2(cmd, "Redis XTrim Failed: ")
}

// XTrimApprox trims the stream to a given number of items, evicting older items (items with lower IDs) if needed
func (x *STREAM) XTrimApprox(key string, maxLen int64) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-XTrimApprox", x.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-XTrimApprox-Key", key)
			_ = seg.Seg.AddMetadata("Redis-XTrimApprox-MaxLen", maxLen)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = x.xtrimApproxInternal(key, maxLen)
		return err
	} else {
		return x.xtrimApproxInternal(key, maxLen)
	}
}

// xtrimApproxInternal trims the stream to a given number of items, evicting older items (items with lower IDs) if needed
func (x *STREAM) xtrimApproxInternal(key string, maxLen int64) error {
	// validate
	if x.core == nil {
		return errors.New("Redis XTrimApprox Failed: " + "Base is Nil")
	}

	if !x.core.cnAreReady {
		return errors.New("Redis XTrimApprox Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis XTrimApprox Failed: " + "Key is Required")
	}

	if maxLen < 0 {
		return errors.New("Redis XTrimApprox Failed: " + "MaxLen Must Not Be Negative")
	}

	cmd := x.core.cnWriter.XTrimApprox(x.core.cnWriter.Context(), key, maxLen)
	return x.core.handleIntCmd2(cmd, "Redis XTrimApprox Failed: ")
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PSubscribe", ps.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-PSubscribe-Channels", channel)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		psObj, err = ps.psubscribeInternal(channel...)
		return psObj, err
	} else {
		return ps.psubscribeInternal(channel...)
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
func (ps *PUBSUB) psubscribeInternal(channel ...string) (*redis.PubSub, error) {
	// validate
	if ps.core == nil {
		return nil, errors.New("Redis PSubscribe Failed: " + "Base is Nil")
	}

	if !ps.core.cnAreReady {
		return nil, errors.New("Redis PSubscribe Failed: " + "Endpoint Connections Not Ready")
	}

	if len(channel) <= 0 {
		return nil, errors.New("Redis PSubscribe Failed: " + "At Least 1 Channel is Required")
	}

	return ps.core.cnWriter.PSubscribe(ps.core.cnWriter.Context(), channel...), nil
}

// Subscribe (Non-Pattern Subscribe) will subscribe client to the given channels,
// a pointer to redis PubSub object is returned upon successful subscribe
//
// Once client is subscribed, do not call other redis actions, other than Subscribe, PSubscribe, Ping, Unsubscribe, PUnsubscribe, and Quit (Per Redis Doc)
func (ps *PUBSUB) Subscribe(channel ...string) (psObj *redis.PubSub, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Subscribe", ps.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Subscribe-Channels", channel)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		psObj, err = ps.subscribeInternal(channel...)
		return psObj, err
	} else {
		return ps.subscribeInternal(channel...)
	}
}

// subscribeInternal (Non-Pattern Subscribe) will subscribe client to the given channels,
// a pointer to redis PubSub object is returned upon successful subscribe
//
// Once client is subscribed, do not call other redis actions, other than Subscribe, PSubscribe, Ping, Unsubscribe, PUnsubscribe, and Quit (Per Redis Doc)
func (ps *PUBSUB) subscribeInternal(channel ...string) (*redis.PubSub, error) {
	// validate
	if ps.core == nil {
		return nil, errors.New("Redis Subscribe Failed: " + "Base is Nil")
	}

	if !ps.core.cnAreReady {
		return nil, errors.New("Redis Subscribe Failed: " + "Endpoint Connections Not Ready")
	}

	if len(channel) <= 0 {
		return nil, errors.New("Redis Subscribe Failed: " + "At Least 1 Channel is Required")
	}

	return ps.core.cnWriter.Subscribe(ps.core.cnWriter.Context(), channel...), nil
}

// Publish will post a message to a given channel,
// returns number of clients that received the message
func (ps *PUBSUB) Publish(channel string, message interface{}) (valReceived int64, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Publish", ps.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Publish-Channel", channel)
			_ = seg.Seg.AddMetadata("Redis-Publish-Message", message)
			_ = seg.Seg.AddMetadata("Redis-Publish-Received-Clients-Count", valReceived)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valReceived, err = ps.publishInternal(channel, message)
		return valReceived, err
	} else {
		return ps.publishInternal(channel, message)
	}
}

// publishInternal will post a message to a given channel,
// returns number of clients that received the message
func (ps *PUBSUB) publishInternal(channel string, message interface{}) (valReceived int64, err error) {
	// validate
	if ps.core == nil {
		return 0, errors.New("Redis Publish Failed: " + "Base is Nil")
	}

	if !ps.core.cnAreReady {
		return 0, errors.New("Redis Publish Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := ps.core.cnWriter.Publish(ps.core.cnWriter.Context(), channel, message)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PubSubChannels", ps.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-PubSubChannels-Pattern", pattern)
			_ = seg.Seg.AddMetadata("Redis-PubSubChannels-Result", valChannels)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valChannels, err = ps.pubSubChannelsInternal(pattern)
		return valChannels, err
	} else {
		return ps.pubSubChannelsInternal(pattern)
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
func (ps *PUBSUB) pubSubChannelsInternal(pattern string) (valChannels []string, err error) {
	// validate
	if ps.core == nil {
		return nil, errors.New("Redis PubSubChannels Failed: " + "Base is Nil")
	}

	if !ps.core.cnAreReady {
		return nil, errors.New("Redis PubSubChannels Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := ps.core.cnReader.PubSubChannels(ps.core.cnReader.Context(), pattern)
	valChannels, _, err = ps.core.handleStringSliceCmd(cmd, "Redis PubSubChannels Failed: ")
	return valChannels, err
}

// PubSubNumPat (Pub/Sub Number of Patterns) returns the number of subscriptions to patterns (that were using PSubscribe Command),
// This counts both clients subscribed to patterns, and also total number of patterns all the clients are subscribed to
func (ps *PUBSUB) PubSubNumPat() (valPatterns int64, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PubSubNumPat", ps.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-PubSubNumPat-Result", valPatterns)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valPatterns, err = ps.pubSubNumPatInternal()
		return valPatterns, err
	} else {
		return ps.pubSubNumPatInternal()
	}
}

// pubSubNumPatInternal (Pub/Sub Number of Patterns) returns the number of subscriptions to patterns (that were using PSubscribe Command),
// This counts both clients subscribed to patterns, and also total number of patterns all the clients are subscribed to
func (ps *PUBSUB) pubSubNumPatInternal() (valPatterns int64, err error) {
	// validate
	if ps.core == nil {
		return 0, errors.New("Redis PubSubNumPat Failed: " + "Base is Nil")
	}

	if !ps.core.cnAreReady {
		return 0, errors.New("Redis PubSubNumPat Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := ps.core.cnReader.PubSubNumPat(ps.core.cnReader.Context())
	valPatterns, _, err = ps.core.handleIntCmd(cmd, "Redis PubSubNumPat Failed: ")
	return valPatterns, err
}

// PubSubNumSub (Pub/Sub Number of Subscribers) returns number of subscribers (not counting clients subscribed to patterns) for the specific channels
func (ps *PUBSUB) PubSubNumSub(channel ...string) (val map[string]int64, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-PubSubNumSub", ps.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-PubSubNumSub-Channels", channel)
			_ = seg.Seg.AddMetadata("Redis-PubSubNumSub-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, err = ps.pubSubNumSubInternal(channel...)
		return val, err
	} else {
		return ps.pubSubNumSubInternal(channel...)
	}
}

// pubSubNumSubInternal (Pub/Sub Number of Subscribers) returns number of subscribers (not counting clients subscribed to patterns) for the specific channels
func (ps *PUBSUB) pubSubNumSubInternal(channel ...string) (val map[string]int64, err error) {
	// validate
	if ps.core == nil {
		return nil, errors.New("Redis PubSubNumSub Failed: " + "Base is Nil")
	}

	if !ps.core.cnAreReady {
		return nil, errors.New("Redis PubSubNumSub Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := ps.core.cnReader.PubSubNumSub(ps.core.cnReader.Context(), channel...)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Pipeline", p.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Pipeline-Result", result)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		result, err = p.pipelineInternal()
		return result, err
	} else {
		return p.pipelineInternal()
	}
}

// pipelineInternal allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) pipelineInternal() (redis.Pipeliner, error) {
	// validate
	if p.core == nil {
		return nil, errors.New("Redis Pipeline Failed: " + "Base is Nil")
	}

	if !p.core.cnAreReady {
		return nil, errors.New("Redis Pipeline Failed: " + "Endpoint Connections Not Ready")
	}

	return p.core.cnWriter.Pipeline(), nil
}

// Pipelined allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) Pipelined(fn func(redis.Pipeliner) error) (result []redis.Cmder, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Pipelined", p.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Pipelined-Result", result)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		result, err = p.pipelinedInternal(fn)
		return result, err
	} else {
		return p.pipelinedInternal(fn)
	}
}

// pipelinedInternal allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) pipelinedInternal(fn func(redis.Pipeliner) error) ([]redis.Cmder, error) {
	// validate
	if p.core == nil {
		return nil, errors.New("Redis Pipelined Failed: " + "Base is Nil")
	}

	if !p.core.cnAreReady {
		return nil, errors.New("Redis Pipelined Failed: " + "Endpoint Connections Not Ready")
	}

	return p.core.cnWriter.Pipelined(p.core.cnWriter.Context(), fn)
}

// TxPipeline allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) TxPipeline() (result redis.Pipeliner, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-TxPipeline", p.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-TxPipeline-Result", result)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		result, err = p.txPipelineInternal()
		return result, err
	} else {
		return p.txPipelineInternal()
	}
}

// txPipelineInternal allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) txPipelineInternal() (redis.Pipeliner, error) {
	// validate
	if p.core == nil {
		return nil, errors.New("Redis TxPipeline Failed: " + "Base is Nil")
	}

	if !p.core.cnAreReady {
		return nil, errors.New("Redis TxPipeline Failed: " + "Endpoint Connections Not Ready")
	}

	return p.core.cnWriter.TxPipeline(), nil
}

// TxPipelined allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) TxPipelined(fn func(redis.Pipeliner) error) (result []redis.Cmder, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-TxPipelined", p.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-TxPipelined-Result", result)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		result, err = p.txPipelinedInternal(fn)
		return result, err
	} else {
		return p.txPipelinedInternal(fn)
	}
}

// txPipelinedInternal allows actions against redis to be handled in a batched fashion
func (p *PIPELINE) txPipelinedInternal(fn func(redis.Pipeliner) error) ([]redis.Cmder, error) {
	// validate
	if p.core == nil {
		return nil, errors.New("Redis TxPipelined Failed: " + "Base is Nil")
	}

	if !p.core.cnAreReady {
		return nil, errors.New("Redis TxPipelined Failed: " + "Endpoint Connections Not Ready")
	}

	return p.core.cnWriter.TxPipelined(p.core.cnWriter.Context(), fn)
}

// ----------------------------------------------------------------------------------------------------------------
// TTL functions
// ----------------------------------------------------------------------------------------------------------------

// TTL returns the remainder time to live in seconds or milliseconds, for key that has a TTL set,
// returns -1 if no TTL applicable (forever living)
func (t *TTL) TTL(key string, bGetMilliseconds bool) (valTTL int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-TTL", t.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-TTL-Key", key)
			_ = seg.Seg.AddMetadata("Redis-TTL-Not-Found", notFound)

			if bGetMilliseconds {
				_ = seg.Seg.AddMetadata("Redis-TTL-Remainder-Milliseconds", valTTL)
			} else {
				_ = seg.Seg.AddMetadata("Redis-TTL-Remainder-Seconds", valTTL)
			}

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valTTL, notFound, err = t.ttlInternal(key, bGetMilliseconds)
		return valTTL, notFound, err
	} else {
		return t.ttlInternal(key, bGetMilliseconds)
	}
}

// ttlInternal returns the remainder time to live in seconds or milliseconds, for key that has a TTL set,
// returns -1 if no TTL applicable (forever living)
func (t *TTL) ttlInternal(key string, bGetMilliseconds bool) (valTTL int64, notFound bool, err error) {
	// validate
	if t.core == nil {
		return 0, false, errors.New("Redis TTL Failed: " + "Base is Nil")
	}

	if !t.core.cnAreReady {
		return 0, false, errors.New("Redis TTL Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis TTL Failed: " + "Key is Required")
	}

	var cmd *redis.DurationCmd

	if bGetMilliseconds {
		cmd = t.core.cnReader.PTTL(t.core.cnReader.Context(), key)
	} else {
		cmd = t.core.cnReader.TTL(t.core.cnReader.Context(), key)
	}

	var d time.Duration

	d, notFound, err = t.core.handleDurationCmd(cmd, "Redis TTL Failed: ")

	if err != nil {
		return 0, false, err
	}

	if d == -2 {
		// not found
		return 0, true, nil
	} else if d == -1 {
		// forever living
		return -1, false, nil
	}

	if bGetMilliseconds {
		valTTL = d.Milliseconds()
	} else {
		valTTL = int64(d.Seconds())
	}

	return valTTL, notFound, err
}

// Expire sets a timeout on key (seconds or milliseconds based on input parameter)
//
// expireValue = in seconds or milliseconds
func (t *TTL) Expire(key string, bSetMilliseconds bool, expireValue time.Duration) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Expire", t.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Expire-Key", key)

			if bSetMilliseconds {
				_ = seg.Seg.AddMetadata("Redis-Expire-Milliseconds", expireValue.Milliseconds())
			} else {
				_ = seg.Seg.AddMetadata("Redis-Expire-Seconds", expireValue.Seconds())
			}

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = t.expireInternal(key, bSetMilliseconds, expireValue)
		return err
	} else {
		return t.expireInternal(key, bSetMilliseconds, expireValue)
	}
}

// expireInternal sets a timeout on key (seconds or milliseconds based on input parameter)
//
// expireValue = in seconds or milliseconds
func (t *TTL) expireInternal(key string, bSetMilliseconds bool, expireValue time.Duration) error {
	// validate
	if t.core == nil {
		return errors.New("Redis Expire Failed: " + "Base is Nil")
	}

	if !t.core.cnAreReady {
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
		cmd = t.core.cnWriter.PExpire(t.core.cnWriter.Context(), key, expireValue)
	} else {
		cmd = t.core.cnWriter.Expire(t.core.cnWriter.Context(), key, expireValue)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ExpireAt", t.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ExpireAt-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ExpireAt-Expire-Time", expireTime)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = t.expireAtInternal(key, expireTime)
		return err
	} else {
		return t.expireAtInternal(key, expireTime)
	}
}

// expireAtInternal sets the hard expiration date time based on unix timestamp for a given key
//
// Setting expireTime to the past immediately deletes the key
func (t *TTL) expireAtInternal(key string, expireTime time.Time) error {
	// validate
	if t.core == nil {
		return errors.New("Redis ExpireAt Failed: " + "Base is Nil")
	}

	if !t.core.cnAreReady {
		return errors.New("Redis ExpireAt Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis ExpireAt Failed: " + "Key is Required")
	}

	if expireTime.IsZero() {
		return errors.New("Redis ExpireAt Failed: " + "Expire Time is Required")
	}

	cmd := t.core.cnWriter.ExpireAt(t.core.cnWriter.Context(), key, expireTime)

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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Touch", t.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Touch-Keys", key)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = t.touchInternal(key...)
		return err
	} else {
		return t.touchInternal(key...)
	}
}

// touchInternal alters the last access time of a key or keys,
// if key doesn't exist, it is ignored
func (t *TTL) touchInternal(key ...string) error {
	// validate
	if t.core == nil {
		return errors.New("Redis Touch Failed: " + "Base is Nil")
	}

	if !t.core.cnAreReady {
		return errors.New("Redis Touch Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis Touch Failed: " + "At Least 1 Key is Required")
	}

	cmd := t.core.cnWriter.Touch(t.core.cnWriter.Context(), key...)

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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Persist", t.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Persist-Key", key)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = t.persistInternal(key)
		return err
	} else {
		return t.persistInternal(key)
	}
}

// persistInternal removes existing timeout TTL of a key so it lives forever
func (t *TTL) persistInternal(key string) error {
	// validate
	if t.core == nil {
		return errors.New("Redis Persist Failed: " + "Base is Nil")
	}

	if !t.core.cnAreReady {
		return errors.New("Redis Persist Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return errors.New("Redis Persist Failed: " + "Key is Required")
	}

	cmd := t.core.cnWriter.Persist(t.core.cnWriter.Context(), key)

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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Ping", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = u.pingInternal()
		return err
	} else {
		return u.pingInternal()
	}
}

// pingInternal will ping the redis server to see if its up
//
// result nil = success; otherwise error info is returned via error object
func (u *UTILS) pingInternal() error {
	// validate
	if u.core == nil {
		return errors.New("Redis Ping Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return errors.New("Redis Ping Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := u.core.cnReader.Ping(u.core.cnReader.Context())
	return u.core.handleStatusCmd(cmd, "Redis Ping Failed: ")
}

// DBSize returns number of keys in the redis database
func (u *UTILS) DBSize() (val int64, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-DBSize", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-DBSize-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, err = u.dbSizeInternal()
		return val, err
	} else {
		return u.dbSizeInternal()
	}
}

// dbSizeInternal returns number of keys in the redis database
func (u *UTILS) dbSizeInternal() (val int64, err error) {
	// validate
	if u.core == nil {
		return 0, errors.New("Redis DBSize Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return 0, errors.New("Redis DBSize Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := u.core.cnReader.DBSize(u.core.cnReader.Context())
	val, _, err = u.core.handleIntCmd(cmd, "Redis DBSize Failed: ")
	return val, err
}

// Time returns the redis server time
func (u *UTILS) Time() (val time.Time, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Time", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Time-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, err = u.timeInternal()
		return val, err
	} else {
		return u.timeInternal()
	}
}

// timeInternal returns the redis server time
func (u *UTILS) timeInternal() (val time.Time, err error) {
	// validate
	if u.core == nil {
		return time.Time{}, errors.New("Redis Time Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return time.Time{}, errors.New("Redis Time Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := u.core.cnReader.Time(u.core.cnReader.Context())
	val, _, err = u.core.handleTimeCmd(cmd, "Redis Time Failed: ")
	return val, err
}

// LastSave checks if last db save action was successful
func (u *UTILS) LastSave() (val time.Time, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-LastSave", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-LastSave-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, err = u.lastSaveInternal()
		return val, err
	} else {
		return u.lastSaveInternal()
	}
}

// lastSaveInternal checks if last db save action was successful
func (u *UTILS) lastSaveInternal() (val time.Time, err error) {
	// validate
	if u.core == nil {
		return time.Time{}, errors.New("Redis LastSave Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return time.Time{}, errors.New("Redis LastSave Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := u.core.cnReader.LastSave(u.core.cnReader.Context())
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Type", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Type-Key", key)
			_ = seg.Seg.AddMetadata("Redis-Type-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, err = u.typeInternal(key)
		return val, err
	} else {
		return u.typeInternal(key)
	}
}

// typeInternal returns the redis key's value type stored
// expected result in string = list, set, zset, hash, and stream
func (u *UTILS) typeInternal(key string) (val rediskeytype.RedisKeyType, err error) {
	// validate
	if u.core == nil {
		return rediskeytype.UNKNOWN, errors.New("Redis Type Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return rediskeytype.UNKNOWN, errors.New("Redis Type Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return rediskeytype.UNKNOWN, errors.New("Redis Type Failed: " + "Key is Required")
	}

	cmd := u.core.cnReader.Type(u.core.cnReader.Context(), key)

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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ObjectEncoding", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ObjectEncoding-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ObjectEncoding-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ObjectEncoding-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = u.objectEncodingInternal(key)
		return val, notFound, err
	} else {
		return u.objectEncodingInternal(key)
	}
}

// objectEncodingInternal returns the internal representation used in order to store the value associated with a key
func (u *UTILS) objectEncodingInternal(key string) (val string, notFound bool, err error) {
	// validate
	if u.core == nil {
		return "", false, errors.New("Redis ObjectEncoding Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return "", false, errors.New("Redis ObjectEncoding Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return "", false, errors.New("Redis ObjectEncoding Failed: " + "Key is Required")
	}

	cmd := u.core.cnReader.ObjectEncoding(u.core.cnReader.Context(), key)
	return u.core.handleStringCmd2(cmd, "Redis ObjectEncoding Failed: ")
}

// ObjectIdleTime returns the number of seconds since the object stored at the specified key is idle (not requested by read or write operations)
func (u *UTILS) ObjectIdleTime(key string) (val time.Duration, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ObjectIdleTime", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ObjectIdleTime-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ObjectIdleTime-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ObjectIdleTime-Result-Seconds", val.Seconds())

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = u.objectIdleTimeInternal(key)
		return val, notFound, err
	} else {
		return u.objectIdleTimeInternal(key)
	}
}

// objectIdleTimeInternal returns the number of seconds since the object stored at the specified key is idle (not requested by read or write operations)
func (u *UTILS) objectIdleTimeInternal(key string) (val time.Duration, notFound bool, err error) {
	// validate
	if u.core == nil {
		return 0, false, errors.New("Redis ObjectIdleTime Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return 0, false, errors.New("Redis ObjectIdleTime Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ObjectIdleTime Failed: " + "Key is Required")
	}

	cmd := u.core.cnReader.ObjectIdleTime(u.core.cnReader.Context(), key)
	return u.core.handleDurationCmd(cmd, "Redis ObjectIdleTime Failed: ")
}

// ObjectRefCount returns the number of references of the value associated with the specified key
func (u *UTILS) ObjectRefCount(key string) (val int64, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-ObjectRefCount", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-ObjectRefCount-Key", key)
			_ = seg.Seg.AddMetadata("Redis-ObjectRefCount-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-ObjectRefCount-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = u.objectRefCountInternal(key)
		return val, notFound, err
	} else {
		return u.objectRefCountInternal(key)
	}
}

// objectRefCountInternal returns the number of references of the value associated with the specified key
func (u *UTILS) objectRefCountInternal(key string) (val int64, notFound bool, err error) {
	// validate
	if u.core == nil {
		return 0, false, errors.New("Redis ObjectRefCount Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return 0, false, errors.New("Redis ObjectRefCount Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return 0, false, errors.New("Redis ObjectRefCount Failed: " + "Key is Required")
	}

	cmd := u.core.cnReader.ObjectRefCount(u.core.cnReader.Context(), key)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Scan", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Scan-Match", match)
			_ = seg.Seg.AddMetadata("Redis-Scan-Scan-Cursor", cursor)
			_ = seg.Seg.AddMetadata("Redis-Scan-Scan-Count", count)
			_ = seg.Seg.AddMetadata("Redis-Scan-Result-Cursor", resultCursor)
			_ = seg.Seg.AddMetadata("Redis-Scan-Keys-Found", keys)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		keys, resultCursor, err = u.scanInternal(cursor, match, count)
		return keys, resultCursor, err
	} else {
		return u.scanInternal(cursor, match, count)
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
func (u *UTILS) scanInternal(cursor uint64, match string, count int64) (keys []string, resultCursor uint64, err error) {
	// validate
	if u.core == nil {
		return nil, 0, errors.New("Redis Scan Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return nil, 0, errors.New("Redis Scan Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := u.core.cnReader.Scan(u.core.cnReader.Context(), cursor, match, count)
	return u.core.handleScanCmd(cmd, "Redis Scan Failed: ")
}

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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Keys", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Keys-Match", match)
			_ = seg.Seg.AddMetadata("Redis-Keys-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-Keys-Keys-Found", valKeys)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		valKeys, notFound, err = u.keysInternal(match)
		return valKeys, notFound, err
	} else {
		return u.keysInternal(match)
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
func (u *UTILS) keysInternal(match string) (valKeys []string, notFound bool, err error) {
	// validate
	if u.core == nil {
		return nil, false, errors.New("Redis Keys Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return nil, false, errors.New("Redis Keys Failed: " + "Endpoint Connections Not Ready")
	}

	if len(match) <= 0 {
		return nil, false, errors.New("Redis Keys Failed: " + "Match is Required")
	}

	cmd := u.core.cnReader.Keys(u.core.cnReader.Context(), match)
	return u.core.handleStringSliceCmd(cmd, "Redis Keys Failed: ")
}

// RandomKey returns a random key from redis
func (u *UTILS) RandomKey() (val string, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-RandomKey", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-RandomKey-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, err = u.randomKeyInternal()
		return val, err
	} else {
		return u.randomKeyInternal()
	}
}

// randomKeyInternal returns a random key from redis
func (u *UTILS) randomKeyInternal() (val string, err error) {
	// validate
	if u.core == nil {
		return "", errors.New("Redis RandomKey Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return "", errors.New("Redis RandomKey Failed: " + "Endpoint Connections Not Ready")
	}

	cmd := u.core.cnReader.RandomKey(u.core.cnReader.Context())
	val, _, err = u.core.handleStringCmd2(cmd, "Redis RandomKey Failed: ")
	return val, err
}

// Rename will rename the keyOriginal to be keyNew in redis,
// if keyNew already exist in redis, then Rename will override existing keyNew with keyOriginal
// if keyOriginal is not in redis, error is returned
func (u *UTILS) Rename(keyOriginal string, keyNew string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Rename", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Rename-OriginalKey", keyOriginal)
			_ = seg.Seg.AddMetadata("Redis-Rename-NewKey", keyNew)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = u.renameInternal(keyOriginal, keyNew)
		return err
	} else {
		return u.renameInternal(keyOriginal, keyNew)
	}
}

// renameInternal will rename the keyOriginal to be keyNew in redis,
// if keyNew already exist in redis, then Rename will override existing keyNew with keyOriginal
// if keyOriginal is not in redis, error is returned
func (u *UTILS) renameInternal(keyOriginal string, keyNew string) error {
	// validate
	if u.core == nil {
		return errors.New("Redis Rename Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return errors.New("Redis Rename Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyOriginal) <= 0 {
		return errors.New("Redis Rename Failed: " + "Key Original is Required")
	}

	if len(keyNew) <= 0 {
		return errors.New("Redis Rename Failed: " + "Key New is Required")
	}

	cmd := u.core.cnWriter.Rename(u.core.cnWriter.Context(), keyOriginal, keyNew)
	return u.core.handleStatusCmd(cmd, "Redis Rename Failed: ")
}

// RenameNX will rename the keyOriginal to be keyNew IF keyNew does not yet exist in redis
// if RenameNX fails due to keyNew already exist, or other errors, the error is returned
func (u *UTILS) RenameNX(keyOriginal string, keyNew string) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-RenameNX", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-RenameNX-OriginalKey", keyOriginal)
			_ = seg.Seg.AddMetadata("Redis-RenameNX-NewKey", keyNew)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = u.renameNXInternal(keyOriginal, keyNew)
		return err
	} else {
		return u.renameNXInternal(keyOriginal, keyNew)
	}
}

// renameNXInternal will rename the keyOriginal to be keyNew IF keyNew does not yet exist in redis
// if RenameNX fails due to keyNew already exist, or other errors, the error is returned
func (u *UTILS) renameNXInternal(keyOriginal string, keyNew string) error {
	// validate
	if u.core == nil {
		return errors.New("Redis RenameNX Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return errors.New("Redis RenameNX Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyOriginal) <= 0 {
		return errors.New("Redis RenameNX Failed: " + "Key Original is Required")
	}

	if len(keyNew) <= 0 {
		return errors.New("Redis RenameNX Failed: " + "Key New is Required")
	}

	cmd := u.core.cnWriter.RenameNX(u.core.cnWriter.Context(), keyOriginal, keyNew)
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
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-Sort", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-Sort-Key", key)
			_ = seg.Seg.AddMetadata("Redis-Sort-SortPattern", sortPattern)
			_ = seg.Seg.AddMetadata("Redis-Sort-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-Sort-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = u.sortInternal(key, sortPattern)
		return val, notFound, err
	} else {
		return u.sortInternal(key, sortPattern)
	}
}

// sortInternal will sort values as defined by keyToSort, along with sortPattern, and then return the sorted data via string slice
// sort is applicable to list, set, or sorted set as defined by key
//
// sortPattern = defines the sort conditions (see redis sort documentation for details)
func (u *UTILS) sortInternal(key string, sortPattern *redis.Sort) (val []string, notFound bool, err error) {
	// validate
	if u.core == nil {
		return nil, false, errors.New("Redis Sort Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return nil, false, errors.New("Redis Sort Failed: " + "Endpoint Connections Not Ready")
	}

	if len(key) <= 0 {
		return nil, false, errors.New("Redis Sort Failed: " + "Key is Required")
	}

	cmd := u.core.cnReader.Sort(u.core.cnReader.Context(), key, sortPattern)
	return u.core.handleStringSliceCmd(cmd, "Redis Sort Failed: ")
}

// SortInterfaces will sort values as defined by keyToSort, along with sortPattern, and then return the sorted data via []interface{}
// sort is applicable to list, set, or sorted set as defined by key
//
// sortPattern = defines the sort conditions (see redis sort documentation for details)
func (u *UTILS) SortInterfaces(keyToSort string, sortPattern *redis.Sort) (val []interface{}, notFound bool, err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SortInterfaces", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SortInterfaces-SortKey", keyToSort)
			_ = seg.Seg.AddMetadata("Redis-SortInterfaces-SortPattern", sortPattern)
			_ = seg.Seg.AddMetadata("Redis-SortInterfaces-Not-Found", notFound)
			_ = seg.Seg.AddMetadata("Redis-SortInterfaces-Result", val)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		val, notFound, err = u.sortInterfacesInternal(keyToSort, sortPattern)
		return val, notFound, err
	} else {
		return u.sortInterfacesInternal(keyToSort, sortPattern)
	}
}

// sortInterfacesInternal will sort values as defined by keyToSort, along with sortPattern, and then return the sorted data via []interface{}
// sort is applicable to list, set, or sorted set as defined by key
//
// sortPattern = defines the sort conditions (see redis sort documentation for details)
func (u *UTILS) sortInterfacesInternal(keyToSort string, sortPattern *redis.Sort) (val []interface{}, notFound bool, err error) {
	// validate
	if u.core == nil {
		return nil, false, errors.New("Redis SortInterfaces Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return nil, false, errors.New("Redis SortInterfaces Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyToSort) <= 0 {
		return nil, false, errors.New("Redis SortInterfaces Failed: " + "KeyToSort is Required")
	}

	cmd := u.core.cnReader.SortInterfaces(u.core.cnReader.Context(), keyToSort, sortPattern)
	return u.core.handleSliceCmd(cmd, "Redis SortInterfaces Failed: ")
}

// SortStore will sort values defined by keyToSort, and sort according to sortPattern, and set sorted results into keyToStore in redis
// sort is applicable to list, set, or sorted set as defined by key
//
// sortPattern = defines the sort conditions (see redis sort documentation for details)
func (u *UTILS) SortStore(keyToSort string, keyToStore string, sortPattern *redis.Sort) (err error) {
	// get new xray segment for tracing
	seg := xray.NewSegmentNullable("Redis-SortStore", u.core._parentSegment)

	if seg != nil {
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Redis-SortStore-SortKey", keyToSort)
			_ = seg.Seg.AddMetadata("Redis-SortStore-SortPattern", sortPattern)
			_ = seg.Seg.AddMetadata("Redis-SortStore-StoreKey", keyToStore)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = u.sortStoreInternal(keyToSort, keyToStore, sortPattern)
		return err
	} else {
		return u.sortStoreInternal(keyToSort, keyToStore, sortPattern)
	}
}

// sortStoreInternal will sort values defined by keyToSort, and sort according to sortPattern, and set sorted results into keyToStore in redis
// sort is applicable to list, set, or sorted set as defined by key
//
// sortPattern = defines the sort conditions (see redis sort documentation for details)
func (u *UTILS) sortStoreInternal(keyToSort string, keyToStore string, sortPattern *redis.Sort) error {
	// validate
	if u.core == nil {
		return errors.New("Redis SortStore Failed: " + "Base is Nil")
	}

	if !u.core.cnAreReady {
		return errors.New("Redis SortStore Failed: " + "Endpoint Connections Not Ready")
	}

	if len(keyToSort) <= 0 {
		return errors.New("Redis SortStore Failed: " + "KeyToSort is Required")
	}

	if len(keyToStore) <= 0 {
		return errors.New("Redis SortStore Failed: " + "KeyToStore is Required")
	}

	cmd := u.core.cnWriter.SortStore(u.core.cnWriter.Context(), keyToSort, keyToStore, sortPattern)
	_, _, err := u.core.handleIntCmd(cmd, "Redis SortStore Failed: ")
	return err
}
