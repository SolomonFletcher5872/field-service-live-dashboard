#!/bin/sh
set -eu

curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"device_id":"meter-17","work_order_id":"wo-2048","dispatch_status":"on_site","photo_url":"https://photos.example/wo-2048.jpg","technician_follow_up":true}' \
  http://localhost:8080/device-updates
