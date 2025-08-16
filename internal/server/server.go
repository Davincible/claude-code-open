package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Davincible/claude-code-open/internal/config"
	"github.com/Davincible/claude-code-open/internal/handlers"
	"github.com/Davincible/claude-code-open/internal/middleware"
	"github.com/Davincible/claude-code-open/internal/providers"
)

type Server struct {
	config   *config.Manager
	registry *providers.Registry
	logger   *slog.Logger
	server   *http.Server
}

func New(configManager *config.Manager, logger *slog.Logger) *Server {
    registry := providers.NewRegistry()
    registry.Initialize()
    
    // Apply domain mappings from config
    cfg := configManager.Get()
    if cfg != nil && cfg.DomainMappings != nil {
        registry.SetDomainMappings(cfg.DomainMappings)
    }
    
    return &Server{
        config:   configManager,
        registry: registry,
        logger:   logger,
    }
}

func (s *Server) Start() error {
	cfg := s.config.Get()
	if cfg == nil {
		return errors.New("configuration not loaded")
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Setup routes
	mux := s.setupRoutes()

	s.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}

	s.logger.Info("Starting server", "address", addr)

	// Start server in goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Server error", "error", err)
			// Check if it's an address-in-use error
			if strings.Contains(err.Error(), "address already in use") || strings.Contains(err.Error(), "bind: address already in use") {
				s.handleAddressInUse(addr)
				os.Exit(1)
			}
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	s.logger.Info("Server is shutting down...")

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	s.logger.Info("Server exited")

	return nil
}

func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

func (s *Server) setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Create handlers
	proxyHandler := handlers.NewProxyHandler(s.config, s.registry, s.logger)
	healthHandler := handlers.NewHealthHandler(s.logger)

	// Setup middleware chains
	middlewareSet := middleware.NewMiddlewareSet(s.config, s.logger)

	// Apply middleware chains to routes
	mux.Handle("/health", middlewareSet.HealthChain().Handler(healthHandler))
	mux.Handle("/", middlewareSet.DefaultChain().Handler(proxyHandler))

	return mux
}

// handleAddressInUse attempts to find and display the PID using the specified address
func (s *Server) handleAddressInUse(addr string) {
	s.logger.Error("Address already in use", "address", addr)

	// Extract port from address
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		s.logger.Error("Failed to parse address", "address", addr, "error", err)
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		s.logger.Error("Invalid port number", "port", portStr, "error", err)
		return
	}

	pid := s.findProcessUsingPort(port)
	if pid > 0 {
		processInfo := s.getProcessInfo(pid)
		s.logger.Error("Port is being used by another process",
			"port", port,
			"pid", pid,
			"process", processInfo)
	} else {
		s.logger.Error("Could not determine which process is using the port", "port", port)
	}
}

// findProcessUsingPort attempts to find the PID of the process using the specified port
func (s *Server) findProcessUsingPort(port int) int {
	switch runtime.GOOS {
	case "linux", "darwin":
		return s.findProcessUsingPortUnix(port)
	case "windows":
		return s.findProcessUsingPortWindows(port)
	default:
		s.logger.Warn("Unsupported OS for port detection", "os", runtime.GOOS)
		return 0
	}
}

// findProcessUsingPortUnix finds process using port on Unix-like systems
func (s *Server) findProcessUsingPortUnix(port int) int {
	// Try netstat first
	if pid := s.tryNetstat(port); pid > 0 {
		return pid
	}

	// Try lsof as fallback
	if pid := s.tryLsof(port); pid > 0 {
		return pid
	}

	// Try ss as another fallback
	if pid := s.trySS(port); pid > 0 {
		return pid
	}

	return 0
}

// tryNetstat attempts to find PID using netstat
func (s *Server) tryNetstat(port int) int {
	cmd := exec.Command("netstat", "-tlnp")

	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	portPattern := fmt.Sprintf(":%d ", port)

	for _, line := range lines {
		if strings.Contains(line, portPattern) && strings.Contains(line, "LISTEN") {
			// Extract PID from netstat output (format: PID/program_name)
			parts := strings.Fields(line)
			if len(parts) >= 7 {
				pidProgram := parts[6]
				if pidStr := strings.Split(pidProgram, "/")[0]; pidStr != "-" {
					if pid, err := strconv.Atoi(pidStr); err == nil {
						return pid
					}
				}
			}
		}
	}

	return 0
}

// tryLsof attempts to find PID using lsof
func (s *Server) tryLsof(port int) int {
	// Validate port range for security
	if port < 1 || port > 65535 {
		return 0
	}
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port))

	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	pidStr := strings.TrimSpace(string(output))
	if pidStr != "" {
		if pid, err := strconv.Atoi(pidStr); err == nil {
			return pid
		}
	}

	return 0
}

// trySS attempts to find PID using ss command
func (s *Server) trySS(port int) int {
	cmd := exec.Command("ss", "-tlnp")

	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	portPattern := fmt.Sprintf(":%d ", port)

	for _, line := range lines {
		if strings.Contains(line, portPattern) && strings.Contains(line, "LISTEN") {
			// Extract PID from ss output
			if idx := strings.Index(line, "pid="); idx != -1 {
				pidPart := line[idx+4:]
				if commaIdx := strings.Index(pidPart, ","); commaIdx != -1 {
					pidStr := pidPart[:commaIdx]
					if pid, err := strconv.Atoi(pidStr); err == nil {
						return pid
					}
				}
			}
		}
	}

	return 0
}

// findProcessUsingPortWindows finds process using port on Windows
func (s *Server) findProcessUsingPortWindows(port int) int {
	cmd := exec.Command("netstat", "-ano")

	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	portPattern := fmt.Sprintf(":%d ", port)

	for _, line := range lines {
		if strings.Contains(line, portPattern) && strings.Contains(line, "LISTENING") {
			parts := strings.Fields(line)
			if len(parts) >= 5 {
				pidStr := parts[4]
				if pid, err := strconv.Atoi(pidStr); err == nil {
					return pid
				}
			}
		}
	}

	return 0
}

// getProcessInfo attempts to get information about a process
func (s *Server) getProcessInfo(pid int) string {
	switch runtime.GOOS {
	case "linux", "darwin":
		return s.getProcessInfoUnix(pid)
	case "windows":
		return s.getProcessInfoWindows(pid)
	default:
		return fmt.Sprintf("PID %d", pid)
	}
}

// getProcessInfoUnix gets process info on Unix-like systems
func (s *Server) getProcessInfoUnix(pid int) string {
	// Validate PID range for security
	if pid < 1 || pid > 4194304 { // Max PID on most systems
		return fmt.Sprintf("PID %d (invalid)", pid)
	}
	// Try ps command
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=")

	output, err := cmd.Output()
	if err == nil {
		processName := strings.TrimSpace(string(output))
		if processName != "" {
			return fmt.Sprintf("%s (PID: %d)", processName, pid)
		}
	}

	return fmt.Sprintf("PID: %d", pid)
}

// getProcessInfoWindows gets process info on Windows
func (s *Server) getProcessInfoWindows(pid int) string {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")

	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) > 0 && lines[0] != "" {
			// Parse CSV output
			parts := strings.Split(lines[0], ",")
			if len(parts) >= 1 {
				processName := strings.Trim(parts[0], "\"")
				return fmt.Sprintf("%s (PID: %d)", processName, pid)
			}
		}
	}

	return fmt.Sprintf("PID: %d", pid)
}
