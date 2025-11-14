# openperouter-mcp

MCP (Model Context Protocol) server for OpenPERouter tools, providing network debugging and configuration extraction capabilities for Claude Code.

## MCP Server

This repository includes a Go-based MCP server (`mcp-server`) that exposes the bash scripts as tools for Claude Code.

### Building the MCP Server

#### Native Build

```sh
go build -o mcp-server main.go
# or using make
make build
```

#### Container Build

The project supports both Podman (default) and Docker for containerization:

```sh
# Build with podman (default)
make container-build

# Build with docker
make CONTAINER_RUNTIME=docker container-build

# Run the container
make container-run

# Or with docker
make CONTAINER_RUNTIME=docker container-run
```

The old `docker-build` and `docker-run` targets are still available for backward compatibility.

### MCP Tools Available

The MCP server exposes three tools:

1. **extract_leaf_configs** - Extracts FRR running configurations from all leaf nodes in the CLAB topology. Configurations are saved to a timestamped directory.

2. **start_traffic_capture** - Starts capturing network traffic from Kubernetes cluster nodes and spine router using tshark. This operation starts in the background and returns immediately. Automatically installs tshark on nodes if needed.
   - Parameters:
     - `output_dir` (optional): Directory where capture files will be saved. Defaults to `./captures/capture_<timestamp>`.
     - `capture_filter` (optional): Tshark capture filter (e.g., 'arp or icmp'). Defaults to capturing all traffic.

3. **stop_traffic_capture** - Stops all running traffic captures, retrieves the pcap files from containers, and saves them to the host directory. This will gracefully terminate all tshark processes and copy the capture files.

### Using with Claude Code

The MCP server is configured in the openperouter project at `.claude/mcp.json`. After building, restart Claude Code to load the tools.

## The tools (Direct Usage)
This repo provides 2 different bash scripts that can also be used directly:
- `extract-leaf-configs.sh`
- `capture-traffic.sh`

### Extract router's running configurations
This tool accesses all the routers in the CLAB topology and extracts the FRR
routers running configuration, saving it in a folder.

The user should just invoke the script:
```sh
./extract-leaf-configs.sh
```

### Capture traffic
This tool automatically installs `tshark` on all Kubernetes cluster nodes if not already present.

#### Basic usage (auto-generated timestamped directory):
```sh
# Captures to ./captures/capture_<timestamp>
./capture-traffic.sh

# With custom filter
CAPTURE_FILTER="arp or icmp" ./capture-traffic.sh
```

#### Custom output directory:
```sh
mkdir arp-or-icmp # the destination for the captures
CAPTURE_FILTER="arp or icmp" ./capture-traffic.sh arp-or-icmp
```

The user should `ctrl+c` to stop the capture when they're ready.

**Note**: The script now automatically detects the package manager (apt-get, yum, or apk) in each container and installs tshark as needed, eliminating the manual installation step.

