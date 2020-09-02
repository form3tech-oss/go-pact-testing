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
		split_file_exist("consumer-provider.First interaction.json")
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
		split_file_exist("consumer-provider.First interaction.json").and().
		split_file_exist("consumer-provider.Second interaction.json").and().
		split_file_exist("consumer-provider.Third interaction.json").and()
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
		split_file_exist("consumer-provider-1.First interaction.json").and().
		split_file_exist("consumer-provider-1.Second interaction.json").and().
		split_file_exist("consumer-provider-1.Third interaction.json").and().
		split_file_exist("consumer-provider-2.First interaction.json").and().
		split_file_exist("consumer-provider-2.Second interaction.json").and().
		split_file_exist("consumer-provider-2.Third interaction.json").and()
}
