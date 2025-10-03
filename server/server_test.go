package server

import (
	"github.com/nixxel-company-limited/escpos-usb-server/adapter"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockAdapter is a mock implementation of the Adapter interface for testing
type MockAdapter struct {
	open      bool
	writeData []byte
}

func (m *MockAdapter) Open() error {
	m.open = true
	return nil
}

func (m *MockAdapter) Write(data []byte) (int, error) {
	m.writeData = append(m.writeData, data...)
	return len(data), nil
}

func (m *MockAdapter) Read(buf []byte) (int, error) {
	return 0, nil
}

func (m *MockAdapter) Close() error {
	m.open = false
	return nil
}

func (m *MockAdapter) IsOpen() bool {
	return m.open
}

func TestNewServer(t *testing.T) {
	mockAdapter := &MockAdapter{}
	address := "localhost:9100"

	server := New(mockAdapter, address)

	assert.NotNil(t, server)
	assert.Equal(t, address, server.Address())
	assert.False(t, server.IsRunning())
	assert.Equal(t, mockAdapter, server.GetAdapter())
}

func TestServerStartStop(t *testing.T) {
	mockAdapter := &MockAdapter{}
	address := "localhost:9101"

	server := New(mockAdapter, address)

	// Test start async (non-blocking)
	err := server.StartAsync()
	require.NoError(t, err)
	assert.True(t, server.IsRunning())
	assert.True(t, mockAdapter.IsOpen())

	// Test double start
	err = server.StartAsync()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Test stop
	err = server.Stop()
	require.NoError(t, err)
	assert.False(t, server.IsRunning())
	assert.False(t, mockAdapter.IsOpen())

	// Test double stop (should not error)
	err = server.Stop()
	assert.NoError(t, err)
}

func TestServerConnection(t *testing.T) {
	mockAdapter := &MockAdapter{}
	address := "localhost:9102"

	server := New(mockAdapter, address)

	err := server.StartAsync()
	require.NoError(t, err)
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Connect to server
	conn, err := net.Dial("tcp", address)
	require.NoError(t, err)
	defer conn.Close()

	// Send data
	testData := []byte("Hello, Printer!")
	n, err := conn.Write(testData)
	require.NoError(t, err)
	assert.Equal(t, len(testData), n)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Check that data was written to adapter
	assert.Equal(t, testData, mockAdapter.writeData)
}

func TestServerMultipleConnections(t *testing.T) {
	mockAdapter := &MockAdapter{}
	address := "localhost:9103"

	server := New(mockAdapter, address)

	err := server.StartAsync()
	require.NoError(t, err)
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create multiple connections
	numConnections := 3
	connections := make([]net.Conn, numConnections)

	for i := 0; i < numConnections; i++ {
		conn, err := net.Dial("tcp", address)
		require.NoError(t, err)
		connections[i] = conn
		defer conn.Close()

		// Send data from each connection
		data := []byte{byte(i + 1)}
		_, err = conn.Write(data)
		require.NoError(t, err)
	}

	// Give server time to process
	time.Sleep(200 * time.Millisecond)

	// Verify all data was received
	assert.Equal(t, 3, len(mockAdapter.writeData))
}

func TestServerWithRealUSBAdapter(t *testing.T) {
	// Try to create a real USB adapter
	usbAdapter, err := adapter.NewUSBAdapterAuto()
	if err != nil {
		t.Skip("No USB printer found, skipping test")
	}
	defer usbAdapter.Close()

	address := "localhost:9104"
	server := New(usbAdapter, address)

	err = server.StartAsync()
	require.NoError(t, err)
	defer server.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Connect and send ESC/POS commands
	conn, err := net.Dial("tcp", address)
	require.NoError(t, err)
	defer conn.Close()

	// Send initialization command
	initCmd := []byte{0x1B, 0x40} // ESC @
	n, err := conn.Write(initCmd)
	require.NoError(t, err)
	assert.Equal(t, len(initCmd), n)

	// Give time for printer to process
	time.Sleep(100 * time.Millisecond)
}

func TestServerAddress(t *testing.T) {
	mockAdapter := &MockAdapter{}
	testCases := []string{
		"localhost:9100",
		"0.0.0.0:9100",
		":9100",
	}

	for _, addr := range testCases {
		t.Run(addr, func(t *testing.T) {
			server := New(mockAdapter, addr)
			assert.Equal(t, addr, server.Address())
		})
	}
}

func TestServerInvalidAddress(t *testing.T) {
	mockAdapter := &MockAdapter{}
	server := New(mockAdapter, "invalid:address:9100")

	err := server.StartAsync()
	assert.Error(t, err)
	assert.False(t, server.IsRunning())
}

func TestServerStartBlocking(t *testing.T) {
	mockAdapter := &MockAdapter{}
	address := "localhost:9105"

	server := New(mockAdapter, address)

	// Start server in a goroutine since it blocks
	started := make(chan error)
	go func() {
		started <- server.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Verify it's running
	assert.True(t, server.IsRunning())

	// Connect to server
	conn, err := net.Dial("tcp", address)
	require.NoError(t, err)
	defer conn.Close()

	// Send data
	testData := []byte("Blocking test")
	_, err = conn.Write(testData)
	require.NoError(t, err)

	// Give time to process
	time.Sleep(100 * time.Millisecond)

	// Stop server
	err = server.Stop()
	require.NoError(t, err)

	// Wait for Start() to return
	select {
	case err := <-started:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Start() did not return after Stop()")
	}
}
