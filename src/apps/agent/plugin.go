package main

import (
	"bytes"
	log "code.google.com/p/log4go"
	"encoding/json"
	"fmt"
	"github.com/errplane/errplane-go"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	. "utils"
)

type PluginStateOutput int

type ProcessState interface {
	ExitStatus() int
}

type ProcessStateWrapper struct {
	status *os.ProcessState
}

func (self *ProcessStateWrapper) ExitStatus() int {
	return self.status.Sys().(syscall.WaitStatus).ExitStatus()
}

func (p *PluginStateOutput) String() string {
	switch *p {
	case OK:
		return "Ok"
	case WARNING:
		return "Warning"
	case CRITICAL:
		return "Critical"
	case UNKNOWN:
		return "Unknown"
	default:
		panic(fmt.Errorf("WTF unknown state %d", *p))
	}
}

const (
	OK PluginStateOutput = iota
	WARNING
	CRITICAL
	UNKNOWN
)

type PluginOutput struct {
	state   PluginStateOutput
	msg     string
	points  []*errplane.JsonPoints
	metrics map[string]float64
}

// handles running plugins

func monitorPlugins(ep *errplane.Errplane) {
	for {
		log.Debug("Iterating through %d plugins", len(AgentConfig.Plugins))

		for _, plugin := range AgentConfig.Plugins {
			for _, instance := range plugin.Instances {
				log.Debug("Running command %s %s", plugin.Cmd, strings.Join(instance.ArgsList, " "))
				go runPlugin(ep, instance, plugin)
			}
		}

		time.Sleep(AgentConfig.Sleep)
	}
}

func runPlugin(ep *errplane.Errplane, instance *Instance, plugin *Plugin) {
	cmdParts := strings.Fields(plugin.Cmd)
	if len(instance.ArgsList) > 0 {
		cmdParts = append(cmdParts, instance.ArgsList...)
	}
	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Error("Cannot run plugin %s. Error: %s", plugin.Cmd, err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Error("Cannot run plugin %s. Error: %s", plugin.Cmd, err)
		return
	}

	ch := make(chan error)
	go killPlugin(plugin, cmd, ch)

	output, err := ioutil.ReadAll(stdout)
	if err != nil {
		log.Error("Error while reading output from plugin %s. Error: %s", plugin.Cmd, err)
		ch <- err
		return
	}

	lines := strings.Split(string(output), "\n")

	err = cmd.Wait()
	ch <- err

	if len(lines) > 0 {
		log.Debug("output of plugin %s is %s", plugin.Cmd, lines[0])
		firstLine := lines[0]
		output, err := parsePluginOutput(plugin, &ProcessStateWrapper{cmd.ProcessState}, firstLine)
		if err != nil {
			log.Error("Cannot parse plugin %s output. Output: %s. Error: %s", plugin.Cmd, firstLine, err)
			return
		}

		log.Debug("parsed output is %#v", output)

		// status are printed to plugins.<plugin-nam>.status with a value of 1 and dimension status that is either ok, warning, critical or unknown
		// other metrics are written to plugins.<plugin-name>.<metric-name> with the given value
		// all metrics have the host name as a dimension

		report(ep, fmt.Sprintf("plugins.%s.%s.status", plugin.Name, instance.Name), 1.0, time.Now(), errplane.Dimensions{
			"host":       AgentConfig.Hostname,
			"status":     output.state.String(),
			"status_msg": output.msg,
		}, nil)

		if output.points != nil {
			// add the plugins.<plugin-name>.<instance-name> to the metric names
			for _, write := range output.points {
				write.Name = fmt.Sprintf("plugins.%s.%s.%s", plugin.Name, instance.Name, write.Name)
			}
			ep.SendHttp(&errplane.WriteOperation{Writes: output.points})
		}

		if output.metrics != nil {
			for name, value := range output.metrics {
				report(ep, fmt.Sprintf("plugins.%s.%s.%s", plugin.Name, instance.Name, name), value, time.Now(), errplane.Dimensions{"host": AgentConfig.Hostname}, nil)
			}
		}
	}
}

func parsePluginOutput(plugin *Plugin, cmdState ProcessState, firstLine string) (*PluginOutput, error) {
	outputType := plugin.Metadata.Output
	switch outputType {
	case "nagios":
		return parseNagiosOutput(cmdState, firstLine)
	case "errplane":
		return parseErrplaneOutput(cmdState, firstLine)
	default:
		return nil, fmt.Errorf("Unknown plugin output type '%s', supported types are 'errplane' and 'nagios'", outputType)
	}
}

func parseErrplaneOutput(cmdState ProcessState, firstLine string) (*PluginOutput, error) {
	exitStatus := cmdState.ExitStatus()
	firstLine = strings.TrimSpace(firstLine)
	statusAndMetrics := strings.Split(firstLine, "|")
	status := strings.TrimSpace(statusAndMetrics[0])
	writes := make([]*errplane.JsonPoints, 0)
	metric := strings.TrimSpace(statusAndMetrics[1])

	err := json.Unmarshal([]byte(metric), &writes)
	if err != nil {
		return nil, err
	}

	return &PluginOutput{PluginStateOutput(exitStatus), status, writes, nil}, nil
}

func parseNagiosOutput(cmdState ProcessState, firstLine string) (*PluginOutput, error) {
	firstLine = strings.TrimSpace(firstLine)

	statusAndMetrics := strings.Split(firstLine, "|")
	switch len(statusAndMetrics) {
	case 1, 2: // that's fine, anything else is an error
	default:
		return nil, fmt.Errorf("First line format doesn't match what the agent expects. See the docs for more details")
	}

	exitStatus := cmdState.ExitStatus()
	status := strings.TrimSpace(statusAndMetrics[0])

	if len(statusAndMetrics) == 1 {
		return &PluginOutput{PluginStateOutput(exitStatus), status, nil, nil}, nil
	}

	metricsLine := strings.TrimSpace(statusAndMetrics[1])

	type ParserState int
	const (
		IN_QUOTED_FIELD = iota
		IN_VALUE
		START
	)

	metricName := ""
	value := ""
	token := bytes.NewBufferString("")
	state := START
	metrics := make(map[string]string)

	for i := 0; i < len(metricsLine); i++ {
		switch metricsLine[i] {
		case '\'':
			switch state {
			case IN_QUOTED_FIELD:
				// if we're in a quoted field and we got double single quotes, treat them as a single quote
				// otherwise a '=' should follow and we'll change state to IN_VALUE
				if i+1 < len(metricsLine) && metricsLine[i+1] == '\'' {
					token.WriteByte('\'')
					i++
				}
			case IN_VALUE:
				// We're probably starting a new metric name
				state = IN_QUOTED_FIELD
				value = value + token.String()
				token = bytes.NewBufferString("")
				metrics[metricName] = value
				metricName, value = "", ""
			case START:
				// quote at the beginning of the metrics
				state = IN_QUOTED_FIELD
			}
		case '=':
			switch state {
			case IN_VALUE:
				// we're parsing a value, and suddently started parsing a new metric, e.g. `name=baz foo=bar`
				//																																						e're here ^ but we're parsing the value of the `name`
				metrics[metricName] = value
				fallthrough
			case START:
				metricName = token.String()
				token = bytes.NewBufferString("")
				value = ""
				state = IN_VALUE
			case IN_QUOTED_FIELD:
				// we finished parsing the metric name and started parsing the value
				state = IN_VALUE
				metricName = token.String()
				token = bytes.NewBufferString("")
			}
		case ' ':
			switch state {
			case IN_VALUE:
				value = value + " " + token.String()
			case IN_QUOTED_FIELD:
				metricName = metricName + " " + token.String()
			}
			token = bytes.NewBufferString("")
		default:
			token.WriteByte(metricsLine[i])
		}
	}

	metrics[metricName] = value + token.String()

	metricsMap := make(map[string]float64)

	for key, value := range metrics {
		value = strings.Split(strings.TrimSpace(value), ";")[0]

		uom := value[len(value)-1]
		switch uom {
		case 's':
			switch value[len(value)-2] {
			case 'u', 'm':
				value = value[0 : len(value)-2]
			default:
				value = value[0 : len(value)-1]
			}
		case 'B':
			switch value[len(value)-2] {
			case 'K', 'M', 'G':
				value = value[0 : len(value)-2]
			default:
				value = value[0 : len(value)-1]
			}
		case '%', 'c':
			value = value[0 : len(value)-1]
		}

		var err error
		metricsMap[key], err = strconv.ParseFloat(value, 64)
		if err != nil {
			delete(metricsMap, key)
			log.Info("Cannot parse the value of metric %s into a float. Error: %s", key, err)
		}
	}

	return &PluginOutput{PluginStateOutput(exitStatus), status, nil, metricsMap}, nil
}

func killPlugin(plugin *Plugin, cmd *exec.Cmd, ch chan error) {
	select {
	case err := <-ch:
		if exitErr, ok := err.(*exec.ExitError); ok && !exitErr.Exited() {
			log.Error("plugin %s didn't die gracefully. Killing it.", plugin.Cmd)
			cmd.Process.Kill()
		}
	case <-time.After(AgentConfig.Sleep):
		err := cmd.Process.Kill()
		if err != nil {
			log.Error("Cannot kill plugin %s. Error: %s", plugin.Cmd, err)
		}
		log.Error("Plugin %s killed because it took more than %s to execute", plugin.Cmd, AgentConfig.Sleep)
	}
}
