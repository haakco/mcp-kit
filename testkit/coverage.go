package testkit

import "testing"

// AssertChecklistCoverage fails if any checklist item is not registered.
func AssertChecklistCoverage(t testing.TB, registered []string, checklist []string) {
	t.Helper()
	seen := map[string]struct{}{}
	for _, name := range registered {
		seen[name] = struct{}{}
	}
	for _, want := range checklist {
		if _, ok := seen[want]; !ok {
			t.Fatalf("checklist item %q is not registered; registered=%v", want, registered)
		}
	}
}
