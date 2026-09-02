package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadBulk(t *testing.T) {
	r := NewResp(strings.NewReader("$5\r\nhello\r\n"))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.typ != "bulk" || v.bulk != "hello" {
		t.Fatalf("got %+v, want bulk \"hello\"", v)
	}
}

func TestReadBulkNull(t *testing.T) {
	// RESP null bulk string ($-1\r\n) must not panic on the negative length.
	r := NewResp(strings.NewReader("$-1\r\n"))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.typ != "null" {
		t.Fatalf("got typ %q, want \"null\"", v.typ)
	}
}

func TestReadArray(t *testing.T) {
	r := NewResp(strings.NewReader("*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n"))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.typ != "array" || len(v.array) != 2 {
		t.Fatalf("got %+v, want 2-element array", v)
	}
	if v.array[0].bulk != "GET" || v.array[1].bulk != "foo" {
		t.Fatalf("got %+v, want [GET foo]", v.array)
	}
}

// Regression test for the bug where handleConnection created a new Resp
// (and therefore a new bufio.Reader) inside the per-command loop, discarding
// any pipelined data the reader had already buffered from a single TCP read.
func TestReadPipelinedCommands(t *testing.T) {
	raw := "*1\r\n$4\r\nPING\r\n*1\r\n$4\r\nPING\r\n"
	r := NewResp(strings.NewReader(raw))

	for i := 0; i < 2; i++ {
		v, err := r.Read()
		if err != nil {
			t.Fatalf("command %d: unexpected error: %v", i, err)
		}
		if v.typ != "array" || len(v.array) != 1 || v.array[0].bulk != "PING" {
			t.Fatalf("command %d: got %+v, want [PING]", i, v)
		}
	}
}

func TestMarshalBulk(t *testing.T) {
	v := Value{typ: "bulk", bulk: "hello"}
	got := v.Marshal()
	want := []byte("$5\r\nhello\r\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMarshalNull(t *testing.T) {
	v := Value{typ: "null"}
	got := v.Marshal()
	want := []byte("$-1\r\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMarshalArray(t *testing.T) {
	v := Value{typ: "array", array: []Value{
		{typ: "bulk", bulk: "a"},
		{typ: "bulk", bulk: "bc"},
	}}
	got := v.Marshal()
	want := []byte("*2\r\n$1\r\na\r\n$2\r\nbc\r\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
