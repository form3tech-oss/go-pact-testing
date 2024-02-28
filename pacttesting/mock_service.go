package pacttesting

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/avast/retry-go/v4"
	log "github.com/sirupsen/logrus"
)

type MockServer struct {
	Port     int    `json:"port"`
	BaseURL  string `json:"base_url"`
	Consumer string `json:"consumer"`
	Provider string `json:"provider"`
	Pid      int    `json:"pid"`
	Running  bool   `json:"-"`
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
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("X-Pact-Mock-Service", "true")
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("proxying request: %w", err)
	}

	responseBody, err := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return errors.New(string(responseBody))
	}

	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	return nil
}

func (m *MockServer) DeleteInteractions() error {
	url := m.BaseURL + "/interactions"
	return m.call("DELETE", url, nil)
}

func (m *MockServer) AddInteraction(interaction interface{}) error {
	url := m.BaseURL + "/interactions"

	interactionBytes, err := json.Marshal(interaction)
	if err != nil {
		return fmt.Errorf("marshaling interaction: %w", err)
	}
	interactionJSON := string(interactionBytes)

	return m.call("POST", url, &interactionJSON)
}

func (m *MockServer) Verify() error {
	url := m.BaseURL + "/interactions/verification"
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
	file := filepath.FromSlash(
		fmt.Sprintf("%s/pact-%s-%s.json", filepath.Join(dir, "pact", "pids"), m.Provider, m.Consumer),
	)
	err = os.WriteFile(file, bytes, os.ModePerm)
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
			return fmt.Errorf("failed to send interrupt to pid '%d': %w", m.Pid, err)
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
		}, retry.Attempts(25), retry.Delay(200*time.Millisecond), retry.DelayType(retry.FixedDelay)); err != nil {
			err = p.Kill()
			if err != nil {
				return fmt.Errorf("failed to kill process: %w", err)
			}
			return errors.New("failed to stop server, attempting kill")
		}

		log.Printf("stopped server")
	} else {
		log.WithError(err).Warnf("cannot find process with pid %d", m.Pid)
	}

	dir, _ := os.Getwd()
	file := filepath.FromSlash(
		fmt.Sprintf("%s/pact-%s-%s.json", filepath.Join(dir, "pact", "pids"), m.Provider, m.Consumer),
	)
	err = os.Remove(file)
	if err != nil {
		log.WithError(err).Warnf("unable to remove pid file%s", file)
	}
	return nil
}

func loadRunningServer(provider, consumer string) *MockServer {
	dir, _ := os.Getwd()
	file := filepath.FromSlash(fmt.Sprintf("%s/pact-%s-%s.json", filepath.Join(dir, "pact", "pids"), provider, consumer))

	bytes, err := os.ReadFile(file)
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
		log.
			WithError(err).
			Errorf("%s pact server defined in %s with pid %d no longer responding. Will start a new one.",
				server.Provider, file, server.Pid,
			)
		err = os.Remove(file)
		if err != nil {
			log.WithError(err).Warnf("unable to remove %s", file)
		}
		return nil
	}

	server.Running = true
	pactServers[provider+consumer] = &server
	exposeServerURL(provider, server.BaseURL)
	log.Infof("Reusing existing mock service for %s at %s, pid %d", server.Provider, server.BaseURL, server.Pid)
	return &server
}
