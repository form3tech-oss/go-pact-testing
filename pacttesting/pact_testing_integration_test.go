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

func TestAcc_verify_pact_with_single_pact_dsl(t *testing.T) {
	given, when, then := InlinePactTestingTest(t)

	given.
		test_service_a_returns_200_for_get()

	when.
		test_service_a_is_called()

	then.
		test_service_a_was_invoked()
}

func TestAcc_verify_pact_with_single_pact_file(t *testing.T) {
	given, when, then := InlinePactTestingTest(t)

	given.
		test_service_a_returns_200_for_get_from_file()

	when.
		test_service_a_is_called()

	then.
		test_service_a_was_invoked()
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

func TestProvider_Should_Split_Bulk_File(t *testing.T) {

	given, when, then := PactTestingTest(t)

	given.
		a_bulk_pact_file()

	when.
		file_is_split()

	then.
		many_small_pact_files_are_created()

}

func TestProvider_GeneratesPactFiles_WithValidNames(t *testing.T) {

	given, when, then := PactTestingTest(t)

	given.
		a_bulk_pact_file_with_invalid_descriptions()

	when.
		file_is_split()

	then.
		pact_file_names_exclude_path_separator()
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

func TestAcc_pact_server_reused(t *testing.T) {
	given, when, then := InlinePactTestingTest(t)

	given.
		test_service_a_returns_200_for_get_from_file().and().
		test_service_a_is_called().and().
		the_test_completes_and_a_new_test_process_starts()

	when.
		a_mock_interaction_is_added()

	then.
		no_new_server_is_started()
}

func TestAcc_pact_server_started(t *testing.T) {
	given, when, then := InlinePactTestingTest(t)

	given.
		test_service_a_returns_200_for_get_from_file().and().
		test_service_a_is_called().and().
		the_pact_server_is_manually_stopped().and().
		the_test_completes_and_a_new_test_process_starts()

	when.
		a_mock_interaction_is_added()

	then.
		a_new_mock_server_is_started()
}

func TestAcc_pact_file_written_to_disk(t *testing.T) {

	given, when, then := InlinePactTestingTest(t)

	given.
		clean_slate().and().
		pact_verification_completed()

	when.
		a_mock_server_stops()

	then.
		pact_verification_written_to_disk()
}

func TestAcc_mock_server_stops_cleanly(t *testing.T) {

	given, when, then := InlinePactTestingTest(t)

	given.
		a_mock_server()

	when.
		a_mock_server_stops()

	then.
		the_process_is_not_running().and().
		the_corresponding_PID_file_is_removed()
}

func TestAcc_main_process_fails_on_mock_server_startup_crash(t *testing.T) {
	given, when, then := InlinePactTestingTest(t)

	given.
		a_broken_mock_server()

	when.
		the_mock_server_crashes_on_startup()

	then.
		the_main_process_fails()
}

func TestMain(m *testing.M) {
	result := m.Run()
	StopMockServers()
	os.Exit(result)
}
