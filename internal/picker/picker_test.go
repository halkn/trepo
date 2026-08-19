package picker_test

import (
	"errors"
	"testing"

	"github.com/halkn/trepo/internal/picker"
)

func TestPickWithNoRowsReturnsNothing(t *testing.T) {
	got, err := picker.Picker{}.Pick(nil)
	if err != nil || got != nil {
		t.Errorf("Pick(nil) = %v, %v; want nil, nil", got, err)
	}
}

// A machine without fzf must get a clear answer rather than a failed exec, so
// the caller can fall back to printing the candidates.
func TestPickWithoutFzfSaysSo(t *testing.T) {
	if picker.Available() {
		t.Skip("fzf is installed here")
	}
	_, err := picker.Picker{}.Pick([]picker.Row{{Display: []string{"a"}, Key: "/a"}})
	if !errors.Is(err, picker.ErrUnavailable) {
		t.Errorf("Pick() = %v, want ErrUnavailable", err)
	}
}
