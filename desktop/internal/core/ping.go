package core

import (
	"net"
	"time"
)

// PingEndpoint: TCP connect latency to the profile's endpoint. This is a
// reachability probe of the server port, not a tunnel RTT — cheap and good
// enough to sort a server list.
func PingEndpoint(host string, port int, timeout time.Duration) (int64, error) {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", joinHostPort(host, port), timeout)
	if err != nil {
		return -1, err
	}
	_ = conn.Close()
	return time.Since(start).Milliseconds(), nil
}
