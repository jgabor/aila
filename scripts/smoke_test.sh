#!/bin/bash
rm -rf .aila/sessions
tmux kill-session -t smoke-final 2>/dev/null || true
tmux new-session -d -s smoke-final "./build/aila 2> stderr_smoke.log"
sleep 5
tmux send-keys -t smoke-final 'SmokeTest' C-m
sleep 5
tmux send-keys -t smoke-final '/quit' C-m
sleep 2
tmux new-session -d -s smoke-final "./build/aila 2>> stderr_smoke.log"
sleep 5
tmux send-keys -t smoke-final '/continue' C-m
sleep 5
tmux capture-pane -t smoke-final -p > capture_final.txt
tmux kill-session -t smoke-final
