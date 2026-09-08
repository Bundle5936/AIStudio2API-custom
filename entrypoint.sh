#!/bin/sh
set -e

./aistudio-omni -open-ui=false &
PID=$!

# 后台等待服务就绪后自动激活数据面
(
  for i in $(seq 1 30); do
    if curl -s http://127.0.0.1:2048/health >/dev/null 2>&1; then
      sleep 1
      curl -s -X POST http://127.0.0.1:2048/api/control/start >/dev/null 2>&1
      break
    fi
    sleep 1
  done
) &

wait "$PID"
