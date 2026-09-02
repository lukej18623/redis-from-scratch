package main

import (
	"strconv"
	"sync"
	"time"
)


var Handlers = map[string]func([]Value) Value{
	"PING": ping,
	"SET": set,
	"GET": get,
	"DEL": del,
	"EXPIRE": expire,
	"TTL": ttl,
	"HSET": hset,
	"HGET": hget,
	"HGETALL": hgetall,
}

func ping(args []Value) Value {
	if len(args) == 0{
		return Value{typ: "string", str: "PONG"}
	}
	return Value{typ: "string", str: args[0].bulk}
}

var SETs = map[string]string{}
var SETsMu = sync.RWMutex{}

func set(args []Value) Value {
	if len(args) != 2 {
		return Value{typ: "error", str: "ERR wrong number of arguments for 'set' command"}
	}

	key := args[0].bulk
	value := args[1].bulk

	SETsMu.Lock()
	SETs[key] = value
	SETsMu.Unlock()

	// A fresh SET clears any TTL left over from a previous EXPIRE, matching
	// real Redis semantics.
	clearExpire(key)

	return Value{typ: "string", str: "OK"}
}

func get(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "ERR wrong number of arguments for 'get' command"}
	}

	key := args[0].bulk

	expireIfNeeded(key)

	SETsMu.RLock()
	value, ok := SETs[key]
	SETsMu.RUnlock()

	if !ok {
		return Value{typ: "null"}
	}

	return Value{typ: "bulk", bulk: value}
}

func del(args []Value) Value {
	if len(args) == 0 {
		return Value{typ: "error", str: "ERR wrong number of arguments for 'del' command"}
	}

	count := 0

	SETsMu.Lock()
	for _, a := range args {
		if _, ok := SETs[a.bulk]; ok {
			delete(SETs, a.bulk)
			count++
		}
	}
	SETsMu.Unlock()

	for _, a := range args {
		clearExpire(a.bulk)
	}

	return Value{typ: "integer", num: count}
}

// Expires holds absolute expiry times for keys set via EXPIRE. Only string
// keys (SETs) support TTLs in this implementation.
var Expires = map[string]time.Time{}
var ExpiresMu = sync.Mutex{}

func setExpire(key string, ttl time.Duration) {
	ExpiresMu.Lock()
	Expires[key] = time.Now().Add(ttl)
	ExpiresMu.Unlock()
}

func clearExpire(key string) {
	ExpiresMu.Lock()
	delete(Expires, key)
	ExpiresMu.Unlock()
}

// expireIfNeeded evicts key from SETs if its TTL has passed. It must be
// called before any read of SETs that should honor expiry.
func expireIfNeeded(key string) {
	ExpiresMu.Lock()
	exp, ok := Expires[key]
	if !ok || time.Now().Before(exp) {
		ExpiresMu.Unlock()
		return
	}
	delete(Expires, key)
	ExpiresMu.Unlock()

	SETsMu.Lock()
	delete(SETs, key)
	SETsMu.Unlock()
}

func expire(args []Value) Value {
	if len(args) != 2 {
		return Value{typ: "error", str: "ERR wrong number of arguments for 'expire' command"}
	}

	key := args[0].bulk
	seconds, err := strconv.Atoi(args[1].bulk)
	if err != nil {
		return Value{typ: "error", str: "ERR value is not an integer or out of range"}
	}

	expireIfNeeded(key)

	SETsMu.RLock()
	_, ok := SETs[key]
	SETsMu.RUnlock()

	if !ok {
		return Value{typ: "integer", num: 0}
	}

	setExpire(key, time.Duration(seconds)*time.Second)

	return Value{typ: "integer", num: 1}
}

func ttl(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "ERR wrong number of arguments for 'ttl' command"}
	}

	key := args[0].bulk

	expireIfNeeded(key)

	SETsMu.RLock()
	_, ok := SETs[key]
	SETsMu.RUnlock()

	if !ok {
		return Value{typ: "integer", num: -2}
	}

	ExpiresMu.Lock()
	exp, hasExpiry := Expires[key]
	ExpiresMu.Unlock()

	if !hasExpiry {
		return Value{typ: "integer", num: -1}
	}

	remaining := int(time.Until(exp).Seconds())
	if remaining < 0 {
		remaining = 0
	}

	return Value{typ: "integer", num: remaining}
}

var HSETs = map[string]map[string]string{}
var HSETsMu = sync.RWMutex{}

func hset(args []Value) Value {
	if len(args) != 3 {
		return Value{typ: "error", str: "ERR wrong number of arguments for 'hset' command"}
	}

	hash := args[0].bulk
	key := args[1].bulk
	value := args[2].bulk

	HSETsMu.Lock()
	if _, ok := HSETs[hash]; !ok {
		HSETs[hash] = map[string]string{}
	}
	HSETs[hash][key] = value
	HSETsMu.Unlock()

	return Value{typ: "string", str: "OK"}
}

func hget(args []Value) Value {
	if len(args) != 2 {
		return Value{typ: "error", str: "ERR wrong number of arguments for 'hget' command"}
	}

	hash := args[0].bulk
	key := args[1].bulk

	HSETsMu.RLock()
	value, ok := HSETs[hash][key]
	HSETsMu.RUnlock()

	if !ok {
		return Value{typ: "null"}
	}

	return Value{typ: "bulk", bulk: value}
}

func hgetall(args []Value) Value {
	if len(args) != 1 {
		return Value{typ: "error", str: "ERR wrong number of arguments for 'hgetall' command"}
	}

	hash := args[0].bulk

	HSETsMu.RLock()
	value, ok := HSETs[hash]
	HSETsMu.RUnlock()

	if !ok {
		return Value{typ: "array", array: []Value{}}
	}

	values := make([]Value, 0, len(value)*2)
	for k, v := range value {
		values = append(values, Value{typ: "bulk", bulk: k})
		values = append(values, Value{typ: "bulk", bulk: v})
	}

	return Value{typ: "array", array: values}
}