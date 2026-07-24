package evidence

import "testing"

func TestStateValid(t *testing.T) {
	tests := []struct {
		state State
		want  bool
	}{
		{Observed, true},
		{Inferred, true},
		{Claimed, true},
		{Unknown, true},
		{"", false},
		{"proven", false},
	}

	for _, test := range tests {
		if got := test.state.Valid(); got != test.want {
			t.Errorf("State(%q).Valid() = %v, want %v", test.state, got, test.want)
		}
	}
}
