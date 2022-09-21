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

	"github.com/avast/retry-go/v3"
	"github.com/pact-foundation/pact-go/dsl"
	"github.com/pact-foundation/pact-go/types"
	"github.com/pact-foundation/pact-go/utils"
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
	pathOnce sync.Once
)

type Server struct {
	once        sync.Once
	pactClient  *dsl.PactClient
	pactServers map[string]*MockServer
}

var defaultServer = &Server{
	pactServers: make(map[string]*MockServer),
}

var defaultRetryOptions = []retry.Option{
	retry.Attempts(150000),
	retry.Delay(200 * time.Millisecond),
	retry.DelayType(retry.FixedDelay),
}

func readPactFile(pactFilePath string) *pact {
	dir, _ := os.Getwd()

	var file string
	if strings.HasSuffix(pactFilePath, ".json") {
		file = pactFilePath
	} else {
		file = fmt.Sprintf("%s.json", pactFilePath)
	}
	path := filepath.FromSlash(filepath.Join(dir, "pacts", file))

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
	gitCommand := exec.Command("git", "describe", "--tags")
	var out bytes.Buffer
	gitCommand.Stdout = &out
	err := gitCommand.Run()

	if err != nil {
		return "", err
	}

	version := strings.TrimRight(out.String(), "\n")
	return version, nil
}

func setBinPath() {
	pathOnce.Do(func() {
		if _, err := exec.LookPath("pact-mock-service"); err == nil {
			return
		}

		binPath := os.Getenv("PACTTESTING_PATH")

		if binPath == "" {
			topLevelDir, err := getTopLevelDir()

			if err != nil {
				panic(err)
			}

			binPath = filepath.Join(topLevelDir, "pact/bin") + ":" + filepath.Join(topLevelDir, "tools/pact/bin")
		}

		os.Setenv("PATH", os.Getenv("PATH")+":"+binPath)
	})
}

func (s *Server) buildPactClientOnce() {
	s.once.Do(func() {
		setBinPath()
		s.pactClient = dsl.NewClient()
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
			defaultServer.assignPort(p.Provider.Name, p.Consumer.Name)
		}
	}
}

func (s *Server) assignPort(provider, consumer string) int {
	key := provider + consumer
	_, ok := s.pactServers[key]
	if !ok {
		port, err := utils.GetFreePort()

		if err != nil {
			panic(err)
		}
		s.pactServers[key] = &MockServer{
			Port:     port,
			BaseURL:  fmt.Sprintf("http://%s:%d", getBindAddress(), port),
			Consumer: consumer,
			Provider: provider,
		}
		exposeServerUrl(provider, s.pactServers[key].BaseURL)
	}
	return s.pactServers[key].Port
}

func exposeServerUrl(provider, serverUrl string) {
	viper.Set(provider, serverUrl)
	//Also set the base url as an environment variable to remove dependency on viper
	key := "PACTTESTING_" + strings.ToUpper(strings.ReplaceAll(provider, "-", "_"))
	err := os.Setenv(key, serverUrl)
	if err != nil {
		log.WithError(err).Errorf("Failed to set environment variable %s", key)
	}
}

func (s *Server) ResetPacts() {
	for key, pactServer := range s.pactServers {
		if !pactServer.Running {
			continue
		}
		err := pactServer.DeleteInteractions()
		if err != nil {
			log.WithError(err).Errorf("unable to delete configured interactions for %s", key)
		}
	}
}

// TestWithStubServices runs testFunc with stub services defined by given pacts. Does not verify that the stubs are called
func TestWithStubServices(pactFilePaths []Pact, testFunc func()) {
	defaultServer.TestWithStubServices(pactFilePaths, testFunc)
}

func (s *Server) TestWithStubServices(pactFilePaths []Pact, testFunc func()) {
	defer s.ResetPacts()

	PreassignPorts(pactFilePaths)

	pacts := groupByProvider(readAllPacts(pactFilePaths))

	for _, server := range s.pactServers {
		server.DeleteInteractions()
	}

	for _, p := range pacts {
		key := p.Provider.Name + p.Consumer.Name
		s.EnsurePactRunning(p.Provider.Name, p.Consumer.Name)

		for _, i := range p.Interactions {
			err := s.pactServers[key].AddInteraction(i)
			if err != nil {
				log.Errorf("Error adding pact: %v", err)
			}
		}
	}

	testFunc()
}

// AddPact loads a pact definition from a file and ensures that stub servers are running.
func AddPact(filename string) error {
	return defaultServer.AddPact(filename)
}

func (s *Server) AddPact(filename string) error {
	pactFilePaths := []string{filename}
	pacts := groupByProvider(readAllPacts(pactFilePaths))
	for _, p := range pacts {
		key := p.Provider.Name + p.Consumer.Name
		s.EnsurePactRunning(p.Provider.Name, p.Consumer.Name)

		for _, i := range p.Interactions {
			err := s.pactServers[key].AddInteraction(i)
			if err != nil {
				return fmt.Errorf("error adding pact from %s: %v", filename, err)
			}
		}
	}
	return nil
}

// AddPactInteraction ensures that a stub server is running for the provided provider/consumer and returns an
// interaction to be configured
func (s *Server) AddPactInteraction(provider, consumer string, interaction *dsl.Interaction) error {
	key := provider + consumer
	s.EnsurePactRunning(provider, consumer)
	return s.pactServers[key].AddInteraction(interaction)
}

func (s *Server) VerifyInteractions(provider, consumer string, retryOptions ...retry.Option) error {
	verify := func() error {
		key := provider + consumer
		err := s.pactServers[key].Verify()

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

func (s *Server) EnsurePactRunning(provider, consumer string) string {
	dir, _ := os.Getwd()

	// Allow binding to 0.0.0.0 if desired
	bind := getBindAddress()

	key := provider + consumer
	mockServer, ok := s.pactServers[key]
	if !ok || !mockServer.Running {
		mockServer = loadRunningServer(provider, consumer)
		if mockServer != nil {
			return mockServer.BaseURL
		}

		log.Infof("starting new mock server for consumer: %s, provider: %s", consumer, provider)
		// This is done manually rather than using pact-go's service manager code, since that pipes the output streams,
		// so isn't suitable for long-running pact-servers if there is a problem that triggers stdout/stderr output.
		// It also prevents the servers from remaining started when run from goland or compiled test binaries
		port := s.assignPort(provider, consumer)
		args := []string{"service",
			"--pact-specification-version", fmt.Sprintf("%d", 3),
			"--pact-dir", filepath.FromSlash(filepath.Join(dir, "target")),
			"--log", filepath.FromSlash(filepath.Join(dir, "pact", "logs") + "/" + "pact-" + provider + ".log"),
			"--consumer", consumer,
			"--provider", provider,
			"--pact-file-write-mode", "merge",
			"--host", bind,
			"--port", strconv.Itoa(port)}
		setBinPath()

		cmd := exec.Command("pact-mock-service", args...)

		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf

		cmd.Env = os.Environ()

		log.Debugf("%s %s", "pact-mock-service", strings.Join(args, " "))
		err := cmd.Start()
		if err != nil {
			log.WithError(err).Fatalf("failed to start mock server")
		}

		// Avoid zombies
		go func() {
			cmd.Wait()
		}()

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
		}, retry.Delay(25*time.Millisecond), retry.Attempts(200))
		if err != nil {
			log.WithError(err).Fatalf(`timed out waiting for mock server to report healthy, pid:%d stdout: %s`, mockServer.Pid, outBuf.String())
		}

		mockServer.writePidFile()
		exposeServerUrl(provider, mockServer.BaseURL)
		s.pactServers[key] = mockServer
	}
	return mockServer.BaseURL
}

// Runs mock services defined by the given pacts, invokes testFunc then verifies that the pacts have been invoked successfully
func RunIntegrationTest(t *testing.T, pactFilePaths []Pact, testFunc func(), retryOptions ...retry.Option) {
	defaultServer.RunIntegrationTest(t, pactFilePaths, testFunc, retryOptions...)
}

func (s *Server) RunIntegrationTest(t *testing.T, pactFilePaths []Pact, testFunc func(), retryOptions ...retry.Option) {
	TestWithStubServices(pactFilePaths, func() {
		testFunc()

		// (Re-)try verification according to the specified options (if any).
		// If no options are specified, defaults are used.
		// Otherwise, it is assumed the caller wants full control of the retry behaviour.
		if len(retryOptions) == 0 {
			retryOptions = defaultRetryOptions
		}
		verify := func() error { return s.checkVerificationStatus(pactFilePaths) }
		if err := retry.Do(verify, retryOptions...); err != nil {
			log.Error("Pact verification failed!! For more info on the error check the logs/pact*.log files, they are quite detailed")
			t.Errorf(err.Error())
		}
	})
}

// Runs mock services defined by the given pacts, invokes testFunc then verifies that the pacts have been invoked successfully
func IntegrationTest(pactFilePaths []Pact, testFunc func(), retryOptions ...retry.Option) {
	defaultServer.IntegrationTest(pactFilePaths, testFunc, retryOptions...)
}

func (s *Server) IntegrationTest(pactFilePaths []Pact, testFunc func(), retryOptions ...retry.Option) {
	TestWithStubServices(pactFilePaths, func() {
		testFunc()

		// (Re-)try verification according to the specified options (if any).
		// If no options are specified, defaults are used.
		// Otherwise, it is assumed the caller wants full control of the retry behaviour.
		if len(retryOptions) == 0 {
			retryOptions = defaultRetryOptions
		}
		verify := func() error { return s.checkVerificationStatus(pactFilePaths) }
		if err := retry.Do(verify, retryOptions...); err != nil {
			log.Fatalf("Pact verification failed!! For more info on the error check the logs/pact*.log files, they are quite detailed")
		}
	})
}

func (s *Server) checkVerificationStatus(pactFilePaths []Pact) error {
	pacts := groupByProvider(readAllPacts(pactFilePaths))
	for _, p := range pacts { //verify only pacts defined for this TC
		key := p.Provider.Name + p.Consumer.Name
		err := s.pactServers[key].Verify()

		if err != nil {
			return fmt.Errorf("pact verification failed: %v", err)
		}
	}
	log.Infof("Pacts verified successfully!")
	return nil
}

func StopMockServers() {
	defaultServer.StopMockServers()
}

func (s *Server) StopMockServers() {
	for key, ps := range s.pactServers {
		err := ps.Stop()
		if err != nil {
			log.WithError(err).Errorf("failed to stop server for consumer(%s), provider(%s)", ps.Consumer, ps.Provider)
		} else {
			delete(s.pactServers, key)
		}
	}
}

func VerifyAll() error {
	return defaultServer.VerifyAll()
}

func (s *Server) VerifyAll() error {
	for _, s := range s.pactServers {
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
	defaultServer.VerifyProviderPacts(params)
}

func (s *Server) VerifyProviderPacts(params PactProviderTestParams) {
	s.buildPactClientOnce()

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

			responses, verifyErr := s.pactClient.VerifyProvider(request)
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
func (s *Server) clearInternalState() {
	s.pactServers = map[string]*MockServer{}
	s.once = sync.Once{}
	s.pactClient = &dsl.PactClient{}
}
