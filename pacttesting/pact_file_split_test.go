package pacttesting

import "testing"

func Test_SplitSingleConsumerProvider_SingleInteraction(t *testing.T) {
	given, when, then := PactFileSplitTest(t)

	given.
		a_bulk_pact_file().
		with_consumer("consumer").and().
		with_provider("provider").and().
		with_interaction("First interaction").exists()

	when.
		bulk_pact_file_is_split()

	then.
		split_files_count_is(1).and().
		split_files_by_prefix_count_is("consumer-provider.First interaction", 1)
}

func Test_SplitSingleConsumerProvider_ManyInteractions(t *testing.T) {
	given, when, then := PactFileSplitTest(t)

	given.
		a_bulk_pact_file().
		with_consumer("consumer").and().
		with_provider("provider").and().
		with_interaction("First interaction").and().
		with_interaction("Second interaction").and().
		with_interaction("Third interaction").exists()

	when.
		bulk_pact_file_is_split()

	then.
		split_files_count_is(3).and().
		split_files_by_prefix_count_is("consumer-provider.First interaction", 1).and().
		split_files_by_prefix_count_is("consumer-provider.Second interaction", 1).and().
		split_files_by_prefix_count_is("consumer-provider.Third interaction", 1).and()
}

func Test_SplitManyConsumerProvider_ManyInteractions(t *testing.T) {
	given, when, then := PactFileSplitTest(t)

	given.
		a_bulk_pact_file().
		with_consumer("consumer").and().
		with_provider("provider-1").and().
		with_interaction(
			"First interaction",
			"Second interaction",
			"Third interaction",
		).exists().
		and().
		a_bulk_pact_file().
		with_consumer("consumer").and().
		with_provider("provider-2").and().
		with_interaction(
			"First interaction",
			"Second interaction",
			"Third interaction",
		).exists()

	when.
		bulk_pact_files_are_split()

	then.
		split_files_count_is(6).and().
		split_files_by_prefix_count_is("consumer-provider-1.First interaction", 1).and().
		split_files_by_prefix_count_is("consumer-provider-1.Second interaction", 1).and().
		split_files_by_prefix_count_is("consumer-provider-1.Third interaction", 1).and().
		split_files_by_prefix_count_is("consumer-provider-2.First interaction", 1).and().
		split_files_by_prefix_count_is("consumer-provider-2.Second interaction", 1).and().
		split_files_by_prefix_count_is("consumer-provider-2.Third interaction", 1).and()
}

func Test_SplitManyConsumerProvider_ManyInteractionsWithDuplicatedName(t *testing.T) {
	given, when, then := PactFileSplitTest(t)

	given.
		a_bulk_pact_file().
		with_consumer("consumer").and().
		with_provider("provider-1").and().
		with_interaction(
			"interaction",
			"interaction",
			"interaction",
		).exists().
		and().
		a_bulk_pact_file().
		with_consumer("consumer").and().
		with_provider("provider-2").and().
		with_interaction(
			"interaction",
			"interaction",
			"interaction",
		).exists()

	when.
		bulk_pact_files_are_split()

	then.
		split_files_count_is(6).and().
		split_files_by_prefix_count_is("consumer-provider-1.interaction", 3).and().
		split_files_by_prefix_count_is("consumer-provider-2.interaction", 3)
}
