package route53

import (
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	awsroute53 "github.com/aws/aws-sdk-go/service/route53"
)

const (
	localstackEndpoint = "http://localhost:4566"
	testRegion         = "us-east-1"
	testDomain         = "test.localstack.internal"
	testRecordName     = "app.test.localstack.internal"
	testIP             = "192.168.1.100"
	testTTL            = uint(60)
)

// localstackAvailable returns true if LocalStack is reachable on port 4566.
func localstackAvailable() bool {
	conn, err := net.DialTimeout("tcp", "localhost:4566", 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// setTestEnv configures AWS credentials and region environment variables for LocalStack.
func setTestEnv() {
	os.Setenv("AWS_ACCESS_KEY_ID", "test")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	os.Setenv("AWS_DEFAULT_REGION", testRegion)
	os.Setenv("AWS_REGION", testRegion)
}

// newTestRoute53 returns a Route53 struct configured for LocalStack.
func newTestRoute53(t *testing.T) *Route53 {
	t.Helper()
	return &Route53{
		CustomEndpoint: localstackEndpoint,
	}
}

// newVerificationClient creates a raw AWS SDK Route53 client for verification queries.
func newVerificationClient(t *testing.T) *awsroute53.Route53 {
	t.Helper()
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(testRegion),
		Endpoint:    aws.String(localstackEndpoint),
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
	})
	if err != nil {
		t.Fatalf("failed to create verification session: %v", err)
	}
	return awsroute53.New(sess)
}

// createHostedZone creates a hosted zone and returns the zone ID (without /hostedzone/ prefix).
func createHostedZone(t *testing.T, client *awsroute53.Route53, domain string) string {
	t.Helper()
	out, err := client.CreateHostedZone(&awsroute53.CreateHostedZoneInput{
		Name:            aws.String(domain),
		CallerReference: aws.String("test-ref-" + time.Now().Format("20060102150405.000")),
	})
	if err != nil {
		t.Fatalf("failed to create hosted zone: %v", err)
	}

	// The hosted zone ID is returned as "/hostedzone/ZXXXXX". Extract just the ID part.
	zoneID := aws.StringValue(out.HostedZone.Id)
	zoneID = strings.TrimPrefix(zoneID, "/hostedzone/")
	return zoneID
}

// deleteHostedZone deletes a hosted zone by ID.
func deleteHostedZone(t *testing.T, client *awsroute53.Route53, zoneID string) {
	t.Helper()
	_, err := client.DeleteHostedZone(&awsroute53.DeleteHostedZoneInput{
		Id: aws.String(zoneID),
	})
	if err != nil {
		t.Errorf("failed to delete hosted zone %s: %v", zoneID, err)
	}
}

// listRecordSets lists all record sets in a hosted zone.
func listRecordSets(t *testing.T, client *awsroute53.Route53, zoneID string) []*awsroute53.ResourceRecordSet {
	t.Helper()
	out, err := client.ListResourceRecordSets(&awsroute53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
	})
	if err != nil {
		t.Fatalf("failed to list record sets: %v", err)
	}
	return out.ResourceRecordSets
}

// findARecord searches for an A record matching the given name and IP.
func findARecord(records []*awsroute53.ResourceRecordSet, name, ip string) bool {
	// Route53 returns names with a trailing dot.
	if !strings.HasSuffix(name, ".") {
		name = name + "."
	}
	for _, r := range records {
		if aws.StringValue(r.Type) == "A" && aws.StringValue(r.Name) == name {
			for _, rr := range r.ResourceRecords {
				if aws.StringValue(rr.Value) == ip {
					return true
				}
			}
		}
	}
	return false
}

func TestConnect(t *testing.T) {
	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}
	setTestEnv()

	r := newTestRoute53(t)
	err := r.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer r.Disconnect()
}

func TestCreateAndListARecord(t *testing.T) {
	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}
	setTestEnv()

	verifyCli := newVerificationClient(t)
	zoneID := createHostedZone(t, verifyCli, testDomain)

	r := newTestRoute53(t)
	err := r.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer r.Disconnect()

	// Create an A record.
	err = r.CreateUpdateResourceRecordset(zoneID, testRecordName, testIP, testTTL, "A")
	if err != nil {
		t.Fatalf("CreateUpdateResourceRecordset failed: %v", err)
	}

	// Verify the A record appears in the zone.
	records := listRecordSets(t, verifyCli, zoneID)
	if !findARecord(records, testRecordName, testIP) {
		t.Fatalf("A record for %s -> %s not found in zone %s; records: %+v",
			testRecordName, testIP, zoneID, records)
	}

	// Clean up: delete the A record before deleting the zone.
	err = r.DeleteResourceRecordset(zoneID, testRecordName, testIP, testTTL, "A")
	if err != nil {
		t.Fatalf("cleanup DeleteResourceRecordset failed: %v", err)
	}
	deleteHostedZone(t, verifyCli, zoneID)
}

func TestDeleteARecord(t *testing.T) {
	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}
	setTestEnv()

	verifyCli := newVerificationClient(t)
	zoneID := createHostedZone(t, verifyCli, testDomain)
	defer deleteHostedZone(t, verifyCli, zoneID)

	r := newTestRoute53(t)
	err := r.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer r.Disconnect()

	// Create the A record first.
	err = r.CreateUpdateResourceRecordset(zoneID, testRecordName, testIP, testTTL, "A")
	if err != nil {
		t.Fatalf("CreateUpdateResourceRecordset failed: %v", err)
	}

	// Delete the A record.
	err = r.DeleteResourceRecordset(zoneID, testRecordName, testIP, testTTL, "A")
	if err != nil {
		t.Fatalf("DeleteResourceRecordset failed: %v", err)
	}

	// Verify the A record is gone.
	records := listRecordSets(t, verifyCli, zoneID)
	if findARecord(records, testRecordName, testIP) {
		t.Fatalf("A record for %s -> %s should have been deleted but still exists",
			testRecordName, testIP)
	}
}

func TestCreateRecordInNonExistentZone(t *testing.T) {
	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}
	setTestEnv()

	r := newTestRoute53(t)
	err := r.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer r.Disconnect()

	// Use a fake hosted zone ID that does not exist.
	err = r.CreateUpdateResourceRecordset("Z000NONEXISTENT", "fake.example.com", "10.0.0.1", 60, "A")
	if err == nil {
		t.Fatal("expected error when creating record in non-existent zone, got nil")
	}
}

func TestDeleteHostedZoneLifecycle(t *testing.T) {
	if !localstackAvailable() {
		t.Skip("LocalStack not available at localhost:4566")
	}
	setTestEnv()

	verifyCli := newVerificationClient(t)

	// Create a hosted zone.
	zoneID := createHostedZone(t, verifyCli, "lifecycle.localstack.internal")

	// Create a record, then delete it, then delete the zone.
	r := newTestRoute53(t)
	err := r.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer r.Disconnect()

	recordName := "svc.lifecycle.localstack.internal"
	recordIP := "10.20.30.40"

	err = r.CreateUpdateResourceRecordset(zoneID, recordName, recordIP, 60, "A")
	if err != nil {
		t.Fatalf("CreateUpdateResourceRecordset failed: %v", err)
	}

	err = r.DeleteResourceRecordset(zoneID, recordName, recordIP, 60, "A")
	if err != nil {
		t.Fatalf("DeleteResourceRecordset failed: %v", err)
	}

	// Delete the hosted zone itself.
	deleteHostedZone(t, verifyCli, zoneID)

	// Verify the zone is gone by trying to list records -- should fail.
	_, listErr := verifyCli.ListResourceRecordSets(&awsroute53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(zoneID),
	})
	if listErr == nil {
		t.Fatal("expected error listing records in deleted zone, got nil")
	}
}
