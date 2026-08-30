package cmd

import (
	"fmt"
	"net"

	"github.com/pkg/errors"
)

const (
	controlPortBase = 42442
	controlPortSpan = 58
)

func controlPortWalkEnd(fromPort int, explicit bool) int {
	if explicit {
		return fromPort
	}
	return fromPort + controlPortSpan - 1
}

func listenLoopback(fromPort, toPort int) (net.Listener, error) {
	var lastErr error
	for port := fromPort; port <= toPort; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			lastErr = err
			continue
		}

		return ln, nil
	}

	if lastErr == nil {
		return nil, errors.Errorf("listenLoopback: Empty port range %d-%d", fromPort, toPort)
	}

	return nil, errors.Wrapf(lastErr, "listenLoopback: No free port in %d-%d", fromPort, toPort)
}
