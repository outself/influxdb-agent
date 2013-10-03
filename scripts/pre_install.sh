#/usr/bin/env bash

# create the anomalous user if it doesn't exist already
id anomalous >/dev/null || (echo "Creating 'anomalous' user" && useradd -r anomalous)
