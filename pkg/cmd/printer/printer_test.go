package printer_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khulnasoft-labs/tracker/pkg/cmd/flags"
	"github.com/khulnasoft-labs/tracker/pkg/config"
)

func TestTrackerEbpfPrepareOutputPrinterConfig(t *testing.T) {

	testCases := []struct {
		testName        string
		outputSlice     []string
		expectedPrinter config.PrinterConfig
		expectedError   error
	}{
		{
			testName:        "invalid format",
			outputSlice:     []string{"notaformat"},
			expectedPrinter: config.PrinterConfig{},
			expectedError:   fmt.Errorf("unrecognized output format: %s. Valid format values: 'table', 'table-verbose', 'json', 'gob' or 'gotemplate='. Use '--output help' for more info", "notaformat"),
		},
		{
			testName:        "invalid format with format prefix",
			outputSlice:     []string{"format:notaformat2"},
			expectedPrinter: config.PrinterConfig{},
			expectedError:   fmt.Errorf("unrecognized output format: %s. Valid format values: 'table', 'table-verbose', 'json', 'gob' or 'gotemplate='. Use '--output help' for more info", "notaformat2"),
		},
		{
			testName:    "default",
			outputSlice: []string{},
			expectedPrinter: config.PrinterConfig{
				Kind:    "table",
				OutFile: os.Stdout,
			},
			expectedError: nil,
		},
		{
			testName:    "format: json",
			outputSlice: []string{"format:json"},
			expectedPrinter: config.PrinterConfig{
				Kind:    "json",
				OutFile: os.Stdout,
			},
			expectedError: nil,
		},
		{
			testName:    "option relative timestamp",
			outputSlice: []string{"option:relative-time"},
			expectedPrinter: config.PrinterConfig{
				Kind:       "table",
				OutFile:    os.Stdout,
				RelativeTS: true,
			},
			expectedError: nil,
		},
	}
	for _, testcase := range testCases {
		t.Run(testcase.testName, func(t *testing.T) {
			outputConfig, err := flags.TrackerEbpfPrepareOutput(testcase.outputSlice, false)
			if err != nil {
				assert.ErrorContains(t, err, testcase.expectedError.Error())
			} else {
				assert.Equal(t, testcase.expectedPrinter, outputConfig.PrinterConfigs[0])
			}
		})
	}
}
