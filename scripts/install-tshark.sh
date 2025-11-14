#!/bin/bash

containers=(
    "pe-kind-a-control-plane"
    "pe-kind-b-control-plane"
    "pe-kind-a-worker"
    "pe-kind-b-worker"
)

echo "Installing tshark in docker containers..."

for container in "${containers[@]}"; do
    echo "Installing tshark in container: $container"
    
    if docker exec "$container" which apt-get > /dev/null 2>&1; then
        docker exec "$container" apt-get update
        docker exec "$container" apt-get install -y tshark
    elif docker exec "$container" which yum > /dev/null 2>&1; then
        docker exec "$container" yum install -y wireshark
    elif docker exec "$container" which apk > /dev/null 2>&1; then
        docker exec "$container" apk add --no-cache tshark
    else
        echo "Unknown package manager in container $container"
        continue
    fi
    
    echo "Tshark installed successfully in $container"
    echo "---"
done

echo "Installation complete!"