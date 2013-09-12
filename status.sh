#!/usr/bin/env bash

hosts="r1.apiv3 r2.apiv3 r3.apiv3 w1.apiv3 w2.apiv3 web1.apiv3 web2.apiv3 udp.apiv3"

for host in `echo $hosts | tr ' ' '\n'`; do
    echo "checking status of agent on $host"
    ssh $host "sudo service anomalous-agent status"
done
