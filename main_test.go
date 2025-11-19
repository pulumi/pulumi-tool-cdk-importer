package main

import (
	"testing"
)

func TestStringSlice_Set(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		initial  []string
		expected []string
	}{
		{
			name:     "single value",
			input:    "stack1",
			initial:  nil,
			expected: []string{"stack1"},
		},
		{
			name:     "comma separated",
			input:    "stack1,stack2",
			initial:  nil,
			expected: []string{"stack1", "stack2"},
		},
		{
			name:     "comma separated with spaces",
			input:    "stack1, stack2 , stack3",
			initial:  nil,
			expected: []string{"stack1", "stack2", "stack3"},
		},
		{
			name:     "append to existing",
			input:    "stack2",
			initial:  []string{"stack1"},
			expected: []string{"stack1", "stack2"},
		},
		{
			name:     "empty parts",
			input:    "stack1,,stack2",
			initial:  nil,
			expected: []string{"stack1", "stack2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := stringSlice(tt.initial)
			err := s.Set(tt.input)
			if err != nil {
				t.Errorf("Set() error = %v", err)
			}
			if len(s) != len(tt.expected) {
				t.Errorf("Set() length = %v, want %v", len(s), len(tt.expected))
				return
			}
			for i, v := range s {
				if v != tt.expected[i] {
					t.Errorf("Set() [%d] = %v, want %v", i, v, tt.expected[i])
				}
			}
		})
	}
}
