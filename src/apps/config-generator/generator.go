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
general:
  udp-host: udp.apiv3.errplane.com:8126
  http-host: w.apiv3.errplane.com

  api-key:     %s # your api key (Settings/Organization)
  app-key:     %s # your app key (Settings/Applications)
  environment: %s # your environment (Settings/Applications)

  sleep: 10s                                    # frequency of sampling (accepted suffix, s for seconds, m for minutes and h for hours)
  proxy:                                        # proxy to use when making http requests (e.g. https://201.20.177.185:8080/)
  log-file: /data/errplane-agent/shared/log.txt # the log file of the agent
  log-level: debug                              # debug, info, warn, error
  top-n-processes: 10                           # For processes stats the agent will report the top n processes (by memory and cpu usage)

# processes:
#   - mysqld:
#       start:  service mysql start
#       stop:   service mysql start
#       status: ps
#       user:   root
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
