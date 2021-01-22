package pacttesting

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type MockServer struct {
	Port     int
	BaseURL  string
	Consumer string
	Provider string
	Pid      int
	Running  bool `json:"-"`
}

// call sends a message to the Pact service
func (m *MockServer) call(method string, url string, content *string) error {

	client := &http.Client{}
	var req *http.Request
	var err error

	if method == "POST" && content != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader([]byte(*content)))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return err
	}

	req.Header.Set("X-Pact-Mock-Service", "true")
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	responseBody, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return errors.New(string(responseBody))
	}
	return err
}

func (m *MockServer) DeleteInteractions() error {
	url := fmt.Sprintf("%s/interactions", m.BaseURL)
	return m.call("DELETE", url, nil)
}

func (m *MockServer) AddInteraction(interaction interface{}) error {
	url := fmt.Sprintf("%s/interactions", m.BaseURL)

	interactionBytes, _ := json.Marshal(interaction)
	interactionJson := string(interactionBytes)

	return m.call("POST", url, &interactionJson)
}

func (m *MockServer) Verify() error {
	url := fmt.Sprintf("%s/interactions/verification", m.BaseURL)
	return m.call("GET", url, nil)
}

func (m *MockServer) writePidFile() {
	bytes, err := json.Marshal(m)
	if err != nil {
		log.WithError(err).Errorf("unable to convert mock server to json")
		return
	}
	dir, _ := os.Getwd()
	_ = os.MkdirAll(filepath.FromSlash(filepath.Join(dir, "pact", "pids")), os.ModePerm)
	file := filepath.FromSlash(fmt.Sprintf("%s/pact-%s-%s.json", filepath.Join(dir, "pact", "pids"), m.Provider, m.Consumer))
	err = ioutil.WriteFile(file, bytes, os.ModePerm)
	if err != nil {
		log.WithError(err).Errorf("unable to store mock server details")
		return
	}
}

func loadRunningServer(provider, consumer string) *MockServer {
	dir, _ := os.Getwd()
	file := filepath.FromSlash(fmt.Sprintf("%s/pact-%s-%s.json", filepath.Join(dir, "pact", "pids"), provider, consumer))

	bytes, err := ioutil.ReadFile(file)
	if os.IsNotExist(err) {
		return nil
	}
	var server MockServer
	err = json.Unmarshal(bytes, &server)
	if err != nil {
		log.WithError(err).Errorf("unable to read pid file %s", file)
		return nil
	}

	err = server.call("GET", server.BaseURL, nil)
	if err != nil {
		log.WithError(err).Errorf("%s pact server defined in %s with pid %d no longer responding. Will start a new one.", server.Provider, file, server.Pid)
		err = os.Remove(file)
		if err != nil {
			log.WithError(err).Warnf("unable to remove %s", file)
		}
		return nil
	}

	server.Running = true
	pactServers[provider+consumer] = &server
	viper.Set(provider, server.BaseURL)
	log.Infof("Reusing existing mock service for %s at %s, pid %d", server.Provider, server.BaseURL, server.Pid)
	return &server
}
