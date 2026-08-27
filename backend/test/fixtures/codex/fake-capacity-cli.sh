#!/bin/sh
set -eu
cat >/dev/null
printf '%s\n' \
  '{"type":"thread.started","thread_id":"capacity-fixture"}' \
  '{"type":"turn.started"}' \
  '{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"{\"status\":\"ok\"}"}}' \
  '{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":1}}'
