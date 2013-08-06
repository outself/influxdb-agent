package utils

import (
	log "code.google.com/p/log4go"
	"fmt"
	"io/ioutil"
	"net/http"
)

func GetBody(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		log.Error("Cannot download from '%s'. Error: %s", url, err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Received status code %d", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}
