package tailcat_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/frandustry/FranTransport/pkg/transport"
	"github.com/frandustry/FranTransport/pkg/transport/contract"
	fttailcat "github.com/frandustry/FranTransport/pkg/transport/tailcat"
)

func TestContract(t *testing.T) {
	if os.Getenv("FRANTRANSPORT_TAILCAT_INTEGRATION") != "1" {
		t.Skip("set FRANTRANSPORT_TAILCAT_INTEGRATION=1 for public DERP integration test")
	}
	contract.TestTransport(t, func(t *testing.T) (transport.Transport, transport.Transport) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		a, err := fttailcat.New(ctx, fttailcat.Config{})
		if err != nil {
			t.Fatal(err)
		}
		b, err := fttailcat.New(ctx, fttailcat.Config{})
		if err != nil {
			_ = a.Close()
			t.Fatal(err)
		}
		return a, b
	})
}
