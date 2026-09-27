package mock_test

import (
	"testing"

	"github.com/frandustry/FranTransport/pkg/transport"
	"github.com/frandustry/FranTransport/pkg/transport/contract"
	"github.com/frandustry/FranTransport/pkg/transport/mock"
)

func TestContract(t *testing.T) {
	contract.TestTransport(t, func(t *testing.T) (transport.Transport, transport.Transport) {
		n := mock.NewNetwork()
		a, err := n.NewTransport("a")
		if err != nil {
			t.Fatal(err)
		}
		b, err := n.NewTransport("b")
		if err != nil {
			t.Fatal(err)
		}
		return a, b
	})
}
