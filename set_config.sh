#!/usr/bin/env bash

curl -v -X POST 'http://c.apiv3.errplane.com/databases/app4you2lovestaging/agent/staging.apiv3/configuration?api_key=962cdc9b-15e7-4b25-9a0d-24a45cfc6bc1' --data @- <<EOF
{
 "plugins":{"mysql":[]},
 "processes": [
   {"name": "mysqld", "status": "name", "start": "service mysql start"},
   {"name": "ruby", "status": "regex", "user":"rails", "regex":".*ruby.*v2_series_monitor.*", "start": "/var/www/app/current/lib/with_env.sh /var/www/app/current/lib/http_monitor/boot.rb staging"},
   {"name": "ruby", "status": "regex", "user":"rails", "regex":".*ruby.*http_monitor.*", "start": "/var/www/app/current/lib/with_env.sh /var/www/app/current/lib/v2_series_monitor/boot.rb staging"}
 ]
}
EOF

# curl -v -X POST 'http://c.staging.apiv3.errplane.com/databases/app4you2loveproduction/agent/r1.apiv3/configuration?api_key=962cdc9b-15e7-4b25-9a0d-24a45cfc6bc1' --data @- <<EOF
# {
#   "processes":[{"name":"chronos_server","regex":".*chronos_server.*chronos_server_config.*json.*","status":"regex","start":"/mnt/data-collector/bin/start.sh chronos_server","user":"ubuntu"}]
# }
# EOF

# curl -v -X POST 'http://c.staging.apiv3.errplane.com/databases/app4you2loveproduction/agent/app1/configuration?api_key=962cdc9b-15e7-4b25-9a0d-24a45cfc6bc1' --data @- <<EOF
# {
#   "plugins":{
#     "mysql":[{"name": "rds", "args": {"host": "errplaneprod.cjyimruzutir.us-east-1.rds.amazonaws.com", "user": "errplanepilotdb", "password": "3rrplan3", "port": "4406"}}]
#   },
#   "processes":[]
# }
# EOF
