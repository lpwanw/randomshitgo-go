package state

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("os/signal.loop"),
		goleak.IgnoreTopFunction("testing.(*M).Run"),
	)
}
