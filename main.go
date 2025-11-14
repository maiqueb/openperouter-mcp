package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

//go:embed scripts/extract-leaf-configs.sh
var extractLeafConfigsScript string

//go:embed scripts/capture-traffic.sh
var captureTrafficScript string

//go:embed scripts/install-tshark.sh
var installTsharkScript string

// JSON-RPC 2.0 types
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP types
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    ServerCapabilities     `json:"capabilities"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
}

type ServerCapabilities struct {
	Tools map[string]interface{} `json:"tools,omitempty"`
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
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type CallToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MCPServer struct{}

func NewMCPServer() *MCPServer {
	return &MCPServer{}
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

func (s *MCPServer) handleInitialize(id interface{}, params InitializeParams) JSONRPCResponse {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools: map[string]interface{}{
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

func (s *MCPServer) handleToolsList(id interface{}) JSONRPCResponse {
	tools := []Tool{
		{
			Name:        "extract_leaf_configs",
			Description: "Extracts FRR running configurations from all leaf nodes in the CLAB topology. The configurations are saved to a timestamped directory.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]interface{}{},
			},
		},
		{
			Name:        "capture_traffic",
			Description: "Captures network traffic from Kubernetes cluster nodes and spine router using tshark. Requires tshark to be installed (use install_tshark first).",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"output_dir": map[string]interface{}{
						"type":        "string",
						"description": "Directory where capture files will be saved",
					},
					"capture_filter": map[string]interface{}{
						"type":        "string",
						"description": "Tshark capture filter (e.g., 'arp or icmp'). Optional, defaults to capturing all traffic.",
					},
				},
				Required: []string{"output_dir"},
			},
		},
		{
			Name:        "install_tshark",
			Description: "Installs tshark on all Kubernetes cluster nodes. This is a prerequisite for using capture_traffic.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]interface{}{},
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

func (s *MCPServer) handleToolCall(id interface{}, params CallToolParams) JSONRPCResponse {
	var result CallToolResult

	switch params.Name {
	case "extract_leaf_configs":
		result = s.extractLeafConfigs()
	case "capture_traffic":
		result = s.captureTraffic(params.Arguments)
	case "install_tshark":
		result = s.installTshark()
	default:
		return s.errorResponse(id, -32602, "Unknown tool: "+params.Name)
	}

	return JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// executeScript runs a bash script from embedded content
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

func (s *MCPServer) captureTraffic(args map[string]interface{}) CallToolResult {
	outputDir, ok := args["output_dir"].(string)
	if !ok {
		return CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: "Missing or invalid 'output_dir' argument",
			}},
			IsError: true,
		}
	}

	// Build the command with the output directory argument
	scriptWithArgs := fmt.Sprintf("%s %s", captureTrafficScript, outputDir)

	// Set CAPTURE_FILTER environment variable if provided
	var env []string
	if captureFilter, ok := args["capture_filter"].(string); ok && captureFilter != "" {
		env = []string{fmt.Sprintf("CAPTURE_FILTER=%s", captureFilter)}
	}

	output, err := executeScript(scriptWithArgs, nil, env)
	if err != nil {
		return CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: fmt.Sprintf("Error executing capture-traffic.sh: %v\nOutput: %s", err, output),
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

func (s *MCPServer) installTshark() CallToolResult {
	output, err := executeScript(installTsharkScript, nil, nil)
	if err != nil {
		return CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: fmt.Sprintf("Error executing install-tshark.sh: %v\nOutput: %s", err, output),
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

func (s *MCPServer) errorResponse(id interface{}, code int, message string) JSONRPCResponse {
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
	server := NewMCPServer()
	scanner := bufio.NewScanner(os.Stdin)

	// Increase buffer size for large requests
	const maxCapacity = 1024 * 1024 // 1MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			// Write error response
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
