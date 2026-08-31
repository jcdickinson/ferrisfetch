package cmd

import (
	"reflect"
	"testing"
)

func TestNewSearchRequest(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		filters      []string
		wantQuery    string
		wantCrates   []string
		wantVersions map[string]string
	}{
		{
			name:      "query only",
			args:      []string{"linear sampler"},
			wantQuery: "linear sampler",
		},
		{
			name:         "positional crate version",
			args:         []string{"bevy_image@0.19.0", "linear sampler"},
			wantQuery:    "linear sampler",
			wantCrates:   []string{"bevy_image"},
			wantVersions: map[string]string{"bevy_image": "0.19.0"},
		},
		{
			name:         "versioned flag",
			args:         []string{"linear sampler"},
			filters:      []string{"bevy_image@0.19.0"},
			wantQuery:    "linear sampler",
			wantCrates:   []string{"bevy_image"},
			wantVersions: map[string]string{"bevy_image": "0.19.0"},
		},
		{
			name:       "latest is unpinned",
			args:       []string{"bevy_image@latest", "linear sampler"},
			wantQuery:  "linear sampler",
			wantCrates: []string{"bevy_image"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newSearchRequest(tt.args, tt.filters, 7)
			if got.Query != tt.wantQuery {
				t.Errorf("query = %q, want %q", got.Query, tt.wantQuery)
			}
			if !reflect.DeepEqual(got.Crates, tt.wantCrates) {
				t.Errorf("crates = %v, want %v", got.Crates, tt.wantCrates)
			}
			if !reflect.DeepEqual(got.CrateVersions, tt.wantVersions) {
				t.Errorf("crate versions = %v, want %v", got.CrateVersions, tt.wantVersions)
			}
			if got.Limit != 7 {
				t.Errorf("limit = %d, want 7", got.Limit)
			}
		})
	}
}
