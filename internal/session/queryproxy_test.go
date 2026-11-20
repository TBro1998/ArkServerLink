package session

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// infoReply 拼一个 A2S_INFO 回复，EDF 带游戏端口和 SteamID。
func infoReply(gamePort uint16) []byte {
	var b bytes.Buffer
	b.Write([]byte{0xff, 0xff, 0xff, 0xff, 'I', 17})
	for _, s := range []string{"测试服务器 - (v358.24)", "TheIsland", "ark_survival_evolved", "ARK: Survival Evolved"} {
		b.WriteString(s)
		b.WriteByte(0)
	}
	b.Write([]byte{0, 0, 3, 70, 0, 'd', 'w', 0, 1})
	b.WriteString("1.0.0.0")
	b.WriteByte(0)
	b.WriteByte(0x80 | 0x10 | 0x20)
	binary.Write(&b, binary.LittleEndian, gamePort)
	binary.Write(&b, binary.LittleEndian, uint64(90000000000000000))
	b.WriteString(",OWNINGID:1")
	b.WriteByte(0)
	return b.Bytes()
}

func TestRewriteInfoPort(t *testing.T) {
	got := rewriteInfoPort(infoReply(7777), 50000)
	if want := infoReply(50000); !bytes.Equal(got, want) {
		t.Fatalf("got %x\nwant %x", got, want)
	}

	// 不是 A2S_INFO 回复（challenge、规则）或者截断的报文原样返回
	for _, b := range [][]byte{
		{0xff, 0xff, 0xff, 0xff, 'A', 1, 2, 3, 4},
		{0xff, 0xff, 0xff, 0xff, 'E', 0, 0},
		infoReply(7777)[:20],
		{0xff, 0xff, 0xff, 0xff, 'I'},
	} {
		orig := bytes.Clone(b)
		if got := rewriteInfoPort(b, 50000); !bytes.Equal(got, orig) {
			t.Errorf("%x 被改成 %x", orig, got)
		}
	}

	// EDF 不带游戏端口时不改
	b := infoReply(7777)
	i := bytes.Index(b, []byte("1.0.0.0\x00")) + 8
	b[i] = 0x10
	orig := bytes.Clone(b)
	if got := rewriteInfoPort(b, 50000); !bytes.Equal(got, orig) {
		t.Error("EDF 不带端口时不应修改")
	}
}

// 代理把查询转给上游，回复里的游戏端口改成本机游戏端口，其他报文原样转发
func TestQueryProxy(t *testing.T) {
	up, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := up.ReadFromUDP(buf)
			if err != nil {
				return
			}
			switch string(buf[4:n]) {
			case "TSource Engine Query\x00":
				up.WriteToUDP([]byte{0xff, 0xff, 0xff, 0xff, 'A', 1, 2, 3, 4}, from)
			case "TSource Engine Query\x00\x01\x02\x03\x04":
				up.WriteToUDP(infoReply(7777), from)
			}
		}
	}()

	listen, err := randomUDPPort()
	if err != nil {
		t.Fatal(err)
	}
	p, err := startQueryProxy(listen, up.LocalAddr().(*net.UDPAddr).Port, 50000)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: listen})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	query := append([]byte{0xff, 0xff, 0xff, 0xff}, "TSource Engine Query\x00"...)
	ask := func(req []byte) []byte {
		t.Helper()
		c.Write(req)
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 1500)
		n, err := c.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		return buf[:n]
	}
	challenge := ask(query)
	if !bytes.Equal(challenge, []byte{0xff, 0xff, 0xff, 0xff, 'A', 1, 2, 3, 4}) {
		t.Fatalf("challenge = %x", challenge)
	}
	if got := ask(append(query, challenge[5:]...)); !bytes.Equal(got, infoReply(50000)) {
		t.Fatalf("reply = %x", got)
	}

	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}
