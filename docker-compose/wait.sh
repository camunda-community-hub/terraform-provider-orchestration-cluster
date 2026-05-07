#!/bin/bash

set -o errexit -o nounset -o pipefail
IFS=$'\n\t'

while true
do
    if curl --silent --fail http://localhost:8080/v2/status > /dev/null; then
        echo "Application is up and running!"
        break
    else
        echo "Waiting for the application to be ready..."
        sleep 2
    fi
done
