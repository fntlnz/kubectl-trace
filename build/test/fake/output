#!/bin/bash
set -euo pipefail

while [[ ! -f "/var/run/trace-uploader" ]]; do
  echo $(date)
  sleep 1
done
echo "trace-uploader pid found at /var/run/trace-uploader"
