package main

import (
	"testing"
)

func TestShouldCheckForWFO(t *testing.T) {
	cfg := DefaultConfig()
	api := &APIClient{} // Not used by shouldCheckForWFO
	oum := NewOptUploadManager(cfg, api)

	tests := []struct {
		name      string
		filename  string
		expectWFO bool
	}{
		{"OPT file should not trigger WFO", "484d6b19-8a36-49c3-9297-6a97391c0e28_@ES_60_OPT_Results.opt", false},
		{"WFO file should trigger WFO", "484d6b19-8a36-49c3-9297-6a97391c0e28_@ES_60_WFO_Results.opt", true},
		{"WFM file should trigger WFO", "abc123_@NQ_240_WFM_Results.opt", true},
		{"DWFM file should trigger WFO", "xyz789_@CL_15_DWFM_Results.opt", true},
		{"Lowercase opt should not trigger WFO", "lowercase_@es_60_opt_results.opt", false},
		{"Mixed case WFO should trigger", "MIXED_case_@ES_60_WFO_Results.opt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := oum.shouldCheckForWFO(tt.filename)
			if got != tt.expectWFO {
				t.Fatalf("shouldCheckForWFO(%q) = %v; want %v", tt.filename, got, tt.expectWFO)
			}
		})
	}
}