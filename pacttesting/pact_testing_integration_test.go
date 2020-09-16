package pacttesting

import (
	"os"
	"testing"
)

func TestAcc_verify_pact_with_single_pact(t *testing.T) {
	IntegrationTest([]Pact{"testservicea.get.test"}, func() {

		given, when, then := PactTestingTest(t)

		given.
			the_test_is_using_a_single_pact()

		when.
			the_pact_for_service_a_is_called()

		then.
			the_response_for_service_a_should_be_200_ok().and().
			no_error_should_be_returned_from_service_a()

	})

}

func TestAcc_verify_two_pacts_from_two_providers(t *testing.T) {

	IntegrationTest([]Pact{"testservicea.get.test", "testserviceb.get.test"}, func() {

		given, when, then := PactTestingTest(t)

		given.
			the_test_is_using_a_single_pact()

		when.
			the_pact_for_service_a_is_called().and().
			the_pact_for_service_b_is_called()

		then.
			the_response_for_service_a_should_be_200_ok().and().
			no_error_should_be_returned_from_service_a().and().
			the_response_for_service_b_should_be_200_ok().and().
			no_error_should_be_returned_from_service_b()
	})

}

func TestAcc_first_test_with_two_providers_second_test_with_one(t *testing.T) {

	IntegrationTest([]Pact{"testservicea.get.test", "testserviceb.get.test"}, func() {

		given, when, then := PactTestingTest(t)

		given.
			the_test_is_using_a_single_pact()

		when.
			the_pact_for_service_a_is_called().and().
			the_pact_for_service_b_is_called()

		then.
			the_response_for_service_a_should_be_200_ok().and().
			no_error_should_be_returned_from_service_a().and().
			the_response_for_service_b_should_be_200_ok().and().
			no_error_should_be_returned_from_service_b()
	})

	IntegrationTest([]Pact{"testservicea.get.test"}, func() {

		given, when, then := PactTestingTest(t)

		given.
			the_test_is_using_a_single_pact()

		when.
			the_pact_for_service_a_is_called()

		then.
			the_response_for_service_a_should_be_200_ok().and().
			no_error_should_be_returned_from_service_a()

	})

}

func TestProvider_Verification_Success(t *testing.T) {

	given, when, then := PactTestingTest(t)

	given.
		the_test_is_using_a_single_pact()

	when.
		provider_pacts_are_verified()

	then.
		provider_pact_verification_is_successful()

}

func TestAcc_verify_all(t *testing.T) {
	IntegrationTest([]Pact{"testservicea.get.test"}, func() {

		given, when, then := PactTestingTest(t)

		given.
			the_test_is_using_a_single_pact().and().
			an_error_should_be_returned_from_the_verify()

		when.
			the_pact_for_service_a_is_called()

		then.
			the_response_for_service_a_should_be_200_ok().and().
			no_error_should_be_returned_from_the_verify()

	})

}

func TestAcc_Preassign_Ports(t *testing.T) {

	given, when, then := PactTestingTest(t)

	given.
		the_service_does_not_have_preassigned_port()

	when.
		the_service_gets_preassigned()

	then.
		the_service_has_a_preassigned_port()

}

func TestAcc_verify_pact_with_single_pact_interactions_are_deleted(t *testing.T) {
	defer func() {
		if err := recover(); err != nil {
			// API CAll to check if verifications = 200
			if VerifyAll() != nil {
				t.Fatal("Expected no interactions to be verified")
			}
		} else {
			t.Fatal("was expecting error")
		}
	}()

	IntegrationTest([]Pact{"testservicea.get.test"}, func() {

		given, when, _ := PactTestingTest(t)

		given.
			the_test_is_using_a_single_pact()
		when.
			the_test_panics()
	})
}

func TestMain(m *testing.M) {

	result := m.Run()

	StopMockServers()

	os.Exit(result)

}
