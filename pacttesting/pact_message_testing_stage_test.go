package pacttesting

import (
	"math/rand"
	"testing"

	"github.com/pact-foundation/pact-go/dsl"
)

type pactMessageTestingStage struct {
	t                *testing.T
	messageProducers dsl.MessageHandlers
}

func PactMessageTestingTest(t *testing.T) (*pactMessageTestingStage, *pactMessageTestingStage, *pactMessageTestingStage) {
	s := &pactMessageTestingStage{
		t: t,
	}

	return s, s, s
}

func (s *pactMessageTestingStage) and() *pactMessageTestingStage {
	return s
}

type testMessage struct {
	One    string   `json:"one"`
	Two    string   `json:"two"`
	Random int      `json:"id"`
	List   []string `json:"list"`
}

func (s *pactMessageTestingStage) message_producers_are_configured() *pactMessageTestingStage {
	s.messageProducers = dsl.MessageHandlers{
		"message 1": func(message dsl.Message) (interface{}, error) {
			return "Hello, Pact Message Verifier", nil
		},
		"message 2": func(message dsl.Message) (interface{}, error) {
			return &testMessage{
				One:    "First Entry",
				Two:    "Second entry",
				Random: rand.Int(),
				List:   []string{"a", "b", "c"},
			}, nil
		},
	}
	return s
}

func (s *pactMessageTestingStage) messages_are_verified_against_pacts() {
	VerifyProviderMessagingPacts(PactProviderTestParams{
		Testing:   s.t,
		Pacts:     "pacttesting/messagepacts/*.json",
		AuthToken: "Hello",
	}, s.messageProducers)
}

func (s *pactMessageTestingStage) validation_is_successful() {

}
