package tun

import (
	"context"
	"fmt"
	"sync"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func (h *tunHandler) NewPacketConnectionEx(
	ctx context.Context, conn N.PacketConn,
	source M.Socksaddr, destination M.Socksaddr,
	onClose N.CloseHandlerFunc,
) {
	isDNS := destination.Port == 53
	if isDNS {
		h.logger.Debug(fmt.Sprintf("[dns] %v -> %v", source, destination))
	} else {
		h.logger.Debug(fmt.Sprintf("[udp] %v -> %v | direct", source, destination))
	}

	if isDNS {
		buffer := buf.NewPacket()
		defer buffer.Release()
		buffer.Reset()
		_, err := conn.ReadPacket(buffer)
		if err != nil {
			conn.Close()
			return
		}
		payload := buffer.Bytes()
		if handled := h.tryFakeDNSResponse(conn, payload, destination); handled {
			conn.Close()
			return
		}
		// If destination is the TUN DNS (fake), redirect to a real upstream DNS
		dnsDest := destination
		if dnsDest.Addr == h.tunAddr || dnsDest.Addr == h.tunAddr.Next() {
			dnsDest = M.ParseSocksaddr("8.8.8.8:53")
		}
		outbound, err := h.directDialer.DialContext(ctx, "udp", dnsDest)
		if err != nil {
			h.logger.Error(fmt.Sprintf("[udp] dial %v: %v", dnsDest, err))
			conn.Close()
			return
		}
		if _, err := outbound.Write(payload); err != nil {
			h.logger.Error(fmt.Sprintf("[udp] write first packet: %v", err))
			outbound.Close()
			conn.Close()
			return
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			defer outbound.Close()
			buf2 := buf.NewPacket()
			defer buf2.Release()
			for {
				buf2.Reset()
				_, err := conn.ReadPacket(buf2)
				if err != nil {
					return
				}
				_, err = outbound.Write(buf2.Bytes())
				if err != nil {
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			packet := make([]byte, 65535)
			for {
				n, err := outbound.Read(packet)
				if err != nil {
					return
				}
				h.parseDNSResponse(packet[:n])
				respBuf := buf.NewPacket()
				respBuf.Write(packet[:n])
				conn.WritePacket(respBuf, destination)
			}
		}()
		wg.Wait()
		conn.Close()
		return
	}

	outbound, err := h.directDialer.DialContext(ctx, "udp", destination)
	if err != nil {
		h.logger.Error(fmt.Sprintf("[udp] dial %v: %v", destination, err))
		conn.Close()
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		defer outbound.Close()
		buffer := buf.NewPacket()
		defer buffer.Release()
		for {
			buffer.Reset()
			_, err := conn.ReadPacket(buffer)
			if err != nil {
				return
			}
			_, err = outbound.Write(buffer.Bytes())
			if err != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		packet := make([]byte, 65535)
		for {
			n, err := outbound.Read(packet)
			if err != nil {
				return
			}
			if isDNS {
				h.parseDNSResponse(packet[:n])
			}
			buffer := buf.NewPacket()
			buffer.Write(packet[:n])
			conn.WritePacket(buffer, destination)
		}
	}()

	wg.Wait()
	conn.Close()
}
