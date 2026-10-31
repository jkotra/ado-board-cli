package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ErrAppNotRunning is returned when no interactive TUI application is running or listening.
var ErrAppNotRunning = errors.New("interactive ado-board-cli TUI application is not running")

// Command represents an IPC command name.
type Command string

const (
	CmdPing           Command = "ping"
	CmdSwitchTab      Command = "switch_tab"
	CmdNavigateToItem Command = "navigate_to_item"
	CmdRefresh        Command = "refresh"
	CmdSetFilter      Command = "set_filter"
	CmdClearFilter    Command = "clear_filter"
	CmdRenameTab      Command = "rename_tab"
	CmdShowToast      Command = "show_toast"
	CmdSetScreen      Command = "set_screen"
	CmdGetState       Command = "get_state"
)

// WorkItemSummary captures a clean view of a work item for IPC context.
type WorkItemSummary struct {
	ID               int     `json:"id"`
	Type             string  `json:"type"`
	Title            string  `json:"title"`
	State            string  `json:"state"`
	AssignedTo       string  `json:"assigned_to,omitempty"`
	AssignedEmail    string  `json:"assigned_email,omitempty"`
	OriginalEstimate float64 `json:"original_estimate,omitempty"`
	RemainingWork    float64 `json:"remaining_work,omitempty"`
	CompletedWork    float64 `json:"completed_work,omitempty"`
	Effort           float64 `json:"effort,omitempty"`
	ParentID         int     `json:"parent_id,omitempty"`
	AreaPath         string  `json:"area_path,omitempty"`
	IterationPath    string  `json:"iteration_path,omitempty"`
	Tags             string  `json:"tags,omitempty"`
	Activity         string  `json:"activity,omitempty"`
	URL              string  `json:"url,omitempty"`
}

// TabSummary captures tab information in the running TUI.
type TabSummary struct {
	Index    int    `json:"index"`
	Name     string `json:"name"`
	RootID   int    `json:"root_id,omitempty"`
	IsQuery  bool   `json:"is_query,omitempty"`
	QueryID  string `json:"query_id,omitempty"`
	IsActive bool   `json:"is_active"`
}

// FilterSummary captures filter status in the running TUI.
type FilterSummary struct {
	Statuses   []string `json:"statuses,omitempty"`
	Assignees  []string `json:"assignees,omitempty"`
	ItemTypes  []string `json:"item_types,omitempty"`
	SearchTerm string   `json:"search_term,omitempty"`
}

// AppStateSummary captures the comprehensive live TUI state.
type AppStateSummary struct {
	IsRunning         bool               `json:"is_running"`
	ActiveScreen      string             `json:"active_screen"` // "tree", "item_details", "settings", "create"
	ActiveModal       string             `json:"active_modal,omitempty"`
	ActiveTab         TabSummary         `json:"active_tab"`
	OpenTabs          []TabSummary       `json:"open_tabs,omitempty"`
	SelectedItem      *WorkItemSummary   `json:"selected_item,omitempty"`
	ParentItem        *WorkItemSummary   `json:"parent_item,omitempty"`
	ChildItems        []*WorkItemSummary `json:"child_items,omitempty"`
	SiblingItems      []*WorkItemSummary `json:"sibling_items,omitempty"`
	GlobalFilter      FilterSummary      `json:"global_filter,omitempty"`
	TabFilter         FilterSummary      `json:"tab_filter,omitempty"`
	TotalVisibleItems int                `json:"total_visible_items,omitempty"`
	CursorIndex       int                `json:"cursor_index,omitempty"`
}

// Request encapsulates the parameters sent to the running TUI application.
type Request struct {
	Command      Command  `json:"command"`
	TabIndex     int      `json:"tab_index,omitempty"`
	TabName      string   `json:"tab_name,omitempty"`
	NewName      string   `json:"new_name,omitempty"`
	RootID       int      `json:"root_id,omitempty"`
	WorkItemID   int      `json:"work_item_id,omitempty"`
	OpenDetails  bool     `json:"open_details,omitempty"`
	FilterScope  string   `json:"filter_scope,omitempty"` // "all", "global", or "tab"
	ClearAll     bool     `json:"clear_all,omitempty"`
	Statuses     []string `json:"statuses,omitempty"`
	Assignees    []string `json:"assignees,omitempty"`
	ItemTypes    []string `json:"item_types,omitempty"`
	Search       string   `json:"search,omitempty"`
	ToastMessage string   `json:"toast_message,omitempty"`
	IsError      bool     `json:"is_error,omitempty"`
	Screen       string   `json:"screen,omitempty"` // "tree", "item", "settings", "create"
}

// Response encapsulates the result returned by the running TUI application.
type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// HandlerFunc processes an incoming IPC request.
type HandlerFunc func(req Request) Response

// GetIPCDir returns the directory storing active instance socket/port descriptors.
func GetIPCDir() string {
	if runtime.GOOS != "windows" {
		if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
			dir := filepath.Join(xdgRuntime, "ado-board-cli", "ipc")
			_ = os.MkdirAll(dir, 0700)
			return dir
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}

	dir := filepath.Join(home, ".ado-board-cli", "ipc")
	_ = os.MkdirAll(dir, 0700)
	return dir
}

// isPIDAlive checks if a process with the given PID is currently running.
func isPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS != "windows" {
		// Signal 0 tests for process existence without sending a real signal
		return process.Signal(syscall.Signal(0)) == nil
	}
	// On Windows, os.FindProcess always succeeds; liveness is verified by connection probe
	return true
}

// CleanStaleEndpoints removes socket/port files for deceased processes.
func CleanStaleEndpoints() {
	ipcDir := GetIPCDir()
	entries, err := os.ReadDir(ipcDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "ipc_") {
			continue
		}

		parts := strings.Split(name, "_")
		if len(parts) >= 2 {
			pidStr := strings.TrimSuffix(parts[1], filepath.Ext(parts[1]))
			if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
				if runtime.GOOS != "windows" && !isPIDAlive(pid) {
					_ = os.Remove(filepath.Join(ipcDir, name))
				}
			}
		}
	}
}

// Server listens for IPC commands for a specific TUI process.
type Server struct {
	listener net.Listener
	handler  HandlerFunc
	filePath string
	closed   bool
	mu       sync.Mutex
}

// StartServer starts listening for IPC requests for the current process.
func StartServer(handler HandlerFunc) (*Server, error) {
	CleanStaleEndpoints()

	ipcDir := GetIPCDir()
	pid := os.Getpid()

	var listener net.Listener
	var filePath string
	var err error

	// On Windows, use TCP loopback directly for maximum reliability across all Windows versions
	if runtime.GOOS == "windows" {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, fmt.Errorf("failed to start Windows TCP IPC listener: %w", err)
		}
		filePath = filepath.Join(ipcDir, fmt.Sprintf("ipc_%d.port", pid))
		if tcpAddr, ok := listener.Addr().(*net.TCPAddr); ok {
			_ = os.WriteFile(filePath, []byte(strconv.Itoa(tcpAddr.Port)), 0600)
		}
	} else {
		// On Unix (Linux/macOS), use Unix Domain Socket
		filePath = filepath.Join(ipcDir, fmt.Sprintf("ipc_%d.sock", pid))
		_ = os.Remove(filePath)
		listener, err = net.Listen("unix", filePath)
		if err != nil {
			// Fallback to TCP loopback if unix socket is restricted
			listener, err = net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return nil, fmt.Errorf("failed to start IPC server: %w", err)
			}
			filePath = filepath.Join(ipcDir, fmt.Sprintf("ipc_%d.port", pid))
			if tcpAddr, ok := listener.Addr().(*net.TCPAddr); ok {
				_ = os.WriteFile(filePath, []byte(strconv.Itoa(tcpAddr.Port)), 0600)
			}
		}
	}

	srv := &Server{
		listener: listener,
		handler:  handler,
		filePath: filePath,
	}

	go srv.serve()
	return srv, nil
}

func (s *Server) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}

		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return
	}
	if len(line) == 0 {
		return
	}

	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		resp := Response{Success: false, Message: fmt.Sprintf("invalid JSON payload: %v", err)}
		respBytes, _ := json.Marshal(resp)
		_, _ = conn.Write(append(respBytes, '\n'))
		return
	}

	var resp Response
	if req.Command == CmdPing {
		resp = Response{Success: true, Message: "pong"}
	} else if s.handler != nil {
		resp = s.handler(req)
	} else {
		resp = Response{Success: false, Message: "no IPC handler configured"}
	}

	respBytes, err := json.Marshal(resp)
	if err == nil {
		_, _ = conn.Write(append(respBytes, '\n'))
	}
}

// Close stops the IPC server and removes its descriptor file.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true

	var err error
	if s.listener != nil {
		err = s.listener.Close()
	}
	if s.filePath != "" {
		_ = os.Remove(s.filePath)
	}
	return err
}

type instanceEndpoint struct {
	network string
	address string
	file    string
}

// discoverEndpoints returns all available TUI instance endpoints.
func discoverEndpoints() []instanceEndpoint {
	ipcDir := GetIPCDir()
	entries, err := os.ReadDir(ipcDir)
	if err != nil {
		return nil
	}

	var endpoints []instanceEndpoint
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "ipc_") {
			continue
		}

		fullPath := filepath.Join(ipcDir, name)
		if strings.HasSuffix(name, ".sock") {
			endpoints = append(endpoints, instanceEndpoint{
				network: "unix",
				address: fullPath,
				file:    fullPath,
			})
		} else if strings.HasSuffix(name, ".port") {
			data, err := os.ReadFile(fullPath)
			if err == nil {
				portStr := strings.TrimSpace(string(data))
				if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
					endpoints = append(endpoints, instanceEndpoint{
						network: "tcp",
						address: fmt.Sprintf("127.0.0.1:%d", port),
						file:    fullPath,
					})
				}
			}
		}
	}
	return endpoints
}

// sendToEndpoint connects to a single instance and executes the request.
func sendToEndpoint(ctx context.Context, ep instanceEndpoint, req Request) (*Response, error) {
	dialer := net.Dialer{Timeout: 600 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, ep.network, ep.address)
	if err != nil {
		// Clean up dead endpoint file
		_ = os.Remove(ep.file)
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if _, err := conn.Write(append(reqBytes, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write to IPC endpoint: %w", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read from IPC endpoint: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse IPC response: %w", err)
	}

	return &resp, nil
}

// SendCommand dispatches an IPC command across all running TUI instances.
// - For broadcast actions (refresh, show_toast, set_filter), all active instances receive the update.
// - For targeted actions (switch_tab, navigate_to_item), succeeds if any active instance matches.
func SendCommand(ctx context.Context, req Request) (*Response, error) {
	endpoints := discoverEndpoints()
	if len(endpoints) == 0 {
		return nil, ErrAppNotRunning
	}

	var lastSuccessResp *Response
	var successCount int
	var lastErr error

	for _, ep := range endpoints {
		resp, err := sendToEndpoint(ctx, ep, req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.Success {
			successCount++
			lastSuccessResp = resp
		}
	}

	if successCount > 0 {
		if successCount > 1 && (req.Command == CmdRefresh || req.Command == CmdShowToast || req.Command == CmdSetFilter) {
			return &Response{
				Success: true,
				Message: fmt.Sprintf("%s (applied across %d active TUI instances)", lastSuccessResp.Message, successCount),
			}, nil
		}
		return lastSuccessResp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrAppNotRunning, lastErr)
	}

	return nil, ErrAppNotRunning
}
