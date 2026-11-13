#!/bin/sh
echo "=== Testing Network Connectivity ==="
ping -c 3 8.8.8.8 || ping -c 3 1.1.1.1
