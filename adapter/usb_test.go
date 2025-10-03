package adapter

import (
	"testing"

	"github.com/google/gousb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUSBAdapterAuto(t *testing.T) {
	adapter, err := NewUSBAdapterAuto()
	if err != nil {
		t.Skip("No USB printer found, skipping test")
	}
	defer adapter.Close()

	assert.NotNil(t, adapter)
	assert.NotNil(t, adapter.ctx)
	assert.NotNil(t, adapter.device)
	assert.NotNil(t, adapter.eventListeners)
}

func TestNewUSBAdapter(t *testing.T) {
	// Test with common printer VID/PIDs
	// These will fail if no device is connected, which is expected
	testCases := []struct {
		name string
		vid  uint16
		pid  uint16
	}{
		{"Epson", 0x04b8, 0x0202},
		{"Star", 0x0519, 0x0001},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adapter, err := NewUSBAdapter(tc.vid, tc.pid)
			if err != nil {
				t.Skip("USB printer not found, skipping test")
			}
			defer adapter.Close()

			assert.NotNil(t, adapter)
			assert.NotNil(t, adapter.device)
		})
	}
}

func TestFindPrinters(t *testing.T) {
	ctx := gousb.NewContext()
	defer ctx.Close()

	printers := FindPrinters(ctx)

	// This test will pass even if no printers are found
	assert.NotNil(t, printers)

	if len(printers) > 0 {
		t.Logf("Found %d printer(s)", len(printers))
		for _, printer := range printers {
			assert.True(t, IsPrinter(printer))
			printer.Close()
		}
	} else {
		t.Skip("No USB printers found")
	}
}

func TestIsPrinter(t *testing.T) {
	t.Run("NilDevice", func(t *testing.T) {
		assert.False(t, IsPrinter(nil))
	})

	t.Run("RealDevice", func(t *testing.T) {
		ctx := gousb.NewContext()
		defer ctx.Close()

		devices := FindPrinters(ctx)
		if len(devices) == 0 {
			t.Skip("No USB printers found")
		}

		for _, dev := range devices {
			defer dev.Close()
			assert.True(t, IsPrinter(dev))
		}
	})
}

func TestUSBAdapterOpenClose(t *testing.T) {
	adapter, err := NewUSBAdapterAuto()
	if err != nil {
		t.Skip("No USB printer found, skipping test")
	}
	defer adapter.Close()

	// Test initial state
	assert.False(t, adapter.IsOpen())

	// Test Open
	err = adapter.Open()
	require.NoError(t, err)
	assert.True(t, adapter.IsOpen())

	// Test double open
	err = adapter.Open()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already open")

	// Test Close
	err = adapter.Close()
	require.NoError(t, err)
	assert.False(t, adapter.IsOpen())

	// Test double close (should not error)
	err = adapter.Close()
	assert.NoError(t, err)
}

func TestUSBAdapterWrite(t *testing.T) {
	adapter, err := NewUSBAdapterAuto()
	if err != nil {
		t.Skip("No USB printer found, skipping test")
	}
	defer adapter.Close()

	// Test write without opening
	_, err = adapter.Write([]byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not open")

	// Open device
	err = adapter.Open()
	require.NoError(t, err)
	defer adapter.Close()

	// Test write with valid data
	testData := []byte{0x1B, 0x40} // ESC @ (Initialize printer)
	n, err := adapter.Write(testData)
	assert.NoError(t, err)
	assert.Equal(t, len(testData), n)

	// Test write with empty data
	n, err = adapter.Write([]byte{})
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestUSBAdapterRead(t *testing.T) {
	adapter, err := NewUSBAdapterAuto()
	if err != nil {
		t.Skip("No USB printer found, skipping test")
	}
	defer adapter.Close()

	// Test read without opening
	buf := make([]byte, 64)
	_, err = adapter.Read(buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not open")

	// Open device
	err = adapter.Open()
	require.NoError(t, err)
	defer adapter.Close()

	// Test read (may fail if no input endpoint or no data available)
	buf = make([]byte, 64)
	_, err = adapter.Read(buf)
	// We don't assert on error here because many printers don't have input endpoints
	// or may not have data available
}

func TestUSBAdapterEventListeners(t *testing.T) {
	adapter, err := NewUSBAdapterAuto()
	if err != nil {
		t.Skip("No USB printer found, skipping test")
	}
	defer adapter.Close()

	// Test event listeners
	connectCalled := false
	closeCalled := false
	dataCalled := false

	adapter.On(EventConnect, func(e Event) {
		connectCalled = true
		assert.Equal(t, EventConnect, e.Type)
		assert.NotNil(t, e.Device)
	})

	adapter.On(EventClose, func(e Event) {
		closeCalled = true
		assert.Equal(t, EventClose, e.Type)
	})

	adapter.On(EventData, func(e Event) {
		dataCalled = true
		assert.Equal(t, EventData, e.Type)
		assert.NotNil(t, e.Data)
	})

	// Open should trigger connect event
	err = adapter.Open()
	require.NoError(t, err)

	// Write should trigger data event
	_, err = adapter.Write([]byte{0x1B, 0x40})
	assert.NoError(t, err)

	// Close should trigger close event
	err = adapter.Close()
	require.NoError(t, err)

	// Give goroutines time to execute
	// Note: In production code, you'd want to use proper synchronization
	assert.Eventually(t, func() bool {
		return connectCalled && closeCalled && dataCalled
	}, 1000, 10, "All events should have been triggered")
}

func TestGetDeviceByVIDPID(t *testing.T) {
	ctx := gousb.NewContext()
	defer ctx.Close()

	// Test with invalid VID/PID
	_, err := GetDeviceByVIDPID(ctx, 0xFFFF, 0xFFFF)
	assert.Error(t, err)

	// Test with valid VID/PID if printer is available
	printers := FindPrinters(ctx)
	if len(printers) == 0 {
		t.Skip("No USB printers found")
	}

	// Get VID/PID from first printer
	desc := printers[0].Desc
	require.NoError(t, err)

	// Close all found printers
	for _, p := range printers {
		p.Close()
	}

	// Try to open by VID/PID
	device, err := GetDeviceByVIDPID(ctx, uint16(desc.Vendor), uint16(desc.Product))
	if err == nil {
		defer device.Close()
		assert.NotNil(t, device)
	}
}

func TestGetDeviceBySerial(t *testing.T) {
	ctx := gousb.NewContext()
	defer ctx.Close()

	// Test with invalid serial
	_, err := GetDeviceBySerial(ctx, "INVALID_SERIAL_NUMBER")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Test with valid serial if printer is available
	printers := FindPrinters(ctx)
	if len(printers) == 0 {
		t.Skip("No USB printers found")
	}

	// Try to get serial number from first printer
	serial, err := printers[0].SerialNumber()

	// Close all found printers
	for _, p := range printers {
		p.Close()
	}

	if err != nil || serial == "" {
		t.Skip("Printer doesn't have a serial number")
	}

	// Try to open by serial
	device, err := GetDeviceBySerial(ctx, serial)
	if err == nil {
		defer device.Close()
		assert.NotNil(t, device)
	}
}

func TestGetDevice(t *testing.T) {
	adapter, err := NewUSBAdapterAuto()
	if err != nil {
		t.Skip("No USB printer found, skipping test")
	}
	defer adapter.Close()

	device := adapter.GetDevice()
	assert.NotNil(t, device)
}
