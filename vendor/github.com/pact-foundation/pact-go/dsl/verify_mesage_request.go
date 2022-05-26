package dsl

import (
	"fmt"

	"github.com/pact-foundation/pact-go/types"
)

// VerifyMessageRequest contains the verification logic
// to send to the Pact Message verifier
type VerifyMessageRequest struct {
	// Local/HTTP paths to Pact files.
	PactURLs []string

	// Pact Broker URL for broker-based verification
	BrokerURL string

	// Tags to find in Broker for matrix-based testing
	Tags []string

	// Selectors are the way we specify which pacticipants and
	// versions we want to use when configuring verifications
	// See https://docs.pact.io/selectors for more
	ConsumerVersionSelectors []types.ConsumerVersionSelector

	// Username when authenticating to a Pact Broker.
	BrokerUsername string

	// Password when authenticating to a Pact Broker.
	BrokerPassword string

	// BrokerToken is required when authenticating using the Bearer token mechanism
	BrokerToken string

	// PublishVerificationResults to the Pact Broker.
	PublishVerificationResults bool

	// ProviderVersion is the semantical version of the Provider API.
	ProviderVersion string

	// ProviderTags is the set of tags to apply to the provider application version when results are published to the broker
	ProviderTags []string

	// MessageHandlers contains a mapped list of message handlers for a provider
	// that will be rable to produce the correct message format for a given
	// consumer interaction
	MessageHandlers MessageHandlers

	// StateHandlers contain a mapped list of message states to functions
	// that are used to setup a given provider state prior to the message
	// verification step.
	StateHandlers StateHandlers

	// Specify an output directory to log all of the verification request/responses
	// seen by the verification process. Useful to debug issues with your contract
	// and API
	PactLogDir string

	// Specify the log verbosity of the CLI verifier process spawned through verification
	// Useful for debugging issues with the framework itself
	PactLogLevel string

	// Arguments to the VerificationProvider
	// Deprecated: This will be deleted after the native library replaces Ruby deps.
	Args []string
}

// Validate checks that the minimum fields are provided.
// Deprecated: This map be deleted after the native library replaces Ruby deps,
// and should not be used outside of this library.
func (v *VerifyMessageRequest) Validate() error {
	v.Args = []string{}

	if len(v.PactURLs) != 0 {
		v.Args = append(v.Args, v.PactURLs...)
	} else {
		return fmt.Errorf("Pact URLs is mandatory")
	}

	v.Args = append(v.Args, "--format", "json")

	if v.BrokerUsername != "" {
		v.Args = append(v.Args, "--broker-username", v.BrokerUsername)
	}

	if v.BrokerPassword != "" {
		v.Args = append(v.Args, "--broker-password", v.BrokerPassword)
	}

	if v.ProviderVersion != "" {
		v.Args = append(v.Args, "--provider_app_version", v.ProviderVersion)
	}

	if v.PublishVerificationResults {
		v.Args = append(v.Args, "--publish_verification_results", "true")
	}

	if v.PactLogDir != "" {
		v.Args = append(v.Args, "--log-dir", v.PactLogDir)
	}

	if v.PactLogLevel != "" {
		v.Args = append(v.Args, "--log-level", v.PactLogLevel)
	}

	return nil
}
