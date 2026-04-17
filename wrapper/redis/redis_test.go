package redis

/*
 * Integration tests for the Redis wrapper.
 *
 * Prerequisites:
 *   - Redis 7 server running on localhost:6379 (no password, no TLS)
 *   - Run with: go test -race -count=1 -timeout=30s -v ./wrapper/redis/
 *
 * All tests use the key prefix "test:redis:" and clean up after themselves.
 */

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aldelo/common/wrapper/redis/redisdatatype"
)

// testKeyPrefix ensures test keys are isolated from production data.
const testKeyPrefix = "test:redis:"

// connectTestRedis creates a connected Redis instance for testing.
// It skips the test if Redis is not available on localhost:6379.
func connectTestRedis(t *testing.T) *Redis {
	t.Helper()

	r := &Redis{
		AwsRedisWriterEndpoint: "localhost:6379",
		AwsRedisReaderEndpoint: "localhost:6379",
	}

	if err := r.Connect(); err != nil {
		t.Skipf("Redis not available, skipping integration test: %v", err)
	}

	return r
}

// cleanupKeys deletes the specified keys before AND after the test.
// Pre-delete ensures idempotency if a prior run left keys behind.
// Post-delete ensures cleanup even if the test fails partway through.
func cleanupKeys(t *testing.T, r *Redis, keys ...string) {
	t.Helper()
	if len(keys) > 0 {
		_, _ = r.Del(keys...) // pre-delete
		t.Cleanup(func() {
			_, _ = r.Del(keys...) // post-delete (best-effort)
		})
	}
}

// ================================================================================================================
// 1. Connection lifecycle
// ================================================================================================================

func TestConnect_Success(t *testing.T) {
	r := &Redis{
		AwsRedisWriterEndpoint: "localhost:6379",
		AwsRedisReaderEndpoint: "localhost:6379",
	}

	err := r.Connect()
	if err != nil {
		t.Skipf("Redis not available, skipping: %v", err)
	}

	// Verify sub-structs are initialized
	if r.UTILS == nil {
		t.Fatal("expected UTILS to be initialized after Connect")
	}
	if r.HASH == nil {
		t.Fatal("expected HASH to be initialized after Connect")
	}
	if r.LIST == nil {
		t.Fatal("expected LIST to be initialized after Connect")
	}
	if r.SET == nil {
		t.Fatal("expected SET to be initialized after Connect")
	}
	if r.TTL == nil {
		t.Fatal("expected TTL to be initialized after Connect")
	}

	r.Disconnect()
}

func TestConnect_MissingWriterEndpoint(t *testing.T) {
	r := &Redis{
		AwsRedisReaderEndpoint: "localhost:6379",
	}

	err := r.Connect()
	if err == nil {
		r.Disconnect()
		t.Fatal("expected error when writer endpoint is missing, got nil")
	}
}

func TestConnect_MissingReaderEndpoint(t *testing.T) {
	r := &Redis{
		AwsRedisWriterEndpoint: "localhost:6379",
	}

	err := r.Connect()
	if err == nil {
		r.Disconnect()
		t.Fatal("expected error when reader endpoint is missing, got nil")
	}
}

func TestConnect_InvalidHost(t *testing.T) {
	r := &Redis{
		AwsRedisWriterEndpoint: "localhost:19999",
		AwsRedisReaderEndpoint: "localhost:19999",
	}

	err := r.Connect()
	if err == nil {
		r.Disconnect()
		t.Fatal("expected error when connecting to invalid host, got nil")
	}
}

func TestPing(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	err := r.UTILS.Ping()
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

// ================================================================================================================
// 2. Basic string operations: SET / GET / DELETE
// ================================================================================================================

func TestSet_Get_Del_String(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "string:basic"
	cleanupKeys(t, r, key)

	// SET
	err := r.Set(key, "hello-redis")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// GET
	val, notFound, err := r.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if notFound {
		t.Fatal("expected key to exist, got notFound=true")
	}
	if val != "hello-redis" {
		t.Fatalf("expected 'hello-redis', got %q", val)
	}

	// DEL
	deletedCount, err := r.Del(key)
	if err != nil {
		t.Fatalf("Del failed: %v", err)
	}
	if deletedCount != 1 {
		t.Fatalf("expected deletedCount=1, got %d", deletedCount)
	}

	// Verify deleted
	_, notFound, err = r.Get(key)
	if err != nil {
		t.Fatalf("Get after Del failed: %v", err)
	}
	if !notFound {
		t.Fatal("expected notFound=true after Del")
	}
}

func TestSetInt_GetInt(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "int:basic"
	cleanupKeys(t, r, key)

	err := r.SetInt(key, 42)
	if err != nil {
		t.Fatalf("SetInt failed: %v", err)
	}

	val, notFound, err := r.GetInt(key)
	if err != nil {
		t.Fatalf("GetInt failed: %v", err)
	}
	if notFound {
		t.Fatal("expected key to exist")
	}
	if val != 42 {
		t.Fatalf("expected 42, got %d", val)
	}
}

func TestSetBool_GetBool(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "bool:basic"
	cleanupKeys(t, r, key)

	err := r.SetBool(key, true)
	if err != nil {
		t.Fatalf("SetBool failed: %v", err)
	}

	val, notFound, err := r.GetBool(key)
	if err != nil {
		t.Fatalf("GetBool failed: %v", err)
	}
	if notFound {
		t.Fatal("expected key to exist")
	}
	if val != true {
		t.Fatalf("expected true, got %v", val)
	}
}

func TestGet_NonExistentKey(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "nonexistent:key:12345"

	val, notFound, err := r.Get(key)
	if err != nil {
		t.Fatalf("Get on non-existent key should not error, got: %v", err)
	}
	if !notFound {
		t.Fatal("expected notFound=true for non-existent key")
	}
	if val != "" {
		t.Fatalf("expected empty string for non-existent key, got %q", val)
	}
}

// ================================================================================================================
// 3. Key operations: EXISTS, DEL multiple, Append
// ================================================================================================================

func TestExists(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "exists:check"
	cleanupKeys(t, r, key)

	// Before creation
	count, err := r.Exists(key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count=0 for non-existent key, got %d", count)
	}

	// After creation
	if err := r.Set(key, "value"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	count, err = r.Exists(key)
	if err != nil {
		t.Fatalf("Exists after Set failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count=1 for existing key, got %d", count)
	}
}

func TestDel_MultipleKeys(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key1 := testKeyPrefix + "del:multi:1"
	key2 := testKeyPrefix + "del:multi:2"
	key3 := testKeyPrefix + "del:multi:3"
	cleanupKeys(t, r, key1, key2, key3)

	_ = r.Set(key1, "a")
	_ = r.Set(key2, "b")
	_ = r.Set(key3, "c")

	deletedCount, err := r.Del(key1, key2, key3)
	if err != nil {
		t.Fatalf("Del multiple failed: %v", err)
	}
	if deletedCount != 3 {
		t.Fatalf("expected deletedCount=3, got %d", deletedCount)
	}
}

func TestAppend(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "append:test"
	cleanupKeys(t, r, key)

	// Append to non-existent key creates it
	err := r.Append(key, "hello")
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	err = r.Append(key, " world")
	if err != nil {
		t.Fatalf("Append (second) failed: %v", err)
	}

	val, notFound, err := r.Get(key)
	if err != nil {
		t.Fatalf("Get after Append failed: %v", err)
	}
	if notFound {
		t.Fatal("expected key to exist after Append")
	}
	if val != "hello world" {
		t.Fatalf("expected 'hello world', got %q", val)
	}
}

// ================================================================================================================
// 4. TTL / Expiration
// ================================================================================================================

func TestSet_WithTTL(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "ttl:expire"
	cleanupKeys(t, r, key)

	// Set with 10-second TTL
	err := r.Set(key, "expires-soon", 10*time.Second)
	if err != nil {
		t.Fatalf("Set with TTL failed: %v", err)
	}

	// Verify TTL is set (in seconds)
	ttlVal, notFound, err := r.TTL.TTL(key, false)
	if err != nil {
		t.Fatalf("TTL check failed: %v", err)
	}
	if notFound {
		t.Fatal("expected key to exist for TTL check")
	}
	// TTL should be between 1 and 10 seconds
	if ttlVal < 1 || ttlVal > 10 {
		t.Fatalf("expected TTL between 1 and 10 seconds, got %d", ttlVal)
	}
}

func TestTTL_Expire(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "ttl:expire:cmd"
	cleanupKeys(t, r, key)

	// Create key without TTL
	err := r.Set(key, "no-ttl")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Apply TTL via Expire
	err = r.TTL.Expire(key, false, 15*time.Second)
	if err != nil {
		t.Fatalf("Expire failed: %v", err)
	}

	// Verify TTL was applied
	ttlVal, _, err := r.TTL.TTL(key, false)
	if err != nil {
		t.Fatalf("TTL check failed: %v", err)
	}
	if ttlVal < 1 || ttlVal > 15 {
		t.Fatalf("expected TTL between 1 and 15 seconds, got %d", ttlVal)
	}
}

func TestTTL_Persist(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "ttl:persist"
	cleanupKeys(t, r, key)

	// Create key with TTL
	err := r.Set(key, "will-persist", 60*time.Second)
	if err != nil {
		t.Fatalf("Set with TTL failed: %v", err)
	}

	// Remove TTL
	err = r.TTL.Persist(key)
	if err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// TTL should be -1 (no expiration) — go-redis returns -1 for keys without TTL
	ttlVal, _, err := r.TTL.TTL(key, false)
	if err != nil {
		t.Fatalf("TTL check after Persist failed: %v", err)
	}
	if ttlVal != -1 {
		t.Fatalf("expected TTL=-1 after Persist, got %d", ttlVal)
	}
}

// ================================================================================================================
// 5. Hash operations
// ================================================================================================================

func TestHash_HSet_HGet_HDel(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "hash:basic"
	cleanupKeys(t, r, key)

	// HSet field-value pairs
	err := r.HASH.HSet(key, "field1", "value1", "field2", "value2")
	if err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	// HGet
	var result string
	notFound, err := r.HASH.HGet(key, "field1", redisdatatype.String, &result)
	if err != nil {
		t.Fatalf("HGet failed: %v", err)
	}
	if notFound {
		t.Fatal("expected field1 to exist")
	}
	if result != "value1" {
		t.Fatalf("expected 'value1', got %q", result)
	}

	// HGetAll
	allFields, notFound, err := r.HASH.HGetAll(key)
	if err != nil {
		t.Fatalf("HGetAll failed: %v", err)
	}
	if notFound {
		t.Fatal("expected hash to exist")
	}
	if len(allFields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(allFields))
	}
	if allFields["field1"] != "value1" || allFields["field2"] != "value2" {
		t.Fatalf("unexpected field values: %v", allFields)
	}

	// HDel
	deletedCount, err := r.HASH.HDel(key, "field1")
	if err != nil {
		t.Fatalf("HDel failed: %v", err)
	}
	if deletedCount != 1 {
		t.Fatalf("expected deletedCount=1, got %d", deletedCount)
	}

	// Verify field1 is deleted
	notFound, err = r.HASH.HGet(key, "field1", redisdatatype.String, &result)
	if err != nil {
		t.Fatalf("HGet after HDel failed: %v", err)
	}
	if !notFound {
		t.Fatal("expected field1 to be not found after HDel")
	}
}

func TestHash_HExists(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "hash:exists"
	cleanupKeys(t, r, key)

	err := r.HASH.HSet(key, "present", "yes")
	if err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	exists, err := r.HASH.HExists(key, "present")
	if err != nil {
		t.Fatalf("HExists failed: %v", err)
	}
	if !exists {
		t.Fatal("expected field 'present' to exist")
	}

	exists, err = r.HASH.HExists(key, "absent")
	if err != nil {
		t.Fatalf("HExists for absent field failed: %v", err)
	}
	if exists {
		t.Fatal("expected field 'absent' to not exist")
	}
}

// ================================================================================================================
// 6. List operations
// ================================================================================================================

func TestList_LPush_LPop(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "list:pushpop"
	cleanupKeys(t, r, key)

	// LPush three elements: after push, list is [c, b, a]
	err := r.LIST.LPush(key, false, "a", "b", "c")
	if err != nil {
		t.Fatalf("LPush failed: %v", err)
	}

	// LLen
	length, notFound, err := r.LIST.LLen(key)
	if err != nil {
		t.Fatalf("LLen failed: %v", err)
	}
	if notFound {
		t.Fatal("expected list to exist")
	}
	if length != 3 {
		t.Fatalf("expected length=3, got %d", length)
	}

	// LPop returns first element (head)
	var val string
	notFound, err = r.LIST.LPop(key, redisdatatype.String, &val)
	if err != nil {
		t.Fatalf("LPop failed: %v", err)
	}
	if notFound {
		t.Fatal("expected element to be found")
	}
	if val != "c" {
		t.Fatalf("expected 'c' from LPop, got %q", val)
	}

	// RPop returns last element (tail)
	notFound, err = r.LIST.RPop(key, redisdatatype.String, &val)
	if err != nil {
		t.Fatalf("RPop failed: %v", err)
	}
	if notFound {
		t.Fatal("expected element to be found")
	}
	if val != "a" {
		t.Fatalf("expected 'a' from RPop, got %q", val)
	}
}

func TestList_LRange(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "list:range"
	cleanupKeys(t, r, key)

	// RPush preserves insertion order: [x, y, z]
	err := r.LIST.RPush(key, false, "x", "y", "z")
	if err != nil {
		t.Fatalf("RPush failed: %v", err)
	}

	// LRange 0 -1 returns all elements
	elems, notFound, err := r.LIST.LRange(key, 0, -1)
	if err != nil {
		t.Fatalf("LRange failed: %v", err)
	}
	if notFound {
		t.Fatal("expected list to exist")
	}
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(elems))
	}
	if elems[0] != "x" || elems[1] != "y" || elems[2] != "z" {
		t.Fatalf("expected [x, y, z], got %v", elems)
	}
}

// ================================================================================================================
// 7. Set operations
// ================================================================================================================

func TestSet_SAdd_SMembers_SIsMember(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "set:basic"
	cleanupKeys(t, r, key)

	// SAdd
	err := r.SET.SAdd(key, "alpha", "beta", "gamma")
	if err != nil {
		t.Fatalf("SAdd failed: %v", err)
	}

	// SCard
	count, notFound, err := r.SET.SCard(key)
	if err != nil {
		t.Fatalf("SCard failed: %v", err)
	}
	if notFound {
		t.Fatal("expected set to exist")
	}
	if count != 3 {
		t.Fatalf("expected SCard=3, got %d", count)
	}

	// SIsMember
	isMember, err := r.SET.SIsMember(key, "beta")
	if err != nil {
		t.Fatalf("SIsMember failed: %v", err)
	}
	if !isMember {
		t.Fatal("expected 'beta' to be a member")
	}

	isMember, err = r.SET.SIsMember(key, "delta")
	if err != nil {
		t.Fatalf("SIsMember for non-member failed: %v", err)
	}
	if isMember {
		t.Fatal("expected 'delta' to not be a member")
	}

	// SMembers
	members, notFound, err := r.SET.SMembers(key)
	if err != nil {
		t.Fatalf("SMembers failed: %v", err)
	}
	if notFound {
		t.Fatal("expected set to exist")
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}

	// SRem
	err = r.SET.SRem(key, "gamma")
	if err != nil {
		t.Fatalf("SRem failed: %v", err)
	}
	count, _, _ = r.SET.SCard(key)
	if count != 2 {
		t.Fatalf("expected SCard=2 after SRem, got %d", count)
	}
}

// ================================================================================================================
// 8. JSON round-trip
// ================================================================================================================

func TestSetJson_GetJson(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key := testKeyPrefix + "json:roundtrip"
	cleanupKeys(t, r, key)

	type sample struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	input := sample{Name: "test-item", Value: 99}

	err := r.SetJson(key, input)
	if err != nil {
		t.Fatalf("SetJson failed: %v", err)
	}

	var output sample
	notFound, err := r.GetJson(key, &output)
	if err != nil {
		t.Fatalf("GetJson failed: %v", err)
	}
	if notFound {
		t.Fatal("expected key to exist")
	}
	if output.Name != input.Name || output.Value != input.Value {
		t.Fatalf("expected %+v, got %+v", input, output)
	}
}

// ================================================================================================================
// 9. UTILS: Keys pattern, Scan, Type
// ================================================================================================================

func TestUtils_Keys_Pattern(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key1 := testKeyPrefix + "pattern:a"
	key2 := testKeyPrefix + "pattern:b"
	cleanupKeys(t, r, key1, key2)

	_ = r.Set(key1, "1")
	_ = r.Set(key2, "2")

	keys, notFound, err := r.UTILS.Keys(testKeyPrefix + "pattern:*")
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}
	if notFound {
		t.Fatal("expected pattern to match keys")
	}
	if len(keys) < 2 {
		t.Fatalf("expected at least 2 keys matching pattern, got %d", len(keys))
	}
}

// ================================================================================================================
// 10. Operations after Disconnect (error path)
// ================================================================================================================

func TestOperations_AfterDisconnect(t *testing.T) {
	r := connectTestRedis(t)

	r.Disconnect()

	// SET should fail after disconnect
	err := r.Set(testKeyPrefix+"should:fail", "nope")
	if err == nil {
		t.Fatal("expected error when calling Set after Disconnect")
	}

	// GET should fail after disconnect
	_, _, err = r.Get(testKeyPrefix + "should:fail")
	if err == nil {
		t.Fatal("expected error when calling Get after Disconnect")
	}
}

// ================================================================================================================
// 11. Keys deprecation warning
// ================================================================================================================

func TestKeys_DeprecationWarning_FiresOnce(t *testing.T) {
	// Reset the package-level sync.Once so we can observe the deprecation hook in this test,
	// regardless of test execution order. Restore original state on cleanup.
	origOnce := keysDeprecationOnce
	origHook := KeysDeprecationHook
	t.Cleanup(func() {
		keysDeprecationOnce = origOnce
		KeysDeprecationHook = origHook
	})

	var callCount atomic.Int32
	KeysDeprecationHook = func() {
		callCount.Add(1)
	}
	keysDeprecationOnce = sync.Once{}

	// Use a nil UTILS receiver: the deprecation hook fires before the nil-receiver guard,
	// so we can verify sync.Once behavior without needing a live Redis connection.
	var u *UTILS

	// First call should trigger the deprecation hook exactly once
	_, _, _ = u.Keys("*")
	if callCount.Load() != 1 {
		t.Fatalf("expected deprecation hook to fire once on first Keys call, got %d", callCount.Load())
	}

	// Second call should NOT trigger the hook again (sync.Once guarantees this)
	_, _, _ = u.Keys("*")
	if callCount.Load() != 1 {
		t.Fatalf("expected deprecation hook to fire only once, got %d calls", callCount.Load())
	}
}

func TestKeys_DeprecationWarning_NilReceiver(t *testing.T) {
	// Verify the deprecation hook fires even when the UTILS receiver is nil (before the nil check),
	// so operators always see the warning regardless of caller mistakes.
	origOnce := keysDeprecationOnce
	origHook := KeysDeprecationHook
	t.Cleanup(func() {
		keysDeprecationOnce = origOnce
		KeysDeprecationHook = origHook
	})

	var fired atomic.Bool
	KeysDeprecationHook = func() {
		fired.Store(true)
	}
	keysDeprecationOnce = sync.Once{}

	var u *UTILS // nil receiver
	_, _, err := u.Keys("*")
	if err == nil {
		t.Fatal("expected error for nil UTILS receiver")
	}
	if !fired.Load() {
		t.Fatal("expected deprecation hook to fire even with nil UTILS receiver")
	}
}

// ================================================================================================================
// 12. ScanKeys — SCAN-based alternative to Keys
// ================================================================================================================

func TestScanKeys_BasicPattern(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key1 := testKeyPrefix + "scankeys:alpha"
	key2 := testKeyPrefix + "scankeys:beta"
	key3 := testKeyPrefix + "scankeys:gamma"
	cleanupKeys(t, r, key1, key2, key3)

	_ = r.Set(key1, "1")
	_ = r.Set(key2, "2")
	_ = r.Set(key3, "3")

	keys, err := r.UTILS.ScanKeys(testKeyPrefix + "scankeys:*")
	if err != nil {
		t.Fatalf("ScanKeys failed: %v", err)
	}
	if len(keys) < 3 {
		t.Fatalf("expected at least 3 keys, got %d: %v", len(keys), keys)
	}

	// Verify all expected keys are present
	found := map[string]bool{}
	for _, k := range keys {
		found[k] = true
	}
	for _, expected := range []string{key1, key2, key3} {
		if !found[expected] {
			t.Errorf("expected key %q in ScanKeys result, not found", expected)
		}
	}
}

func TestScanKeys_NoMatch(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	keys, err := r.UTILS.ScanKeys(testKeyPrefix + "scankeys:nonexistent:*")
	if err != nil {
		t.Fatalf("ScanKeys failed: %v", err)
	}
	if keys != nil {
		t.Fatalf("expected nil for no matches, got %v", keys)
	}
}

func TestScanKeys_EmptyMatch(t *testing.T) {
	// Empty match validation happens before any Redis call, so no connection needed.
	u := &UTILS{core: &Redis{}}
	_, err := u.ScanKeys("")
	if err == nil {
		t.Fatal("expected error for empty match pattern")
	}
}

func TestScanKeys_NilReceiver(t *testing.T) {
	var u *UTILS
	_, err := u.ScanKeys("*")
	if err == nil {
		t.Fatal("expected error for nil UTILS receiver")
	}
}

func TestScanKeys_WithCountHint(t *testing.T) {
	r := connectTestRedis(t)
	defer r.Disconnect()

	key1 := testKeyPrefix + "scanhint:one"
	key2 := testKeyPrefix + "scanhint:two"
	cleanupKeys(t, r, key1, key2)

	_ = r.Set(key1, "a")
	_ = r.Set(key2, "b")

	// Use a very small count hint to exercise multiple SCAN iterations
	keys, err := r.UTILS.ScanKeys(testKeyPrefix+"scanhint:*", 1)
	if err != nil {
		t.Fatalf("ScanKeys with count hint failed: %v", err)
	}
	if len(keys) < 2 {
		t.Fatalf("expected at least 2 keys, got %d", len(keys))
	}
}
