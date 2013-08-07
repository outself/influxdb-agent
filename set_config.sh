#!/usr/bin/env bash

curl -v -X POST 'http://c.apiv3.errplane.com/databases/app4you2lovestaging/agent/thoth/configuration?api_key=962cdc9b-15e7-4b25-9a0d-24a45cfc6bc1' --data @- <<EOF
{
 "plugins":{"redis":[]},
 "processes": [
   {"name": "mysqld", "status": "name", "start": "service mysql start"},
   {"name": "redis-server", "status": "name"}
 ]
}
EOF
