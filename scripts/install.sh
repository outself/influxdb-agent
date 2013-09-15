#!/usr/bin/env bash

# This script will install the anomalous agent in these locations:
#   - /data/anomalous-agent/

# exit on errors
set -e

if [ $# -ne 2 ]; then
    echo "usage: $0 <app_key> <api_key>"
    exit 1
fi

api_key=$1
app_key=$2

echo "Using app_key: $app_key and api_key: $api_key"

file=anomalous-agent_latest_amd64.tar.gz
anomalous_dir=/data/anomalous-agent
anomalous_conf=/etc/anomalous-agent/config.yml
link=https://s3.amazonaws.com/errplane-agent/$file

# create the anomalous user if it doesn't exist already
id anomalous || (echo "Creating 'anomalous' user" && useradd -r anomalous)

[ "x$TEMPDIR" == "x" ] && TEMPDIR=/tmp

pushd $TEMPDIR
echo "Downloading package from $link"
wget $link >/dev/null
tar -xvzf $file >/dev/null
version=`cat anomalous-agent/version`
[ ! -d $anomalous_dir ] && mkdir -p $anomalous_dir
cp -r anomalous-agent $anomalous_dir/versions/$version

# create some directories that that agent assume exist
mkdir -p $anomalous_dir/shared/plugins/

# touch the log file if it doesn't exist
[ -e $anomalous_dir/shared/log.txt ] || touch $anomalous_dir/shared/log.txt

# create some symlinks
ln -sfn $anomalous_dir/versions/$version                $anomalous_dir/current
ln -sfn $anomalous_dir/current/agent                    /usr/bin/anomalous-agent
# ln -sfn $anomalous_dir/current/agent_ctl                /usr/bin/anomalous-agent_ctl
# ln -sfn $anomalous_dir/current/anomalous-agent-daemon   /usr/bin/anomalous-agent-daemon
ln -sfn $anomalous_dir/current/config-generator         /usr/bin/anomalous-config-generator
ln -sfn $anomalous_dir/current/sudoers-generator        /usr/bin/anomalous-sudoers-generator
ln -sfn $anomalous_dir/shared/log.txt                   $anomalous_dir/current/log.txt

# make sure the files are owned by the right user
chown anomalous:anomalous -R $anomalous_dir
chown anomalous:anomalous -R /usr/bin/anomalous-agent

if which update-rc.d > /dev/null 2>&1 ; then
    update-rc.d -f anomalous-agent remove
    update-rc.d anomalous-agent defaults
else
    chkconfig --add anomalous-agent
fi

[ -x $anomalous_conf ] || sudo -u anomalous anomalous-config-generator -api-key $api_key -app-key $app_key
popd

# finally restart the agent
service anomalous-agent restart
