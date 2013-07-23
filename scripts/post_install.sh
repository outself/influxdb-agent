#!/usr/bin/env bash

echo "Linking new version"

ln -sfn /data/errplane-agent/versions/REPLACE_VERSION /data/errplane-agent/current
ln -sfn /data/errplane-agent/current/agent /usr/bin/errplane-agent
ln -sfn /data/errplane-agent/shared/log /data/errplane-agent/current/log

if [ ! -e "/etc/errplane-agent/config.yml" ]; then
    cp /data/errplane-agent/current/sample_config.yml /etc/errplane-agent/config.yml
fi

chown -R errplane:errplane /data/errplane-agent/shared/log
chown -R errplane:errplane /data/errplane-agent/current/

echo "Finished updating the Errplane agent"
