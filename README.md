# go-pact-testing
pact testing in go

## Setup

To use pact testing add the following `make` task as part of your build to install the `pact-go` daemon:

```
platform := $(shell uname)

ifeq (${platform},Darwin)
pact_filename := "pact-${pact_version}-osx.tar.gz"
else
pact_filename := "pact-${pact_version}-linux-x86_64.tar.gz"
endif


install-pact-go:
	@if [ ! -d ./pact ]; then \
		echo "pact-go not installed, installing..."; \
		wget https://github.com/pact-foundation/pact-ruby-standalone/releases/download/v${pact_version}/${pact_filename} -O /tmp/pactserver.tar.gz && tar -xvf /tmp/pactserver.tar.gz -C .; \
	fi
```

To integrate with the pact broker, add the following tasks to fetch pacts, publish pacts and publish pact verifications.

```
publish: pact-publish-verification docker-publish

pact-test-config:
	@test "${PACT_BROKER_KEY_ID}" || (echo "you must define PACT_BROKER_KEY_ID to get pacts")
	@test "${PACT_BROKER_ACCESS_KEY}" || (echo "you must define PACT_BROKER_ACCESS_KEY to get pacts")

pact-get: pact-test-config
	@exec sh -c '"$(CURDIR)/scripts/pact-get.sh" {}'

pact-publish: pact-test-config
	@exec sh -c '"$(CURDIR)/scripts/pact-publish.sh" {}'

pact-publish-verification: pact-test-config
	@exec sh -c '"$(CURDIR)/scripts/pact-publish-verifications.sh" {}'
```


You need to add the following lines to your `.gitignore`

```
pact-*.log
pact/
build/
```

## Pact Consumer Testing
Consumer testing uses pact files to define mocks for any dependent services which your tests interact with. These 
can be used to provide expected responses to tests and also verify that interactions were indeed made. Once testing is 
complete, consumer pacts are uploaded to the pact broker via the `pact-publish` task, for verification by provider tests. 

There are two ways to define consumer tests - the original integration test or the newer DSL test. 

### DSL

DSL tests define interactions and verify responses as part of the test. This may result in more expressive tests when using BDD style tests

```go
// pact servers can be optionally started during test setup. A free port is chosen automatically. This may be useful if the url needs to be injected into the service under test.
url := pacttesting.EnsurePactRunning("testservicea", "go-pact-testing")

// given
// test service returns 200 for a get request
// .. either from json
pacttesting.AddPact(s.t,"testservicea.get.test")
// .. or via code
pacttesting.AddPactInteraction(s.t, "testservicea", "go-pact-testing", (&dsl.Interaction{}).
		UponReceiving("Request for a test endpoint A").
		WithRequest(dsl.Request{
			Method: "GET",
			Path:   dsl.String("/v1/test"),
		}).
		WillRespondWith(dsl.Response{
			Status:  200,
			Headers: dsl.MapMatcher{"Content-Type": dsl.String("application/json; charset=utf-8")},
			Body:    map[string]string{"foo": "bar"},
		}))

// when 
// ... functionality that invokes the service

// then
// check that the interactions are called
pacttesting.VerifyInteractions(s.t, "testservicea", "go-pact-testing")
```

### Integration Test
Consumer tests can be written using the `IntegrationTest` function. Pacts should be stored in a directory called 'pacts': 
```go
IntegrationTest([]Pact{"testservicea.get.test", "testserviceb.get.test"}, func() {
    // test-code-here. 
})
```

## Pact Provider Testing
Provider testing involves taking the pacts written by consumers and ensuring that the service produces the output that 
the consumer has declared via the pact that they expect it to produce. 

Pacts can be obtained from the pact broker via the `pact-get` make task. Tests will produce verifications which can 
be uploaded to the broker with `pact-publish-verification`. This includes full details of provider and consumer 
versions, so the broker can be used to check which versions are compatible.

Provider tests can be written with the assistance of VerifyProviderPacts. Note that this can either be configured 
with the pacts from the broker, or locally (which is more useful when developing pacts)
```
pacttesting.VerifyProviderPacts(pacttesting.PactProviderTestParams{
    Testing: t,
    Pacts:                  "build/incoming-pacts/*.json",
    AuthToken:             token,
    ProviderVersion:       "v0.0.1",
    BaseURL:               viper.GetString(settings.ServiceName + "-address"),
    ProviderStateSetupURL: viper.GetString(settings.ServiceName + "-address") + "/pact-setup",
})
``` 

Most pacts will require some existing state on the server. This must be configured via a url on the provider service.
The handler for this will need to check the state name and provide the initial state matching that required by the pact

### Pact Messaging Provider Testing
Pact messaging is typically used for non-http and asynchronous services, such as SQS queues. 
Like regular pacts, provider states can be defined within the pact json. These should be used to invoke the service in such a way that it generates a message on the queue, e.g. submitting a payment to add a message to the validation queue. 
The messaging tests then requires a `messageProducer` to obtain the message from the queue. 
The test framework exposes these message produces through a new http service and invokes the pact client to verify the service against the pact files. 
See the tests for further details of how to configure pact messaging provider tests.  

## Troubleshooting

### Splitting PACT tests before test run

PACT tests may fail while running against bulk files (with many interactions).

To circumvent it, you can split bulk file into smaller ones like following:

```go
func TestPactProviders(t *testing.T) {
	setupProviderStates(t)

	testOrganisationId, _ := uuid.FromString("743d5b63-8e6f-432e-a8fa-c5d8d2ee5fcb")
	testOrganisationId2, _ := uuid.FromString("6e9224ee-9753-47b2-b235-b155e951ab64")
	token := buildDefaultToken(testOrganisationId, testOrganisationId2)

	pactFilesFilter := viper.GetString("PACT_FILES_FILTER")
	if pactFilesFilter == "" {
		pactFilesFilter = "../../../build/incoming-pacts/*.json"
	}
	log.Info("PACT file filter set to: ", pactFilesFilter)

	pactFiles, pactFilesErr := filepath.Glob(pactFilesFilter)
	if len(pactFiles) == 0 {
		t.Fatal("No PACT files found using filter: ", pactFilesFilter)
	} else if pactFilesErr != nil {
		t.Fatal("Couldn't find pact files: ", pactFilesErr)
	}
	testCaseDir := filepath.Join(os.TempDir(), uuid.NewV1().String())
	for _, pactFile := range pactFiles {
		log.Info("Splitting PACT file into smaller ones: ", pactFile)
		if tcError := pacttesting.SplitPactBulkFile(pactFile, testCaseDir); tcError != nil {
			t.Fatal("Couldn't split PACT file - file: ", pactFile, ", error: ", tcError)
		}
	}

	log.Info("Running PACT tests: ", pactFiles)
	t.Run("FOO gateway Pacts", func(t *testing.T) {
		runPactTests(t, token, filepath.Join(testCaseDir, "*.json"))
	})

}
```
