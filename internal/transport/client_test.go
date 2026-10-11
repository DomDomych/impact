package transport

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestResponseLimitsAndPrefixes(t *testing.T) {
	for _, tc := range []struct {
		name, reply, prefix string
		complete, wantError bool
	}{
		{"valid", "OK\n", "OK", true, false},
		{"prefix mismatch", "ERR\n", "OK", true, true},
		{"truncated", "OK", "", false, true},
		{"oversized", strings.Repeat("x", 1024*1024) + "\n", "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			go func() {
				defer server.Close()
				bufio.NewReader(server).ReadString('\n')
				io.WriteString(server, tc.reply)
			}()
			c := Client{conn: client, reader: bufio.NewReader(client), timeout: time.Second}
			complete, err := c.Request("GET key", tc.prefix)
			if complete != tc.complete || (err != nil) != tc.wantError {
				t.Fatalf("complete=%v err=%v", complete, err)
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, e := ln.Accept()
		if e == nil {
			defer c.Close()
			io.Copy(io.Discard, c)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	c, err := Dial(ctx, ln.Addr().String(), time.Second)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer c.Close()
	cancel()
	if _, err = c.Request("GET key", ""); err == nil {
		t.Fatal("expected cancellation to close socket")
	}
	<-done
}
