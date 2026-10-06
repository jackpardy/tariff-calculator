package competitions

import (
	"fmt"
	"testing"
)

func TestLookAlike(t *testing.T) {
	got := LookAlike(map[string]string{
		"i:dara o'néill": "Dara O'Néill", "i:dara oneill": "Dara ONeill", // accents and punctuation
		"m:1": "Siobhan Murphy", "i:siobhan murphey": "Siobhan Murphey", // one letter apart
		"m:2": "Ann Lee", "m:3": "Anne Lee", // too short to call
		"m:4": "Tom Kelly", "m:5": "Tim Kelly",
	})
	if fmt.Sprint(got) != "[[Dara O'Néill Dara ONeill] [Siobhan Murphey Siobhan Murphy]]" {
		t.Errorf("look alike: %v", got)
	}
}
