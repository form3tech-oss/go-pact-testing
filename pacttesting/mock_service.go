package pacttesting

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

type MockServer struct {
	BaseURL  string
	Consumer string
	Provider string
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
