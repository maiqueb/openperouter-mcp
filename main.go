package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

//go:embed scripts/extract-leaf-configs.sh
var extractLeafConfigsScript string

//go:embed scripts/capture-traffic.sh
var captureTrafficScript string

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      ClientInfo     `json:"clientInfo"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

type ServerCapabilities struct {
	Tools map[string]any `json:"tools,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
	Required   []string       `json:"required,omitempty"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type CallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type CallToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ActiveCall struct {
	ID     any
	Cancel context.CancelFunc
	Cmd    *exec.Cmd
}

type MCPServer struct {
	activeCalls map[string]*ActiveCall
	mu          sync.Mutex
	writer      io.Writer
}

func NewMCPServer(writer io.Writer) *MCPServer {
	return &MCPServer{
		activeCalls: make(map[string]*ActiveCall),
		writer:      writer,
	}
}

func (s *MCPServer) handleRequest(req JSONRPCRequest) JSONRPCResponse {
	switch req.Method {
	case "initialize":
		var params InitializeParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return s.errorResponse(req.ID, -32602, "Invalid params")
		}
		return s.handleInitialize(req.ID, params)
	case "tools/list":
		return s.handleToolsList(req.ID)
	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return s.errorResponse(req.ID, -32602, "Invalid params")
		}
		return s.handleToolCall(req.ID, params)
	default:
		return s.errorResponse(req.ID, -32601, "Method not found")
	}
}

func (s *MCPServer) handleInitialize(id any, params InitializeParams) JSONRPCResponse {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools: map[string]any{
				"listChanged": true,
			},
		},
		ServerInfo: ServerInfo{
			Name:    "openperouter-mcp",
			Version: "1.0.0",
		},
	}
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func (s *MCPServer) handleToolsList(id any) JSONRPCResponse {
	tools := []Tool{
		{
			Name:        "extract_leaf_configs",
			Description: "Extracts FRR running configurations from all leaf nodes in the CLAB topology. The configurations are saved to a timestamped directory.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]any{},
			},
		},
		{
			Name:        "start_traffic_capture",
			Description: "Starts capturing network traffic from Kubernetes cluster nodes and spine router using tshark. This operation starts in the background and returns immediately. Use stop_traffic_capture to stop the capture and retrieve files. Automatically installs tshark on nodes if needed.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]any{
					"output_dir": map[string]any{
						"type":        "string",
						"description": "Directory where capture files will be saved. Optional, defaults to './captures/capture_<timestamp>'.",
					},
					"capture_filter": map[string]any{
						"type":        "string",
						"description": "Tshark capture filter (e.g., 'arp or icmp'). Optional, defaults to capturing all traffic.",
					},
				},
				Required: []string{},
			},
		},
		{
			Name:        "stop_traffic_capture",
			Description: "Stops all running traffic captures, retrieves the pcap files from containers, and saves them to the host directory. This will gracefully terminate all tshark processes and copy the capture files.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]any{},
			},
		},
	}

	result := ToolsListResult{Tools: tools}
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func (s *MCPServer) handleToolCall(id any, params CallToolParams) JSONRPCResponse {
	var result CallToolResult

	switch params.Name {
	case "extract_leaf_configs":
		result = s.extractLeafConfigs()
	case "start_traffic_capture":
		result = s.startTrafficCapture(id, params.Arguments)
	case "stop_traffic_capture":
		result = s.stopTrafficCapture()
	default:
		return s.errorResponse(id, -32602, "Unknown tool: "+params.Name)
	}

	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

func executeScript(script string, args []string, env []string) (string, error) {
	cmd := exec.Command("bash", "-c", script)
	if len(args) > 0 {
		cmd.Args = append(cmd.Args, args...)
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (s *MCPServer) extractLeafConfigs() CallToolResult {
	output, err := executeScript(extractLeafConfigsScript, nil, nil)
	if err != nil {
		return CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: fmt.Sprintf("Error executing extract-leaf-configs.sh: %v\nOutput: %s", err, output),
			}},
			IsError: true,
		}
	}

	return CallToolResult{
		Content: []ContentItem{{
			Type: "text",
			Text: output,
		}},
	}
}

func (s *MCPServer) startTrafficCapture(id any, args map[string]any) CallToolResult {
	var scriptWithArgs string
	if outputDir, ok := args["output_dir"].(string); ok && outputDir != "" {
		scriptWithArgs = fmt.Sprintf("%s %s", captureTrafficScript, outputDir)
	} else {
		scriptWithArgs = captureTrafficScript
	}

	var env []string
	if captureFilter, ok := args["capture_filter"].(string); ok && captureFilter != "" {
		env = []string{fmt.Sprintf("CAPTURE_FILTER=%s", captureFilter)}
	}

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, "bash", "-c", scriptWithArgs)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: fmt.Sprintf("Error creating stdout pipe: %v", err),
			}},
			IsError: true,
		}
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: fmt.Sprintf("Error creating stderr pipe: %v", err),
			}},
			IsError: true,
		}
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: fmt.Sprintf("Error starting capture-traffic.sh: %v", err),
			}},
			IsError: true,
		}
	}

	requestID := fmt.Sprintf("%v", id)
	s.mu.Lock()
	s.activeCalls[requestID] = &ActiveCall{
		ID:     id,
		Cancel: cancel,
		Cmd:    cmd,
	}
	s.mu.Unlock()

	outputChan := make(chan string, 1)

	go func() {
		defer func() {
			cmd.Wait()
			s.mu.Lock()
			delete(s.activeCalls, requestID)
			s.mu.Unlock()
			cancel()
		}()

		scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
		var lines []string
		lineCount := 0
		maxLines := 20

		for scanner.Scan() && lineCount < maxLines {
			lines = append(lines, scanner.Text())
			lineCount++
		}

		if len(lines) > 0 {
			outputChan <- fmt.Sprintf("%s", lines[0])
		} else {
			outputChan <- "Capture started (no initial output yet)"
		}

		for scanner.Scan() {
		}
	}()

	var initialOutput string
	select {
	case initialOutput = <-outputChan:
	case <-time.After(5 * time.Second):
		initialOutput = "Capture process started (waiting for initial output timed out after 5s)"
	case <-ctx.Done():
		return CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: "Traffic capture was cancelled before starting.",
			}},
			IsError: false,
		}
	}

	return CallToolResult{
		Content: []ContentItem{{
			Type: "text",
			Text: fmt.Sprintf("Traffic capture started successfully and is running in the background (Request ID: %s).\n\nInitial output:\n%s\n\nThe capture will continue running. Use the stop_traffic_capture tool to stop all captures and retrieve the files.", requestID, initialOutput),
		}},
		IsError: false,
	}
}

func (s *MCPServer) stopTrafficCapture() CallToolResult {
	s.mu.Lock()

	var captureProcesses []*exec.Cmd
	var captureIDs []string

	for reqID, call := range s.activeCalls {
		if call.Cmd != nil && call.Cmd.Process != nil {
			captureProcesses = append(captureProcesses, call.Cmd)
			captureIDs = append(captureIDs, reqID)
		}
	}
	s.mu.Unlock()

	if len(captureProcesses) == 0 {
		return CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: "No active traffic captures found.",
			}},
			IsError: false,
		}
	}

	var stoppedCount int
	for i, cmd := range captureProcesses {
		reqID := captureIDs[i]
		if cmd.Process != nil {
			fmt.Fprintf(os.Stderr, "Stopping capture for request %s (PID: %d)\n", reqID, cmd.Process.Pid)
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to send SIGTERM to PID %d: %v\n", cmd.Process.Pid, err)
			} else {
				stoppedCount++
			}
		}
	}

	fmt.Fprintf(os.Stderr, "Waiting for captures to cleanup and copy files...\n")

	done := make(chan bool, 1)
	go func() {
		for _, cmd := range captureProcesses {
			cmd.Wait()
		}
		done <- true
	}()

	select {
	case <-done:
		fmt.Fprintf(os.Stderr, "All captures stopped successfully\n")
	case <-time.After(15 * time.Second):
		fmt.Fprintf(os.Stderr, "Timeout waiting for captures to stop, forcing kill\n")
		for _, cmd := range captureProcesses {
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
		}
	}

	return CallToolResult{
		Content: []ContentItem{{
			Type: "text",
			Text: fmt.Sprintf("Successfully stopped %d traffic capture(s).\n\nThe cleanup process has:\n- Terminated all tshark processes in containers\n- Copied pcap files from containers to the host\n\nCheck the output directory for the capture files.", stoppedCount),
		}},
		IsError: false,
	}
}

func (s *MCPServer) errorResponse(id any, code int, message string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
}

func main() {
	server := NewMCPServer(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)

	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			resp := server.errorResponse(nil, -32700, "Parse error")
			server.writeResponse(resp)
			continue
		}

		resp := server.handleRequest(req)
		server.writeResponse(resp)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(1)
	}
}

func (s *MCPServer) writeResponse(resp JSONRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling response: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
