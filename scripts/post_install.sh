#!/usr/bin/env bash

echo "Linking new version"

if [ -d /data/anomalous-agent/plugins ]; then
    mv /data/anomalous-agent/plugins/* /data/anomalous-agent/shared/plugins/
    rmdir /data/anomalous-agent/plugins/
fi
ln -sfn /data/anomalous-agent/versions/REPLACE_VERSION /data/anomalous-agent/current
ln -sfn /data/anomalous-agent/current/agent /usr/bin/anomalous-agent
ln -sfn /data/anomalous-agent/current/agent_ctl /usr/bin/anomalous-agent_ctl
ln -sfn /data/anomalous-agent/current/anomalous-agent-daemon /usr/bin/anomalous-agent-daemon
ln -sfn /data/anomalous-agent/current/config-generator /usr/bin/anomalous-config-generator
ln -sfn /data/anomalous-agent/current/sudoers-generator /usr/bin/anomalous-sudoers-generator
ln -sfn /data/anomalous-agent/shared/log.txt /data/anomalous-agent/current/log.txt

chown anomalous:anomalous -R /data/anomalous-agent/current
chown anomalous:anomalous -R /usr/bin/anomalous-agent

if which update-rc.d > /dev/null 2>&1 ; then
    update-rc.d -f anomalous-agent remove
    update-rc.d anomalous-agent defaults
else
    chkconfig --add anomalous-agent
fi

echo "Finished updating the Anomalous agent"
