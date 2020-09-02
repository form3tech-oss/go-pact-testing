package pacttesting

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type PactRequestMatchingFilter = func(map[string]interface{})

//SplitPactBulkFile reads bulk PACT files, splits it into smaller ones
//and writes output to destination directory
func SplitPactBulkFile(bulkFilePath string, outputDirPath string, requestFilters ...PactRequestMatchingFilter) error {
	//prepare output directory
	if _, outputDirErr := os.Stat(outputDirPath); os.IsNotExist(outputDirErr) {
		if newDirErr := os.MkdirAll(outputDirPath, os.ModePerm); newDirErr != nil {
			return errors.Wrap(newDirErr, "Couldn't create output directory: "+outputDirPath)
		}
	}
	//read bulk file
	file, fileErr := ioutil.ReadFile(bulkFilePath)
	if fileErr != nil {
		return errors.Wrap(fileErr, "Couldn't read PACT tests from file: "+bulkFilePath)
	}
	pactFile, pactFileErr := NewPactFile(file)
	if pactFileErr != nil {
		return errors.Wrap(pactFileErr, "Couldn't parse PACT file: "+bulkFilePath)
	}
	//split into smaller files
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
			return errors.Wrap(jsonErr, "Couldn't change interaction to test case - interaction idx: "+strconv.Itoa(idx))
		}

		//prefix the output filename with source file base name
		prefix := filepath.Base(bulkFilePath)
		prefix = strings.TrimSuffix(prefix, filepath.Ext(prefix))
		tcFilePath := filepath.Join(
			outputDirPath,
			fmt.Sprintf("%s.%s.json", prefix, tc.Interactions[0].Description),
		)
		if writeErr := ioutil.WriteFile(tcFilePath, json, os.ModePerm); writeErr != nil {
			return errors.Wrap(writeErr, "Couldn't write test case to file - interaction idx: "+strconv.Itoa(idx)+", output file path: "+tcFilePath)
		}
	}
	return nil
}
