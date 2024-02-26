package pacttesting

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/dsl"
	"github.com/pact-foundation/pact-go/types"
	"github.com/pact-foundation/pact-go/utils"
)

const providerHTTPScheme = "http://"

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
			responses, err := VerifyMessageProviderRaw(params, dsl.VerifyMessageRequest{
				PactURLs:        []string{url},
				MessageHandlers: messageProducers,
			})

			// report the results using the test framework
			for _, response := range responses {
				for _, example := range response.Examples {
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

				if err != nil {
					t.Errorf("Error verifying message provider: %s", err)
				}

				t.Run("==> Writing verification.json", func(t *testing.T) {
					verificationJSON := fmt.Sprintf("{\"success\": %v,\"providerApplicationVersion\": \"%s\"}",
						err == nil && allTestsSucceeded,
						version)
					verificationDir := filepath.Join(topLevelDir, "build", "pact-verifications")
					_ = os.MkdirAll(verificationDir+"/", 0o744)
					verificationFile := filepath.Join(verificationDir, filename)
					if err := os.WriteFile(verificationFile, []byte(verificationJSON), 0o600); err != nil {
						t.Fatal(err)
					}
					outputJSON, err := json.Marshal(response)
					if err != nil {
						t.Fatal(err)
					}

					outFile := filepath.Join(topLevelDir, "build/pact-verifications/", "output-"+filename)
					if err := os.WriteFile(outFile, outputJSON, 0o600); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func messageHandler(messageHandlers dsl.MessageHandlers, stateHandlers dsl.StateHandlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		// Extract message
		var message dsl.Message
		body, err := io.ReadAll(r.Body)
		r.Body.Close()

		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = json.Unmarshal(body, &message)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

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
			log.Println("[ERROR] error marshalling objcet:", errM)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(resBody)
	}
}

// VerifyMessageProviderRaw runs provider message verification.
func VerifyMessageProviderRaw(
	params PactProviderTestParams,
	request dsl.VerifyMessageRequest,
) ([]types.ProviderVerifierResponse, error) {
	emptyResponse := []types.ProviderVerifierResponse{}

	// Starts the message wrapper API with hooks back to the message handlers
	// This maps the 'description' field of a message pact, to a function handler
	// that will implement the message producer. This function must return an object and optionally
	// and error. The object will be marshalled to JSON for comparison.
	mux := http.NewServeMux()

	port, err := utils.GetFreePort()
	if err != nil {
		return emptyResponse, fmt.Errorf("unable to allocate a port for verification: %w", err)
	}

	// Construct verifier request
	verificationRequest := types.VerifyRequest{
		ProviderBaseURL:            providerHTTPScheme + net.JoinHostPort(getBindAddress(), strconv.Itoa(port)),
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

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", getBindAddress(), port))
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	server := http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}

	log.Printf("[DEBUG] API handler starting: port %d (%s)", port, ln.Addr())
	go func() { _ = server.Serve(ln) }()

	portErr := waitForPort(port, "tcp", getBindAddress(), 5*time.Second,
		fmt.Sprintf(`Timed out waiting for Daemon on port %d - are you sure it's running?`, port))

	if portErr != nil {
		log.Print("Error:", err)
		return emptyResponse, portErr
	}

	log.Println("[DEBUG] pact provider verification")
	response, err := pactClient.VerifyProvider(verificationRequest)
	if err != nil {
		return emptyResponse, fmt.Errorf("pact provider verification: %w", err)
	}
	return response, nil
}

func waitForPort(port int, network string, address string, timeoutDuration time.Duration, message string) error {
	log.Println("[DEBUG] waiting for port", port, "to become available")
	timeout := time.After(timeoutDuration)

	for {
		select {
		case <-timeout:
			log.Printf("[ERROR] Expected server to start < %s. %s", timeoutDuration, message)
			return fmt.Errorf("expected server to start < %s. %s", timeoutDuration, message)
		case <-time.After(50 * time.Millisecond):
			_, err := net.Dial(network, fmt.Sprintf("%s:%d", address, port))
			if err == nil {
				return nil
			}
		}
	}
}
