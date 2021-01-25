package pacttesting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	retry "github.com/giantswarm/retry-go"
	"github.com/pact-foundation/pact-go/dsl"
	"github.com/pact-foundation/pact-go/types"
	"github.com/phayes/freeport"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Pact = string

type pact struct {
	Consumer     pactName      `json:"consumer"`
	Provider     pactName      `json:"provider"`
	Interactions []interface{} `json:"interactions"`
}

type pactName struct {
	Name string `json:"name"`
}

var (
	pathOnce    sync.Once
	once        sync.Once
	pactClient  *dsl.PactClient
	pactServers = make(map[string]*MockServer)
)

var defaultRetryOptions = []retry.RetryOption{
	retry.MaxTries(150000),
	retry.Sleep(200 * time.Millisecond),
	retry.Timeout(3 * time.Minute),
}

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
	err = json.Unmarshal(pactString, p)

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
	gitCommand := exec.Command("git", "rev-parse", "--show-toplevel")
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
	gitCommand := exec.Command("git", "describe")
	var out bytes.Buffer
	gitCommand.Stdout = &out
	err := gitCommand.Run()

	if err != nil {
		return "", err
	}

	version := strings.TrimRight(out.String(), "\n")
	return version, nil
}

func setPathOnce() {
	pathOnce.Do(func() {
		topLevelDir, err := getTopLevelDir()
		if err != nil {
			panic(err)
		}
		pactPath := filepath.Join(topLevelDir, "pact/bin")

		os.Setenv("PATH", pactPath+":"+os.Getenv("PATH"))
	})
}

func buildPactClientOnce() {
	once.Do(func() {
		setPathOnce()
		pactClient = dsl.NewClient()
	})
}

// PreassignPorts sets a random port for all future mocked instances and configures viper to point to them.
// This is necessary to get viper configuration before actually loading pact files.
// This function can be called multiple times for the same files, it will only initialise them once.
func PreassignPorts(pactFilePaths []Pact) {
	pacts := groupByProvider(readAllPacts(pactFilePaths))
	for _, p := range pacts {
		mockServer := loadRunningServer(p.Provider.Name, p.Consumer.Name)
		if mockServer == nil {
			assignPort(p.Provider.Name, p.Consumer.Name)
		}
	}
}

func assignPort(provider, consumer string) int {
	key := provider + consumer
	_, ok := pactServers[key]
	if !ok {
		port, err := freeport.GetFreePort()

		if err != nil {
			panic(err)
		}
		pactServers[key] = &MockServer{
			Port:     port,
			BaseURL:  fmt.Sprintf("http://%s:%d", getBindAddress(), port),
			Consumer: consumer,
			Provider: provider,
		}
		viper.Set(provider, pactServers[key].BaseURL)
	}
	return pactServers[key].Port
}

func ResetPacts() {
	for key, pactServer := range pactServers {
		if !pactServer.Running {
			continue
		}
		err := pactServer.DeleteInteractions()
		if err != nil {
			log.WithError(err).Errorf("unable to delete configured interactions for %s", key)
		}
	}
}

// Runs testFunc with stub services defined by given pacts. Does not verify that the stubs are called
func TestWithStubServices(pactFilePaths []Pact, testFunc func()) {
	defer ResetPacts()

	PreassignPorts(pactFilePaths)

	pacts := groupByProvider(readAllPacts(pactFilePaths))

	for _, server := range pactServers {
		server.DeleteInteractions()
	}

	for _, p := range pacts {
		key := p.Provider.Name + p.Consumer.Name
		EnsurePactRunning(p.Provider.Name, p.Consumer.Name)

		for _, i := range p.Interactions {
			err := pactServers[key].AddInteraction(i)
			if err != nil {
				log.Fatalf("Error adding pact: %v", err)
				panic(err)
			}
		}
	}

	testFunc()
}

// AddPact loads a pact definition from a file and ensures that stub servers are running.
func AddPact(filename string) error {
	pactFilePaths := []string{filename}
	pacts := groupByProvider(readAllPacts(pactFilePaths))
	for _, p := range pacts {
		key := p.Provider.Name + p.Consumer.Name
		EnsurePactRunning(p.Provider.Name, p.Consumer.Name)

		for _, i := range p.Interactions {
			err := pactServers[key].AddInteraction(i)
			if err != nil {
				return fmt.Errorf("error adding pact from %s: %v", filename, err)
			}
		}
	}
	return nil
}

// AddPactInteraction ensures that a stub server is running for the provided provider/consumer and returns an
// interaction to be configured
func AddPactInteraction(provider, consumer string, interaction *dsl.Interaction) error {
	key := provider + consumer
	EnsurePactRunning(provider, consumer)
	return pactServers[key].AddInteraction(interaction)
}

func VerifyInteractions(provider, consumer string, retryOptions ...retry.RetryOption) error {
	verify := func() error {
		key := provider + consumer
		err := pactServers[key].Verify()

		if err != nil {
			return fmt.Errorf("pact verification failed: %v", err)
		}
		log.Infof("Pacts verified successfully!")
		return nil
	}

	// (Re-)try verification according to the specified options (if any).
	// If no options are specified, defaults are used.
	// Otherwise, it is assumed the caller wants full control of the retry behaviour.
	if len(retryOptions) == 0 {
		retryOptions = defaultRetryOptions
	}
	cwd, _ := os.Getwd()
	if err := retry.Do(verify, retryOptions...); err != nil {
		return fmt.Errorf("pact interactions not matched - for details see %s/pact/logs/pact-%s.log", cwd, provider)
	}
	return nil
}

func EnsurePactRunning(provider, consumer string) string {
	dir, _ := os.Getwd()

	// Allow binding to 0.0.0.0 if desired
	bind := getBindAddress()

	key := provider + consumer
	mockServer, ok := pactServers[key]
	if !ok || !mockServer.Running {
		mockServer = loadRunningServer(provider, consumer)
		if mockServer != nil {
			return mockServer.BaseURL
		}

		log.Infof("starting new mock server for consumer: %s, provider: %s", consumer, provider)
		// This is done manually rather than using pact-go's service manager code, since that pipes the output streams,
		// so isn't suitable for long-running pact-servers if there is a problem that triggers stdout/stderr output.
		// It also prevents the servers from remaining started when run from goland or compiled test binaries
		port := assignPort(provider, consumer)
		args := []string{"service",
			"--pact-specification-version",
			fmt.Sprintf("%d", 3),
			"--pact-dir",
			filepath.FromSlash(fmt.Sprintf(filepath.Join(dir, "target"))),
			"--log",
			filepath.FromSlash(fmt.Sprintf(filepath.Join(dir, "pact", "logs")) + "/" + "pact-" + provider + ".log"),
			"--consumer",
			consumer,
			"--provider",
			provider,
			"--pact-file-write-mode",
			"merge",
			"--host",
			bind,
			"--port",
			strconv.Itoa(port)}
		cmd := exec.Command("pact-mock-service", args...)
		setPathOnce()
		cmd.Env = os.Environ()

		log.Debugf("%s %s", "pact-mock-service", strings.Join(args, " "))
		err := cmd.Start()
		if err != nil {
			log.WithError(err).Fatalf("failed to start mock server")
		}

		serverAddress := fmt.Sprintf("http://%s:%d", bind, port)
		mockServer = &MockServer{
			Port:     port,
			BaseURL:  serverAddress,
			Consumer: consumer,
			Provider: provider,
			Pid:      cmd.Process.Pid,
			Running:  true,
		}
		err = retry.Do(func() error {
			return mockServer.call("GET", serverAddress, nil)
		}, retry.Timeout(5*time.Second), retry.Sleep(25*time.Millisecond), retry.MaxTries(1000))
		if err != nil {
			log.WithError(err).Fatalf(`timed out waiting for mock server to report healthy, pid %d`, mockServer.Pid)
		}

		mockServer.writePidFile()
		viper.Set(provider, mockServer.BaseURL)
		pactServers[key] = mockServer
	}
	return mockServer.BaseURL
}

// Runs mock services defined by the given pacts, invokes testFunc then verifies that the pacts have been invoked successfully
func IntegrationTest(pactFilePaths []Pact, testFunc func(), retryOptions ...retry.RetryOption) {
	TestWithStubServices(pactFilePaths, func() {
		testFunc()

		verify := func() error {
			pacts := groupByProvider(readAllPacts(pactFilePaths))
			for _, p := range pacts { //verify only pacts defined for this TC
				key := p.Provider.Name + p.Consumer.Name
				err := pactServers[key].Verify()

				if err != nil {
					return fmt.Errorf("pact verification failed: %v", err)
				}
			}
			log.Infof("Pacts verified successfully!")
			return nil
		}

		// (Re-)try verification according to the specified options (if any).
		// If no options are specified, defaults are used.
		// Otherwise, it is assumed the caller wants full control of the retry behaviour.
		if len(retryOptions) == 0 {
			retryOptions = defaultRetryOptions
		}
		if err := retry.Do(verify, retryOptions...); err != nil {
			log.Fatalf("Pact verification failed!! For more info on the error check the logs/pact*.log files, they are quite detailed")
		}
	})
}

// Deprecated: StopMockServers left here for backwards compatibility, does not stop anything.
func StopMockServers() {
}

func VerifyAll() error {
	for _, s := range pactServers {
		if !s.Running {
			continue
		}
		if err := s.Verify(); err != nil {
			return err
		}
	}
	return nil
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

			responses, verifyErr := pactClient.VerifyProvider(request)
			allTestsSucceeded := true

			for _, response := range responses {
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
			}

			if verifyErr != nil {
				t.Fatal(verifyErr)
			}
		})
	}
}

func getBindAddress() string {
	// Allow binding to 0.0.0.0 if desired
	bind := "127.0.0.1"
	if b := os.Getenv("PACT_BIND_ADDRESS"); len(b) > 0 {
		bind = b
	}
	return bind
}

// clearInternalState is a hack for test purposes to simulate a test running in a different process
func clearInternalState() {
	pactServers = map[string]*MockServer{}
	once = sync.Once{}
	pactClient = &dsl.PactClient{}
}
