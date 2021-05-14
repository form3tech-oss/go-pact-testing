package pacttesting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/avast/retry-go"
	"github.com/pkg/errors"
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

// Stop gracefully shuts does the underlying pact-mock-service process
// and removes the server metadata (pid) file.
func (m *MockServer) Stop() error {
	p, err := os.FindProcess(m.Pid)
	if err == nil {
		err = p.Signal(syscall.SIGTERM)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("failed to send interrupt to pid %d", m.Pid))
		}

		// wait for process to exit after interrupt, if it fails to stop
		// then kill it
		if err := retry.Do(func() error {
			// check if the process is still alive
			err := p.Signal(syscall.Signal(0))
			if err == nil {
				return errors.New("server process is still alive")
			}
			return nil
		}, retry.Attempts(25), retry.Delay(200*time.Millisecond)); err != nil {
			err = p.Kill()
			if err != nil {
				return errors.WithMessage(err, "failed to kill process")
			}
			return errors.New("failed to stop server, attempting kill")
		} else {
			log.Printf("stopped server")
		}
	} else {
		log.WithError(err).Warnf("cannot find process with pid %d", m.Pid)
	}

	dir, _ := os.Getwd()
	file := filepath.FromSlash(fmt.Sprintf("%s/pact-%s-%s.json", filepath.Join(dir, "pact", "pids"), m.Provider, m.Consumer))
	err = os.Remove(file)
	if err != nil {
		log.WithError(err).Warnf("unable to remove pid file%s", file)
	}
	return nil
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
