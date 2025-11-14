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

1. **extract_leaf_configs** - Extracts FRR running configurations from all leaf nodes
2. **capture_traffic** - Captures network traffic from cluster nodes and spine router
3. **install_tshark** - Installs tshark prerequisite on all Kubernetes cluster nodes

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
This tool has a requirement: `tshark` must be installed in all the Kubernetes
cluster's nodes.

For that, you must execute the `install-tshark.sh` script before running it.

Once that is finished, you can just run the tool; the example below would
capture all the arp or ICMP packets in the Kubernetes cluster nodes, plus
in the spine router:
```sh
mkdir arp-or-icmp # the destination for the captures
CAPTURE_FILTER="arp or icmp" ./capture-traffic.sh arp-or-icmp
```

The user should `ctrl+c` to stop the capture when they're ready.

