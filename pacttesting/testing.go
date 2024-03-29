package pacttesting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
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

//nolint:gochecknoglobals // fixing is a breaking API change
var (
	pathOnce    sync.Once
	once        sync.Once
	pactClient  *dsl.PactClient
	pactServers = make(map[string]*MockServer)
)

func defaultRetryOptions() []retry.Option {
	return []retry.Option{
		retry.Attempts(150000),
		retry.Delay(200 * time.Millisecond),
		retry.DelayType(retry.FixedDelay),
	}
}

func readPactFile(pactFilePath string) *pact {
	dir, _ := os.Getwd()

	var file string
	if strings.HasSuffix(pactFilePath, ".json") {
		file = pactFilePath
	} else {
		file = pactFilePath + ".json"
	}
	path := filepath.FromSlash(filepath.Join(dir, "pacts", file))

	pactString, err := os.ReadFile(path)
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
	results := make([]*pact, len(pacts))
	for i, p := range pacts {
		results[i] = readPactFile(p)
	}

	return results
}

func groupByProvider(pacts []*pact) []*pact {
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

	results := make([]*pact, len(pactMap))
	i := 0
	for _, v := range pactMap {
		results[i] = v
		i++
	}

	return results
}

func getTopLevelDir() (string, error) {
	gitCommand := exec.Command("git", "rev-parse", "--show-toplevel")
	var out bytes.Buffer
	gitCommand.Stdout = &out
	err := gitCommand.Run()
	if err != nil {
		return "", fmt.Errorf("getting git top level dir: %w", err)
	}

	topLevelDir := strings.TrimRight(out.String(), "\n")
	return topLevelDir, nil
}

func getVersion() (string, error) {
	gitCommand := exec.Command("git", "describe", "--tags")
	var out bytes.Buffer
	var errOut bytes.Buffer
	gitCommand.Stdout = &out
	gitCommand.Stderr = &errOut
	err := gitCommand.Run()
	if err != nil {
		return "", fmt.Errorf("getting git version: %w; out: %s", err, errOut.String())
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

func buildPactClientOnce() {
	once.Do(func() {
		setBinPath()
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
		port, err := utils.GetFreePort()
		if err != nil {
			panic(err)
		}
		pactServers[key] = &MockServer{
			Port:     port,
			BaseURL:  providerHTTPScheme + net.JoinHostPort(getBindAddress(), strconv.Itoa(port)),
			Consumer: consumer,
			Provider: provider,
		}
		exposeServerURL(provider, pactServers[key].BaseURL)
	}
	return pactServers[key].Port
}

func exposeServerURL(provider, serverURL string) {
	viper.Set(provider, serverURL)
	// Also set the base url as an environment variable to remove dependency on viper
	key := "PACTTESTING_" + strings.ToUpper(strings.ReplaceAll(provider, "-", "_"))
	err := os.Setenv(key, serverURL)
	if err != nil {
		log.WithError(err).Errorf("Failed to set environment variable %s", key)
	}
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

// TestWithStubServices runs testFunc with stub services defined by given pacts.
// Does not verify that the stubs are called
func TestWithStubServices(pactFilePaths []Pact, testFunc func()) error {
	defer ResetPacts()

	PreassignPorts(pactFilePaths)

	pacts := groupByProvider(readAllPacts(pactFilePaths))

	for _, server := range pactServers {
		err := server.DeleteInteractions()
		if err != nil {
			log.WithError(err).Errorf("Error deleting interactions")
		}
	}

	var err error
	for _, p := range pacts {
		key := p.Provider.Name + p.Consumer.Name
		EnsurePactRunning(p.Provider.Name, p.Consumer.Name)

		for _, i := range p.Interactions {
			err = pactServers[key].AddInteraction(i)
			if err != nil {
				log.Errorf("Error adding pact: %v", err)
			}
		}
	}

	testFunc()
	return err
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
				return fmt.Errorf("error adding pact from %s: %w", filename, err)
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

func VerifyInteractions(provider, consumer string, retryOptions ...retry.Option) error {
	verify := func() error {
		key := provider + consumer
		err := pactServers[key].Verify()
		if err != nil {
			return fmt.Errorf("pact verification failed: %w", err)
		}
		log.Infof("Pacts verified successfully!")
		return nil
	}

	// (Re-)try verification according to the specified options (if any).
	// If no options are specified, defaults are used.
	// Otherwise, it is assumed the caller wants full control of the retry behaviour.
	if len(retryOptions) == 0 {
		retryOptions = defaultRetryOptions()
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
		args := []string{
			"service",
			"--pact-specification-version",
			"3",
			"--pact-dir",
			filepath.FromSlash(filepath.Join(dir, "target")),
			"--log",
			filepath.FromSlash(filepath.Join(dir, "pact", "logs") + "/" + "pact-" + provider + ".log"),
			"--consumer",
			consumer,
			"--provider",
			provider,
			"--pact-file-write-mode",
			"merge",
			"--host",
			bind,
			"--port",
			strconv.Itoa(port),
		}
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
			err := cmd.Wait()
			if err != nil {
				log.WithError(err).Error("mock server exited with error")
			}
		}()

		serverAddress := providerHTTPScheme + net.JoinHostPort(bind, strconv.Itoa(port))
		mockServer = &MockServer{
			Port:     port,
			BaseURL:  serverAddress,
			Consumer: consumer,
			Provider: provider,
		}
		err = retry.Do(func() error {
			err := mockServer.call("GET", serverAddress, nil)
			if err != nil && cmd.ProcessState != nil {
				return fmt.Errorf("calling mock server: %w", retry.Unrecoverable(err))
			}
			return err
		}, retry.DelayType(retry.FixedDelay), retry.Delay(100*time.Millisecond), retry.Attempts(100))
		if err != nil {
			log.
				WithError(err).
				Fatalf(`timed out waiting for mock server to report healthy, pid:%d stdout: %s`,
					mockServer.Pid,
					outBuf.String(),
				)
		}

		mockServer.Pid = cmd.Process.Pid
		mockServer.Running = true
		mockServer.writePidFile()
		exposeServerURL(provider, mockServer.BaseURL)
		pactServers[key] = mockServer
	}
	return mockServer.BaseURL
}

// Runs mock services defined by the given pacts,
// invokes testFunc then verifies that the pacts have been invoked successfully
func RunIntegrationTest(t *testing.T, pactFilePaths []Pact, testFunc func(), retryOptions ...retry.Option) error {
	t.Helper()
	return TestWithStubServices(pactFilePaths, func() {
		testFunc()

		// (Re-)try verification according to the specified options (if any).
		// If no options are specified, defaults are used.
		// Otherwise, it is assumed the caller wants full control of the retry behaviour.
		if len(retryOptions) == 0 {
			retryOptions = defaultRetryOptions()
		}
		verify := func() error { return checkVerificationStatus(pactFilePaths) }
		if err := retry.Do(verify, retryOptions...); err != nil {
			log.Error("Pact verification failed!!" +
				"For more info on the error check the logs/pact*.log files, they are quite detailed")
			t.Errorf(err.Error())
		}
	})
}

// Runs mock services defined by the given pacts,
// invokes testFunc then verifies that the pacts have been invoked successfully
func IntegrationTest(pactFilePaths []Pact, testFunc func(), retryOptions ...retry.Option) error {
	return TestWithStubServices(pactFilePaths, func() {
		testFunc()

		// (Re-)try verification according to the specified options (if any).
		// If no options are specified, defaults are used.
		// Otherwise, it is assumed the caller wants full control of the retry behaviour.
		if len(retryOptions) == 0 {
			retryOptions = defaultRetryOptions()
		}
		verify := func() error { return checkVerificationStatus(pactFilePaths) }
		if err := retry.Do(verify, retryOptions...); err != nil {
			log.Fatalf("Pact verification failed!!" +
				"For more info on the error check the logs/pact*.log files, they are quite detailed")
		}
	})
}

func checkVerificationStatus(pactFilePaths []Pact) error {
	pacts := groupByProvider(readAllPacts(pactFilePaths))
	for _, p := range pacts { // verify only pacts defined for this TC
		key := p.Provider.Name + p.Consumer.Name
		err := pactServers[key].Verify()
		if err != nil {
			return fmt.Errorf("pact verification failed: %w", err)
		}
	}
	log.Infof("Pacts verified successfully!")
	return nil
}

func StopMockServers() {
	for key, s := range pactServers {
		err := s.Stop()
		if err != nil {
			log.WithError(err).Errorf("failed to stop server for consumer(%s), provider(%s)", s.Consumer, s.Provider)
		} else {
			delete(pactServers, key)
		}
	}
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

	var pactsFilter string
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
					verificationJSON := fmt.Sprintf("{\"success\": %v,\"providerApplicationVersion\": \"%s\"}",
						allTestsSucceeded,
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
