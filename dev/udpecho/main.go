// udpecho 在给定 UDP 端口上回显，联调时充当游戏服务器。
package main

import (
	"log"
	"net"
	"os"
)

func main() {
	port := "7777"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}
	c, err := net.ListenPacket("udp", ":"+port)
	if err != nil {
		log.Fatal(err)
	}
	host, _ := os.Hostname()
	buf := make([]byte, 2048)
	for {
		n, addr, err := c.ReadFrom(buf)
		if err != nil {
			continue
		}
		c.WriteTo(append([]byte("pong from "+host+": "), buf[:n]...), addr)
	}
}
