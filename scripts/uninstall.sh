#!/usr/bin/env bash

set -e

# stop the agent
/etc/init.d/anomalous-agent stop

id anomalous >/dev/null && (echo "Removing 'anomalous' user" && userdel -f anomalous)

anomalous_dir=/data/anomalous-agent
anomalous_conf=/etc/anomalous-agent

if which update-rc.d > /dev/null 2>&1 ; then
    update-rc.d -f anomalous-agent remove
else
    chkconfig --del anomalous-agent
fi

rm -rf $anomalous_dir
rm -rf $anomalous_conf
rm -f /usr/bin/anomalous-agent
rm -f /usr/bin/anomalous-agent-daemon
rm -f /usr/bin/anomalous-config-generator
rm -f /usr/bin/anomalous-sudoers-generator

rm /etc/init.d/anomalous-agent
