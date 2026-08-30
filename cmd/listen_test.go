package cmd

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func occupyPort(t *testing.T) (int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return ln.Addr().(*net.TCPAddr).Port, func() { ln.Close() }
}

func TestListenLoopback_BindsFreePort(t *testing.T) {
	port, release := occupyPort(t)
	release()

	ln, err := listenLoopback(port, port+3)
	require.NoError(t, err)
	defer ln.Close()

	bound := ln.Addr().(*net.TCPAddr).Port
	assert.GreaterOrEqual(t, bound, port)
	assert.LessOrEqual(t, bound, port+3)
}

func TestListenLoopback_WalksPastOccupied(t *testing.T) {
	port, release := occupyPort(t)
	defer release()

	ln, err := listenLoopback(port, port+3)
	require.NoError(t, err)
	defer ln.Close()

	assert.NotEqual(t, port, ln.Addr().(*net.TCPAddr).Port)
}

func TestControlPortWalkEnd(t *testing.T) {
	// explicit-binds-exact
	assert.Equal(t, 42450, controlPortWalkEnd(42450, true))
	// default-walks-span
	assert.Equal(t, 42442+controlPortSpan-1, controlPortWalkEnd(42442, false))
}

func TestListenLoopback_ExplicitOccupiedFails(t *testing.T) {
	port, release := occupyPort(t)
	defer release()

	ln, err := listenLoopback(port, controlPortWalkEnd(port, true))
	assert.Error(t, err)
	assert.Nil(t, ln)
}

func TestListenLoopback_Exhausted(t *testing.T) {
	port, release := occupyPort(t)
	defer release()

	ln, err := listenLoopback(port, port)
	assert.Error(t, err)
	assert.Nil(t, ln)
}
