package pacttesting

import (
	"encoding/json"
	"fmt"
)

// PactFile describes expectations between provider and consumer
type PactFile struct {
	Provider struct {
		Name string `json:"name"`
	} `json:"provider"`
	Consumer struct {
		Name string `json:"name"`
	} `json:"consumer"`
	Interactions []struct {
		Description    string      `json:"description"`
		ProviderStates interface{} `json:"providerStates"`
		Request        interface{} `json:"request"`
		Response       interface{} `json:"response"`
	} `json:"interactions"`
	Metadata interface{} `json:"metadata"`
}

// NewPactFile create new PACT file representation
func NewPactFile(data []byte) (*PactFile, error) {
	pactFile := &PactFile{}
	err := json.Unmarshal(data, pactFile)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling pack file: %w", err)
	}
	return pactFile, nil
}

// Split divides bulk file with many interactions to single-interaction PACT files.
// It's required as a workaround to make bigger PACT test runs working.
func (f *PactFile) Split() *[]*PactFile {
	interactionsCount := len(f.Interactions)
	if interactionsCount == 0 {
		return nil
	}
	files := make([]*PactFile, len(f.Interactions))
	if interactionsCount == 1 {
		files[0] = f
	} else {
		for idx, interaction := range f.Interactions {
			singleFile := PactFile{
				Provider: f.Provider,
				Consumer: f.Consumer,
				Metadata: f.Metadata,
			}
			singleFile.Interactions = append(singleFile.Interactions, interaction)
			files[idx] = &singleFile
		}
	}
	return &files
}
