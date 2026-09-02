# Redis Server

A minimal Redis server implemented from scratch in Go: a RESP protocol
parser/serializer, a handful of commands (`PING`, `SET`, `GET`, `DEL`,
`EXPIRE`, `TTL`, `HSET`, `HGET`, `HGETALL`), and an append-only file (AOF)
for persistence.

## Running

```
go run .
```

The server listens on `:6379` and speaks the standard RESP protocol, so it
can be driven with `redis-cli` or `nc`. On startup it replays `database.aof`
(created in the working directory) to restore state, then keeps appending
`SET`/`HSET`/`DEL` commands to that file as they're executed.

## Testing

```
go test ./...
```

Includes unit tests for the RESP parser/marshaler and command handlers, plus
an integration test (`main_test.go`) that drives the real `handleConnection`
over a `net.Pipe` with two pipelined commands in a single write — a
regression test for bug #1 below.

## Project layout

- `main.go` — TCP server, per-connection command loop, AOF replay on boot
- `resp.go` — RESP protocol reader/writer (`Value`, `Resp`, `Writer`)
- `handler.go` — command implementations and the `Handlers` dispatch table
- `aof.go` — append-only file persistence

## Bugs found and fixed

While reviewing the in-progress multi-connection/AOF changes, I found and
fixed three bugs:

### 1. Pipelined commands were silently dropped (`main.go`)

`handleConnection` created a new `Resp` (and therefore a new `bufio.Reader`)
**inside** the per-command loop:

```go
for {
    resp := NewResp(conn)
    value, err := resp.Read()
    ...
}
```

`bufio.Reader` eagerly reads ahead from the socket in chunks. If a client
sent multiple commands in a single TCP write (pipelining — which is exactly
how AOF replay and most real clients behave under load), the second and
later commands would already be sitting in that reader's internal buffer.
Discarding the `Resp`/reader on every iteration and building a fresh one
threw that buffered data away, so those commands were silently lost and the
connection would appear to hang waiting for bytes that had already arrived.

**Fix:** create the `Resp` once per connection, outside the loop, so the
same buffered reader is reused across all commands on that connection.

### 2. Bulk string reads could be truncated or corrupted (`resp.go`)

`readBulk` filled its buffer with a single call:

```go
bulk := make([]byte, len)
r.reader.Read(bulk)
```

`io.Reader.Read` is explicitly allowed to return fewer bytes than the
buffer size on a single call (e.g. a large value split across TCP packets).
The return value (both the byte count and the error) was also ignored, so a
short or failed read went unnoticed and produced a corrupted/truncated
value instead of an error.

**Fix:** use `io.ReadFull(r.reader, bulk)`, which loops internally until
the buffer is completely filled (or returns an error), and propagate that
error instead of swallowing it.

### 3. Negative bulk length caused a panic (`resp.go`)

RESP represents a null bulk string as `$-1\r\n`. `readBulk` fed the parsed
length straight into `make([]byte, len)` with no bounds check, so a `-1`
length (a legitimate, spec-compliant value — e.g. what `GET` on a missing
key marshals to, per `marshallNull`) would panic with `makeslice: len out
of range` and take down the connection's goroutine.

**Fix:** treat a negative length as the null-bulk case and return a
`Value{typ: "null"}` instead of allocating.

## Known limitations (not fixed, out of scope for this pass)

- `EXPIRE` is not persisted to the AOF (only `SET`/`HSET`/`DEL` are), so TTLs
  set before a restart are lost on replay — the key itself survives with no
  expiry.
- The AOF is replayed and appended to without ever being compacted/rewritten,
  so it grows unbounded over long-running processes.
- `EXPIRE`/`TTL` only apply to string keys (`SET`), not hashes.
