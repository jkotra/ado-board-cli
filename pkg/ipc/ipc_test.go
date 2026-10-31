package ipc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIPCServerAndClientLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tempDir)

	// 1. Client fails when server is not running
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := SendCommand(ctx, Request{Command: CmdPing})
	if err == nil {
		t.Fatalf("expected error when server is not running")
	}

	// 2. Start server
	receivedCmd := make(chan Request, 1)
	srv, err := StartServer(func(req Request) Response {
		receivedCmd <- req
		return Response{
			Success: true,
			Message: "handled " + string(req.Command),
			Data:    map[string]any{"echo_tab": req.TabIndex},
		}
	})
	if err != nil {
		t.Fatalf("failed to start IPC server: %v", err)
	}
	defer srv.Close()

	// 3. Test ping
	pingResp, err := SendCommand(ctx, Request{Command: CmdPing})
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if !pingResp.Success || pingResp.Message != "pong" {
		t.Fatalf("unexpected ping response: %+v", pingResp)
	}

	// 4. Test custom command (switch_tab)
	tabResp, err := SendCommand(ctx, Request{
		Command:  CmdSwitchTab,
		TabIndex: 3,
		TabName:  "Feature X",
	})
	if err != nil {
		t.Fatalf("switch_tab failed: %v", err)
	}
	if !tabResp.Success || tabResp.Message != "handled switch_tab" {
		t.Fatalf("unexpected switch_tab response: %+v", tabResp)
	}

	select {
	case req := <-receivedCmd:
		if req.Command != CmdSwitchTab || req.TabIndex != 3 || req.TabName != "Feature X" {
			t.Fatalf("unexpected received command: %+v", req)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for command on server")
	}
}

func TestMultiInstanceBroadcastAndCleanup(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tempDir)

	ctx := context.Background()

	// 1. Create a dummy dead endpoint file to test automatic stale cleanup
	ipcDir := GetIPCDir()
	deadSockFile := filepath.Join(ipcDir, "ipc_99999999.sock")
	_ = os.WriteFile(deadSockFile, []byte("fake"), 0600)

	deadPortFile := filepath.Join(ipcDir, "ipc_99999998.port")
	_ = os.WriteFile(deadPortFile, []byte("65534"), 0600)

	// 2. Start two mock servers (simulating 2 running TUI instances)
	s1Received := make(chan Request, 2)
	srv1, err := StartServer(func(req Request) Response {
		s1Received <- req
		return Response{Success: true, Message: "Instance 1 OK"}
	})
	if err != nil {
		t.Fatalf("srv1 failed: %v", err)
	}
	defer srv1.Close()

	// Create simulated stale port file
	s2FilePath := filepath.Join(ipcDir, "ipc_88888.port")
	_ = os.WriteFile(s2FilePath, []byte("65533"), 0600)

	// Test broadcast refresh
	resp, err := SendCommand(ctx, Request{Command: CmdRefresh})
	if err != nil {
		t.Fatalf("refresh broadcast failed: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response: %+v", resp)
	}

	// Verify dead files were cleaned up
	if _, err := os.Stat(deadSockFile); err == nil {
		t.Errorf("expected dead sock file to be removed")
	}
}

func TestIPCGetState(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tempDir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mockState := &AppStateSummary{
		IsRunning:    true,
		ActiveScreen: "tree",
		ActiveTab: TabSummary{
			Index:    0,
			Name:     "Feature 100",
			RootID:   100,
			IsActive: true,
		},
		SelectedItem: &WorkItemSummary{
			ID:            101,
			Type:          "Task",
			Title:         "Inspect Item",
			State:         "Active",
			RemainingWork: 4.5,
		},
	}

	srv, err := StartServer(func(req Request) Response {
		if req.Command == CmdGetState {
			return Response{
				Success: true,
				Message: "TUI state retrieved",
				Data:    mockState,
			}
		}
		return Response{Success: false, Message: "unknown command"}
	})
	if err != nil {
		t.Fatalf("failed to start IPC server: %v", err)
	}
	defer srv.Close()

	resp, err := SendCommand(ctx, Request{Command: CmdGetState})
	if err != nil {
		t.Fatalf("failed to send CmdGetState: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got %+v", resp)
	}

	dataMap, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be map[string]any, got %T", resp.Data)
	}
	if dataMap["is_running"] != true {
		t.Fatalf("expected is_running to be true, got %v", dataMap["is_running"])
	}
	selItem, ok := dataMap["selected_item"].(map[string]any)
	if !ok || selItem["id"] != float64(101) || selItem["title"] != "Inspect Item" {
		t.Fatalf("unexpected selected_item: %v", dataMap["selected_item"])
	}
}
