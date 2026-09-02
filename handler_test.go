package main

import (
	"testing"
	"time"
)

// resetStore clears package-level state between tests since handlers
// operate on shared maps.
func resetStore() {
	SETsMu.Lock()
	SETs = map[string]string{}
	SETsMu.Unlock()

	HSETsMu.Lock()
	HSETs = map[string]map[string]string{}
	HSETsMu.Unlock()

	ExpiresMu.Lock()
	Expires = map[string]time.Time{}
	ExpiresMu.Unlock()
}

func TestSetGet(t *testing.T) {
	resetStore()

	if v := set([]Value{{bulk: "foo"}, {bulk: "bar"}}); v.typ != "string" || v.str != "OK" {
		t.Fatalf("SET returned %+v, want OK", v)
	}

	if v := get([]Value{{bulk: "foo"}}); v.typ != "bulk" || v.bulk != "bar" {
		t.Fatalf("GET returned %+v, want bulk \"bar\"", v)
	}
}

func TestGetMissingKey(t *testing.T) {
	resetStore()

	if v := get([]Value{{bulk: "missing"}}); v.typ != "null" {
		t.Fatalf("GET returned %+v, want null", v)
	}
}

func TestDel(t *testing.T) {
	resetStore()
	set([]Value{{bulk: "a"}, {bulk: "1"}})
	set([]Value{{bulk: "b"}, {bulk: "2"}})

	v := del([]Value{{bulk: "a"}, {bulk: "missing"}, {bulk: "b"}})
	if v.typ != "integer" || v.num != 2 {
		t.Fatalf("DEL returned %+v, want integer 2", v)
	}

	if v := get([]Value{{bulk: "a"}}); v.typ != "null" {
		t.Fatalf("GET after DEL returned %+v, want null", v)
	}
}

func TestExpireAndTTL(t *testing.T) {
	resetStore()
	set([]Value{{bulk: "foo"}, {bulk: "bar"}})

	if v := ttl([]Value{{bulk: "foo"}}); v.typ != "integer" || v.num != -1 {
		t.Fatalf("TTL before EXPIRE returned %+v, want integer -1", v)
	}

	if v := expire([]Value{{bulk: "foo"}, {bulk: "100"}}); v.typ != "integer" || v.num != 1 {
		t.Fatalf("EXPIRE returned %+v, want integer 1", v)
	}

	if v := ttl([]Value{{bulk: "foo"}}); v.typ != "integer" || v.num <= 0 || v.num > 100 {
		t.Fatalf("TTL after EXPIRE returned %+v, want integer in (0,100]", v)
	}

	if v := expire([]Value{{bulk: "missing"}, {bulk: "10"}}); v.typ != "integer" || v.num != 0 {
		t.Fatalf("EXPIRE on missing key returned %+v, want integer 0", v)
	}

	if v := expire([]Value{{bulk: "foo"}, {bulk: "not-a-number"}}); v.typ != "error" {
		t.Fatalf("EXPIRE with non-integer seconds returned %+v, want error", v)
	}
}

func TestKeyExpires(t *testing.T) {
	resetStore()
	set([]Value{{bulk: "foo"}, {bulk: "bar"}})
	expire([]Value{{bulk: "foo"}, {bulk: "0"}})

	time.Sleep(10 * time.Millisecond)

	if v := get([]Value{{bulk: "foo"}}); v.typ != "null" {
		t.Fatalf("GET after expiry returned %+v, want null", v)
	}
	if v := ttl([]Value{{bulk: "foo"}}); v.typ != "integer" || v.num != -2 {
		t.Fatalf("TTL after expiry returned %+v, want integer -2", v)
	}
}

func TestSetClearsExistingExpiry(t *testing.T) {
	resetStore()
	set([]Value{{bulk: "foo"}, {bulk: "bar"}})
	expire([]Value{{bulk: "foo"}, {bulk: "100"}})

	set([]Value{{bulk: "foo"}, {bulk: "baz"}}) // SET should clear the prior TTL

	if v := ttl([]Value{{bulk: "foo"}}); v.typ != "integer" || v.num != -1 {
		t.Fatalf("TTL after re-SET returned %+v, want integer -1", v)
	}
}

func TestHSetHGetHGetAll(t *testing.T) {
	resetStore()

	if v := hset([]Value{{bulk: "h"}, {bulk: "f1"}, {bulk: "v1"}}); v.typ != "string" || v.str != "OK" {
		t.Fatalf("HSET returned %+v, want OK", v)
	}
	hset([]Value{{bulk: "h"}, {bulk: "f2"}, {bulk: "v2"}})

	if v := hget([]Value{{bulk: "h"}, {bulk: "f1"}}); v.typ != "bulk" || v.bulk != "v1" {
		t.Fatalf("HGET returned %+v, want bulk \"v1\"", v)
	}

	if v := hget([]Value{{bulk: "h"}, {bulk: "missing"}}); v.typ != "null" {
		t.Fatalf("HGET on missing field returned %+v, want null", v)
	}

	v := hgetall([]Value{{bulk: "h"}})
	if v.typ != "array" || len(v.array) != 4 {
		t.Fatalf("HGETALL returned %+v, want 4-element array", v)
	}
}

func TestWrongArgCounts(t *testing.T) {
	cases := []struct {
		name string
		fn   func([]Value) Value
		args []Value
	}{
		{"SET", set, []Value{{bulk: "only-one"}}},
		{"GET", get, nil},
		{"HSET", hset, []Value{{bulk: "h"}, {bulk: "f"}}},
		{"HGET", hget, []Value{{bulk: "h"}}},
		{"HGETALL", hgetall, nil},
		{"DEL", del, nil},
		{"EXPIRE", expire, []Value{{bulk: "k"}}},
		{"TTL", ttl, nil},
	}

	for _, c := range cases {
		if v := c.fn(c.args); v.typ != "error" {
			t.Errorf("%s with wrong arg count returned %+v, want error", c.name, v)
		}
	}
}

func TestPing(t *testing.T) {
	if v := ping(nil); v.typ != "string" || v.str != "PONG" {
		t.Fatalf("PING returned %+v, want PONG", v)
	}
	if v := ping([]Value{{bulk: "hello"}}); v.typ != "string" || v.str != "hello" {
		t.Fatalf("PING with arg returned %+v, want echo \"hello\"", v)
	}
}
