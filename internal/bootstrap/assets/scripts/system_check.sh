#!/bin/sh
echo "=== System Information ==="
uname -a
echo ""
echo "=== Disk Usage ==="
df -h
echo ""
echo "=== Memory Info ==="
if [ -f /proc/meminfo ]; then
    head -n 5 /proc/meminfo
fi
