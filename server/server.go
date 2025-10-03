package server

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"github.com/nixxel-company-limited/escpos-usb-server/adapter"
)

// Server represents a TCP server that forwards data to a printer adapter
type Server struct {
	adapter  adapter.Adapter
	listener net.Listener
	address  string
	mu       sync.Mutex
	running  bool
	wg       sync.WaitGroup
	logger   *log.Logger
}

// New creates a new server instance
func New(device adapter.Adapter, address string) *Server {
	logger := log.New(os.Stdout, "[SERVER] ", log.LstdFlags|log.Lmsgprefix)
	return &Server{
		adapter: device,
		address: address,
		logger:  logger,
	}
}

// NewWithLogger creates a new server instance with a custom logger
func NewWithLogger(device adapter.Adapter, address string, logger *log.Logger) *Server {
	return &Server{
		adapter: device,
		address: address,
		logger:  logger,
	}
}

// Start starts the TCP server and blocks until Stop is called
func (s *Server) Start() error {
	s.mu.Lock()

	s.logger.Printf("Starting server on %s (blocking mode)", s.address)

	if s.running {
		s.mu.Unlock()
		s.logger.Println("Error: Server already running")
		return fmt.Errorf("server already running")
	}

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		s.mu.Unlock()
		s.logger.Printf("Error: Failed to start server: %v", err)
		return fmt.Errorf("failed to start server: %w", err)
	}

	s.listener = listener
	s.running = true
	s.logger.Printf("Server listening on %s", s.address)

	// Open the adapter if not already open
	if !s.adapter.IsOpen() {
		s.logger.Println("Opening printer adapter...")
		if err := s.adapter.Open(); err != nil {
			s.listener.Close()
			s.running = false
			s.mu.Unlock()
			s.logger.Printf("Error: Failed to open adapter: %v", err)
			return fmt.Errorf("failed to open adapter: %w", err)
		}
		s.logger.Println("Printer adapter opened successfully")
	} else {
		s.logger.Println("Printer adapter already open")
	}

	s.mu.Unlock()

	// Block and accept connections (freezes current goroutine)
	s.logger.Println("Ready to accept connections")
	s.acceptConnections()

	return nil
}

// StartAsync starts the TCP server in a goroutine (non-blocking)
func (s *Server) StartAsync() error {
	s.mu.Lock()

	s.logger.Printf("Starting server on %s (async mode)", s.address)

	if s.running {
		s.mu.Unlock()
		s.logger.Println("Error: Server already running")
		return fmt.Errorf("server already running")
	}

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		s.mu.Unlock()
		s.logger.Printf("Error: Failed to start server: %v", err)
		return fmt.Errorf("failed to start server: %w", err)
	}

	s.listener = listener
	s.running = true
	s.logger.Printf("Server listening on %s", s.address)

	// Open the adapter if not already open
	if !s.adapter.IsOpen() {
		s.logger.Println("Opening printer adapter...")
		if err := s.adapter.Open(); err != nil {
			s.listener.Close()
			s.running = false
			s.mu.Unlock()
			s.logger.Printf("Error: Failed to open adapter: %v", err)
			return fmt.Errorf("failed to open adapter: %w", err)
		}
		s.logger.Println("Printer adapter opened successfully")
	} else {
		s.logger.Println("Printer adapter already open")
	}

	s.mu.Unlock()

	s.wg.Add(1)
	go s.acceptConnections()
	s.logger.Println("Server started in background, ready to accept connections")

	return nil
}

// acceptConnections handles incoming client connections
func (s *Server) acceptConnections() {
	for {
		s.logger.Println("Waiting for client connection...")
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			running := s.running
			s.mu.Unlock()

			if !running {
				// Server is shutting down
				s.logger.Println("Server shutting down, stopping accept loop")
				return
			}
			s.logger.Printf("Error accepting connection: %v", err)
			continue
		}

		s.logger.Printf("Client connected from %s", conn.RemoteAddr())
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection handles a single client connection
func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.logger.Printf("Client disconnected: %s", conn.RemoteAddr())
		conn.Close()
	}()

	clientAddr := conn.RemoteAddr().String()
	s.logger.Printf("Handling connection from %s", clientAddr)

	// Buffer for reading data
	buf := make([]byte, 4096)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				s.logger.Printf("Error reading from client %s: %v", clientAddr, err)
			} else {
				s.logger.Printf("Client %s closed connection", clientAddr)
			}
			return
		}

		if n > 0 {
			s.logger.Printf("Received %d bytes from %s", n, clientAddr)

			// Write data to the printer adapter
			written, writeErr := s.adapter.Write(buf[:n])
			if writeErr != nil {
				s.logger.Printf("Error writing to adapter: %v", writeErr)
				return
			}
			s.logger.Printf("Wrote %d bytes to printer", written)
		}
	}
}

// Stop stops the TCP server
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		s.logger.Println("Stop called but server is not running")
		return nil
	}

	s.logger.Println("Stopping server...")
	s.running = false
	listener := s.listener
	s.mu.Unlock()

	if listener != nil {
		s.logger.Println("Closing listener...")
		listener.Close()
	}

	// Wait for all connections to finish
	s.logger.Println("Waiting for active connections to close...")
	s.wg.Wait()
	s.logger.Println("All connections closed")

	// Close the adapter
	if s.adapter.IsOpen() {
		s.logger.Println("Closing printer adapter...")
		err := s.adapter.Close()
		if err != nil {
			s.logger.Printf("Error closing adapter: %v", err)
			return err
		}
		s.logger.Println("Printer adapter closed")
	}

	s.logger.Println("Server stopped successfully")
	return nil
}

// IsRunning returns whether the server is running
func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Address returns the server address
func (s *Server) Address() string {
	return s.address
}

// GetAdapter returns the underlying adapter
func (s *Server) GetAdapter() adapter.Adapter {
	return s.adapter
}
