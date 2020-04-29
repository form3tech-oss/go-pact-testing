package pacttesting

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/dsl"
	"github.com/pact-foundation/pact-go/types"
	"github.com/pact-foundation/pact-go/utils"
)

func VerifyProviderMessagingPacts(params PactProviderTestParams, messageProducers dsl.MessageHandlers) {
	buildPactClientOnce()

	version, err := getVersion()
	if err != nil {
		params.Testing.Error(err)
		return
	}

	topLevelDir, err := getTopLevelDir()
	if err != nil {
		params.Testing.Error(err)
		return
	}

	urls, err := filepath.Glob(filepath.Join(topLevelDir, params.Pacts))
	if err != nil {
		params.Testing.Fatal(err)
	}

	if len(urls) == 0 {
		params.Testing.Error("No pacts found")
	}

	for _, url := range urls {
		urlparts := strings.SplitAfter(url, "/")
		filename := urlparts[len(urlparts)-1]
		params.Testing.Run(filename, func(t *testing.T) {
			allTestsSucceeded := true

			// perform the verification
			response, err := VerifyMessageProviderRaw(params, dsl.VerifyMessageRequest{
				PactURLs:        []string{url},
				MessageHandlers: messageProducers,
			})

			// report the results using the test framework
			for _, test := range response {
				for _, notice := range test.Summary.Notices {
					if notice.When == "before_verification" {
						t.Logf("notice: %s", notice.Text)
					}
				}
				for _, example := range test.Examples {
					testSuccessful := t.Run(example.Description, func(st *testing.T) {
						st.Log(example.FullDescription)
						if example.Status != "passed" {
							st.Errorf("%s\n", example.Exception.Message)
							st.Error("Check to ensure that all message expectations have corresponding message handlers")
						} else {
							st.Log(example.FullDescription)
						}
					})
					allTestsSucceeded = allTestsSucceeded && testSuccessful
				}
				for _, notice := range test.Summary.Notices {
					if notice.When == "after_verification" {
						t.Logf("notice: %s", notice.Text)
					}
				}
			}

			if err != nil {
				t.Errorf("Error verifying message provider: %s", err)
			}

			t.Run("==> Writing verification.json", func(t *testing.T) {
				verificationJson := fmt.Sprintf("{\"success\": %v,\"providerApplicationVersion\": \"%s\"}",
					err == nil && allTestsSucceeded,
					version)
				verificationDir := filepath.Join(topLevelDir, "build", "pact-verifications")
				_ = os.MkdirAll(verificationDir+"/", 0744)
				verificationFile := filepath.Join(verificationDir, filename)
				if err := ioutil.WriteFile(verificationFile, []byte(verificationJson), 0644); err != nil {
					t.Fatal(err)
				}
				outputJson, err := json.Marshal(response)

				if err != nil {
					t.Fatal(err)
				}

				outFile := filepath.Join(topLevelDir, "build/pact-verifications/", "output-"+filename)
				if err := ioutil.WriteFile(outFile, outputJson, 0644); err != nil {
					t.Fatal(err)
				}
			})

		})
	}
}

var messageHandler = func(messageHandlers dsl.MessageHandlers, stateHandlers dsl.StateHandlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		// Extract message
		var message dsl.Message
		body, err := ioutil.ReadAll(r.Body)
		r.Body.Close()

		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		json.Unmarshal(body, &message)

		// Setup any provider state
		for _, state := range message.States {
			sf, stateFound := stateHandlers[state.Name]

			if !stateFound {
				log.Printf("[WARN] state handler not found for state: %v", state.Name)
			} else {
				// Execute state handler
				if err = sf(state); err != nil {
					log.Printf("[WARN] state handler for '%v' return error: %v", state.Name, err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}
		}

		// Lookup key in function mapping
		f, messageFound := messageHandlers[message.Description]

		if !messageFound {
			log.Printf("[ERROR] message handler not found for message description: %v", message.Description)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Execute function handler
		res, handlerErr := f(message)

		if handlerErr != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		wrappedResponse := map[string]interface{}{
			"contents": res,
		}

		// Write the body back
		resBody, errM := json.Marshal(wrappedResponse)
		if errM != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Println("[ERROR] error marshalling objcet:", errM)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(resBody)
	}
}

// VerifyMessageProviderRaw runs provider message verification.
func VerifyMessageProviderRaw(params PactProviderTestParams, request dsl.VerifyMessageRequest) ([]types.ProviderVerifierResponse, error) {
	response := []types.ProviderVerifierResponse{}

	// Starts the message wrapper API with hooks back to the message handlers
	// This maps the 'description' field of a message pact, to a function handler
	// that will implement the message producer. This function must return an object and optionally
	// and error. The object will be marshalled to JSON for comparison.
	mux := http.NewServeMux()

	port, err := utils.GetFreePort()
	if err != nil {
		return response, fmt.Errorf("unable to allocate a port for verification: %v", err)
	}

	// Construct verifier request
	verificationRequest := types.VerifyRequest{
		ProviderBaseURL:            fmt.Sprintf("http://localhost:%d", port),
		PactURLs:                   request.PactURLs,
		BrokerURL:                  request.BrokerURL,
		Tags:                       request.Tags,
		BrokerUsername:             request.BrokerUsername,
		BrokerPassword:             request.BrokerPassword,
		PublishVerificationResults: request.PublishVerificationResults,
		ProviderVersion:            request.ProviderVersion,
		ProviderStatesSetupURL:     params.ProviderStateSetupURL,
		CustomProviderHeaders:      []string{"Authorization: Bearer " + params.AuthToken},
	}

	mux.HandleFunc("/", messageHandler(request.MessageHandlers, request.StateHandlers))

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	log.Printf("[DEBUG] API handler starting: port %d (%s)", port, ln.Addr())
	go http.Serve(ln, mux)

	portErr := waitForPort(port, "tcp", "localhost", 5*time.Second,
		fmt.Sprintf(`Timed out waiting for Daemon on port %d - are you sure it's running?`, port))

	if portErr != nil {
		log.Fatal("Error:", err)
		return response, portErr
	}

	log.Println("[DEBUG] pact provider verification")
	return pactClient.VerifyProvider(verificationRequest)
}

var waitForPort = func(port int, network string, address string, timeoutDuration time.Duration, message string) error {
	log.Println("[DEBUG] waiting for port", port, "to become available")
	timeout := time.After(timeoutDuration)

	for {
		select {
		case <-timeout:
			log.Printf("[ERROR] Expected server to start < %s. %s", timeoutDuration, message)
			return fmt.Errorf("Expected server to start < %s. %s", timeoutDuration, message)
		case <-time.After(50 * time.Millisecond):
			_, err := net.Dial(network, fmt.Sprintf("%s:%d", address, port))
			if err == nil {
				return nil
			}
		}
	}
}
