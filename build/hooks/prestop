#!/bin/bash
set -uo pipefail

if [[ "$#" -gt "1" ]]; then
  echo "usage: prestop [SLEEP_SECONDS]"
  exit 41
fi

sleep_seconds=0
if [[ "$#" -eq "1" ]]; then
  sleep_seconds="$1"
fi

tpid=`pgrep --oldest trace-runner`
if [[ -z "$tpid" ]]; then
  echo "could not find trace-runner"
  exit 21
fi

cpid=`pgrep --oldest -P $tpid`
if [[ -z "$cpid" ]]; then
  echo "could not find first child of trace-runner"
  exit 22
fi

kill -SIGINT $cpid

## Give some time to trace-runner to cleanup before pod kill timeout starts.
sleep $sleep_seconds
