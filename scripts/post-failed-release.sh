#!/bin/sh
set -eu

curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"job":"release-cli","run_id":"832","revision":"b20d","operation":"publish","status":"failed","detail":"artifact checksum mismatch"}' \
  http://127.0.0.1:8080/job-events
