package hystrixgo

/*
 * Copyright 2020 Aldelo, LP
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
	"context"
	"errors"
	"github.com/afex/hystrix-go/hystrix"
	metricCollector "github.com/afex/hystrix-go/hystrix/metric_collector"
	"github.com/afex/hystrix-go/plugins"
	data "github.com/aldelo/common/wrapper/zap"
	util "github.com/aldelo/common"
	"net"
	"net/http"
	"strconv"
)

// CircuitBreaker defines one specific circuit breaker by command name
//
// Config Properties:
//		1) Timeout = how long to wait for command to complete, in milliseconds, default = 1000
//		2) MaxConcurrentRequests = how many commands of the same type can run at the same time, default = 10
//		3) RequestVolumeThreshold = minimum number of requests needed before a circuit can be tripped due to health, default = 20
//		4) SleepWindow = how long to wait after a circuit opens before testing for recovery, in milliseconds, default = 5000
//		5) ErrorPercentThreshold = causes circuits to open once the rolling measure of errors exceeds this percent of requests, default = 50
//		6) Logger = indicates the logger that will be used in the Hystrix package, default = logs nothing
type CircuitBreaker struct {
	// circuit breaker command name for this instance
	CommandName string

	// config fields
	TimeOut int
	MaxConcurrentRequests int
	RequestVolumeThreshold int
	SleepWindow int
	ErrorPercentThreshold int

	// config logger
	Logger *data.ZapLog

	// config to disable circuit breaker temporarily
	DisableCircuitBreaker bool

	//
	// local state variables
	//
	streamHandler *hystrix.StreamHandler
}

// RunLogic declares func alias for internal Run logic handler
type RunLogic func(dataIn interface{}, ctx ...context.Context) (dataOut interface{}, err error)

// FallbackLogic declares func alias for internal Fallback logic handler
type FallbackLogic func(dataIn interface{}, errIn error, ctx ...context.Context) (dataOut interface{}, err error)

// Init will initialize the circuit break with the given command name,
// a command name represents a specific service or api method that has circuit breaker being applied
func (c *CircuitBreaker) Init() error {
	// validate
	if util.LenTrim(c.CommandName) <= 0 {
		return errors.New("CircuitBreaker Init Failed: " + "Command Name is Required")
	}

	// set config fields to proper value
	if c.TimeOut <= 0 {
		c.TimeOut = 1000
	}

	if c.MaxConcurrentRequests <= 0 {
		c.MaxConcurrentRequests = 10
	}

	if c.RequestVolumeThreshold <= 0 {
		c.RequestVolumeThreshold = 20
	}

	if c.SleepWindow <= 0 {
		c.SleepWindow = 5000
	}

	if c.ErrorPercentThreshold <= 0 {
		c.ErrorPercentThreshold = 50
	}

	// setup circuit breaker for the given command name
	hystrix.ConfigureCommand(c.CommandName, hystrix.CommandConfig{
		Timeout: c.TimeOut,
		MaxConcurrentRequests: c.MaxConcurrentRequests,
		RequestVolumeThreshold: c.RequestVolumeThreshold,
		SleepWindow: c.SleepWindow,
		ErrorPercentThreshold: c.ErrorPercentThreshold,
	})

	// setup logger
	if c.Logger != nil {
		hystrix.SetLogger(c.Logger)
	} else {
		hystrix.SetLogger(hystrix.NoopLogger{})
	}

	// success
	return nil
}

// FlushAll will purge all circuits and metrics from memory
func (c *CircuitBreaker) FlushAll() {
	hystrix.Flush()
}

// UpdateConfig will update the hystrixgo command config data to the current value in struct for a given command name
func (c *CircuitBreaker) UpdateConfig() {
	// command name must exist
	if util.LenTrim(c.CommandName) <= 0 {
		return
	}

	// set config fields to proper value
	if c.TimeOut <= 0 {
		c.TimeOut = 1000
	}

	if c.MaxConcurrentRequests <= 0 {
		c.MaxConcurrentRequests = 10
	}

	if c.RequestVolumeThreshold <= 0 {
		c.RequestVolumeThreshold = 20
	}

	if c.SleepWindow <= 0 {
		c.SleepWindow = 5000
	}

	if c.ErrorPercentThreshold <= 0 {
		c.ErrorPercentThreshold = 50
	}

	// setup circuit breaker for the given command name
	hystrix.ConfigureCommand(c.CommandName, hystrix.CommandConfig{
		Timeout: c.TimeOut,
		MaxConcurrentRequests: c.MaxConcurrentRequests,
		RequestVolumeThreshold: c.RequestVolumeThreshold,
		SleepWindow: c.SleepWindow,
		ErrorPercentThreshold: c.ErrorPercentThreshold,
	})
}

// UpdateLogger will udpate the hystrixgo package wide logger,
// based on the Logger set in the struct field
func (c *CircuitBreaker) UpdateLogger() {
	if c.Logger != nil {
		hystrix.SetLogger(c.Logger)
	} else {
		hystrix.SetLogger(hystrix.NoopLogger{})
	}
}

// Go will execute async with circuit breaker
//
// Parameters:
// 		1) run = required, defines either inline or external function to be executed,
//				 it is meant for a self contained function and accepts no parameter, returns error
// 		2) fallback = optional, defines either inline or external function to be executed as fallback when run fails,
//					  it is meat for a self contained function and accepts only error parameter, returns error,
//					  set to nil if fallback is not specified
//		3) dataIn = optional, input parameter to run and fallback func, may be nil if not needed
func (c *CircuitBreaker) Go(run RunLogic,
						    fallback FallbackLogic,
						    dataIn interface{}) (interface{}, error) {
	// validate
	if util.LenTrim(c.CommandName) <= 0 {
		return nil, errors.New("Exec Async Failed: " + "CircuitBreaker Command Name is Required")
	}

	if run == nil {
		return nil, errors.New("Exec Async for '" + c.CommandName + "' Failed: " + "Run Func Implementation is Required")
	}

	// execute async via circuit breaker
	if !c.DisableCircuitBreaker {
		//
		// using circuit breaker
		//
		result := make(chan interface{})

		errChan := hystrix.Go(c.CommandName,
							  func() error{
							  		//
							  		// run func
							  		//
							  		outInf, outErr := run(dataIn)

							  		if outErr != nil {
							  			// pass error back
							  			return outErr
									} else {
										// pass result back
										if outInf != nil {
											result <- outInf
										} else {
											result <- true
										}

										return nil
									}
							  },
							  func(er error) error {
							  		//
							  		// fallback func
							  		//
							  		if fallback != nil {
							  			// fallback is defined
							  			outInf, outErr := fallback(dataIn, er)

										if outErr != nil {
											// pass error back
											return outErr
										} else {
											// pass result back
											if outInf != nil {
												result <- outInf
											} else {
												result <- true
											}

											return nil
										}
									} else {
										// fallback is not defined
										return er
									}
							  })

		var err error
		var output interface{}

		select {
		case output = <-result:
			// when no error
		case err = <-errChan:
			// when has error
		}

		if err != nil {
			return nil, errors.New("Exec Async for '" + c.CommandName + "' Failed: (Go Action) " + err.Error())
		} else {
			return output, nil
		}
	} else {
		//
		// not using circuit breaker - pass thru
		//
		if obj, err := run(dataIn); err != nil {
			return nil, errors.New("Exec Directly for '" + c.CommandName + "' Failed: (Non-CircuitBreaker Go Action) " + err.Error())
		} else {
			return obj, nil
		}
	}
}

// GoC will execute async with circuit breaker in given context
//
// Parameters:
//		1) ctx = required, defines the context in which this method is to be run under
// 		2) run = required, defines either inline or external function to be executed,
//				 it is meant for a self contained function and accepts context.Context parameter, returns error
// 		3) fallback = optional, defines either inline or external function to be executed as fallback when run fails,
//					  it is meat for a self contained function and accepts context.Context and error parameters, returns error,
//					  set to nil if fallback is not specified
//		4) dataIn = optional, input parameter to run and fallback func, may be nil if not needed
func (c *CircuitBreaker) GoC(ctx context.Context,
							 run RunLogic,
							 fallback FallbackLogic,
							 dataIn interface{}) (interface{}, error) {
	// validate
	if util.LenTrim(c.CommandName) <= 0 {
		return nil, errors.New("Exec with Context Async Failed: " + "CircuitBreaker Command Name is Required")
	}

	if ctx == nil {
		return nil, errors.New("Exec with Context Async Failed: " + "CircuitBreaker Context is Required")
	}

	if run == nil {
		return nil, errors.New("Exec with Context Async for '" + c.CommandName + "' Failed: " + "Run Func Implementation is Required")
	}

	// execute async via circuit breaker
	if !c.DisableCircuitBreaker {
		//
		// using circuit breaker
		//
		result := make(chan interface{})

		errChan := hystrix.GoC(ctx, c.CommandName,
			func(ct context.Context) error{
				//
				// run func
				//
				outInf, outErr := run(dataIn, ct)

				if outErr != nil {
					// pass error back
					return outErr
				} else {
					// pass result back
					if outInf != nil {
						result <- outInf
					} else {
						result <- true
					}

					return nil
				}
			},
			func(ct context.Context, er error) error {
				//
				// fallback func
				//
				if fallback != nil {
					// fallback is defined
					outInf, outErr := fallback(dataIn, er, ct)

					if outErr != nil {
						// pass error back
						return outErr
					} else {
						// pass result back
						if outInf != nil {
							result <- outInf
						} else {
							result <- true
						}

						return nil
					}
				} else {
					// fallback is not defined
					return er
				}
			})

		var err error
		var output interface{}

		select {
		case output = <-result:
			// when no error
		case err = <-errChan:
			// when has error
		}

		if err != nil {
			return nil, errors.New("Exec with Context Async for '" + c.CommandName + "' Failed: (GoC Action) " + err.Error())
		} else {
			return output, nil
		}
	} else {
		//
		// not using circuit breaker - pass thru
		//
		if obj, err := run(dataIn, ctx); err != nil {
			return nil, errors.New("Exec with Context Directly for '" + c.CommandName + "' Failed: (Non-CircuitBreaker GoC Action) " + err.Error())
		} else {
			return obj, nil
		}
	}
}

// Do will execute synchronous with circuit breaker
//
// Parameters:
// 		1) run = required, defines either inline or external function to be executed,
//				 it is meant for a self contained function and accepts no parameter, returns error
// 		2) fallback = optional, defines either inline or external function to be executed as fallback when run fails,
//					  it is meat for a self contained function and accepts only error parameter, returns error,
//					  set to nil if fallback is not specified
//		3) dataIn = optional, input parameter to run and fallback func, may be nil if not needed
func (c *CircuitBreaker) Do(run RunLogic, fallback FallbackLogic, dataIn interface{}) (interface{}, error) {
	// validate
	if util.LenTrim(c.CommandName) <= 0 {
		return nil, errors.New("Exec Synchronous Failed: " + "CircuitBreaker Command Name is Required")
	}

	if run == nil {
		return nil, errors.New("Exec Synchronous for '" + c.CommandName + "' Failed: " + "Run Func Implementation is Required")
	}

	// execute synchronous via circuit breaker
	if !c.DisableCircuitBreaker {
		// circuit breaker
		var result interface{}

		if err := hystrix.Do(c.CommandName,
							 func() error {
							 	// run func
								outInf, outErr := run(dataIn)

								 if outErr != nil {
									 // pass error back
									 return outErr
								 } else {
									 // pass result back
									 if outInf != nil {
										 result = outInf
									 } else {
										 result = true
									 }

									 return nil
								 }
							 },
							 func(er error) error {
							 	// fallback func
								 if fallback != nil {
									 // fallback is defined
									 outInf, outErr := fallback(dataIn, er)

									 if outErr != nil {
										 // pass error back
										 return outErr
									 } else {
										 // pass result back
										 if outInf != nil {
											 result = outInf
										 } else {
											 result = true
										 }

										 return nil
									 }
								 } else {
									 // fallback is not defined
									 return er
								 }
							 }); err != nil {
			return nil, errors.New("Exec Synchronous for '" + c.CommandName + "' Failed: (Do Action) " + err.Error())
		} else {
			return result, nil
		}
	} else {
		// non circuit breaker - pass thru
		if obj, err := run(dataIn); err != nil {
			return nil, errors.New("Exec Directly for '" + c.CommandName + "' Failed: (Non-CircuitBreaker Do Action) " + err.Error())
		} else {
			return obj, nil
		}
	}
}

// DoC will execute synchronous with circuit breaker in given context
//
// Parameters:
//		1) ctx = required, defines the context in which this method is to be run under
// 		2) run = required, defines either inline or external function to be executed,
//				 it is meant for a self contained function and accepts context.Context parameter, returns error
// 		3) fallback = optional, defines either inline or external function to be executed as fallback when run fails,
//					  it is meant for a self contained function and accepts context.Context and error parameters, returns error,
//					  set to nil if fallback is not specified
//		4) dataIn = optional, input parameter to run and fallback func, may be nil if not needed
func (c *CircuitBreaker) DoC(ctx context.Context, run RunLogic, fallback FallbackLogic, dataIn interface{}) (interface{}, error) {
	// validate
	if util.LenTrim(c.CommandName) <= 0 {
		return nil, errors.New("Exec with Context Synchronous Failed: " + "CircuitBreaker Command Name is Required")
	}

	if ctx == nil {
		return nil, errors.New("Exec with Context Synchronous for '" + c.CommandName + "' Failed: " + "CircuitBreaker Context is Required")
	}

	if run == nil {
		return nil, errors.New("Exec with Context Synchronous for '" + c.CommandName + "' Failed: " + "Run Func Implementation is Required")
	}

	// execute synchronous via circuit breaker
	if !c.DisableCircuitBreaker {
		// circuit breaker
		var result interface{}

		if err := hystrix.DoC(ctx, c.CommandName,
							 func(ct context.Context) error {
								 // run func
								 outInf, outErr := run(dataIn, ct)

								 if outErr != nil {
									 // pass error back
									 return outErr
								 } else {
									 // pass result back
									 if outInf != nil {
										 result = outInf
									 } else {
										 result = true
									 }

									 return nil
								 }
							 },
							 func(ct context.Context, er error) error {
								 // fallback func
								 if fallback != nil {
									 // fallback is defined
									 outInf, outErr := fallback(dataIn, er, ct)

									 if outErr != nil {
									 	 // pass error back
										 return outErr
									 } else {
										 // pass result back
										 if outInf != nil {
											 result = outInf
										 } else {
											 result = true
										 }

										 return nil
									 }
								 } else {
									 // fallback is not defined
									 return er
								 }
							 }); err != nil {
			return nil, errors.New("Exec with Context Synchronous for '" + c.CommandName + "' Failed: (DoC Action) " + err.Error())
		} else {
			return result, nil
		}
	} else {
		// non circuit breaker - pass thru
		if obj, err := run(dataIn, ctx); err != nil {
			return nil, errors.New("Exec with Context Directly for '" + c.CommandName + "' Failed: (Non-CircuitBreaker DoC Action) " + err.Error())
		} else {
			return obj, nil
		}
	}
}

// StartStreamHttpServer will start a simple HTTP server on local host with given port,
// this will launch in goroutine, and return immediately
//
// This method call is on entire hystrixgo package, not just the current circuit breaker struct
//
// To view stream data, launch browser, point to http://localhost:port
//
// default port = 81
func (c *CircuitBreaker) StartStreamHttpServer(port ...int) {
	if c.streamHandler == nil {
		c.streamHandler = hystrix.NewStreamHandler()
		c.streamHandler.Start()

		p := 81

		if len(port) > 0 {
			p = port[0]
		}

		go http.ListenAndServe(net.JoinHostPort("", strconv.Itoa(p)), c.streamHandler)
	}
}

// StopStreamHttpServer will stop the currently running stream server if already started
func (c *CircuitBreaker) StopStreamHttpServer() {
	if c.streamHandler != nil {
		c.streamHandler.Stop()
		c.streamHandler = nil
	}
}

// StartStatsdCollector will register and initiate the hystrixgo package for metric collection into statsd,
// each action from hystrixgo will be tracked into metrics and pushed into statsd via udp
//
// Parameters:
//		1) appName = name of the app working with hystrixgo
//		2) statsdIp = IP address of statsd service host
//		3) statsdPort = Port of statsd service host
//
// statsd and graphite is a service that needs to be running on a host, either localhost or another reachable host in linux,
// the easiest way to deploy statsd and graphite is via docker image:
// 		see full docker install info at = https://github.com/graphite-project/docker-graphite-statsd
//
// docker command to run statsd and graphite as a unit as follows:
//		docker run -d --name graphite --restart=always -p 80:80 -p 2003-2004:2003-2004 -p 2023-2024:2023-2024 -p 8125:8125/udp -p 8126:8126 graphiteapp/graphite-statsd
//
// once docker image is running, to view graphite:
//		http://localhost/dashboard
func (c *CircuitBreaker) StartStatsdCollector(appName string, statsdIp string, statsdPort ...int) error {
	// validate
	if util.LenTrim(appName) <= 0 {
		return errors.New("Start Statsd Collector Failed: " + "App Name is Required")
	}

	// get ip and port
	p := 8125

	if len(statsdPort) > 0 {
		p = statsdPort[0]
	}

	ip := "127.0.0.1"

	if util.LenTrim(statsdIp) > 0 {
		ip = statsdIp
	}

	// compose statsd collection
	sdc, err := plugins.InitializeStatsdCollector(&plugins.StatsdCollectorConfig{
		StatsdAddr: ip + ":" + strconv.Itoa(p),
		Prefix: appName + ".hystrixgo",
	})

	// register statsd
	if err != nil {
		return errors.New("Start Statsd Collector Failed: (Init Statsd Collector Action) " + err.Error())
	} else {
		metricCollector.Registry.Register(sdc.NewStatsdCollector)
		return nil
	}
}
