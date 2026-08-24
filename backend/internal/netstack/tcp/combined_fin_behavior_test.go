package tcp_test

import (
	"errors"
	"io"
	"net/netip"
	"testing"
	"time"

	"gostack/internal/netstack/tcp"
)

type combinedFINHost struct{}

func (combinedFINHost) SendTCP(netip.Addr, netip.Addr, tcp.Header, []byte) error {
	return nil
}

func (combinedFINHost) Remove(tcp.Quad)        {}
func (combinedFINHost) Now() time.Time         { return time.Now() }
func (combinedFINHost) MSL() time.Duration     { return time.Second }
func (combinedFINHost) MaxRetries() int        { return 3 }
func (combinedFINHost) RTOMin() time.Duration  { return 10 * time.Millisecond }
func (combinedFINHost) Emit(tcp.Event)         {}

func TestCombinedDataFINReturnsEOF(t *testing.T) {
	const (
		localISN = uint32(1000)
		peerISN  = uint32(2000)
	)
	q := tcp.Quad{
		LocalIP:    netip.MustParseAddr("10.0.0.2"),
		LocalPort:  9000,
		RemoteIP:   netip.MustParseAddr("10.0.0.1"),
		RemotePort: 40000,
	}
	c := tcp.NewConn(combinedFINHost{}, q, 1460, 4096, 4096)
	c.StartActive(localISN)
	c.Handle(&tcp.Header{
		Seq:    peerISN,
		Ack:    localISN + 1,
		Flags:  tcp.FlagSYN | tcp.FlagACK,
		Window: 4096,
	}, nil)
	if err := c.WaitEstablished(time.Second); err != nil {
		t.Fatalf("establish connection: %v", err)
	}

	payload := []byte("final response")
	finSeq := peerISN + 1 + uint32(len(payload))
	defer func() {
		c.Handle(&tcp.Header{Seq: finSeq, Flags: tcp.FlagRST}, nil)
		c.Handle(&tcp.Header{Seq: finSeq + 1, Flags: tcp.FlagRST}, nil)
	}()
	c.Handle(&tcp.Header{
		Seq:    peerISN + 1,
		Ack:    localISN + 1,
		Flags:  tcp.FlagACK | tcp.FlagPSH | tcp.FlagFIN,
		Window: 4096,
	}, payload)

	got := make([]byte, len(payload))
	if _, err := io.ReadFull(c, got); err != nil {
		t.Fatalf("read final payload: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("final payload = %q, want %q", got, payload)
	}
	if err := c.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, 1)
	n, err := c.Read(buf)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("read after final payload = (%d, %v), want (0, EOF)", n, err)
	}
}
