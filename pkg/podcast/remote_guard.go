package podcast

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"pod/pkg/util"
)

// Feeds control every URL pod fetches: the feed itself, each enclosure, and
// the cover image. Without a guard a hostile or compromised feed can point
// those at 127.0.0.1, the LAN, or a cloud metadata address and have pod
// fetch it. The check runs at dial time, on the resolved address, so it
// covers redirects and DNS answers alike; allow_private_hosts turns it off
// for people whose feeds really do live on the LAN.
var (
	privateHostsMu      util.Mutex
	privateHostsAllowed bool
)

// SetAllowPrivateHosts permits fetching feeds, enclosures and covers from
// loopback, private and link-local addresses.
func SetAllowPrivateHosts(allow bool) {
	privateHostsMu.Lock()
	defer privateHostsMu.Unlock()
	privateHostsAllowed = allow
}

func privateHostsAllowedNow() bool {
	privateHostsMu.Lock()
	defer privateHostsMu.Unlock()
	return privateHostsAllowed
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// guardedDial wraps a dialer so that every connection first resolves the
// host and refuses private addresses unless SetAllowPrivateHosts is on.
func guardedDial(next dialFunc) dialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if privateHostsAllowedNow() {
			return next(ctx, network, addr)
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := resolveHost(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return nil, fmt.Errorf("refusing to connect to %s: %s is a private or local address (set allow_private_hosts to permit LAN feeds)", host, ip)
			}
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := next(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

func resolveHost(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return []net.IP{ip}, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses for %s", host)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

// newGuardedTransport is the transport every remote fetch in this package
// uses: feed, enclosure and cover alike.
func newGuardedTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         guardedDial(dialer.DialContext),
		MaxIdleConns:        128,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}
