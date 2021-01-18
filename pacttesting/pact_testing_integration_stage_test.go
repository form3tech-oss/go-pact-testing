package pacttesting

import (
	"github.com/pact-foundation/pact-go/dsl"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"fmt"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type pactTestingStage struct {
	t         *testing.T
	errA      error
	errB      error
	responseA *http.Response
	responseB *http.Response

	pactFilePath   string
	pactFile       *PactFile
	splitPactFiles []*PactFile
}

func PactTestingTest(t *testing.T) (*pactTestingStage, *pactTestingStage, *pactTestingStage) {
	s := &pactTestingStage{
		t: t,
	}

	return s, s, s
}

func InlinePactTestingTest(t *testing.T) (*pactTestingStage, *pactTestingStage, *pactTestingStage) {
	ResetPacts()
	s := &pactTestingStage{
		t: t,
	}

	return s, s, s
}

func (s *pactTestingStage) and() *pactTestingStage {

	return s
}

func (s *pactTestingStage) the_test_is_using_a_single_pact() *pactTestingStage {

	return s
}

func (s *pactTestingStage) parsePactFile(fileName string) *PactFile {
	file, fileErr := ioutil.ReadFile(fileName)
	if fileErr != nil {
		log.Fatal("Couldn't read PACT tests from file: ", fileName, ", error: ", fileErr.Error())
	}
	pactFile, pactFileErr := NewPactFile(file)
	if pactFileErr != nil {
		log.Fatal("Couldn't parse PACT file: ", fileName, ", error: ", pactFileErr.Error())
	}
	return pactFile
}

func (s *pactTestingStage) a_bulk_pact_file() *pactTestingStage {
	wd, _ := os.Getwd()
	s.pactFilePath = filepath.Join(wd, "pacts/testservices.get.bulk.test.json")
	s.pactFile = s.parsePactFile(s.pactFilePath)
	return s
}

func (s *pactTestingStage) the_pact_for_service_a_is_called() *pactTestingStage {

	s.responseA, s.errA = http.Get(fmt.Sprintf("%s/v1/test", viper.GetString("testservicea")))

	return s
}

func (s *pactTestingStage) the_pact_for_service_b_is_called() *pactTestingStage {

	s.responseB, s.errB = http.Get(fmt.Sprintf("%s/v1/test", viper.GetString("testserviceb")))

	return s
}

func (s *pactTestingStage) the_response_for_service_a_should_be_200_ok() *pactTestingStage {

	assert.Equal(s.t, 200, s.responseA.StatusCode)

	return s
}
func (s *pactTestingStage) the_response_for_service_b_should_be_200_ok() *pactTestingStage {

	assert.Equal(s.t, 200, s.responseB.StatusCode)

	return s
}

func (s *pactTestingStage) no_error_should_be_returned_from_service_a() *pactTestingStage {

	assert.Nil(s.t, s.errA)

	return s
}

func (s *pactTestingStage) no_error_should_be_returned_from_service_b() *pactTestingStage {

	assert.Nil(s.t, s.errB)

	return s
}

func (s *pactTestingStage) an_error_should_be_returned_from_the_verify() *pactTestingStage {

	assert.Error(s.t, VerifyAll())

	return s
}

func (s *pactTestingStage) no_error_should_be_returned_from_the_verify() *pactTestingStage {

	assert.NoError(s.t, VerifyAll())

	return s
}

func (s *pactTestingStage) provider_pacts_are_verified() *pactTestingStage {
	VerifyProviderPacts(PactProviderTestParams{
		Testing:   s.t,
		Pacts:     "pacttesting/providerpacts/*.json",
		AuthToken: "anything",
		BaseURL:   fmt.Sprintf("%s/v1/test", viper.GetString("testservicea")),
	})

	return s
}

func (s *pactTestingStage) file_is_split() *pactTestingStage {
	testCaseDir := filepath.Join(os.TempDir(), time.Now().String())
	if err := SplitPactBulkFile(s.pactFilePath, testCaseDir); err != nil {
		log.Fatal("Couldn't split bulk files: ", err)
	}
	files, _ := filepath.Glob(filepath.Join(testCaseDir, "*.json"))
	s.splitPactFiles = make([]*PactFile, 0)
	for _, f := range files {
		s.splitPactFiles = append(s.splitPactFiles, s.parsePactFile(f))
	}
	return s
}

func (s *pactTestingStage) many_small_pact_files_are_created() *pactTestingStage {
	assert.Equal(s.t, 2, len(s.splitPactFiles), "Not all tests cases have been created")
	for _, f := range s.splitPactFiles {
		assert.Equal(s.t, 1, len(f.Interactions), "Each test case should have only one interaction")
	}
	assert.Equal(s.t, "Request for a test endpoint A", s.splitPactFiles[0].Interactions[0].Description)
	assert.Equal(s.t, "Request for a test endpoint B", s.splitPactFiles[1].Interactions[0].Description)
	return s
}

func (s *pactTestingStage) provider_pact_verification_is_successful() *pactTestingStage {
	_, filename, _, _ := runtime.Caller(0)
	dir := filename[0:strings.LastIndex(filename, "/")]
	b, err := ioutil.ReadFile(dir + "/../build/pact-verifications/providerA.json")
	assert.Nil(s.t, err)

	str := string(b)
	assert.Contains(s.t, str, "\"success\": true")

	return s
}

func (s *pactTestingStage) the_service_does_not_have_preassigned_port() *pactTestingStage {
	assert.NotContains(s.t, serverPortMap, "testservice-prego-pact-testing")
	assert.Equal(s.t, "", viper.GetString("testservice-pre"))
	return s
}

func (s *pactTestingStage) the_service_gets_preassigned() *pactTestingStage {
	PreassignPorts([]Pact{"testservice-pre.get.test"})
	return s
}

func (s *pactTestingStage) the_service_has_a_preassigned_port() *pactTestingStage {
	assert.NotEqual(s.t, "", viper.GetString("testservice-pre"))
	assert.Contains(s.t, serverPortMap, "testservice-prego-pact-testing")
	return s
}

func (s *pactTestingStage) the_test_panics() {
	panic("Test Panic")
}
func (s *pactTestingStage) test_service_a_returns_200_for_get() *pactTestingStage {
	AddPactInteraction(s.t, "testservicea", "go-pact-testing", (&dsl.Interaction{}).
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
	return s
}

func (s *pactTestingStage) test_service_a_returns_200_for_get_from_file() *pactTestingStage {
	AddPact(s.t, "testservicea.get.test")
	return s
}

func (s *pactTestingStage) test_service_a_is_called() *pactTestingStage {
	return s.the_pact_for_service_a_is_called()
}

func (s *pactTestingStage) test_service_a_was_invoked() *pactTestingStage {
	VerifyInteractions(s.t, "testservicea", "go-pact-testing")
	return s
}
