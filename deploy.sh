#!/usr/bin/env bash

version=latest
# these are the hosts I have in my ssh config (it won't work unless you have my ssh config)
hosts="r1.apiv3 r2.apiv3 r3.apiv3 w1.apiv3 w2.apiv3 udp.apiv3 web1.apiv3 web2.apiv3 redis.apiv3 chronos1 chronos2 staging.apiv3"

for host in `echo $hosts | tr ' ' '\n'`; do
    echo "deploying to $host"

    staging=".staging"
    env="production"
    if [ $host = staging.apiv3 ]; then
        staging=""
        env="staging"
    fi

    # scp $file $host:/tmp
    # ssh $host "sudo useradd -r errplane; echo $host | sudo tee /etc/hostname && sudo hostname $host"
    ssh $host "cd /tmp && rm errplane-agent*; wget https://s3.amazonaws.com/errplane-agent/errplane-agent_${version}_amd64.deb && \
    sudo dpkg -i /tmp/errplane-agent_${version}_amd64.deb ; \
    sudo -u errplane errplane-config-generator -api-key ignored -app-key app4you2love -http-host w$staging.apiv3.errplane.com \
    -udp-host udp$staging.apiv3.errplane.com -config-host c$staging.apiv3.errplane.com -environment $env; \
    sudo pkill errplane-agent; \
    sudo service errplane-agent restart; \
    sudo service errplane-agent status"
done
