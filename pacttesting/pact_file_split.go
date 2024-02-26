package pacttesting

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PactRequestMatchingFilter = func(map[string]interface{})

// SplitPactBulkFile reads bulk PACT files, splits it into smaller ones
// and writes output to destination directory
func SplitPactBulkFile(bulkFilePath string, outputDirPath string, requestFilters ...PactRequestMatchingFilter) error {
	// prepare output directory
	if _, outputDirErr := os.Stat(outputDirPath); os.IsNotExist(outputDirErr) {
		if newDirErr := os.MkdirAll(outputDirPath, os.ModePerm); newDirErr != nil {
			return fmt.Errorf("couldn't create output directory '%s': %w", outputDirPath, newDirErr)
		}
	}
	// read bulk file
	file, fileErr := os.ReadFile(bulkFilePath)
	if fileErr != nil {
		return fmt.Errorf("couldn't read PACT tests from file '%s': %w", bulkFilePath, fileErr)
	}
	pactFile, pactFileErr := NewPactFile(file)
	if pactFileErr != nil {
		return fmt.Errorf("couldn't parse PACT file '%s': %w", bulkFilePath, pactFileErr)
	}
	// split into smaller files
	testCases := pactFile.Split()
	if testCases == nil {
		return errors.New("No test cases have been found in file: " + bulkFilePath)
	}
	for idx, tc := range *testCases {
		for _, i := range tc.Interactions {
			requestMatchingRules, reqTypeOk := i.Request.(map[string]interface{})
			if reqTypeOk {
				for _, reqFilter := range requestFilters {
					reqFilter(requestMatchingRules)
				}
			}
		}

		json, jsonErr := json.Marshal(tc)
		if jsonErr != nil {
			return fmt.Errorf("couldn't change interaction to test case - interaction idx: %d err: %w", idx, jsonErr)
		}

		description := sanitize(tc.Interactions[0].Description)
		tcFilePath := filepath.Join(outputDirPath, description+".json")
		if writeErr := os.WriteFile(tcFilePath, json, os.ModePerm); writeErr != nil {
			return fmt.Errorf("couldn't write test case to file - interaction idx: %d , "+
				"output file path: %s, err: %w", idx, tcFilePath, writeErr)
		}
	}
	return nil
}

func sanitize(value string) string {
	value = strings.ReplaceAll(value, string(os.PathSeparator), "")

	return value
}
