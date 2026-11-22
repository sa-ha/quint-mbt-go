package itf

import (
	"strings"
	"testing"
	"time"
)

func nestedTags(depth int) string {
	in := `"x"`
	for range depth {
		in = `{"tag": ` + in + `, "value": 1}`
	}
	return `{"states": [{"v": ` + in + `}]}`
}

func TestStringIsNotExponentialInDepth(t *testing.T) {
	tr, err := Decode(strings.NewReader(nestedTags(64)))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = tr.States[0].String()
		_ = Diff("v", tr.States[0], Record(nil))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("String or Diff did not finish within 10s at depth 64")
	}
}
