package main

import (
	"errors"
	"strings"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var (
	ERR_LOST_NETLAYER     = errors.New("Lost net layer")
	ERR_NOT_TCP_PROTOCOL  = errors.New("Not Tcp protocol")
	ERR_NOT_IPV4          = errors.New("Not Ipv4 protocol")
	ERR_NOT_EQUAL_SRCIP   = errors.New("Not equal src ip")
	ERR_NOT_EQUAL_DSTIP   = errors.New("Not equal dst ip")
	ERR_NOT_EQUAL_SRCPORT = errors.New("Not euqal src port")
	ERR_NOT_EQUAL_DSTPORT = errors.New("Not equal dst port")
	ERR_TCP_LEN_TOO_SHORT = errors.New("Tcp data len is too short")
)

func GetTcpData(pkg gopacket.Packet, srcIp, dstIp []string, srcPort, dstPort []uint16, minLen uint16) ([]byte, bool, *layers.IPv4, *layers.TCP, error) {
	netLayer := pkg.NetworkLayer()
	if netLayer == nil {
		return []byte{}, false, nil, nil, ERR_LOST_NETLAYER
	}
	if netLayer.LayerType() != layers.LayerTypeIPv4 {
		return []byte{}, false, nil, nil, ERR_NOT_IPV4
	}
	// 解析IPV4头
	ipv4 := &layers.IPv4{}
	err := ipv4.DecodeFromBytes(netLayer.LayerContents(), gopacket.NilDecodeFeedback)
	if err != nil {
		return []byte{}, false, ipv4, nil, err
	}
	if ipv4.Protocol != layers.IPProtocolTCP {
		return []byte{}, false, ipv4, nil, ERR_NOT_TCP_PROTOCOL
	}
	// 匹配IP地址
	if len(srcIp) > 0 {
		found := false
		for _, ip := range srcIp {
			if strings.Contains(ipv4.SrcIP.String(), ip) {
				found = true
				break
			}
		}
		if !found {
			return []byte{}, false, ipv4, nil, ERR_NOT_EQUAL_SRCIP
		}
	}
	if len(dstIp) > 0 {
		found := false
		for _, ip := range dstIp {
			if strings.Contains(ipv4.DstIP.String(), ip) {
				found = true
				break
			}
		}
		if !found {
			return []byte{}, false, ipv4, nil, ERR_NOT_EQUAL_DSTIP
		}
	}
	// 解析TCP头
	tcp := &layers.TCP{}
	err = tcp.DecodeFromBytes(netLayer.LayerPayload(), gopacket.NilDecodeFeedback)
	if err != nil {
		return []byte{}, false, ipv4, tcp, err
	}
	// 匹配端口
	if len(srcPort) > 0 {
		found := false
		for _, p := range srcPort {
			if p > 0 && p < 65535 {
				if tcp.SrcPort == layers.TCPPort(p) {
					found = true
					break
				}
			}
		}
		if !found {
			return []byte{}, false, ipv4, tcp, ERR_NOT_EQUAL_SRCPORT
		}
	}
	if len(dstPort) > 0 {
		found := false
		for _, p := range dstPort {
			if p > 0 && p < 65535 {
				if tcp.DstPort == layers.TCPPort(p) {
					found = true
					break
				}
			}
		}
		if !found {
			return []byte{}, false, ipv4, tcp, ERR_NOT_EQUAL_DSTPORT
		}
	}

	tcpLen := ipv4.Length - 52
	if minLen > 0 {
		if tcpLen < minLen {
			return []byte{}, false, ipv4, tcp, ERR_TCP_LEN_TOO_SHORT
		}
	}
	return tcp.Payload, true, ipv4, tcp, nil
}
