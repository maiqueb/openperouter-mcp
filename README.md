# openperouter-mcp

## The tools
This repo provides 2 different tools:
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

