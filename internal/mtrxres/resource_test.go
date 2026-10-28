package mtrxres

import "testing"

func TestBaseRefName(t *testing.T) {
	tests := []struct {
		event    string
		baseRef  string
		refName  string
		expected string
	}{
		{"push", "", "main", "main"},
		{"pull_request", "main", "13140/merge", "main"},
		{"merge_group", "", "gh-readonly-queue/main/pr-13140-1cd33039dc2f8a7f1d6c2b07c0ab5e4c8fd1e3a2", "main"},
		{"merge_group", "", "gh-readonly-queue/release/v8.368.0/pr-13131-7b56d377763a1f0b6e1e0c3c2d8a9b4f5e6d7c8b", "release/v8.368.0"},
		{"workflow_dispatch", "", "emo/ci-mtrix-grafana", "emo/ci-mtrix-grafana"},
		{"local", "", "", ""},
	}
	for _, test := range tests {
		t.Setenv("GITHUB_BASE_REF", test.baseRef)
		t.Setenv("GITHUB_REF_NAME", test.refName)
		if got := baseRefName(); got != test.expected {
			t.Errorf("%v %q: got %q, expected %q", test.event, test.refName, got, test.expected)
		}
	}
}
