package systemd

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
	"log"
	"github.com/kardianos/service"
)

// =====================================================================================================================
// systemd service setup info
// =====================================================================================================================

/*
	xyz.service for linux systemd setup

	1) create file 'xyz.service' with following content:
			xyz = name of service app program

	[Unit]
	Description=XYZ App Title
	After=network.target
	StartLimitIntervalSec=0

	[Service]
	Type=simple
	ExecStart=/home/ubuntu/xyzFolder/xyzAppName -svc -port=8080
	WorkingDirectory=/home/ubuntu/xyzFolder
	User=ubuntu
	StandardOutput=console
	Restart=always
	RestartSec=1

	[Install]
	WantedBy=multi-user.target
	Alias=xyzAppName.service

	2) place 'xyz.service' file at home directory
	3) note: port 80 seems to be restricted on ubuntu, so we use port 8080 rather than having to reconfigure the os
	4) note: -svc -port=8080 are optional based on the xyzAppName flags

	5) place the xyz.service file to '/etc/systemd/system'
			$> sudo cp xyz.service /etc/systemd/system

	6) to enable and load service
			$> sudo systemctl enable xyz.service
	7) to start service
			$> sudo systemctl start xyz.service
	8) to stop service
			$> sudo systemctl stop xyz.service
	9) to disable service
			$> sudo systemctl disable xyz.service
*/

// =====================================================================================================================
// systemd usage in main.go
// =====================================================================================================================

/*
	func main() {
		//
		// define service program base
		//
		svc := &systemd.ServiceProgram{
			ServiceName: "abc.xyz",
			DisplayName: "App Name Descriptive",
			Description: "More info about the app service",

			Port: 8080,							// port that this service will run on, if port is not used, set to 0
			StartServiceHandler: startHandler,	// startHandler is a function that performs service launch code, such as starting web server or micro service
			StopServiceHandler: stopHandler,	// stopHandler is a function that performs service stop code, such as clean up
		}

		//
		// launch service
		//
		svc.Launch()
	}

	func startHandler(port int) {
		// place application logic in handler
		// such as setup gin handlers and start logic handling services etc
	}

	func stopHandler() {
		// place applicatipn clean up code here
	}
 */

// ---------------------------------------------------------------------------------------------------------------------
// service base definition
// ---------------------------------------------------------------------------------------------------------------------

//
// define service logger
//
var logger service.Logger

//
// define service program
//
type ServiceProgram struct {
	ServiceName string
	DisplayName string
	Description string

	Port int
	StartServiceHandler func(port int)
	StopServiceHandler func()
}

// Launch will initialize and start the service for operations
//
// Launch is called from within main() to start the service
func (p *ServiceProgram) Launch() {
	//
	// define service configuration
	//
	svcConfig := &service.Config{
		Name: p.ServiceName,
		DisplayName: p.DisplayName,
		Description: p.Description,
	}

	//
	// instantiate service object
	//
	svc, err := service.New(p, svcConfig)

	if err != nil {
		log.Fatalf("%s Init Service Failed: %s", p.ServiceName, err.Error())
	}

	//
	// setup logger
	//
	logger, err = svc.Logger(nil)

	if err != nil {
		log.Fatalf("%s Init Logger Failed: %s", p.ServiceName, err.Error())
	}

	//
	// run the service
	//
	err = svc.Run()

	if err != nil {
		log.Fatalf("%s Run Service Failed: %s", p.ServiceName, err.Error())
	}
}

//
// Start the service
//
func (p *ServiceProgram) Start(s service.Service) error {
	// start service - do not block, actual work should be done async
	go p.run()
	return nil
}

//
// run is async goroutine to handle service code
//
func (p *ServiceProgram) run() {
	// do actual work async in this go-routine
	log.Println("Starting Service Program...")

	if p != nil {
		if p.Port >= 0 && p.Port < 65535 {
			// run service handler
			if p.StartServiceHandler != nil {
				log.Println("Start Service Handler Invoked...")
				p.StartServiceHandler(p.Port)
			}
		}
	}
}

//
// Stop will stop the service
//
func (p *ServiceProgram) Stop(s service.Service) error {
	// stop the service, should not block
	log.Println("Stopping Service Program...")

	if p != nil {
		if p.StopServiceHandler != nil {
			log.Println("Stop Service Handler Invoked...")
			p.StopServiceHandler()
		}
	}

	return nil
}
