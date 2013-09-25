#!/usr/bin/env bash

cd `dirname $0`

set -e

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version.number>"
    exit 1
fi

version=$1

function package_files {
    if [ $# -ne 1 ]; then
        echo "Usage: $0 architecture"
        return 1
    fi
    rm -rf package/anomalous-agent
    mkdir -p package/anomalous-agent
    pushd package
    cp ../agent anomalous-agent/
    cp ../sudoers-generator anomalous-agent/
    cp ../config-generator anomalous-agent/
    cp ../opensource.md anomalous-agent/
    cp ../scripts/init.d.sh anomalous-agent/
    cp ../scripts/anomalous-agent-daemon anomalous-agent/
    echo -n $version > anomalous-agent/version
    tar -cvzf anomalous-agent_${version}_$1.tar.gz anomalous-agent
    popd
}

# bulid and package the x86_64 version
UPDATE=on ./build.sh -v $version && package_files amd64 && \
    CGO_ENABLED=1 GOARCH=386 UPDATE=on ./build.sh -v $version && package_files 386
