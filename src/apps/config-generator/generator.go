package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var (
		apiKey           = flag.String("api-key", "", "The api key from (Settings/Orginzation)")
		appKey           = flag.String("app-key", "", "The application key from (Settings/Applications)")
		env              = flag.String("environment", "production", "The environment from (Settings/Applications)")
		httpHost         = flag.String("http-host", "w.apiv3.errplane.com", "The path to the generated config file")
		configHost       = flag.String("config-host", "c.apiv3.errplane.com", "The path to the generated config file")
		path             = flag.String("path", "/etc/anomalous-agent/config.yml", "The path to the generated config file")
		ws               = flag.String("config-ws", "ec2-23-20-52-199.compute-1.amazonaws.com:8095", "The url of the configuration service websocket")
		pluginsDir       = flag.String("plugins-dir", "/data/anomalous-agent/shared/plugins", "The directory where the plugins will be downloaded")
		customPluginsDir = flag.String("custom-plugins-dir", "/data/anomalous-agent/shared/custom-plugins", "The directory where custom plugins will be looked up")
	)

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

http-host: %s

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
log-file: /data/anomalous-agent/shared/log.txt # the log file of the agent
log-level: info                               # debug, info, warn, error
top-n-processes: 5                            # For processes stats the agent will report the top n processes (by memory and cpu usage)
top-n-sleep:     1m                           # Sampling frequency of the top n processes
monitored-sleep: 10s                          # Sampling frequency of the monitored processes
config-service:  %s											      # the location of the configuration service
datastore-dir: /data/anomalous-agent/shared/db

config-websocket: %s												  # the location of the configuration server websocket
websocket-ping: 60s													  # the websocket ping interval

# plugins directories
plugins-dir: %s
custom-plugins-dir: %s

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

	content := fmt.Sprintf(sample, *httpHost, *apiKey, *appKey, *env, *configHost, *ws, *pluginsDir, *customPluginsDir)
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
