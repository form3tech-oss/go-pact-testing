package pacttesting

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pactFileSplitStage struct {
	t              *testing.T
	require        *require.Assertions
	assert         *assert.Assertions
	bulkPactsDir   string
	bulkPactFile   *PactFile
	splitPactsDir  string
	splitPactFiles []string
}

func tempDirectory(t *testing.T) string {
	dir, err := ioutil.TempDir(
		os.TempDir(),
		uuid.New().String(),
	)

	if err != nil {
		t.Fatal("cannot create temp directory")
	}

	t.Cleanup(func() {
		err := os.RemoveAll(dir)
		if err != nil {
			t.Fatalf("error removing %s: %s", dir, err)
		}
	})

	return dir
}

func PactFileSplitTest(t *testing.T) (*pactFileSplitStage, *pactFileSplitStage, *pactFileSplitStage) {
	stage := &pactFileSplitStage{
		t:             t,
		require:       require.New(t),
		assert:        assert.New(t),
		bulkPactsDir:  tempDirectory(t),
		splitPactsDir: tempDirectory(t),
	}

	return stage, stage, stage
}

/**
 * given
 */

func (s *pactFileSplitStage) a_bulk_pact_file() *pactFileSplitStage {
	s.bulkPactFile = &PactFile{}
	return s
}

func (s *pactFileSplitStage) with_consumer(consumer string) *pactFileSplitStage {
	s.bulkPactFile.Consumer.Name = consumer
	return s
}

func (s *pactFileSplitStage) with_provider(provider string) *pactFileSplitStage {
	s.bulkPactFile.Provider.Name = provider
	return s
}

func (s *pactFileSplitStage) with_interaction(descriptions ...string) *pactFileSplitStage {
	for _, description := range descriptions {
		s.bulkPactFile.Interactions = append(s.bulkPactFile.Interactions, PactFileInteraction{
			Description: description,
		})
	}
	return s
}

func (s *pactFileSplitStage) exists() *pactFileSplitStage {
	name := fmt.Sprintf("%s-%s.json", s.bulkPactFile.Consumer.Name, s.bulkPactFile.Provider.Name)
	path := filepath.Join(s.bulkPactsDir, name)

	file, err := os.Create(path)
	s.require.NoErrorf(err, "error creating file %s", path)

	err = json.NewEncoder(file).Encode(s.bulkPactFile)
	s.require.NoError(err, "encoding error")

	err = file.Close()
	s.require.NoErrorf(err, "error closing file %s", path)

	return s
}

func (s *pactFileSplitStage) and() *pactFileSplitStage {
	return s
}

/**
 * when
 */

func (s *pactFileSplitStage) bulk_pact_file_is_split() *pactFileSplitStage {
	return s.split()
}

func (s *pactFileSplitStage) bulk_pact_files_are_split() *pactFileSplitStage {
	return s.split()
}

func (s *pactFileSplitStage) split() *pactFileSplitStage {
	files, err := filepath.Glob(filepath.Join(s.bulkPactsDir, "*.json"))
	s.require.NoError(err)

	for _, file := range files {
		s.require.NoError(SplitPactBulkFile(file, s.splitPactsDir))
	}

	return s
}

/**
 * then
 */

func (s *pactFileSplitStage) split_files_count_is(count int) *pactFileSplitStage {
	pattern := fmt.Sprintf("%s/*.json", s.splitPactsDir)
	splitPactFiles, err := filepath.Glob(pattern)
	s.require.NoErrorf(err, "error seeking for files under path %s", s.splitPactsDir)

	s.splitPactFiles = splitPactFiles
	s.assert.Len(s.splitPactFiles, count)

	return s
}

func (s *pactFileSplitStage) split_files_by_prefix_count_is(prefix string, count int) *pactFileSplitStage {
	var matchFiles []string
	for _, pactFile := range s.splitPactFiles {
		if !strings.HasPrefix(filepath.Base(pactFile), prefix) {
			continue
		}

		matchFiles = append(matchFiles, pactFile)
	}

	s.assert.Len(matchFiles, count)

	return s
}
