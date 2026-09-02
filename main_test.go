package main

import (
	"bufio"
	"net"
	"os"
	"testing"
	"time"
)

// TestHandleConnectionPipelinedCommands is a full-stack regression test for
// the bug (see README) where handleConnection recreated the Resp/bufio.Reader
// on every loop iteration, discarding buffered data from a pipelined write.
// It drives the real handleConnection over a net.Pipe with two commands sent
// in a single Write, exactly as a pipelining client would.
func TestHandleConnectionPipelinedCommands(t *testing.T) {
	aofPath := t.TempDir() + "/test.aof"
	aof, err := NewAof(aofPath)
	if err != nil {
		t.Fatalf("failed to open aof: %v", err)
	}
	defer aof.Close()
	defer os.Remove(aofPath)

	client, server := net.Pipe()
	defer client.Close()

	done := make(chan struct{})
	go func() {
		handleConnection(server, aof)
		close(done)
	}()

	raw := "*1\r\n$4\r\nPING\r\n*1\r\n$4\r\nPING\r\n"
	writeErr := make(chan error, 1)
	go func() {
		_, err := client.Write([]byte(raw))
		writeErr <- err
	}()
	if err := <-writeErr; err != nil {
		t.Fatalf("write failed: %v", err)
	}

	reader := bufio.NewReader(client)
	for i := 0; i < 2; i++ {
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("response %d: read failed (command likely dropped): %v", i, err)
		}
		if line != "+PONG\r\n" {
			t.Fatalf("response %d: got %q, want %q", i, line, "+PONG\r\n")
		}
	}

	client.Close()
	<-done
}
