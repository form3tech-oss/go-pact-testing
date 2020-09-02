package pacttesting

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type pactTestingStage struct {
	t         *testing.T
	errA      error
	errB      error
	responseA *http.Response
	responseB *http.Response

	pactFilePath string
	pactFile     *PactFile
}

func PactTestingTest(t *testing.T) (*pactTestingStage, *pactTestingStage, *pactTestingStage) {
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
		Pacts:     "pacttesting/testdata/providerpacts/*.json",
		AuthToken: "anything",
		BaseURL:   fmt.Sprintf("%s/v1/test", viper.GetString("testservicea")),
	})

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
