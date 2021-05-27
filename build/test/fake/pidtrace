#!/bin/bash
set -euo pipefail

pid=$1

if [ "$pid" = "" ]; then
    echo "Usage: pidtrace <PID>"
    exit 1
fi

if [ ! -d "/proc/$pid" ]; then
    echo "PID $pid not found!"
    exit 1
fi

echo "Tracing PID $pid..."
echo
echo "  nspid: $(cat /proc/$pid/status | awk '/NSpid/ { print $NF }')"
echo "    exe: $(readlink /proc/$pid/exe)"
echo "   comm: $(cat /proc/$pid/comm)"
echo "cmdline: $(cat /proc/$pid/cmdline | tr -d \\0)"

while [[ ! -f "/var/run/trace-uploader" ]]; do
  sleep 1
done
