package adapter

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync"

	"github.com/google/gousb"
)

// Interface class codes
// Reference: http://www.usb.org/developers/defined_class
const (
	IfaceClassAudio   = 0x01
	IfaceClassHID     = 0x03
	IfaceClassPrinter = 0x07
	IfaceClassHub     = 0x09
)

// EventType represents device events
type EventType int

const (
	EventConnect EventType = iota
	EventDisconnect
	EventDetach
	EventData
	EventClose
)

// Event represents a device event
type Event struct {
	Type   EventType
	Device *gousb.Device
	Data   []byte
	Error  error
}

// USBAdapter manages USB printer communication
type USBAdapter struct {
	device         *gousb.Device
	ctx            *gousb.Context
	outEndpoint    *gousb.OutEndpoint
	inEndpoint     *gousb.InEndpoint
	iface          *gousb.Interface
	eventListeners map[EventType][]func(Event)
	listenersMutex sync.RWMutex
	isOpen         bool
	mu             sync.Mutex
}

// NewUSBAdapter creates a new USB adapter instance
func NewUSBAdapter(vid, pid uint16) (*USBAdapter, error) {
	ctx := gousb.NewContext()
	adapter := &USBAdapter{
		ctx:            ctx,
		eventListeners: make(map[EventType][]func(Event)),
	}

	// Find device by VID/PID
	device, err := ctx.OpenDeviceWithVIDPID(gousb.ID(vid), gousb.ID(pid))
	if err != nil || device == nil {
		// Try to find any printer device
		devices := FindPrinters(ctx)
		if len(devices) == 0 {
			ctx.Close()
			return nil, errors.New("cannot find printer")
		}
		adapter.device = devices[0]
	} else {
		adapter.device = device
	}

	return adapter, nil
}

// NewUSBAdapterAuto creates adapter with auto-detection
func NewUSBAdapterAuto() (*USBAdapter, error) {
	ctx := gousb.NewContext()
	adapter := &USBAdapter{
		ctx:            ctx,
		eventListeners: make(map[EventType][]func(Event)),
	}

	devices := FindPrinters(ctx)
	if len(devices) == 0 {
		ctx.Close()
		return nil, errors.New("cannot find printer")
	}

	adapter.device = devices[0]
	return adapter, nil
}

// IsPrinter checks if a device is a printer
func IsPrinter(dev *gousb.Device) bool {
	if dev == nil {
		return false
	}

	cfg, err := dev.ActiveConfigNum()
	if err != nil {
		return false
	}

	cfgDesc, err := dev.Config(cfg)
	if err != nil {
		return false
	}
	defer cfgDesc.Close()

	for _, iface := range cfgDesc.Desc.Interfaces {
		log.Println("Interface: ", iface.String())
		for _, alt := range iface.AltSettings {
			if alt.Class == IfaceClassPrinter {
				return true
			}
		}
	}

	return false
}

// FindPrinters returns all USB printer devices
func FindPrinters(ctx *gousb.Context) []*gousb.Device {
	var printers []*gousb.Device

	devices, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return true // Check all devices
	})

	if err != nil {
		return printers
	}

	for _, dev := range devices {
		log.Println("Found device: ", dev.Desc)
		if IsPrinter(dev) {
			printers = append(printers, dev)
		} else {
			dev.Close()
		}
	}

	return printers
}

// GetDeviceByVIDPID opens a device by VID and PID
func GetDeviceByVIDPID(ctx *gousb.Context, vid, pid uint16) (*gousb.Device, error) {
	device, err := ctx.OpenDeviceWithVIDPID(gousb.ID(vid), gousb.ID(pid))
	if err != nil {
		return nil, err
	}
	if device == nil {
		return nil, errors.New("device not found")
	}
	return device, nil
}

// GetDeviceBySerial opens a device by serial number
func GetDeviceBySerial(ctx *gousb.Context, serial string) (*gousb.Device, error) {
	devices, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return true
	})
	if err != nil {
		return nil, err
	}

	for _, dev := range devices {
		s, err := dev.SerialNumber()
		if err == nil && s == serial {
			// Close other devices
			for _, d := range devices {
				if d != dev {
					d.Close()
				}
			}
			return dev, nil
		}
		dev.Close()
	}

	return nil, errors.New("device with serial number not found")
}

// On adds an event listener
func (a *USBAdapter) On(eventType EventType, handler func(Event)) {
	a.listenersMutex.Lock()
	defer a.listenersMutex.Unlock()

	a.eventListeners[eventType] = append(a.eventListeners[eventType], handler)
}

// emit triggers an event
func (a *USBAdapter) emit(event Event) {
	a.listenersMutex.RLock()
	defer a.listenersMutex.RUnlock()

	if listeners, ok := a.eventListeners[event.Type]; ok {
		for _, handler := range listeners {
			go handler(event)
		}
	}
}

// Open opens the USB device and claims the interface
func (a *USBAdapter) Open() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.isOpen {
		return errors.New("device already open")
	}

	if a.device == nil {
		return errors.New("device not found")
	}

	// Set auto-detach kernel driver on Linux
	if runtime.GOOS == "linux" {
		a.device.SetAutoDetach(true)
	}

	// Get active configuration
	cfgNum, err := a.device.ActiveConfigNum()
	if err != nil {
		return fmt.Errorf("failed to get active config: %w", err)
	}

	cfg, err := a.device.Config(cfgNum)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	defer cfg.Close()

	// Find printer interface
	var printerIfaceNum int = -1
	for _, iface := range cfg.Desc.Interfaces {
		for _, alt := range iface.AltSettings {
			if alt.Class == IfaceClassPrinter {
				printerIfaceNum = iface.Number
				break
			}
		}
		if printerIfaceNum >= 0 {
			break
		}
	}

	if printerIfaceNum < 0 {
		return errors.New("no printer interface found")
	}

	// Claim interface
	iface, err := cfg.Interface(printerIfaceNum, 0)
	if err != nil {
		return fmt.Errorf("failed to claim interface: %w", err)
	}

	a.iface = iface

	// Find endpoints
	for _, epDesc := range iface.Setting.Endpoints {
		if epDesc.Direction == gousb.EndpointDirectionOut && a.outEndpoint == nil {
			ep, err := iface.OutEndpoint(epDesc.Number)
			if err == nil {
				a.outEndpoint = ep
			}
		}
		if epDesc.Direction == gousb.EndpointDirectionIn && a.inEndpoint == nil {
			ep, err := iface.InEndpoint(epDesc.Number)
			if err == nil {
				a.inEndpoint = ep
			}
		}
	}

	if a.outEndpoint == nil {
		return errors.New("cannot find output endpoint from printer")
	}

	a.isOpen = true
	a.emit(Event{Type: EventConnect, Device: a.device})

	return nil
}

// Write sends data to the printer
func (a *USBAdapter) Write(data []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.isOpen {
		return 0, errors.New("device not open")
	}

	if a.outEndpoint == nil {
		return 0, errors.New("output endpoint not available")
	}

	a.emit(Event{Type: EventData, Data: data})

	n, err := a.outEndpoint.Write(data)
	if err != nil {
		return n, fmt.Errorf("write failed: %w", err)
	}

	return n, nil
}

// Read reads data from the printer
func (a *USBAdapter) Read(buf []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.isOpen {
		return 0, errors.New("device not open")
	}

	if a.inEndpoint == nil {
		return 0, errors.New("input endpoint not available")
	}

	n, err := a.inEndpoint.Read(buf)
	if err != nil {
		return n, fmt.Errorf("read failed: %w", err)
	}

	return n, nil
}

// Close closes the USB device
func (a *USBAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.isOpen {
		return nil
	}

	var errs []error

	if a.iface != nil {
		a.iface.Close()
		a.iface = nil
	}

	if a.device != nil {
		if err := a.device.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if a.ctx != nil {
		if err := a.ctx.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	a.isOpen = false
	a.emit(Event{Type: EventClose, Device: a.device})

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}

// IsOpen returns whether the device is open
func (a *USBAdapter) IsOpen() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.isOpen
}

// GetDevice returns the underlying USB device
func (a *USBAdapter) GetDevice() *gousb.Device {
	return a.device
}
