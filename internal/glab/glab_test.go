package glab

import "testing"

func TestCheckInstalled(t *testing.T) {
	c := New()
	err := c.CheckInstalled()
	if err != nil {
		t.Skip("glab not installed, skipping")
	}
}
