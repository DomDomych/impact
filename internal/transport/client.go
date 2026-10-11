package transport

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

type Client struct {
	conn    net.Conn
	reader  *bufio.Reader
	timeout time.Duration
	stop    func() bool
}

func Dial(ctx context.Context, target string, timeout time.Duration) (*Client, error) {
	c, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, err
	}
	return &Client{c, bufio.NewReader(c), timeout, context.AfterFunc(ctx, func() { c.Close() })}, nil
}
func (c *Client) Close() { c.stop(); c.conn.Close() }

// Request returns whether a complete response was received, even if its prefix did not match.
func (c *Client) Request(command, prefix string) (bool, error) {
	if err := c.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return false, err
	}
	if _, err := fmt.Fprint(c.conn, command, "\n"); err != nil {
		return false, err
	}
	var response []byte
	for {
		part, err := c.reader.ReadSlice('\n')
		if len(response)+len(part) > 1024*1024 {
			return false, fmt.Errorf("response exceeds 1 MiB")
		}
		response = append(response, part...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			return false, err
		}
		break
	}
	if !strings.HasPrefix(string(response), prefix) {
		return true, fmt.Errorf("response does not match expected prefix %q", prefix)
	}
	return true, nil
}
