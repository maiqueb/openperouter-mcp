#!/bin/bash

# Script to extract FRR running configurations from all leaf nodes
# Handles both regular containerlab FRR containers and FRR containers inside kind clusters

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Create output directory with timestamp
OUTPUT_DIR="leaf_configs_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$OUTPUT_DIR"

echo -e "${GREEN}=== Extracting FRR configurations from all leaf nodes ===${NC}"
echo -e "${BLUE}Output directory: $OUTPUT_DIR${NC}"
echo

# Function to extract config from regular containerlab FRR container
extract_regular_leaf_config() {
    local container_name="$1"
    local output_file="$2"
    
    echo -e "${YELLOW}Processing regular leaf: $container_name${NC}"
    
    if docker exec "$container_name" vtysh -c "show running-config" > "$output_file" 2>/dev/null; then
        echo -e "${GREEN}  ✓ Config saved to $output_file${NC}"
        return 0
    else
        echo -e "${RED}  ✗ Failed to extract config from $container_name${NC}"
        return 1
    fi
}

# Function to extract config from FRR container inside kind cluster
extract_kind_leaf_config() {
    local kind_cluster="$1"
    
    echo -e "${YELLOW}Processing kind cluster: $kind_cluster${NC}"
    
    # Get the kind nodes for this cluster
    local kind_nodes=$(docker ps --filter "name=${kind_cluster}" --format "{{.Names}}" | grep -E "(control-plane|worker)")
    
    if [[ -z "$kind_nodes" ]]; then
        echo -e "${RED}  ✗ No nodes found for cluster $kind_cluster${NC}"
        return 1
    fi
    
    for node in $kind_nodes; do
        echo -e "  ${BLUE}Checking node: $node${NC}"
        
        # Get FRR container IDs inside the kind node
        local frr_container_ids=$(docker exec "$node" crictl ps --name "^frr$" -q 2>/dev/null || true)
        
        if [[ -n "$frr_container_ids" ]]; then
            local count=1
            while IFS= read -r frr_container_id; do
                if [[ -n "$frr_container_id" ]]; then
                    echo "    Found FRR container: $frr_container_id"
                    
                    # Create filename based on node type and container count
                    local node_type=${node##*-}  # control-plane or worker
                    local output_file="$OUTPUT_DIR/${kind_cluster}_${node_type}_frr${count}_config.txt"
                    
                    if docker exec "$node" crictl exec "$frr_container_id" vtysh -c "show running-config" > "$output_file" 2>/dev/null; then
                        echo -e "${GREEN}    ✓ Config saved to $output_file${NC}"
                    else
                        echo -e "${RED}    ✗ Failed to extract config from FRR container $frr_container_id${NC}"
                    fi
                    ((count++))
                fi
            done <<< "$frr_container_ids"
        else
            echo -e "${RED}    ✗ No FRR containers found in $node${NC}"
        fi
    done
}

# Extract configs from regular containerlab FRR leaf containers
echo -e "${GREEN}=== Processing regular containerlab leaf nodes ===${NC}"
regular_leaves=$(docker ps --filter "name=clab-kind-leaf" --format "{{.Names}}" | grep -E "leaf[A-Z]|leafkind" || true)

if [[ -n "$regular_leaves" ]]; then
    while IFS= read -r leaf; do
        if [[ -n "$leaf" ]]; then
            # Extract just the leaf name (remove clab-kind- prefix)
            leaf_name=${leaf#clab-kind-}
            output_file="$OUTPUT_DIR/${leaf_name}_config.txt"
            extract_regular_leaf_config "$leaf" "$output_file"
        fi
    done <<< "$regular_leaves"
else
    echo -e "${YELLOW}No regular leaf containers found${NC}"
fi
echo

# Extract configs from FRR containers inside kind clusters
echo -e "${GREEN}=== Processing kind cluster leaf nodes ===${NC}"
kind_clusters=$(kind get clusters 2>/dev/null || true)

if [[ -n "$kind_clusters" ]]; then
    while IFS= read -r cluster; do
        if [[ -n "$cluster" ]]; then
            extract_kind_leaf_config "$cluster"
        fi
    done <<< "$kind_clusters"
else
    echo -e "${YELLOW}No kind clusters found${NC}"
fi
echo

echo -e "${GREEN}=== Configuration extraction complete ===${NC}"
echo -e "${BLUE}All configurations saved to: $OUTPUT_DIR${NC}"

# List all extracted files
echo
echo -e "${BLUE}Extracted configuration files:${NC}"
if ls "$OUTPUT_DIR"/*.txt >/dev/null 2>&1; then
    ls -la "$OUTPUT_DIR"/*.txt
else
    echo -e "${YELLOW}No configuration files were extracted${NC}"
fi
