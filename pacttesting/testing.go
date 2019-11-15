package pacttesting

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bytes"
	c "os/exec"
	"sync"

	"testing"

	retry "github.com/giantswarm/retry-go"
	"github.com/pact-foundation/pact-go/dsl"
	"github.com/pact-foundation/pact-go/types"
	"github.com/phayes/freeport"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Pact = string

var once sync.Once
var pactClient *dsl.PactClient

type pact struct {
	Consumer     pactName      `json:"consumer"`
	Provider     pactName      `json:"provider"`
	Interactions []interface{} `json:"interactions"`
}

type pactName struct {
	Name string `json:"name"`
}

type pactServer struct {
	server     *types.MockServer
	mockServer *MockServer
}

var serverMap = make(map[string]*pactServer)
var serverPortMap = make(map[string]int)

func readPactFile(pactFilePath string) *pact {
	dir, _ := os.Getwd()

	var file string
	if strings.HasSuffix(pactFilePath, ".json") {
		file = pactFilePath
	} else {
		file = fmt.Sprintf("%s.json", pactFilePath)
	}
	path := filepath.FromSlash(fmt.Sprintf(filepath.Join(dir, "pacts", file)))

	pactString, err := ioutil.ReadFile(path)

	if err != nil {
		panic(err)
	}

	p := &pact{}
	err = json.Unmarshal([]byte(pactString), p)

	if err != nil {
		panic(err)
	}

	return p
}

func readAllPacts(pacts []string) []*pact {
	var results []*pact

	for _, p := range pacts {
		results = append(results, readPactFile(p))
	}

	return results
}

func groupByProvider(pacts []*pact) []*pact {
	var results []*pact

	pactMap := make(map[string]*pact)

	for _, p := range pacts {
		pt, ok := pactMap[fmt.Sprintf("%s%s", p.Provider.Name, p.Consumer.Name)]
		if ok {
			pt.Interactions = append(pt.Interactions, p.Interactions...)
		} else {
			newPact := &pact{
				Consumer:     p.Consumer,
				Provider:     p.Provider,
				Interactions: p.Interactions,
			}
			pactMap[fmt.Sprintf("%s%s", p.Provider.Name, p.Consumer.Name)] = newPact
		}
	}

	for _, v := range pactMap {
		results = append(results, v)
	}

	return results
}

func getTopLevelDir() (string, error) {
	gitCommand := c.Command("git", "rev-parse", "--show-toplevel")
	var out bytes.Buffer
	gitCommand.Stdout = &out
	err := gitCommand.Run()

	if err != nil {
		return "", err
	}

	topLevelDir := strings.TrimRight(out.String(), "\n")
	return topLevelDir, nil
}

func getVersion() (string, error) {
	gitCommand := c.Command("git", "describe")
	var out bytes.Buffer
	gitCommand.Stdout = &out
	err := gitCommand.Run()

	if err != nil {
		return "", err
	}

	version := strings.TrimRight(out.String(), "\n")
	return version, nil
}

func newPactClient() (*dsl.PactClient, error) {
	topLevelDir, err := getTopLevelDir()
	if err != nil {
		return nil, err
	}
	pactPath := filepath.Join(topLevelDir, "pact/bin")

	os.Setenv("PATH", pactPath+":"+os.Getenv("PATH"))
	return dsl.NewClient(), nil
}

func buildPactClientOnce() {
	once.Do(func() {
		client, err := newPactClient()
		if err != nil {
			panic(err)
		}
		pactClient = client
	})
}

// PreassignPorts sets a random port for all future mocked instances and configures viper to point to them.
// This is necessary to get viper configuration before actually loading pact files.
// This function can be called multiple times for the same files, it will only initialise them once.
func PreassignPorts(pactFilePaths []Pact) {
	pacts := groupByProvider(readAllPacts(pactFilePaths))
	for _, p := range pacts {
		key := p.Provider.Name + p.Consumer.Name
		_, ok := serverPortMap[key]
		if !ok {
			port, err := freeport.GetFreePort()

			if err != nil {
				panic(err)
			}
			serverPortMap[key] = port
			serverAddress := fmt.Sprintf("http://localhost:%d", port)
			viper.Set(p.Provider.Name, serverAddress)
		}
	}
}

// Runs testFunc with stub services defined by given pacts. Does not verify that the stubs are called
func TestWithStubServices(pactFilePaths []Pact, testFunc func()) {
	PreassignPorts(pactFilePaths)
	buildPactClientOnce()

	pacts := groupByProvider(readAllPacts(pactFilePaths))

	dir, _ := os.Getwd()

	for _, p := range pacts {
		key := p.Provider.Name + p.Consumer.Name
		_, ok := serverMap[key]
		if !ok {
			args := []string{
				"--pact-specification-version",
				fmt.Sprintf("%d", 3),
				"--pact-dir",
				filepath.FromSlash(fmt.Sprintf(filepath.Join(dir, "target"))),
				"--log",
				filepath.FromSlash(fmt.Sprintf(filepath.Join(dir, "logs")) + "/" + "pact.log"),
				"--consumer",
				p.Consumer.Name,
				"--provider",
				p.Provider.Name,
				"--pact-file-write-mode",
				"merge",
			}

			log.Infof("starting new mock server for consumer: %s, provider: %s", p.Consumer.Name, p.Provider.Name)

			// This exists because we've called PreassignPorts
			port := serverPortMap[key]
			server := pactClient.StartServer(args, port)
			serverAddress := fmt.Sprintf("http://localhost:%d", port)
			mockServer := &MockServer{
				BaseURL:  serverAddress,
				Consumer: p.Consumer.Name,
				Provider: p.Provider.Name,
			}

			serverMap[key] = &pactServer{mockServer: mockServer, server: server}
		}

		for _, i := range p.Interactions {
			err := serverMap[p.Provider.Name+p.Consumer.Name].mockServer.AddInteraction(i)
			if err != nil {
				log.Fatalf("Error adding pact: %v", err)
				panic(err)
			}
		}
	}

	testFunc()

	for _, pactServer := range serverMap {
		pactServer.mockServer.DeleteInteractions()
	}
}

// Runs mock services defined by the given pacts, invokes testFunc then verifies that the pacts have been invoked successfully
func IntegrationTest(pactFilePaths []Pact, testFunc func()) {
	TestWithStubServices(pactFilePaths, func() {
		testFunc()

		verify := func() error {
			for _, pactServer := range serverMap {
				err := pactServer.mockServer.Verify()

				if err != nil {
					return fmt.Errorf("pact verification failed: %v", err)
				}
				log.Infof("pacts verified successfully")
			}
			return nil
		}

		err := retry.Do(verify, retry.MaxTries(150000), retry.Sleep(200*time.Millisecond), retry.Timeout(3*time.Minute))

		if err != nil {
			log.Fatalf("Pact verification failed!! For more info on the error check the logs/pact.log file it is quite detailed")
		}
	})
}

func StopMockServers() {

	for _, pactServer := range serverMap {
		pactClient.StopServer(pactServer.server)
	}
}

type PactProviderTestParams struct {
	Pacts                 string
	AuthToken             string
	BaseURL               string
	ProviderStateSetupURL string
	Testing               *testing.T
}

func VerifyProviderPacts(params PactProviderTestParams) {
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

	pactsFilter := ""
	if filepath.IsAbs(params.Pacts) {
		pactsFilter = params.Pacts
	} else {
		pactsFilter = filepath.Join(topLevelDir, params.Pacts)
	}
	urls, err := filepath.Glob(pactsFilter)

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
			request := types.VerifyRequest{
				ProviderBaseURL:        params.BaseURL,
				PactURLs:               []string{url},
				CustomProviderHeaders:  []string{"Authorization: Bearer " + params.AuthToken},
				ProviderVersion:        version,
				ProviderStatesSetupURL: params.ProviderStateSetupURL,
			}

			response, verifyErr := pactClient.VerifyProvider(request)
			allTestsSucceeded := true
			for _, example := range response.Examples {
				if !t.Run(example.Description, func(st *testing.T) {
					if example.Status != "passed" {
						st.Errorf("%s\n%s\n", example.FullDescription, example.Exception.Message)
					} else {
						st.Log(example.FullDescription)
					}
				}) {
					allTestsSucceeded = false
				}

			}

			t.Run("==> Writing verification.json", func(t *testing.T) {
				verificationJson := fmt.Sprintf("{\"success\": %v,\"providerApplicationVersion\": \"%s\"}",
					allTestsSucceeded,
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

			if verifyErr != nil {
				t.Fatal(verifyErr)
			}
		})
	}
}
