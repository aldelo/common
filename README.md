# common
Common helper utilities and wrappers for code reuse.
 
When we code projects, we constantly encounter a similar set of functionality and logic. This package's intent is to wrap those commonly recurring functionalities and access points into a reusable helper package, so that we don't have to keep maintaining separate code bases.

This package will continue to be updated with more reusable code as well.

# Usage
- To use the common package:
  - in the project folder root: 
  - go mod init
  - go mod tidy
- For example, if project is "HelloWorld":
  - /HelloWorld/go mod init
  - /HelloWorld/go mod tidy

# types of helpers
- string helpers
- number helpers
- io helpers
- converter helpers
- db type helpers
- net helpers
- reflection helpers
- regex helpers
- time and date helpers
- uuid helpers
- crypto helpers (aes, gcm, rsa, sha, etc)
- csv parser helpers
- wrappers for aws related services
  - service discovery / cloud map wrapper (using aws sdk)
  - dynamodb / dax wrapper (using aws sdk)
  - kms wrapper (using aws sdk)
  - redis wrapper (using go-redis package)
  - s3 wrapper (using aws sdk)
  - ses wrapper (using aws sdk)
  - sns wrapper (using aws sdk)
- wrappers for relational database access
  - mysql wrapper (using sqlx package)
  - sqlite wrapper (using sqlx package)
  - sqlserver wrapper (using sqlx package)
- other wrappers
  - for running as systemd service
  - for logging and config
  - for circuit breaker and rate limit
  - etc.
  
# build and deploy automation
- Create a file such as 'build.sh' within project
- Edit file content as:
```
    #!/bin/zsh

    GOOS=linux GOARC=amd64 go build
    scp -i ~/.ssh/YourKey.pem YourBinary hostUserName@hostIP:/home/hostUserName/targetFolder
```
- Descriptions:
  - YourKey.pem = the Linux Host SSH Certificate Key
  - YourBinary = the Binary Build by go build to Upload
  - hostUserName = the Linux Host Login Username
  - hostIP = the Linux Host IP Address
  - hostUserName = the Linux Host Login Username
  - targetFolder = the Linux Host Folder Where SCP Will Upload YourBinary To
 
