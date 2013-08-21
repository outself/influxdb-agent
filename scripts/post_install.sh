#!/usr/bin/env bash

echo "Linking new version"

ln -sfn /data/errplane-agent/versions/REPLACE_VERSION /data/errplane-agent/current
ln -sfn /data/errplane-agent/current/agent /usr/bin/errplane-agent
ln -sfn /data/errplane-agent/current/agent_ctl /usr/bin/errplane-agent_ctl
ln -sfn /data/errplane-agent/current/config-generator /usr/bin/errplane-config-generator
ln -sfn /data/errplane-agent/current/sudoers-generator /usr/bin/errplane-sudoers-generator
ln -sfn /data/errplane-agent/shared/log.txt /data/errplane-agent/current/log.txt

chown errplane:errplane -R /data/errplane-agent/current
chown errplane:errplane -R /usr/bin/errplane-agent

if [ -x update-rc.d ]; then
    update-rc.d -f errplane-agent remove
    update-rc.d errplane-agent defaults
fi

echo "Finished updating the Errplane agent"
