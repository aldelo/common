package cloudmap

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
	"errors"
	"fmt"
	util "github.com/aldelo/common"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/cloudmap/sdhealthchecktype"
	"github.com/aldelo/common/wrapper/cloudmap/sdnamespacefilter"
	"github.com/aldelo/common/wrapper/cloudmap/sdoperationfilter"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/servicediscovery"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
	"log"
	"net/http"
	"time"
)

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// CloudMap struct encapsulates the AWS CloudMap access functionality
type CloudMap struct {
	// define the AWS region that KMS is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// store aws session object
	sdClient *servicediscovery.ServiceDiscovery

	_parentSegment *xray.XRayParentSegment
}

// DnsConf represents a dns config option to be used by CreateService
// for this project we will only use:
//  1. ipv4 - dns record type A
//  2. srv
//
// TTL = dns record time to live in seconds
// MultiValue = true: route 53 returns up to 8 healthy targets if health check is enabled (otherwise, all targets assumed healthy)
//
//	false: route 53 uses WEIGHTED to return a random healthy target if health check is enabled (if no healthy target found, then any random target is used)
//
// SRV = true: dns use SRV; false: dns use A
type DnsConf struct {
	TTL        int64
	MultiValue bool
	SRV        bool
}

// HealthCheckConf represents target health check configuration
//
// Custom = true: use HealthCheckCustomConfig (for Http, Public Dns, Private Dns namespaces)
//
//	false: use HealthCheckConfig (for Public Dns namespace only)
//
// FailureThreshold = if Custom is true:
//
//			*) number of 30-second intervals that cloud map waits after
//			   UpdateInstanceCustomHealthStatus is executed,
//			   before changing the target health status
//	  if Custom is false:
//			*) number of consecutive times health checks of target
//			   must pass or fail for route 53 to consider healthy or unhealthy
//
// PubDns_HealthCheck_Type = for public dns namespace only: the endpoint protocol type used for health check
// PubDns_HealthCheck_Path = for public dns namespace only: (Http and Https type ONLY),
//
//	path to service that responds to health check, that returns http status 2xx or 3xx as healthy
type HealthCheckConf struct {
	Custom                  bool
	FailureThreshold        int64
	PubDns_HealthCheck_Type sdhealthchecktype.SdHealthCheckType
	PubDns_HealthCheck_Path string
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the CloudMap service
func (sd *CloudMap) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			sd._parentSegment = parentSegment[0]
		}

		seg := xray.NewSegment("Cloudmap-Connect", sd._parentSegment)
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-AWS-Region", sd.AwsRegion)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = sd.connectInternal()

		if err == nil {
			awsxray.AWS(sd.sdClient.Client)
		}

		return err
	} else {
		return sd.connectInternal()
	}
}

// Connect will establish a connection to the CloudMap service
func (sd *CloudMap) connectInternal() error {
	// clean up prior sd client
	sd.sdClient = nil

	if !sd.AwsRegion.Valid() || sd.AwsRegion == awsregion.UNKNOWN {
		return errors.New("Connect To CloudMap Failed: (AWS Session Error) " + "Region is Required")
	}

	// create custom http2 client if needed
	var httpCli *http.Client
	var httpErr error

	if sd.HttpOptions == nil {
		sd.HttpOptions = new(awshttp2.HttpClientSettings)
	}

	// use custom http2 client
	h2 := &awshttp2.AwsHttp2Client{
		Options: sd.HttpOptions,
	}

	if httpCli, httpErr = h2.NewHttp2Client(); httpErr != nil {
		return errors.New("Connect to CloudMap Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection
	if sess, err := session.NewSession(
		&aws.Config{
			Region:     aws.String(sd.AwsRegion.Key()),
			HTTPClient: httpCli,
		}); err != nil {
		// aws session error
		return errors.New("Connect To CloudMap Failed: (AWS Session Error) " + err.Error())
	} else {
		// create cached objects for shared use
		sd.sdClient = servicediscovery.New(sess)

		if sd.sdClient == nil {
			return errors.New("Connect To CloudMap Client Failed: (New CloudMap Client Connection) " + "Connection Object Nil")
		}

		return nil
	}
}

// Disconnect clear client
func (sd *CloudMap) Disconnect() {
	sd.sdClient = nil
}

// toTags converts map of tags to slice of tags
func (sd *CloudMap) toTags(tagsMap map[string]string) (t []*servicediscovery.Tag) {
	if tagsMap != nil {
		for k, v := range tagsMap {
			t = append(t, &servicediscovery.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
	}
	return
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (sd *CloudMap) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	sd._parentSegment = parentSegment
}

// ----------------------------------------------------------------------------------------------------------------
// namespace functions
// ----------------------------------------------------------------------------------------------------------------

// CreateHttpNamespace creates an http namespace for AWS cloud map
//
// Service instances registered to http namespace can be discovered using DiscoverInstances(),
//
//	however, service instances cannot be discovered via dns
//
// Parameters:
//  1. name = (required) name of the http namespace to create
//  2. creatorRequestId = (required) random and unique string to identify this create namespace action (such as uuid)
//  3. description = (optional) http namespace description
//  4. tags = (optional) one or more key value pairs to store as namespace tags
//  5. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. operationId = string representing the identifier to be used to check on operation status at a later time
//  2. err = contains error info if error was encountered
func (sd *CloudMap) CreateHttpNamespace(name string,
	creatorRequestId string,
	description string,
	tags map[string]string,
	timeOutDuration ...time.Duration) (operationId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-CreateHttpNamespace", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-CreateHttpNamespace-Name", name)
			_ = seg.Seg.AddMetadata("Cloudmap-CreateHttpNamespace-CreatorRequestID", creatorRequestId)
			_ = seg.Seg.AddMetadata("Cloudmap-CreateHttpNamespace-Result-OperationID", operationId)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = fmt.Errorf("CloudMap CreateHttpNamespace Failed: " + "SD Client is Required")
		return "", err
	}

	if util.LenTrim(name) == 0 {
		err = fmt.Errorf("CloudMap CreateHttpNamespace Failed: " + "Name is Required")
		return "", err
	}

	if util.LenTrim(creatorRequestId) == 0 {
		err = fmt.Errorf("CloudMap CreateHttpNamespace Failed: " + "CreatorRequestId is Required")
		return "", err
	}

	// define input
	input := &servicediscovery.CreateHttpNamespaceInput{
		Name:             aws.String(name),
		CreatorRequestId: aws.String(creatorRequestId),
	}

	if util.LenTrim(description) > 0 {
		input.Description = aws.String(description)
	}

	if tags != nil {
		t := sd.toTags(tags)

		if len(t) > 0 {
			if len(t) > 50 {
				err = fmt.Errorf("CloudMap CreateHttpNamespace Failed: " + "Tags Maximum Entries is 50")
				return "", err
			}

			input.Tags = t
		}
	}

	// invoke action
	var output *servicediscovery.CreateHttpNamespaceOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.CreateHttpNamespaceWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.CreateHttpNamespaceWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.CreateHttpNamespace(input)
		}
	}

	if err != nil {
		err = fmt.Errorf("CloudMap CreateHttpNamespace Failed: (Create Action) " + err.Error())
		return "", err
	}

	// action completed
	return *output.OperationId, nil
}

// CreatePrivateDnsNamespace creates a private dns based namespace, visible only inside a specified aws vpc,
//
//	this namespace defines service naming scheme,
//	for example:
//		if namespace is named as 'example.com', and service is named as 'xyz-service',
//		the resulting dns name for the service will be 'xyz-service.example.com'
//
// Parameters:
//  1. name = (required) name of the private dns namespace to create
//  2. creatorRequestId = (required) random and unique string to identify this create namespace action (such as uuid)
//  3. vpc = (required) aws vpc id that this private dns associated with
//  4. description = (optional) private dns namespace description
//  5. tags = (optional) one or more key value pairs to store as namespace tags
//  6. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. operationId = string representing the identifier to be used to check on operation status at a later time
//  2. err = contains error info if error was encountered
func (sd *CloudMap) CreatePrivateDnsNamespace(name string,
	creatorRequestId string,
	vpc string,
	description string,
	tags map[string]string,
	timeOutDuration ...time.Duration) (operationId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-CreatePrivateDnsNamespace", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-CreatePrivateDnsNamespace-Name", name)
			_ = seg.Seg.AddMetadata("Cloudmap-CreatePrivateDnsNamespace-CreatorRequestID", creatorRequestId)
			_ = seg.Seg.AddMetadata("Cloudmap-CreatePrivateDnsNamespace-VPC", vpc)
			_ = seg.Seg.AddMetadata("Cloudmap-CreatePrivateDnsNamespace-Result-OperationID", operationId)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap CreatePrivateDnsNamespace Failed: " + "SD Client is Required")
		return "", err
	}

	if util.LenTrim(name) == 0 {
		err = errors.New("CloudMap CreatePrivateDnsNamespace Failed: " + "Name is Required")
		return "", err
	}

	if util.LenTrim(creatorRequestId) == 0 {
		err = errors.New("CloudMap CreatePrivateDnsNamespace Failed: " + "CreatorRequestId is Required")
		return "", err
	}

	if util.LenTrim(vpc) == 0 {
		err = errors.New("CloudMap CreatePrivateDnsNamespace Failed: " + "VPC is Required")
		return "", err
	}

	// define input
	input := &servicediscovery.CreatePrivateDnsNamespaceInput{
		Name:             aws.String(name),
		CreatorRequestId: aws.String(creatorRequestId),
		Vpc:              aws.String(vpc),
	}

	if util.LenTrim(description) > 0 {
		input.Description = aws.String(description)
	}

	if tags != nil {
		t := sd.toTags(tags)

		if len(t) > 0 {
			if len(t) > 50 {
				err = errors.New("CloudMap CreatePrivateDnsNamespace Failed: " + "Tags Maximum Entries is 50")
				return "", err
			}

			input.Tags = t
		}
	}

	// invoke action
	var output *servicediscovery.CreatePrivateDnsNamespaceOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.CreatePrivateDnsNamespaceWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.CreatePrivateDnsNamespaceWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.CreatePrivateDnsNamespace(input)
		}
	}

	if err != nil {
		err = errors.New("CloudMap CreatePrivateDnsNamespace Failed: (Create Action) " + err.Error())
		return "", err
	}

	// action completed
	return *output.OperationId, nil
}

// CreatePublicDnsNamespace creates a public dns based namespace, accessible via the public internet,
//
//	this namespace defines service naming scheme,
//	for example:
//		if namespace is named as 'example.com', and service is named as 'xyz-service',
//		the resulting dns name for the service will be 'xyz-service.example.com'
//
// Parameters:
//  1. name = (required) name of the public dns namespace to create
//  2. creatorRequestId = (required) random and unique string to identify this create namespace action (such as uuid)
//  3. description = (optional) public dns namespace description
//  4. tags = (optional) one or more key value pairs to store as namespace tags
//  5. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. operationId = string representing the identifier to be used to check on operation status at a later time
//  2. err = contains error info if error was encountered
func (sd *CloudMap) CreatePublicDnsNamespace(name string,
	creatorRequestId string,
	description string,
	tags map[string]string,
	timeOutDuration ...time.Duration) (operationId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-CreatePublicDnsNamespace", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-CreatePublicDnsNamespace-Name", name)
			_ = seg.Seg.AddMetadata("Cloudmap-CreatePublicDnsNamespace-CreatorRequestID", creatorRequestId)
			_ = seg.Seg.AddMetadata("Cloudmap-CreatePublicDnsNamespace-Result-OperationID", operationId)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap CreatePublicDnsNamespace Failed: " + "SD Client is Required")
		return "", err
	}

	if util.LenTrim(name) == 0 {
		err = errors.New("CloudMap CreatePublicDnsNamespace Failed: " + "Name is Required")
		return "", err
	}

	if util.LenTrim(creatorRequestId) == 0 {
		err = errors.New("CloudMap CreatePublicDnsNamespace Failed: " + "CreatorRequestId is Required")
		return "", err
	}

	// define input
	input := &servicediscovery.CreatePublicDnsNamespaceInput{
		Name:             aws.String(name),
		CreatorRequestId: aws.String(creatorRequestId),
	}

	if util.LenTrim(description) > 0 {
		input.Description = aws.String(description)
	}

	if tags != nil {
		t := sd.toTags(tags)

		if len(t) > 0 {
			if len(t) > 50 {
				err = errors.New("CloudMap CreatePublicDnsNamespace Failed: " + "Tags Maximum Entries is 50")
				return "", err
			}

			input.Tags = t
		}
	}

	// invoke action
	var output *servicediscovery.CreatePublicDnsNamespaceOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.CreatePublicDnsNamespaceWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.CreatePublicDnsNamespaceWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.CreatePublicDnsNamespace(input)
		}
	}

	if err != nil {
		err = errors.New("CloudMap CreatePublicDnsNamespace Failed: (Create Action) " + err.Error())
		return "", err
	}

	// action completed
	return *output.OperationId, nil
}

// GetNamespace gets the information about a specific namespace
//
// Parameters:
//  1. namespaceId = (required) namespace id used for search
//  2. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. namespace = sd namespace object found
//  2. err = error info if any
func (sd *CloudMap) GetNamespace(namespaceId string, timeOutDuration ...time.Duration) (namespace *servicediscovery.Namespace, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-GetNamespace", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-GetNamespace-NamespaceID", namespaceId)
			_ = seg.Seg.AddMetadata("Cloudmap-GetNamespace-Result-NamespaceObject", namespace)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap GetNamespace Failed: " + "SD Client is Required")
		return nil, err
	}

	if util.LenTrim(namespaceId) == 0 {
		err = errors.New("CloudMap GetNamespace Failed: " + "NamespaceId is Required")
		return nil, err
	}

	// define input
	input := &servicediscovery.GetNamespaceInput{
		Id: aws.String(namespaceId),
	}

	// invoke action
	var output *servicediscovery.GetNamespaceOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.GetNamespaceWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.GetNamespaceWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.GetNamespace(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap GetNamespace Failed: (Get Action) " + err.Error())
		return nil, err
	}

	return output.Namespace, nil
}

// ListNamespaces gets summary information about namespaces created already
//
// Parameters:
//  1. filter = (optional) specifies namespace filter options
//  2. maxResults = (optional) specifies maximum count to return
//  3. nextToken = (optional) if initial action, leave blank; if this is a subsequent action to get more, input the moreNextToken returned from a prior action
//  4. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. namespaces = slice of sd namespace summary objects
//  2. moreNextToken = if more data exists, this token can be used in a subsequent action via nextToken parameter
//  3. err = error info if any
func (sd *CloudMap) ListNamespaces(filter *sdnamespacefilter.SdNamespaceFilter,
	maxResults *int64,
	nextToken *string,
	timeOutDuration ...time.Duration) (namespaces []*servicediscovery.NamespaceSummary, moreNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-ListNamespaces", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-ListNamespaces-Filter", filter)
			_ = seg.Seg.AddMetadata("Cloudmap-ListNamespaces-Max-Results", maxResults)
			_ = seg.Seg.AddMetadata("Cloudmap-ListNamespaces-Next-Token", nextToken)
			_ = seg.Seg.AddMetadata("Cloudmap-ListNamespaces-Result-Namespaces", namespaces)
			_ = seg.Seg.AddMetadata("Cloudmap-ListNamespaces-Result-Next-Token", moreNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap ListNamespaces Failed: " + "SD Client is Required")
		return nil, "", err
	}

	if maxResults != nil {
		if *maxResults <= 0 {
			err = errors.New("CloudMap ListNamespaces Failed: " + "MaxResults Must Be Greater Than Zero")
			return nil, "", err
		}
	}

	// define input
	input := &servicediscovery.ListNamespacesInput{}

	if filter != nil && filter.Valid() && *filter != sdnamespacefilter.UNKNOWN {
		input.Filters = []*servicediscovery.NamespaceFilter{
			{
				Name: aws.String("TYPE"),
			},
		}

		switch *filter {
		case sdnamespacefilter.PrivateDnsNamespace:
			input.Filters[0].Condition = aws.String("EQ")
			input.Filters[0].Values = []*string{
				aws.String("DNS_PRIVATE"),
			}
		case sdnamespacefilter.PublicDnsNamespace:
			input.Filters[0].Condition = aws.String("EQ")
			input.Filters[0].Values = []*string{
				aws.String("DNS_PUBLIC"),
			}
		case sdnamespacefilter.Both:
			input.Filters[0].Condition = aws.String("IN")
			input.Filters[0].Values = []*string{
				aws.String("DNS_PRIVATE"),
				aws.String("DNS_PUBLIC"),
			}
		}
	}

	if maxResults != nil {
		input.MaxResults = maxResults
	}

	if nextToken != nil {
		if util.LenTrim(*nextToken) > 0 {
			input.NextToken = nextToken
		}
	}

	// invoke action
	var output *servicediscovery.ListNamespacesOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.ListNamespacesWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.ListNamespacesWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.ListNamespaces(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap ListNamespaces Failed: (List Action) " + err.Error())
		return nil, "", err
	}

	return output.Namespaces, *output.NextToken, nil
}

// ListNamespacesPages gets summary information about namespaces created already
// (issues multiple page requests until max results is met or all data is retrieved)
//
// Parameters:
//  1. filter = (optional) specifies namespace filter options
//  2. maxResults = (optional) specifies maximum count to return
//  3. nextToken = (optional) if initial action, leave blank; if this is a subsequent action to get more, input the moreNextToken returned from a prior action
//  4. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. namespaces = slice of sd namespace summary objects
//  2. moreNextToken = if more data exists, this token can be used in a subsequent action via nextToken parameter
//  3. err = error info if any
func (sd *CloudMap) ListNamespacesPages(filter *sdnamespacefilter.SdNamespaceFilter,
	maxResults *int64,
	nextToken *string,
	timeOutDuration ...time.Duration) (namespaces []*servicediscovery.NamespaceSummary, moreNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-ListNamespacesPages", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-ListNamespacesPages-Filter", filter)
			_ = seg.Seg.AddMetadata("Cloudmap-ListNamespacesPages-Max-Results", maxResults)
			_ = seg.Seg.AddMetadata("Cloudmap-ListNamespacesPages-Next-Token", nextToken)
			_ = seg.Seg.AddMetadata("Cloudmap-ListNamespacesPages-Result-Namespaces", namespaces)
			_ = seg.Seg.AddMetadata("Cloudmap-ListNamespacesPages-Result-Next-Token", moreNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap ListNamespacesPages Failed: " + "SD Client is Required")
		return nil, "", err
	}

	if maxResults != nil {
		if *maxResults <= 0 {
			err = errors.New("CloudMap ListNamespacesPages Failed: " + "MaxResults Must Be Greater Than Zero")
			return nil, "", err
		}
	}

	// define input
	input := &servicediscovery.ListNamespacesInput{}

	if filter != nil && filter.Valid() && *filter != sdnamespacefilter.UNKNOWN {
		input.Filters = []*servicediscovery.NamespaceFilter{
			{
				Name: aws.String("TYPE"),
			},
		}

		switch *filter {
		case sdnamespacefilter.PrivateDnsNamespace:
			input.Filters[0].Condition = aws.String("EQ")
			input.Filters[0].Values = []*string{
				aws.String("DNS_PRIVATE"),
			}
		case sdnamespacefilter.PublicDnsNamespace:
			input.Filters[0].Condition = aws.String("EQ")
			input.Filters[0].Values = []*string{
				aws.String("DNS_PUBLIC"),
			}
		case sdnamespacefilter.Both:
			input.Filters[0].Condition = aws.String("IN")
			input.Filters[0].Values = []*string{
				aws.String("DNS_PRIVATE"),
				aws.String("DNS_PUBLIC"),
			}
		}
	}

	if maxResults != nil {
		input.MaxResults = maxResults
	}

	if nextToken != nil {
		if util.LenTrim(*nextToken) > 0 {
			input.NextToken = nextToken
		}
	}

	// invoke action
	fn := func(pageOutput *servicediscovery.ListNamespacesOutput, lastPage bool) bool {
		if pageOutput != nil {
			moreNextToken = *pageOutput.NextToken
			namespaces = append(namespaces, pageOutput.Namespaces...)
		}

		return !lastPage
	}

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		err = sd.sdClient.ListNamespacesPagesWithContext(ctx, input, fn)
	} else {
		if segCtxSet {
			err = sd.sdClient.ListNamespacesPagesWithContext(segCtx, input, fn)
		} else {
			err = sd.sdClient.ListNamespacesPages(input, fn)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap ListNamespacesPages Failed: (ListPages Action) " + err.Error())
		return nil, "", err
	}

	return namespaces, moreNextToken, nil
}

// DeleteNamespace deletes an existing namespace, however if namespace still has attached services, then action will fail
//
// Parameters:
//  1. namespaceId = (required) namespace id to delete
//
// Return Values:
//  1. operationId = represents the operation to be used for status check on this action via GetOperation()
//  2. err = error info if any
func (sd *CloudMap) DeleteNamespace(namespaceId string, timeOutDuration ...time.Duration) (operationId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-DeleteNamespace", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-DeleteNamespace-NamespaceID", namespaceId)
			_ = seg.Seg.AddMetadata("Cloudmap-DeleteNamespace-Result-OperationID", operationId)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap DeleteNamespace Failed: " + "SD Client is Required")
		return "", err
	}

	if util.LenTrim(namespaceId) == 0 {
		err = errors.New("CloudMap DeleteNamespace Failed: " + "NamespaceId is Required")
		return "", err
	}

	// define input
	input := &servicediscovery.DeleteNamespaceInput{
		Id: aws.String(namespaceId),
	}

	// invoke action
	var output *servicediscovery.DeleteNamespaceOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.DeleteNamespaceWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.DeleteNamespaceWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.DeleteNamespace(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap DeleteNamespace Failed: (Delete Action) " + err.Error())
		return "", err
	}

	return *output.OperationId, nil
}

// ----------------------------------------------------------------------------------------------------------------
// service functions
// ----------------------------------------------------------------------------------------------------------------

// CreateService creates a service under a specific namespace
//
// # After service is created, use RegisterInstance() to register an instance for the given service
//
// Parameters:
//  1. name = (required) name of the service to create, under the given namespaceId
//  2. creatorRequestId = (required) random and unique string to identify this create service action (such as uuid)
//  3. namespaceId = (required) namespace that this service be created under
//  4. dnsConf = (conditional) required for public and private dns namespaces, configures the dns parameters for this service
//  5. healthCheckConf = (optional) nil will not set health check, otherwise sets a health check condition for this services' instances
//  6. description = (optional) public dns namespace description
//  7. tags = (optional) one or more key value pairs to store as namespace tags
//  8. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. service = service object that was created
//  2. err = contains error info if error was encountered
func (sd *CloudMap) CreateService(name string,
	creatorRequestId string,
	namespaceId string,
	dnsConf *DnsConf,
	healthCheckConf *HealthCheckConf,
	description string,
	tags map[string]string,
	timeOutDuration ...time.Duration) (service *servicediscovery.Service, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-CreateService", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-CreateService-ServiceName", name)
			_ = seg.Seg.AddMetadata("Cloudmap-CreateService-CreatorRequestID", creatorRequestId)
			_ = seg.Seg.AddMetadata("Cloudmap-CreateService-NamespaceID", namespaceId)
			_ = seg.Seg.AddMetadata("Cloudmap-CreateService-DNSConf", dnsConf)
			_ = seg.Seg.AddMetadata("Cloudmap-CreateService-HealthCheckConf", healthCheckConf)
			_ = seg.Seg.AddMetadata("Cloudmap-CreateService-Result-ServiceObject", service)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap CreateService Failed: " + "SD Client is Required")
		return nil, err
	}

	if util.LenTrim(name) == 0 {
		err = errors.New("CloudMap CreateService Failed: " + "Name is Required")
		return nil, err
	}

	if util.LenTrim(creatorRequestId) == 0 {
		err = errors.New("CloudMap CreateService Failed: " + "CreatorRequestId is Required")
		return nil, err
	}

	if util.LenTrim(namespaceId) == 0 {
		err = errors.New("CloudMap CreateService Failed: " + "NamespaceId is Required")
		return nil, err
	}

	if dnsConf != nil {
		// dns conf set, public or private dns namespace only
		if dnsConf.TTL <= 0 {
			dnsConf.TTL = 300 // default to 5 minutes ttl if not specified
		}

		if healthCheckConf != nil {
			if healthCheckConf.FailureThreshold <= 0 {
				healthCheckConf.FailureThreshold = 1
			}

			if healthCheckConf.Custom {
				healthCheckConf.PubDns_HealthCheck_Type = sdhealthchecktype.UNKNOWN
				healthCheckConf.PubDns_HealthCheck_Path = ""
			} else {
				if !healthCheckConf.PubDns_HealthCheck_Type.Valid() || healthCheckConf.PubDns_HealthCheck_Type == sdhealthchecktype.UNKNOWN {
					err = errors.New("CloudMap CreateService Failed: " + "Public Dns Namespace Health Check Requires Endpoint Type")
					return nil, err
				}

				if healthCheckConf.PubDns_HealthCheck_Type == sdhealthchecktype.TCP {
					healthCheckConf.PubDns_HealthCheck_Path = ""
				} else {
					if util.LenTrim(healthCheckConf.PubDns_HealthCheck_Path) == 0 {
						err = errors.New("CloudMap CreateService Failed: " + "Health Check Resource Path is Required for HTTP & HTTPS Types")
						return nil, err
					}
				}
			}
		}
	} else {
		// if dns is not defined, this is api only, health check must be custom
		if !healthCheckConf.Custom {
			err = errors.New("CloudMap CreateService Failed: " + "Route 53 Health Check is for Private or Public Dns Namespaces Only")
			return nil, err
		}
	}

	// define input
	input := &servicediscovery.CreateServiceInput{
		Name:             aws.String(name),
		CreatorRequestId: aws.String(creatorRequestId),
		NamespaceId:      aws.String(namespaceId),
	}

	if util.LenTrim(description) > 0 {
		input.Description = aws.String(description)
	}

	if tags != nil {
		t := sd.toTags(tags)

		if len(t) > 0 {
			if len(t) > 50 {
				err = errors.New("CloudMap CreateService Failed: " + "Tags Maximum Entries is 50")
				return nil, err
			}

			input.Tags = t
		}
	}

	if dnsConf != nil {
		routingPolicy := "MULTIVALUE"

		if !dnsConf.MultiValue {
			routingPolicy = "WEIGHTED"
		}

		dnsType := "A"

		if dnsConf.SRV {
			dnsType = "SRV"
		}

		input.DnsConfig = &servicediscovery.DnsConfig{
			RoutingPolicy: aws.String(routingPolicy),
			DnsRecords: []*servicediscovery.DnsRecord{
				{
					TTL:  aws.Int64(dnsConf.TTL),
					Type: aws.String(dnsType),
				},
			},
		}
	}

	if healthCheckConf != nil {
		if healthCheckConf.Custom {
			// custom health config
			input.HealthCheckCustomConfig = &servicediscovery.HealthCheckCustomConfig{
				FailureThreshold: aws.Int64(healthCheckConf.FailureThreshold),
			}
		} else {
			// public dns health config
			input.HealthCheckConfig = &servicediscovery.HealthCheckConfig{
				FailureThreshold: aws.Int64(healthCheckConf.FailureThreshold),
				Type:             aws.String(healthCheckConf.PubDns_HealthCheck_Type.Key()),
			}

			if util.LenTrim(healthCheckConf.PubDns_HealthCheck_Path) > 0 {
				input.HealthCheckConfig.SetResourcePath(healthCheckConf.PubDns_HealthCheck_Path)
			}
		}
	}

	// invoke action
	var output *servicediscovery.CreateServiceOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.CreateServiceWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.CreateServiceWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.CreateService(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap CreateService Failed: (Create Action) " + err.Error())
		return nil, err
	}

	return output.Service, nil
}

// UpdateService submits request for the following operations:
//  1. update the TTL for existing dnsRecords configurations
//  2. add, update, or delete HealthCheckConfig for a specified service,
//     HealthCheckCustomConfig cannot be added, updated or deleted via UpdateService action
//
// Notes:
//  1. public and private dns namespaces,
//     a) if any existing dnsRecords or healthCheckConfig configurations are omitted from the UpdateService request,
//     those omitted configurations ARE deleted from the service
//     b) if any existing HealthCheckCustomConfig configurations are omitted from the UpdateService request,
//     the omitted configurations ARE NOT deleted from the service
//  2. when settings are updated for a service,
//     aws cloud map also updates the corresponding settings in all the records and health checks,
//     that were created by the given service
//
// Parameters:
//  1. serviceId = (required) service to update
//  2. dnsConfUpdate = (required) update dns config to this value, if nil, existing dns configuration will be removed from service
//  3. healthCheckConf = (optional) update health check config to this value, if nil, existing health check config will be removed from service
//  4. descriptionUpdate = (optional) service description to update, if nil, existing description will be removed from service
//  5. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. operationId = this action's operation id to be used in GetOperation for status check
//  2. err = contains error info if error was encountered
func (sd *CloudMap) UpdateService(serviceId string,
	dnsConfUpdate *DnsConf,
	healthCheckConfUpdate *HealthCheckConf,
	descriptionUpdate *string,
	timeOutDuration ...time.Duration) (operationId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-UpdateService", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-UpdateService-ServiceID", serviceId)
			_ = seg.Seg.AddMetadata("Cloudmap-UpdateService-DNSConfUpdate", dnsConfUpdate)
			_ = seg.Seg.AddMetadata("Cloudmap-UpdateService-HealthCheckConfUpdate", healthCheckConfUpdate)
			_ = seg.Seg.AddMetadata("Cloudmap-UpdateService-DescriptionUpdate", descriptionUpdate)
			_ = seg.Seg.AddMetadata("Cloudmap-UpdateService-Result-OperationID", operationId)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap UpdateService Failed: " + "SD Client is Required")
		return "", err
	}

	if util.LenTrim(serviceId) == 0 {
		err = errors.New("CloudMap UpdateService Failed: " + "ServiceId is Required")
		return "", err
	}

	if dnsConfUpdate == nil {
		err = errors.New("CloudMap UpdateService Failed: " + "Dns Config Update is Required")
		return "", err
	}

	if healthCheckConfUpdate != nil && healthCheckConfUpdate.Custom {
		err = errors.New("CloudMap UpdateService Failed: " + "Health Check Custom Config Cannot Be Updated")
		return "", err
	}

	// dns conf set, public or private dns namespace only
	if dnsConfUpdate.TTL <= 0 {
		dnsConfUpdate.TTL = 300 // default to 5 minutes ttl if not specified
	}

	if healthCheckConfUpdate != nil {
		if healthCheckConfUpdate.FailureThreshold <= 0 {
			healthCheckConfUpdate.FailureThreshold = 1
		}

		if !healthCheckConfUpdate.PubDns_HealthCheck_Type.Valid() || healthCheckConfUpdate.PubDns_HealthCheck_Type == sdhealthchecktype.UNKNOWN {
			err = errors.New("CloudMap UpdateService Failed: " + "Public Dns Namespace Health Check Requires Endpoint Type")
			return "", err
		}

		if healthCheckConfUpdate.PubDns_HealthCheck_Type == sdhealthchecktype.TCP {
			healthCheckConfUpdate.PubDns_HealthCheck_Path = ""
		} else {
			if util.LenTrim(healthCheckConfUpdate.PubDns_HealthCheck_Path) == 0 {
				err = errors.New("CloudMap UpdateService Failed: " + "Health Check Resource Path is Required for HTTP & HTTPS Types")
				return "", err
			}
		}
	}

	// define input
	input := &servicediscovery.UpdateServiceInput{
		Id: aws.String(serviceId),
	}

	input.Service = &servicediscovery.ServiceChange{}

	if descriptionUpdate != nil {
		if util.LenTrim(*descriptionUpdate) > 0 {
			input.Service.Description = descriptionUpdate
		}
	}

	// dns update is TTL only but must provide existing dns type
	dnsType := "A"

	if dnsConfUpdate.SRV {
		dnsType = "SRV"
	}

	input.Service.DnsConfig = &servicediscovery.DnsConfigChange{
		DnsRecords: []*servicediscovery.DnsRecord{
			{
				TTL:  aws.Int64(dnsConfUpdate.TTL),
				Type: aws.String(dnsType),
			},
		},
	}

	if healthCheckConfUpdate != nil {
		// update public dns health config
		input.Service.HealthCheckConfig = &servicediscovery.HealthCheckConfig{
			FailureThreshold: aws.Int64(healthCheckConfUpdate.FailureThreshold),
			Type:             aws.String(healthCheckConfUpdate.PubDns_HealthCheck_Type.Key()),
		}

		if util.LenTrim(healthCheckConfUpdate.PubDns_HealthCheck_Path) > 0 {
			input.Service.HealthCheckConfig.ResourcePath = aws.String(healthCheckConfUpdate.PubDns_HealthCheck_Path)
		}
	}

	// invoke action
	var output *servicediscovery.UpdateServiceOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.UpdateServiceWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.UpdateServiceWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.UpdateService(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap UpdateService Failed: (Update Action) " + err.Error())
		return "", err
	}

	return *output.OperationId, nil
}

// GetService gets a specified service's settings
//
// Parameters:
//  1. serviceId = (required) get service based on this service id
//  2. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. service = service object found based on the provided serviceId
//  2. err = contains error info if error was encountered
func (sd *CloudMap) GetService(serviceId string, timeOutDuration ...time.Duration) (service *servicediscovery.Service, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-GetService", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-GetService-ServiceID", serviceId)
			_ = seg.Seg.AddMetadata("Cloudmap-GetService-Result-Service", service)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap GetService Failed: " + "SD Client is Required")
		return nil, err
	}

	if util.LenTrim(serviceId) == 0 {
		err = errors.New("CloudMap GetService Failed: " + "ServiceId is Required")
		return nil, err
	}

	// define input
	input := &servicediscovery.GetServiceInput{
		Id: aws.String(serviceId),
	}

	// invoke action
	var output *servicediscovery.GetServiceOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.GetServiceWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.GetServiceWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.GetService(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap GetService Failed: (Get Action) " + err.Error())
		return nil, err
	}

	return output.Service, nil
}

// ListServices lists summary information about all the services associated with one or more namespaces
//
// Parameters:
//  1. filter = (optional) filter by namespace(s) as specified, slice of namespaceId to filter
//  2. maxResults = (optional) specifies maximum count to return
//  3. nextToken = (optional) if initial action, leave blank; if this is a subsequent action to get more, input the moreNextToken returned from a prior action
//  4. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. services = slice of sd service summary objects
//  2. moreNextToken = if more data exists, this token can be used in a subsequent action via nextToken parameter
//  3. err = error info if any
func (sd *CloudMap) ListServices(filter []string,
	maxResults *int64,
	nextToken *string,
	timeOutDuration ...time.Duration) (services []*servicediscovery.ServiceSummary, moreNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-ListServices", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-ListServices-Filter", filter)
			_ = seg.Seg.AddMetadata("Cloudmap-ListServices-MaxResults", maxResults)
			_ = seg.Seg.AddMetadata("Cloudmap-ListServices-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("Cloudmap-ListServices-Result-Services", services)
			_ = seg.Seg.AddMetadata("Cloudmap-ListServices-Result-NextToken", moreNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap ListService Failed: " + "SD Client is Required")
		return nil, "", err
	}

	if maxResults != nil {
		if *maxResults <= 0 {
			err = errors.New("CloudMap ListServices Failed: " + "MaxResults Must Be Greater Than Zero")
			return nil, "", err
		}
	}

	// define input
	input := &servicediscovery.ListServicesInput{}

	if len(filter) == 1 {
		input.Filters = []*servicediscovery.ServiceFilter{
			{
				Name:      aws.String("NAMESPACE_ID"),
				Condition: aws.String("EQ"),
				Values: []*string{
					aws.String(filter[0]),
				},
			},
		}
	} else if len(filter) > 1 {
		input.Filters = []*servicediscovery.ServiceFilter{
			{
				Name:      aws.String("NAMESPACE_ID"),
				Condition: aws.String("IN"),
			},
		}

		var fv []string

		for _, v := range filter {
			fv = append(fv, v)
		}

		input.Filters[0].Values = aws.StringSlice(fv)
	}

	if maxResults != nil {
		input.MaxResults = maxResults
	}

	if nextToken != nil {
		if util.LenTrim(*nextToken) > 0 {
			input.NextToken = nextToken
		}
	}

	// invoke action
	var output *servicediscovery.ListServicesOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.ListServicesWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.ListServicesWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.ListServices(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap ListServices Failed: (List Action) " + err.Error())
		return nil, "", err
	}

	return output.Services, *output.NextToken, nil
}

// ListServicesPages lists summary information about all the services associated with one or more namespaces
// (issues multiple page requests until max results is met or all data is retrieved)
//
// Parameters:
//  1. filter = (optional) filter by namespace(s) as specified, slice of namespaceId to filter
//  2. maxResults = (optional) specifies maximum count to return
//  3. nextToken = (optional) if initial action, leave blank; if this is a subsequent action to get more, input the moreNextToken returned from a prior action
//  4. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. namespaces = slice of sd service summary objects
//  2. moreNextToken = if more data exists, this token can be used in a subsequent action via nextToken parameter
//  3. err = error info if any
func (sd *CloudMap) ListServicesPages(filter []string,
	maxResults *int64,
	nextToken *string,
	timeOutDuration ...time.Duration) (services []*servicediscovery.ServiceSummary, moreNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-ListServicesPages", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-ListServicesPages-Filter", filter)
			_ = seg.Seg.AddMetadata("Cloudmap-ListServicesPages-MaxResults", maxResults)
			_ = seg.Seg.AddMetadata("Cloudmap-ListServicesPages-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("Cloudmap-ListServicesPages-Result-Services", services)
			_ = seg.Seg.AddMetadata("Cloudmap-ListServicesPages-Result-NextToken", moreNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap ListServicesPages Failed: " + "SD Client is Required")
		return nil, "", err
	}

	if maxResults != nil {
		if *maxResults <= 0 {
			err = errors.New("CloudMap ListServicesPages Failed: " + "MaxResults Must Be Greater Than Zero")
			return nil, "", err
		}
	}

	// define input
	input := &servicediscovery.ListServicesInput{}

	if len(filter) == 1 {
		input.Filters = []*servicediscovery.ServiceFilter{
			{
				Name:      aws.String("NAMESPACE_ID"),
				Condition: aws.String("EQ"),
				Values: []*string{
					aws.String(filter[0]),
				},
			},
		}
	} else if len(filter) > 1 {
		input.Filters = []*servicediscovery.ServiceFilter{
			{
				Name:      aws.String("NAMESPACE_ID"),
				Condition: aws.String("IN"),
			},
		}

		var fv []string

		for _, v := range filter {
			fv = append(fv, v)
		}

		input.Filters[0].Values = aws.StringSlice(fv)
	}

	if maxResults != nil {
		input.MaxResults = maxResults
	}

	if nextToken != nil {
		if util.LenTrim(*nextToken) > 0 {
			input.NextToken = nextToken
		}
	}

	// invoke action
	fn := func(pageOutput *servicediscovery.ListServicesOutput, lastPage bool) bool {
		if pageOutput != nil {
			moreNextToken = *pageOutput.NextToken
			services = append(services, pageOutput.Services...)
		}

		return !lastPage
	}

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		err = sd.sdClient.ListServicesPagesWithContext(ctx, input, fn)
	} else {
		if segCtxSet {
			err = sd.sdClient.ListServicesPagesWithContext(segCtx, input, fn)
		} else {
			err = sd.sdClient.ListServicesPages(input, fn)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap ListServicesPages Failed: (ListPages Action) " + err.Error())
		return nil, "", err
	}

	return services, moreNextToken, nil
}

// DeleteService deletes the specified service,
//
//	if the service still contains one or more registered instances, the delete action will fail
//
// Parameters:
//  1. serviceId = (required) service to be deleted via the specified service id
//  2. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. err = nil indicates success; contains error info if error was encountered
func (sd *CloudMap) DeleteService(serviceId string, timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-DeleteService", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-DeleteService-ServiceID", serviceId)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap DeleteService Failed: " + "SD Client is Required")
		return err
	}

	if util.LenTrim(serviceId) == 0 {
		err = errors.New("CloudMap DeleteService Failed: " + "ServiceId is Required")
		return err
	}

	// define input
	input := &servicediscovery.DeleteServiceInput{
		Id: aws.String(serviceId),
	}

	// invoke action
	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		_, err = sd.sdClient.DeleteServiceWithContext(ctx, input)
	} else {
		if segCtxSet {
			_, err = sd.sdClient.DeleteServiceWithContext(segCtx, input)
		} else {
			_, err = sd.sdClient.DeleteService(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap DeleteService Failed: (Delete Action) " + err.Error())
		return err
	}

	return nil
}

// ----------------------------------------------------------------------------------------------------------------
// instance functions
// ----------------------------------------------------------------------------------------------------------------

// RegisterInstance creates or updates one or more records,
//
//	and optionally creates a health check based on settings from the specified service
//
// When RegisterInstance() request is submitted:
//  1. for each dns record defined in the service as specified by ServiceId,
//     a record is created or updated in the hosted zone that is associated with the corresponding namespace
//  2. if the service includes HealthCheckConfig,
//     a health check is created based on the settings in the health check configuration
//  3. the health check is associated with each of the new or updated records (if applicable)
//
// # One RegisterInstance() request must complete before another is submitted
//
// When AWS cloud map receives a dns query for the specified dns name,
//  1. if the health check is healthy, all records returned
//  2. if the health check is unhealthy, applicable value for the last healthy instance is returned
//  3. if health check configuration wasn't specified, then all records are returned regardless healthy or otherwise
//
// Parameters:
//  1. serviceId = (required) register instance to this serviceId
//  2. instanceId = (required) unique value for this instance, if instanceId already exists, this action will update instead of new
//  3. creatorRequestId = (required) unique request id to use in case of a failure (during fail-retry, use the same creatorRequestId
//  4. attributes = (required) map of attributes to register for this instance with the given serviceId, keys are as follows:
//     a) AWS_ALIAS_DNS_NAME = instruct cloud map to create route 53 alias record to route traffic to an ELB,
//     set the dns name associated with the load balancer to this key,
//     the associated service RoutingPolicy must be WEIGHTED,
//     when this key is set, DO NOT set values to any other AWS_INSTANCE attributes
//     b) AWS_EC2_INSTANCE_ID = for http namespace only, sets this instance's EC2 instance ID,
//     when this key is set, ONLY OTHER key allowed is AWS_INIT_HEALTH_STATUS,
//     when this key is set, the AWS_INSTANCE_IPV4 attribute will be filled with the primary private IPv4 address
//     c) AWS_INIT_HEALTH_STATUS = if associated service includes HealthCheckCustomConfig,
//     then this key may be optionally set to specify the initial status of custom health check: HEALTHY or UNHEALTHY,
//     if this key is not set, then initial status is HEALTHY
//     d) AWS_INSTANCE_IPV4 = if associated service dns record type is A, then set the IPv4 address to this key,
//     this key is required for service dns record type A
//     e) AWS_INSTANCE_PORT = if associated service includes HealthCheckConfig,
//     set the port for this endpoint that route 53 will send health check request to,
//     this key is required for service having HealthCheckConfig set
//     f) Custom Attributes = up to 30 custom attribute key value pairs,
//     key must not exceed 255 chars, value must not exceed 1024 chars,
//     total of all custom attribute key value pairs combined cannot exceed 5000 chars
//
// Return Values:
//  1. operationId = identifier to be used with GetOperation for status check (to verify completion of action)
//  2. err = contains error info if any
func (sd *CloudMap) RegisterInstance(serviceId string,
	instanceId string,
	creatorRequestId string,
	attributes map[string]string,
	timeOutDuration ...time.Duration) (operationId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-RegisterInstance", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-RegisterInstance-ServiceID", serviceId)
			_ = seg.Seg.AddMetadata("Cloudmap-RegisterInstance-InstanceID", instanceId)
			_ = seg.Seg.AddMetadata("Cloudmap-RegisterInstance-CreatorRequestID", creatorRequestId)
			_ = seg.Seg.AddMetadata("Cloudmap-RegisterInstance-Attributes", attributes)
			_ = seg.Seg.AddMetadata("Cloudmap-RegisterInstance-Result-OperationID", operationId)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap RegisterInstance Failed: " + "SD Client is Required")
		return "", err
	}

	if util.LenTrim(serviceId) == 0 {
		err = errors.New("CloudMap RegisterInstance Failed: " + "ServiceId is Required")
		return "", err
	}

	if util.LenTrim(instanceId) == 0 {
		err = errors.New("CloudMap RegisterInstance Failed: " + "InstanceId is Required")
		return "", err
	}

	if util.LenTrim(creatorRequestId) == 0 {
		err = errors.New("CloudMap RegisterInstance Failed: " + "CreatorRequestId is Required")
		return "", err
	}

	if attributes == nil {
		err = errors.New("CloudMap RegisterInstance Failed: " + "Attributes are Required (nil)")
		return "", err
	}

	if len(attributes) == 0 {
		err = errors.New("CloudMap RegisterInstance Failed: " + "Attributes Are Required (len = 0)")
		return "", err
	}

	// define input
	input := &servicediscovery.RegisterInstanceInput{
		InstanceId:       aws.String(instanceId),
		CreatorRequestId: aws.String(creatorRequestId),
		ServiceId:        aws.String(serviceId),
		Attributes:       aws.StringMap(attributes),
	}

	// invoke action
	var output *servicediscovery.RegisterInstanceOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.RegisterInstanceWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.RegisterInstanceWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.RegisterInstance(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap RegisterInstance Failed: (Register Action) " + err.Error())
		return "", err
	}

	return *output.OperationId, nil
}

// UpdateInstanceCustomHealthStatus submits a request to change the health status of a custom health check,
//
//	to healthy or unhealthy
//
// This action works only with configuration of Custom Health Checks,
//
//	which was defined using HealthCheckCustomConfig when creating a service
//
// This action cannot be used to change the status of a route 53 health check,
//
//	which was defined using HealthCheckConfig when creating a service
//
// Parameters:
//  1. instanceId = (required) update healthy status to this instanceId
//  2. serviceId = (required) the associated service
//  3. isHealthy = specify the health status during this update action
//  4. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. err = nil indicates success; otherwise error info is included
func (sd *CloudMap) UpdateInstanceCustomHealthStatus(instanceId string,
	serviceId string,
	isHealthy bool,
	timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-UpdateInstanceCustomHealthStatus", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-UpdateInstanceCustomHealthStatus-InstanceID", instanceId)
			_ = seg.Seg.AddMetadata("Cloudmap-UpdateInstanceCustomHealthStatus-ServiceID", serviceId)
			_ = seg.Seg.AddMetadata("Cloudmap-UpdateInstanceCustomHealthStatus-IsHealthy", isHealthy)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap UpdateInstanceCustomHealthStatus Failed: " + "SD Client is Required")
		return err
	}

	if util.LenTrim(instanceId) == 0 {
		err = errors.New("CloudMap UpdateInstanceCustomHealthStatus Failed: " + "InstanceId is Required")
		return err
	}

	if util.LenTrim(serviceId) == 0 {
		err = errors.New("CloudMap UpdateInstanceCustomHealthStatus Failed: " + "ServiceId is Required")
		return err
	}

	// define input
	healthStatus := ""

	if isHealthy {
		healthStatus = "HEALTHY"
	} else {
		healthStatus = "UNHEALTHY"
	}

	input := &servicediscovery.UpdateInstanceCustomHealthStatusInput{
		InstanceId: aws.String(instanceId),
		ServiceId:  aws.String(serviceId),
		Status:     aws.String(healthStatus),
	}

	// invoke action
	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		_, err = sd.sdClient.UpdateInstanceCustomHealthStatusWithContext(ctx, input)
	} else {
		if segCtxSet {
			_, err = sd.sdClient.UpdateInstanceCustomHealthStatusWithContext(segCtx, input)
		} else {
			_, err = sd.sdClient.UpdateInstanceCustomHealthStatus(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap UpdateInstanceCustomHealthStatus Failed: (Update Action) " + err.Error())
		return err
	}

	return nil
}

// DeregisterInstance deletes the route 53 dns record and health check (if any),
//
//	that was created by cloud map for the specified instance
//
// Parameters:
//  1. instanceId = (required) instance to deregister
//  2. serviceId = (required) the associated service
//
// Return Values:
//  1. operationId = operation identifier to be used with GetOperation for action completion status check
//  2. err = error info if any
func (sd *CloudMap) DeregisterInstance(instanceId string,
	serviceId string,
	timeOutDuration ...time.Duration) (operationId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-DeregisterInstance", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-DeregisterInstance-InstanceID", instanceId)
			_ = seg.Seg.AddMetadata("Cloudmap-DeregisterInstance-ServiceID", serviceId)
			_ = seg.Seg.AddMetadata("Cloudmap-DeregisterInstance-Result-OperationID", operationId)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap DeregisterInstance Failed: " + "SD Client is Required")
		return "", err
	}

	if util.LenTrim(instanceId) == 0 {
		err = errors.New("CloudMap DeregisterInstance Failed: " + "InstanceId is Required")
		return "", err
	}

	if util.LenTrim(serviceId) == 0 {
		err = errors.New("CloudMap DeregisterInstance Failed: " + "ServiceId is Required")
		return "", err
	}

	// define input
	input := &servicediscovery.DeregisterInstanceInput{
		InstanceId: aws.String(instanceId),
		ServiceId:  aws.String(serviceId),
	}

	// invoke action
	var output *servicediscovery.DeregisterInstanceOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.DeregisterInstanceWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.DeregisterInstanceWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.DeregisterInstance(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap DeregisterInstance Failed: (Deregister Action) " + err.Error())
		return "", err
	}

	return *output.OperationId, nil
}

// GetInstance gets information about a specified instance
//
// Parameters:
//  1. instanceId = (required) instance to get
//  2. serviceId = (required) the associated service
//
// Return Values:
//  1. instance = instance object retrieved
//  2. err = error info if any
func (sd *CloudMap) GetInstance(instanceId string,
	serviceId string,
	timeOutDuration ...time.Duration) (instance *servicediscovery.Instance, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-GetInstance", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstance-InstanceID", instanceId)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstance-ServiceID", serviceId)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstance-Result-Instance", instance)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap GetInstance Failed: " + "SD Client is Required")
		return nil, err
	}

	if util.LenTrim(instanceId) == 0 {
		err = errors.New("CloudMap GetInstance Failed: " + "InstanceId is Required")
		return nil, err
	}

	if util.LenTrim(serviceId) == 0 {
		err = errors.New("CloudMap GetInstance Failed: " + "ServiceId is Required")
		return nil, err
	}

	// define input
	input := &servicediscovery.GetInstanceInput{
		InstanceId: aws.String(instanceId),
		ServiceId:  aws.String(serviceId),
	}

	// invoke action
	var output *servicediscovery.GetInstanceOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.GetInstanceWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.GetInstanceWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.GetInstance(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap GetInstance Failed: (Get Action) " + err.Error())
		return nil, err
	}

	return output.Instance, nil
}

// GetInstancesHealthStatus gets the current health status (healthy, unhealthy, unknown) of one or more instances,
//
//	that are associated with a specified service
//
// # There is a brief delay between register an instance and when the health status for the instance is available
//
// Parameters:
//  1. serviceId = (required) service id assciated with the instances being checked
//  2. instanceIds = (optional) list of instance ids to check health status on, if omitted, then all instances of given service is checked
//  3. maxResults = (optional) specifies maximum count to return
//  4. nextToken = (optional) if initial action, leave blank; if this is a subsequent action to get more, input the moreNextToken returned from a prior action
//  5. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. status = map of instance status (key = instance id, value = health status 'healthy', 'unhealthy', 'unknown')
//  2. moreNextToken = if more data exists, this token can be used in a subsequent action via nextToken parameter
//  3. err = error info if any
func (sd *CloudMap) GetInstancesHealthStatus(serviceId string,
	instanceIds []string,
	maxResults *int64,
	nextToken *string,
	timeOutDuration ...time.Duration) (status map[string]string, moreNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-GetInstancesHealthStatus", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatus-ServiceID", serviceId)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatus-InstanceIDs", instanceIds)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatus-MaxResults", maxResults)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatus-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatus-Result-Status", status)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatus-Result-NextToken", moreNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap GetInstancesHealthStatus Failed: " + "SD Client is Required")
		return nil, "", err
	}

	if maxResults != nil {
		if *maxResults <= 0 {
			err = errors.New("CloudMap GetInstancesHealthStatus Failed: " + "MaxResults Must Be Greater Than Zero")
			return nil, "", err
		}
	}

	// define input
	input := &servicediscovery.GetInstancesHealthStatusInput{
		ServiceId: aws.String(serviceId),
	}

	if len(instanceIds) > 0 {
		input.Instances = aws.StringSlice(instanceIds)
	}

	if maxResults != nil {
		input.MaxResults = maxResults
	}

	if nextToken != nil {
		if util.LenTrim(*nextToken) > 0 {
			input.NextToken = nextToken
		}
	}

	// invoke action
	var output *servicediscovery.GetInstancesHealthStatusOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.GetInstancesHealthStatusWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.GetInstancesHealthStatusWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.GetInstancesHealthStatus(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap GetInstancesHealthStatus Failed: (Get Action) " + err.Error())
		return nil, "", err
	}

	return aws.StringValueMap(output.Status), *output.NextToken, nil
}

// GetInstancesHealthStatusPages gets the current health status (healthy, unhealthy, unknown) of one or more instances,
//
//	that are associated with a specified service
//	(issues multiple page requests until max results is met or all data is retrieved)
//
// # There is a brief delay between register an instance and when the health status for the instance is available
//
// Parameters:
//  1. serviceId = (required) service id assciated with the instances being checked
//  2. instanceIds = (optional) list of instance ids to check health status on, if omitted, then all instances of given service is checked
//  3. maxResults = (optional) specifies maximum count to return
//  4. nextToken = (optional) if initial action, leave blank; if this is a subsequent action to get more, input the moreNextToken returned from a prior action
//  5. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. status = map of instance status (key = instance id, value = health status 'healthy', 'unhealthy', 'unknown')
//  2. moreNextToken = if more data exists, this token can be used in a subsequent action via nextToken parameter
//  3. err = error info if any
func (sd *CloudMap) GetInstancesHealthStatusPages(serviceId string,
	instanceIds []string,
	maxResults *int64,
	nextToken *string,
	timeOutDuration ...time.Duration) (status map[string]string, moreNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-GetInstancesHealthStatusPages", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatusPages-ServiceID", serviceId)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatusPages-InstanceIDs", instanceIds)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatusPages-MaxResults", maxResults)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatusPages-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatusPages-Result-Status", status)
			_ = seg.Seg.AddMetadata("Cloudmap-GetInstancesHealthStatusPages-Result-NextToken", moreNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap GetInstancesHealthStatusPages Failed: " + "SD Client is Required")
		return nil, "", err
	}

	if maxResults != nil {
		if *maxResults <= 0 {
			err = errors.New("CloudMap GetInstancesHealthStatusPages Failed: " + "MaxResults Must Be Greater Than Zero")
			return nil, "", err
		}
	}

	// define input
	input := &servicediscovery.GetInstancesHealthStatusInput{
		ServiceId: aws.String(serviceId),
	}

	if len(instanceIds) > 0 {
		input.Instances = aws.StringSlice(instanceIds)
	}

	if maxResults != nil {
		input.MaxResults = maxResults
	}

	if nextToken != nil {
		if util.LenTrim(*nextToken) > 0 {
			input.NextToken = nextToken
		}
	}

	// invoke action
	fn := func(pageOutput *servicediscovery.GetInstancesHealthStatusOutput, lastPage bool) bool {
		if pageOutput != nil {
			moreNextToken = *pageOutput.NextToken
			m := aws.StringValueMap(pageOutput.Status)

			if status == nil {
				status = make(map[string]string)
			}

			for k, v := range m {
				status[k] = v
			}
		}

		return !lastPage
	}

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		err = sd.sdClient.GetInstancesHealthStatusPagesWithContext(ctx, input, fn)
	} else {
		if segCtxSet {
			err = sd.sdClient.GetInstancesHealthStatusPagesWithContext(segCtx, input, fn)
		} else {
			err = sd.sdClient.GetInstancesHealthStatusPages(input, fn)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap GetInstancesHealthStatusPages Failed: (ListPages Action) " + err.Error())
		return nil, "", err
	}

	return status, moreNextToken, nil
}

// DiscoverInstances discovers registered instances for a specified namespace and service
//
// Notes:
//  1. Used to discover instances for any type of namespace (http, private dns, public dns)
//  2. For public and private dns namespaces,
//     may also use dns queries to discover distances instead
//
// Parameters:
//  1. namespaceName = (required) name of the namespace to be discovered
//  2. serviceName = (required) name of the service to be discovered
//  3. isHealthy = (required) discover healthy or unhealthy instances
//  4. queryParameters = (optional) map of key value pairs, containing custom attributes registered during RegisterInstance,
//     if custom attributes is specified, all attributes in the queryParameters must match for the instance to discover
//  5. maxResults = (optional) max count of discovered instances to return, if not specified, up to 100 is returned
//  6. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. instances = slice of discovered instance objects
//  2. err = error info if any
func (sd *CloudMap) DiscoverInstances(namespaceName string,
	serviceName string,
	isHealthy bool,
	queryParameters map[string]string,
	maxResults *int64,
	timeOutDuration ...time.Duration) (instances []*servicediscovery.HttpInstanceSummary, err error) {
	segCtx := context.Background()
	segCtxSet := false

	log.Println("DiscoverInstances Entered")

	seg := xray.NewSegmentNullable("Cloudmap-DiscoverInstances", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-DiscoverInstances-NamespaceName", namespaceName)
			_ = seg.Seg.AddMetadata("Cloudmap-DiscoverInstances-ServiceName", serviceName)
			_ = seg.Seg.AddMetadata("Cloudmap-DiscoverInstances-IsHealthy", isHealthy)
			_ = seg.Seg.AddMetadata("Cloudmap-DiscoverInstances-QueryParameters", queryParameters)
			_ = seg.Seg.AddMetadata("Cloudmap-DiscoverInstances-MaxResults", maxResults)
			_ = seg.Seg.AddMetadata("Cloudmap-DiscoverInstances-Result-Instances", instances)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}
	log.Println("DiscoverInstances Segments Created")

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap DiscoverInstances Failed: " + "SD Client is Required")
		return nil, err
	}

	if util.LenTrim(namespaceName) == 0 {
		err = errors.New("CloudMap DiscoverInstances Failed: " + "Namespace Name is Required")
		return nil, err
	}

	if util.LenTrim(serviceName) == 0 {
		err = errors.New("CloudMap DiscoverInstances Failed: " + "Service Name is Required")
		return nil, err
	}
	log.Println("DiscoverInstances Validated")

	// define input
	healthStatus := ""

	if isHealthy {
		healthStatus = "HEALTHY"
	} else {
		healthStatus = "UNHEALTHY"
	}

	input := &servicediscovery.DiscoverInstancesInput{
		NamespaceName: aws.String(namespaceName),
		ServiceName:   aws.String(serviceName),
		HealthStatus:  aws.String(healthStatus),
	}

	if queryParameters != nil && len(queryParameters) > 0 {
		input.QueryParameters = aws.StringMap(queryParameters)
	}

	if maxResults != nil && *maxResults > 0 {
		input.MaxResults = maxResults
	}
	log.Println("DiscoverInstances params Input Defined")

	// invoke action
	var output *servicediscovery.DiscoverInstancesOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.DiscoverInstancesWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.DiscoverInstancesWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.DiscoverInstances(input)
		}
	}
	log.Println("DiscoverInstances Action Invoked")

	if err != nil {
		// handle error
		err = errors.New("CloudMap DiscoverInstances Failed: (Discover Action) " + err.Error())
		return nil, err
	}

	return output.Instances, nil
}

// ListInstances lists summary information about the instances registered using a specified service
//
// Parameters:
//  1. serviceId = (required) service id assciated with the instances being checked
//  2. maxResults = (optional) specifies maximum count to return
//  3. nextToken = (optional) if initial action, leave blank; if this is a subsequent action to get more, input the moreNextToken returned from a prior action
//  4. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. instances = slice of sd instance summary objects
//  2. moreNextToken = if more data exists, this token can be used in a subsequent action via nextToken parameter
//  3. err = error info if any
func (sd *CloudMap) ListInstances(serviceId string,
	maxResults *int64,
	nextToken *string,
	timeOutDuration ...time.Duration) (instances []*servicediscovery.InstanceSummary, moreNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-ListInstances", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-ListInstances-ServiceID", serviceId)
			_ = seg.Seg.AddMetadata("Cloudmap-ListInstances-MaxResults", maxResults)
			_ = seg.Seg.AddMetadata("Cloudmap-ListInstances-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("Cloudmap-ListInstances-Result-Instances", instances)
			_ = seg.Seg.AddMetadata("Cloudmap-ListInstances-Result-NextToken", moreNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap ListInstances Failed: " + "SD Client is Required")
		return nil, "", err
	}

	if util.LenTrim(serviceId) == 0 {
		err = errors.New("CloudMap ListInstances Failed: " + "Service ID is Required")
		return nil, "", err
	}

	if maxResults != nil {
		if *maxResults <= 0 {
			err = errors.New("CloudMap ListInstances Failed: " + "MaxResults Must Be Greater Than Zero")
			return nil, "", err
		}
	}

	// define input
	input := &servicediscovery.ListInstancesInput{
		ServiceId: aws.String(serviceId),
	}

	if maxResults != nil {
		input.MaxResults = maxResults
	}

	if nextToken != nil {
		if util.LenTrim(*nextToken) > 0 {
			input.NextToken = nextToken
		}
	}

	// invoke action
	var output *servicediscovery.ListInstancesOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.ListInstancesWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.ListInstancesWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.ListInstances(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap ListInstances Failed: (List Action) " + err.Error())
		return nil, "", err
	}

	return output.Instances, *output.NextToken, nil
}

// ListInstancesPages lists summary information about the instances registered using a specified service
// (issues multiple page requests until max results is met or all data is retrieved)
//
// Parameters:
//  1. serviceId = (required) service id assciated with the instances being checked
//  2. maxResults = (optional) specifies maximum count to return
//  3. nextToken = (optional) if initial action, leave blank; if this is a subsequent action to get more, input the moreNextToken returned from a prior action
//  4. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. instances = slice of sd instance summary objects
//  2. moreNextToken = if more data exists, this token can be used in a subsequent action via nextToken parameter
//  3. err = error info if any
func (sd *CloudMap) ListInstancesPages(serviceId string,
	maxResults *int64,
	nextToken *string,
	timeOutDuration ...time.Duration) (instances []*servicediscovery.InstanceSummary, moreNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-ListInstancesPages", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-ListInstancesPages-ServiceID", serviceId)
			_ = seg.Seg.AddMetadata("Cloudmap-ListInstancesPages-MaxResults", maxResults)
			_ = seg.Seg.AddMetadata("Cloudmap-ListInstancesPages-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("Cloudmap-ListInstancesPages-Result-Instances", instances)
			_ = seg.Seg.AddMetadata("Cloudmap-ListInstancesPages-Result-NextToken", moreNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap ListInstancesPages Failed: " + "SD Client is Required")
		return nil, "", err
	}

	if util.LenTrim(serviceId) == 0 {
		err = errors.New("CloudMap ListInstancesPages Failed: " + "Service ID is Required")
		return nil, "", err
	}

	if maxResults != nil {
		if *maxResults <= 0 {
			err = errors.New("CloudMap ListInstancesPages Failed: " + "MaxResults Must Be Greater Than Zero")
			return nil, "", err
		}
	}

	// define input
	input := &servicediscovery.ListInstancesInput{
		ServiceId: aws.String(serviceId),
	}

	if maxResults != nil {
		input.MaxResults = maxResults
	}

	if nextToken != nil {
		if util.LenTrim(*nextToken) > 0 {
			input.NextToken = nextToken
		}
	}

	// invoke action
	fn := func(pageOutput *servicediscovery.ListInstancesOutput, lastPage bool) bool {
		if pageOutput != nil {
			moreNextToken = *pageOutput.NextToken
			instances = append(instances, pageOutput.Instances...)
		}

		return !lastPage
	}

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		err = sd.sdClient.ListInstancesPagesWithContext(ctx, input, fn)
	} else {
		if segCtxSet {
			err = sd.sdClient.ListInstancesPagesWithContext(segCtx, input, fn)
		} else {
			err = sd.sdClient.ListInstancesPages(input, fn)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap ListInstancesPages Failed: (ListPages Action) " + err.Error())
		return nil, "", err
	}

	return instances, moreNextToken, nil
}

// ----------------------------------------------------------------------------------------------------------------
// operation functions
// ----------------------------------------------------------------------------------------------------------------

// GetOperation gets information about any operation that returned an operationId in the response,
//
//	such as CreateHttpNamespace(), CreateService(), etc
//
// Parameters:
//  1. operationId = (required) the operation to retrieve, operationId is obtained during Create, and other related actions
//  2. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. operation = operation object retrieved
//     a) Targets = evaluate Targets to retrieve namespaceId, serviceId, InstanceId etc, using NAMESPACE, SERVICE, INSTANCE key names
//  2. err = error info any
func (sd *CloudMap) GetOperation(operationId string, timeOutDuration ...time.Duration) (operation *servicediscovery.Operation, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-GetOperation", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-GetOperation-OperationID", operationId)
			_ = seg.Seg.AddMetadata("Cloudmap-GetOperation-Result-Operation", operation)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap GetOperation Failed: " + "SD Client is Required")
		return nil, err
	}

	if util.LenTrim(operationId) == 0 {
		err = errors.New("CloudMap GetOperation Failed: " + "OperationId is Required")
		return nil, err
	}

	// define input
	input := &servicediscovery.GetOperationInput{
		OperationId: aws.String(operationId),
	}

	// invoke action
	var output *servicediscovery.GetOperationOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.GetOperationWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.GetOperationWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.GetOperation(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap GetOperation Failed: (Get Action) " + err.Error())
		return nil, err
	}

	return output.Operation, nil
}

// ListOperations lists operations that match the criteria specified in parameters
//
// Parameters:
//  1. filter = (optional) map of filter operations (EQ_ filters allow single value per key)
//     a) EQ_Status / IN_Status = Valid Values: SUBMITTED, PENDING, SUCCEED, FAIL
//     b) EQ_Type / IN_Type = Valid Values: CREATE_NAMESPACE, DELETE_NAMESPACE, UPDATE_SERVICE, REGISTER_INSTANCE, DEREGISTER_INSTANCE
//     c) BETWEEN_UpdateDate = begin and end in Unix DateTime in UTC
//  2. maxResults = (optional) specifies maximum count to return
//  3. nextToken = (optional) if initial action, leave blank; if this is a subsequent action to get more, input the moreNextToken returned from a prior action
//  4. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. operations = slice of sd operation summary objects
//     a) Targets = evaluate Targets to retrieve namespaceId, serviceId, InstanceId etc, using NAMESPACE, SERVICE, INSTANCE key names
//  2. moreNextToken = if more data exists, this token can be used in a subsequent action via nextToken parameter
//  3. err = error info if any
func (sd *CloudMap) ListOperations(filter map[sdoperationfilter.SdOperationFilter][]string,
	maxResults *int64,
	nextToken *string,
	timeOutDuration ...time.Duration) (operations []*servicediscovery.OperationSummary, moreNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-ListOperations", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-ListOperations-Filter", filter)
			_ = seg.Seg.AddMetadata("Cloudmap-ListOperations-MaxResults", maxResults)
			_ = seg.Seg.AddMetadata("Cloudmap-ListOperations-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("Cloudmap-ListOperations-Result-Operations", operations)
			_ = seg.Seg.AddMetadata("Cloudmap-ListOperations-Result-NextToken", moreNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap ListOperations Failed: " + "SD Client is Required")
		return nil, "", err
	}

	if maxResults != nil {
		if *maxResults <= 0 {
			err = errors.New("CloudMap ListOperations Failed: " + "MaxResults Must Be Greater Than Zero")
			return nil, "", err
		}
	}

	// define input
	input := &servicediscovery.ListOperationsInput{}

	if filter != nil {
		var opFilters []*servicediscovery.OperationFilter

		for fk, fv := range filter {
			var sdof *servicediscovery.OperationFilter

			switch fk {
			case sdoperationfilter.EQ_NameSpaceID:
				if len(fv) == 1 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("NAMESPACE_ID"),
						Condition: aws.String("EQ"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.EQ_ServiceID:
				if len(fv) == 1 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("SERVICE_ID"),
						Condition: aws.String("EQ"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.EQ_Status:
				if len(fv) == 1 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("STATUS"),
						Condition: aws.String("EQ"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.EQ_Type:
				if len(fv) == 1 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("TYPE"),
						Condition: aws.String("EQ"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.IN_Status:
				if len(fv) > 0 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("STATUS"),
						Condition: aws.String("IN"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.IN_Type:
				if len(fv) > 0 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("TYPE"),
						Condition: aws.String("IN"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.BETWEEN_UpdateDate:
				if len(fv) == 2 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("UPDATE_DATE"),
						Condition: aws.String("BETWEEN"),
						Values:    aws.StringSlice(fv),
					}
				}
			}

			if sdof != nil {
				opFilters = append(opFilters, sdof)
			}
		}

		if len(opFilters) > 0 {
			input.Filters = opFilters
		}
	}

	if maxResults != nil {
		input.MaxResults = maxResults
	}

	if nextToken != nil {
		if util.LenTrim(*nextToken) > 0 {
			input.NextToken = nextToken
		}
	}

	// invoke action
	var output *servicediscovery.ListOperationsOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = sd.sdClient.ListOperationsWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = sd.sdClient.ListOperationsWithContext(segCtx, input)
		} else {
			output, err = sd.sdClient.ListOperations(input)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap ListOperations Failed: (List Action) " + err.Error())
		return nil, "", err
	}

	return output.Operations, *output.NextToken, nil
}

// ListOperationsPages lists operations that match the criteria specified in parameters
// (issues multiple page requests until max results is met or all data is retrieved)
//
// Parameters:
//  1. filter = (optional) map of filter operations (EQ_ filters allow single value per key)
//     a) EQ_Status / IN_Status = Valid Values: SUBMITTED, PENDING, SUCCEED, FAIL
//     b) EQ_Type / IN_Type = Valid Values: CREATE_NAMESPACE, DELETE_NAMESPACE, UPDATE_SERVICE, REGISTER_INSTANCE, DEREGISTER_INSTANCE
//     c) BETWEEN_UpdateDate = begin and end in Unix DateTime in UTC
//  2. maxResults = (optional) specifies maximum count to return
//  3. nextToken = (optional) if initial action, leave blank; if this is a subsequent action to get more, input the moreNextToken returned from a prior action
//  4. timeOutDuration = (optional) maximum time before timeout via context
//
// Return Values:
//  1. operations = slice of sd operation summary objects
//     a) Targets = evaluate Targets to retrieve namespaceId, serviceId, InstanceId etc, using NAMESPACE, SERVICE, INSTANCE key names
//  2. moreNextToken = if more data exists, this token can be used in a subsequent action via nextToken parameter
//  3. err = error info if any
func (sd *CloudMap) ListOperationsPages(filter map[sdoperationfilter.SdOperationFilter][]string,
	maxResults *int64,
	nextToken *string,
	timeOutDuration ...time.Duration) (operations []*servicediscovery.OperationSummary, moreNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cloudmap-ListOperationsPages", sd._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cloudmap-ListOperationsPages-Filter", filter)
			_ = seg.Seg.AddMetadata("Cloudmap-ListOperationsPages-MaxResults", maxResults)
			_ = seg.Seg.AddMetadata("Cloudmap-ListOperationsPages-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("Cloudmap-ListOperationsPages-Result-Operations", operations)
			_ = seg.Seg.AddMetadata("Cloudmap-ListOperationsPages-Result-NextToken", moreNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if sd.sdClient == nil {
		err = errors.New("CloudMap ListOperationsPages Failed: " + "SD Client is Required")
		return nil, "", err
	}

	if maxResults != nil {
		if *maxResults <= 0 {
			err = errors.New("CloudMap ListOperationsPages Failed: " + "MaxResults Must Be Greater Than Zero")
			return nil, "", err
		}
	}

	// define input
	input := &servicediscovery.ListOperationsInput{}

	if filter != nil {
		var opFilters []*servicediscovery.OperationFilter

		for fk, fv := range filter {
			var sdof *servicediscovery.OperationFilter

			switch fk {
			case sdoperationfilter.EQ_NameSpaceID:
				if len(fv) == 1 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("NAMESPACE_ID"),
						Condition: aws.String("EQ"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.EQ_ServiceID:
				if len(fv) == 1 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("SERVICE_ID"),
						Condition: aws.String("EQ"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.EQ_Status:
				if len(fv) == 1 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("STATUS"),
						Condition: aws.String("EQ"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.EQ_Type:
				if len(fv) == 1 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("TYPE"),
						Condition: aws.String("EQ"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.IN_Status:
				if len(fv) > 0 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("STATUS"),
						Condition: aws.String("IN"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.IN_Type:
				if len(fv) > 0 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("TYPE"),
						Condition: aws.String("IN"),
						Values:    aws.StringSlice(fv),
					}
				}
			case sdoperationfilter.BETWEEN_UpdateDate:
				if len(fv) == 2 {
					sdof = &servicediscovery.OperationFilter{
						Name:      aws.String("UPDATE_DATE"),
						Condition: aws.String("BETWEEN"),
						Values:    aws.StringSlice(fv),
					}
				}
			}

			if sdof != nil {
				opFilters = append(opFilters, sdof)
			}
		}

		if len(opFilters) > 0 {
			input.Filters = opFilters
		}
	}

	if maxResults != nil {
		input.MaxResults = maxResults
	}

	if nextToken != nil {
		if util.LenTrim(*nextToken) > 0 {
			input.NextToken = nextToken
		}
	}

	// invoke action
	fn := func(pageOutput *servicediscovery.ListOperationsOutput, lastPage bool) bool {
		if pageOutput != nil {
			moreNextToken = *pageOutput.NextToken
			operations = append(operations, pageOutput.Operations...)
		}

		return !lastPage
	}

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		err = sd.sdClient.ListOperationsPagesWithContext(ctx, input, fn)
	} else {
		if segCtxSet {
			err = sd.sdClient.ListOperationsPagesWithContext(segCtx, input, fn)
		} else {
			err = sd.sdClient.ListOperationsPages(input, fn)
		}
	}

	if err != nil {
		// handle error
		err = errors.New("CloudMap ListOperationsPages Failed: (ListPages Action) " + err.Error())
		return nil, "", err
	}

	return operations, moreNextToken, nil
}
