package main

import (
	log "code.google.com/p/log4go"
	"github.com/howeyc/fsnotify"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"time"
	. "utils"
)

type LogFile struct {
	Path             string
	size             int64
	lastHundredLines []string
}

var logFiles map[string]*LogFile

func getSize(filename string) (int64, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func watchLogFile(detector Detector) {
	logFiles = make(map[string]*LogFile)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error("Error while initializing watcher: %s", err)
	}

	// Process events
	go func() {
		for {
			select {
			case ev := <-watcher.Event:
				path := ev.Name
				// log.Debug("event: %v", ev)
				if ev.IsModify() {
					log.Debug("File %s was modified", path)
				}
				statSize, err := getSize(path)
				if err != nil {
					log.Error("Cannot get stat for %s. Error: %s", path, err)
					continue
				}
				logFile := logFiles[path]
				lastSize := logFile.size
				if statSize < lastSize {
					log.Warn("File %s was truncated", path)
					lastSize = 0
				}
				if lastSize == statSize {
					continue
				}
				file, err := os.Open(path)
				defer file.Close()
				_, err = file.Seek(lastSize, 0)
				if err != nil {
					log.Error("Cannot seek in the file. Error: %s", err)
					continue
				}
				raw, err := ioutil.ReadAll(file)
				if err != nil {
					log.Error("Cannot read from the file. Error: %s", err)
					continue
				}
				stat, err := file.Stat()
				if err != nil {
					log.Error("Cannot get stat for %s. Error: %s", path, err)
					continue
				}
				logFile.size = stat.Size()
				lines := string(raw)
				oldLines := logFile.lastHundredLines
				if len(oldLines) > 0 {
					lines = oldLines[len(oldLines)-1] + lines
					oldLines = oldLines[:len(oldLines)-1]
				}
				newLines := strings.Split(lines, "\n")
				detector.ReportLogEvent(path, oldLines, newLines)
				oldLines = append(oldLines, newLines...)
				firstLine := int(math.Max(float64(len(oldLines)-100), 0.0))
				logFile.lastHundredLines = oldLines[firstLine:]
				// log.Debug("new content for %s: %s", path, strings.Join(oldLines, "\n"))
			case err := <-watcher.Error:
				log.Error("error: %s", err)
			}
		}
	}()

	// add new watchers if the configuration was changed
	for {
		paths := detector.filesToMonitor()
		newPaths := make(map[string]bool)
		for _, path := range paths {
			newPaths[path] = true
		}

		// add new watchers
		for path, _ := range newPaths {
			if _, ok := logFiles[path]; ok {
				continue
			}
			log.Info("Adding log watcher for %s", path)
			err = watcher.Watch(path)
			if err != nil {
				log.Error("Cannot open %s. Error: %s", path, err)
				continue
			}
			statSize, err := getSize(path)
			if err != nil {
				log.Error("Cannot open %s. Error: %s", path, err)
				continue
			}
			logFiles[path] = &LogFile{path, statSize, nil}
		}

		// remove unused watchers
		for path, _ := range logFiles {
			if _, ok := newPaths[path]; ok {
				continue
			}
			delete(logFiles, path)
			log.Info("Removing log watcher for %s", path)
			watcher.RemoveWatch(path)
		}

		time.Sleep(AgentConfig.Sleep)
	}

	done := make(chan bool)
	<-done

	watcher.Close()
}
