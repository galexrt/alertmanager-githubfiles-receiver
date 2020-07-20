#!/bin/bash

set -x

STATE="${1:-firing}"

curl \
    -v \
    -X POST \
    -H "Content-Type: application/json" \
    --data-binary @"alertmanager-webhook-$STATE.json" \
    http://127.0.0.1:9959/v1/receiver
