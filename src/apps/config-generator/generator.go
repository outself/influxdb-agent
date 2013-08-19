package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	apiKey := flag.String("api-key", "", "The api key from (Settings/Orginzation)")
	appKey := flag.String("app-key", "", "The application key from (Settings/Applications)")
	env := flag.String("environment", "", "The environment from (Settings/Applications)")
	path := flag.String("path", "/etc/errplane-agent/config.yml", "The path to the generated config file")

	flag.Parse()

	if *apiKey == "" {
		fmt.Fprintf(os.Stderr, "Api key is missing\n")
		os.Exit(1)
	}
	if *appKey == "" {
		fmt.Fprintf(os.Stderr, "Application key is missing\n")
		os.Exit(1)
	}
	if *env == "" {
		fmt.Fprintf(os.Stderr, "Environment name is missing\n")
		os.Exit(1)
	}

	sample := `
#####################################
##   Errplane agent configuration  ##
#####################################

udp-host: udp.apiv3.errplane.com:8126
http-host: w.apiv3.errplane.com

api-key:     %s # your api key (Settings/Organization)
app-key:     %s # your app key (Settings/Applications)
environment: %s # your environment (Settings/Applications)

# aggregator configuration
percentiles:						# the percentiles that will be calculated and sent to Errplane
  - 80.0
  - 90.0
  - 95.0
  - 99.0
flush-interval: 10s			# the rollup interval
udp-addr: :8127					# the udp port on which the aggregator will listen

sleep: 1m                                     # frequency of sampling (accepted suffix, s for seconds, m for minutes and h for hours)
proxy:                                        # proxy to use when making http requests (e.g. https://201.20.177.185:8080/)
log-file: /data/errplane-agent/shared/log.txt # the log file of the agent
log-level: debug                              # debug, info, warn, error
top-n-processes: 5                            # For processes stats the agent will report the top n processes (by memory and cpu usage)
top-n-sleep:     1m                           # Sampling frequency of the top n processes
monitored-sleep: 10s                          # Sampling frequency of the monitored processes
config-service:  c.apiv3.errplane.com         # the location of the configuration service

# processes:
#   - name:   mysqld
#     start:  service mysql start             # the command to run to start the service
#     stop:   service mysql start             # the command to run to stop the service
#     status: name                            # check the status of the process using the specified method:
#     regex: .*ruby.*status-monitor.*         # see 'status' above
#     user:   root                            # the agent will run the start and stop command using 'sudo -u username command-to-run'

# enabled-plugins:
#   - name: redis       # the name of the plugin
#     instances:        # optional, otherwise the agent will assume there is one instance running and will pass no args to the plugin
#     - name: default   # optional, default value is 'default'
#       args:
#         port: 6379    # call the plugin with --port 6379
`

	content := fmt.Sprintf(sample, *apiKey, *appKey, *env)
	file, err := os.Create(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot open %s. Error: %s\n", *path, err)
		os.Exit(1)
	}
	_, err = file.Write([]byte(content))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot write to %s. Error: %s\n", *path, err)
		os.Exit(1)
	}
	err = file.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot close file. Error: %s\n", err)
		os.Exit(1)
	}
}
