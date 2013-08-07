#!/usr/bin/env bash

if [ $# -ne 1 ]; then
    echo "Usage: $0 version"
    exit 1
fi

version=$1
hosts="r1.apiv3"

for host in `echo $hosts | tr ' ' '\n'`; do
    echo "deploying to $host"
    # scp $file $host:/tmp
    ssh $host "cd /tmp && wget https://s3.amazonaws.com/errplane-agent/errplane-agent_${version}_amd64.deb && \
    sudo dpkg -i /tmp/errplane-agent_${version}_amd64.deb && \
    sudo -u errplane errplane-config-generator -api-key 962cdc9b-15e7-4b25-9a0d-24a45cfc6bc1 -app-key app4you2love -environment production && \
    sudo service errplane-agent restart && \
    sudo service errplane-agent status"
done
