#!/bin/bash

set -e

# Set capture filter from environment variable (default to ICMP)
CAPTURE_FILTER="${CAPTURE_FILTER:-icmp}"

# Function to ensure tshark is installed in containers
ensure_tshark() {
    echo "Checking tshark installation in containers..."
    echo "============================================================="

    for container in "${containers[@]}"; do
        # Check if container exists and is running
        if ! docker ps --format "table {{.Names}}" | grep -q "^$container$"; then
            echo "  Warning: Container $container not found or not running"
            echo ""
            continue
        fi

        # Check if tshark is installed
        if ! docker exec "$container" which tshark > /dev/null 2>&1; then
            echo "  tshark not found in $container, installing..."

            # Detect package manager and install tshark
            if docker exec "$container" which apt-get > /dev/null 2>&1; then
                docker exec "$container" apt-get update > /dev/null 2>&1
                docker exec "$container" apt-get install -y tshark > /dev/null 2>&1
            elif docker exec "$container" which yum > /dev/null 2>&1; then
                docker exec "$container" yum install -y wireshark > /dev/null 2>&1
            elif docker exec "$container" which apk > /dev/null 2>&1; then
                docker exec "$container" apk add --no-cache tshark > /dev/null 2>&1
            else
                echo "  ✗ Unknown package manager in container $container"
                continue
            fi

            # Verify installation
            if docker exec "$container" which tshark > /dev/null 2>&1; then
                echo "  ✓ tshark installed successfully in $container"
            else
                echo "  ✗ Failed to install tshark in $container"
            fi
        else
            echo "  ✓ tshark already installed in $container"
        fi
        echo ""
    done
    echo ""
}

# Check for optional positional parameter
if [ $# -gt 1 ]; then
    echo "Usage: $0 [host_output_directory]"
    echo "Example: $0 /tmp/captures"
    echo ""
    echo "If no directory is specified, a timestamped directory will be created automatically."
    echo ""
    echo "Environment variables:"
    echo "  CAPTURE_FILTER - tshark capture filter (default: icmp)"
    echo "                   Examples: 'tcp port 22', 'udp', 'host 192.168.1.1'"
    exit 1
fi

# Generate timestamped directory if not provided
if [ $# -eq 0 ]; then
    host_output_dir="./captures/capture_$(date +%Y%m%d_%H%M%S)"
    echo "No output directory specified, using: $host_output_dir"
else
    host_output_dir="$1"
fi

# Validate and create output directory if needed
if [ ! -d "$host_output_dir" ]; then
    echo "Creating output directory: $host_output_dir"
    mkdir -p "$host_output_dir"
fi

containers=(
    "pe-kind-a-control-plane"
    "pe-kind-b-control-plane" 
    "pe-kind-a-worker"
    "pe-kind-b-worker"
    "clab-kind-spine"
)

# Arrays to store container names and tshark PIDs for cleanup
capture_containers=()
capture_pids=()

# Cleanup function
cleanup() {
    echo "Stopping tshark captures..."
    for i in "${!capture_pids[@]}"; do
        pid="${capture_pids[$i]}"
        container="${capture_containers[$i]}"
        
        if docker exec "$container" kill -0 "$pid" 2>/dev/null; then
            # Send SIGTERM first to allow graceful shutdown
            docker exec "$container" kill -TERM "$pid" 2>/dev/null
            echo "  Sent SIGTERM to capture process $pid in container $container"
        fi
    done
    
    # Give processes time to terminate gracefully
    echo "Waiting for processes to terminate and files to be written..."
    sleep 3
    
    # Force kill any remaining processes
    for i in "${!capture_pids[@]}"; do
        pid="${capture_pids[$i]}"
        container="${capture_containers[$i]}"
        
        if docker exec "$container" kill -0 "$pid" 2>/dev/null; then
            docker exec "$container" kill -KILL "$pid" 2>/dev/null
            echo "  Force killed process $pid in container $container"
        fi
    done
    
    echo "Copying capture files from containers to host..."
    for container in "${containers[@]}"; do
        # Create the same safe filename as used during capture
        filter_name=$(echo "$CAPTURE_FILTER" | tr ' ' '_' | tr -cd '[:alnum:]_-')
        capture_file="/${filter_name}_capture_${container}.pcap"
        host_file="${host_output_dir}/${filter_name}_capture_${container}.pcap"
        
        echo "  Checking for files in container $container..."
        docker exec "$container" ls -la /*_capture_* 2>/dev/null || echo "    No capture files found"
        
        # Check if file exists in container first
        if docker exec "$container" test -f "$capture_file" 2>/dev/null; then
            echo "  ✓ Found $capture_file in container $container"
            file_size=$(docker exec "$container" stat -c%s "$capture_file" 2>/dev/null)
            echo "    File size: $file_size bytes"
            
            echo "  Copying from $container:$capture_file to $host_file"
            if docker cp "$container:$capture_file" "$host_file"; then
                echo "    ✓ Successfully copied"
            else
                echo "    ✗ Failed to copy (exit code: $?)"
            fi
        else
            echo "  ✗ File $capture_file not found in container $container"
        fi
    done
    
    echo "Cleanup completed. Capture files saved to: $host_output_dir"
    exit 0
}

# Set up signal handling for cleanup
trap cleanup SIGINT SIGTERM

# Ensure tshark is installed
ensure_tshark

echo "Extracting FRR container PIDs and starting tshark captures..."
echo "Using capture filter: $CAPTURE_FILTER"
echo "============================================================="

for container in "${containers[@]}"; do
    echo "Processing container: $container"
    
    # Check if container exists and is running
    if ! docker ps --format "table {{.Names}}" | grep -q "^$container$"; then
        echo "  Warning: Container $container not found or not running"
        echo ""
        continue
    fi
    
    # Create a safe filename based on the filter
    filter_name=$(echo "$CAPTURE_FILTER" | tr ' ' '_' | tr -cd '[:alnum:]_-')
    capture_file="/${filter_name}_capture_${container}.pcap"
    echo "  Starting tshark capture -> $capture_file"
    
    # Handle different container types
    if [[ "$container" == "clab-kind-spine" ]]; then
        # Direct capture in spine container (no FRR namespace needed)
        echo "  Using direct capture method for spine container"
        
        # Start tshark directly in the container and get its PID
        tshark_pid=$(docker exec "$container" bash -c "tshark -iany -n -t ad -f '$CAPTURE_FILTER' -w $capture_file -q & echo \$!")
        
        if [ -n "$tshark_pid" ]; then
            capture_containers+=("$container")
            capture_pids+=("$tshark_pid")
            echo "  Capture started with PID: $tshark_pid (inside container $container)"
            
            # Wait a moment and check if the process is still running inside the container
            sleep 1
            if docker exec "$container" kill -0 "$tshark_pid" 2>/dev/null; then
                echo "  ✓ Capture process is running inside container"
            else
                echo "  ✗ Capture process has already exited"
            fi
        else
            echo "  ✗ Failed to start tshark capture"
        fi
    else
        # FRR namespace capture for KIND containers
        echo "  Using FRR namespace capture method for KIND container"
        
        # Get FRR container from router pod using crictl inside the KIND container
        frr_container_id=$(docker exec "$container" crictl ps --name frr --state running 2>/dev/null | grep -E "router-[a-zA-Z0-9]+" | awk '{print $1}' | head -1)
        
        if [ -n "$frr_container_id" ]; then
            # Get the actual PID of the FRR container
            actual_pid=$(docker exec "$container" crictl inspect --output go-template --template '{{.info.pid}}' "$frr_container_id")
            
            if [ -n "$actual_pid" ]; then
                echo "  FRR container PID: $actual_pid"
                
                # Start tshark capture in the container's network namespace and capture its PID inside the container
                echo "  Debug: Starting tshark capture inside container namespace"
                
                # Start tshark in background and get its PID from inside the container
                tshark_pid=$(docker exec "$container" bash -c "nsenter -t $actual_pid -n tshark -iany -n -t ad -f '$CAPTURE_FILTER' -w $capture_file -q & echo \$!")
                
                if [ -n "$tshark_pid" ]; then
                    capture_containers+=("$container")
                    capture_pids+=("$tshark_pid")
                    echo "  Capture started with PID: $tshark_pid (inside container $container)"
                    
                    # Wait a moment and check if the process is still running inside the container
                    sleep 1
                    if docker exec "$container" kill -0 "$tshark_pid" 2>/dev/null; then
                        echo "  ✓ Capture process is running inside container"
                    else
                        echo "  ✗ Capture process has already exited"
                    fi
                else
                    echo "  ✗ Failed to start tshark capture"
                fi
            else
                echo "  Could not extract PID for FRR container"
            fi
        else
            echo "  No running FRR container found in router pod"
        fi
    fi
    
    echo ""
done

echo ""
echo "All captures started. Files will be saved inside containers as:"
filter_name=$(echo "$CAPTURE_FILTER" | tr ' ' '_' | tr -cd '[:alnum:]_-')
for container in "${containers[@]}"; do
    echo "  - $container:/${filter_name}_capture_${container}.pcap"
done

echo ""
echo "On cleanup, files will be copied to host directory: $host_output_dir"
echo "Press Ctrl+C to stop all captures and copy files to host."
echo "Captures are running in background..."

# Keep script running to maintain captures
while true; do
    sleep 1
done
