package nspeed

import "testing"

// CommandOrder drives "nspeed -h": it must list every command exactly once, each with a description
func TestCommandOrder(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range CommandOrder {
		if _, ok := Commands[c]; !ok {
			t.Errorf("CommandOrder: %q is not a command", c)
		}
		if seen[c] {
			t.Errorf("CommandOrder: %q is listed twice", c)
		}
		seen[c] = true
		if CommandDescriptions[c] == "" {
			t.Errorf("command %q has no description", c)
		}
	}
	for c := range Commands {
		if !seen[c] {
			t.Errorf("command %q is missing from CommandOrder", c)
		}
	}
}
