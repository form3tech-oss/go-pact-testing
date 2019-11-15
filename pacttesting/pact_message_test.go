package pacttesting

import "testing"

func TestPactProviderMessaging(t *testing.T) {
	given, when, then := PactMessageTestingTest(t)

	given.
		message_producers_are_configured()

	when.
		messages_are_verified_against_pacts()

	then.
		validation_is_successful()
}
