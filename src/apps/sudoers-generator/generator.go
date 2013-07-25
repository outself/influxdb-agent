package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"unicode"
	. "utils"
)

const (
	SUDOERS_BEGIN_SECTION = `############### ERRPLANE BEGIN ####################`
	SUDOERS_END_SECTION   = `###############  ERRPLANE END  ####################`
)

func main() {
	config := flag.String("config", "/etc/errplane-agent/config.yml", "The path to the config file")
	output := flag.String("output", "/etc/sudoers.d/errplane", "The path to the output file")
	appendMode := flag.Bool("append", true, "Whether to generate a new file or append a errplane section to the sudoers file")
	diff := flag.Bool("show-diff", true, "Show diff and prompt before applying changes")

	flag.Parse()

	if err := InitConfig(*config); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot read configuration. Error: %s\n", err)
		os.Exit(1)
	}

	originalContent := ""

	if *appendMode {
		_originalContent, err := ioutil.ReadFile(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot read from %s. Error: %s\nMay be you need to run this tool with sudo!", *output, err)
			os.Exit(1)
		}
		originalContent = string(_originalContent)
	}

	originalContent = removeErrplaneSection(originalContent)

	errplaneSection := bytes.NewBufferString("\n\nerrplane ALL= \\\n")

	for procIdx, proc := range MonitoredProcesses {
		startCmdFields := strings.Fields(proc.StartCmd)
		startCmdPath, err := exec.LookPath(startCmdFields[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot find executable %s on path\n", startCmdFields[0])
			os.Exit(1)
		}
		stopCmdFields := strings.Fields(proc.StopCmd)
		stopCmdPath, err := exec.LookPath(stopCmdFields[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot find executable %s on path\n", startCmdFields[0])
			os.Exit(1)
		}

		startCmdFields[0] = startCmdPath
		stopCmdFields[0] = stopCmdPath

		startCmd := strings.Join(startCmdFields, " ")
		stopCmd := strings.Join(stopCmdFields, " ")

		fmt.Fprintf(errplaneSection, "\t(%s) NOPASSWD: %s, \\\n", proc.User, startCmd)
		fmt.Fprintf(errplaneSection, "\t(%s) NOPASSWD: %s", proc.User, stopCmd)

		if procIdx < len(MonitoredProcesses)-1 {
			fmt.Fprintf(errplaneSection, ", \\\n")
		}
	}

	errplaneSection.WriteString("\n")

	newContent := originalContent + SUDOERS_BEGIN_SECTION + errplaneSection.String() + SUDOERS_END_SECTION + "\n"

	if *diff {
		// write to a temporary file and show the diff

		file, err := ioutil.TempFile(os.TempDir(), "sudoers")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot create a temp file for diffing. Error: %s\n", err)
			os.Exit(1)
		}
		_, err = file.Write([]byte(newContent))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot write to temp file %s. Error: %s\n", file, err)
			os.Exit(1)
		}
		cmd := exec.Command("diff", "-u", *output, file.Name())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating diff. Error: %s\n", err)
		}

		fmt.Printf("Do you want to continue ? [y/N] ")
		var c rune
		fmt.Scanf("%c", &c)
		if 'y' != unicode.ToLower(c) {
			os.Exit(2)
		}
	}

	file, err := os.Create(*output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot open %s for writing. Error: %s\n", *output, err)
		os.Exit(1)
	}

	_, err = file.Write([]byte(newContent))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot write to %s. Error: %s\n", *output, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func removeErrplaneSection(content string) string {
	lines := strings.Split(content, "\n")
	newLines := make([]string, 0)

	insideErrplaneSection := false

	for _, line := range lines {
		if line == SUDOERS_BEGIN_SECTION {
			insideErrplaneSection = true
			continue
		}

		if line == SUDOERS_END_SECTION {
			insideErrplaneSection = false
			continue
		}

		if !insideErrplaneSection {
			newLines = append(newLines, line)
		}
	}

	return strings.Join(newLines, "\n")
}
