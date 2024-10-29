// Copyright 2024 TochusC AOSP Lab. All rights reserved.

// parser_test.go 定义了对 Parser 的相关测试函数。

package godns

import "testing"

// 待测试的数据包数据
var testedPacket = []byte{
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0x00, 0x11, 0x22, 0x33, 0x44, 0x55,
	0x08, 0x00, 0x45, 0x00,
	0x00, 0x5e, 0x00, 0x01, 0x00, 0x00, 0x40, 0x11,
	0x66, 0x76, 0x0a, 0x0a, 0x00, 0x03, 0x0a, 0x0a,
	0x00, 0x02, 0x00, 0x35, 0x63, 0xbf, 0x00, 0x4a,
	0x6f, 0x2c, 0x00, 0x00, 0x85, 0x20, 0x00, 0x01,
	0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x03, 0x77,
	0x77, 0x77, 0x07, 0x6b, 0x65, 0x79, 0x74, 0x72,
	0x61, 0x70, 0x04, 0x74, 0x65, 0x73, 0x74, 0x00,
	0x00, 0x2e, 0x00, 0x01, 0x03, 0x77, 0x77, 0x77,
	0x07, 0x6b, 0x65, 0x79, 0x74, 0x72, 0x61, 0x70,
	0x04, 0x74, 0x65, 0x73, 0x74, 0x00, 0x00, 0x01,
	0x00, 0x01, 0x00, 0x01, 0x51, 0x80, 0x00, 0x04,
	0x0a, 0x0a, 0x00, 0x03,
}
var testedPacket2 = []byte{
	// Ethernet Header 14 bytes
	0x02, 0x42, 0x0a, 0x0a, 0x03, 0x03, // Destination MAC
	0x02, 0x42, 0x0a, 0x0a, 0x03, 0x04, // Source MAC
	0x08, 0x00, // Ethernet Type: IPv4
	// IP Header 20 bytes
	0x45, 0x00, // Version, IHL, TOS
	0x00, 0x47, // Total Length
	0xc2, 0x6b, // Identification
	0x00, 0x00, // Flags, Fragment Offset
	0x40, 0x11, // TTL, Protocol
	0x9e, 0x20, // Header Checksum
	0x0a, 0x0a, 0x03, 0x04, // Source IP
	0x0a, 0x0a, 0x03, 0x03, // Destination IP
	// UDP Header 8 bytes
	0xde, 0xe3, // Source Port
	0x00, 0x35, // Destination Port
	0x00, 0x33, // Length
	0x1a, 0x5f, // Checksum
	// DNS Header 12 bytes
	0x47, 0xa0, // txID
	0x01, 0x20, // Flags
	0x00, 0x01, // QDCount
	0x00, 0x00, // ANCount
	0x00, 0x00, // NSCount
	0x00, 0x01, // ARCount
	// DNS Quetion Section
	0x02, 0x77, 0x65, 0x00, // Name
	0x00, 0x01, // Type
	0x00, 0x01, // Class
	// DNS Additional Section
	// DNS协议的扩展机制(EDNS0)，
	// 通过OPT伪记录来支持更多功能，比如DNSSEC、DNS COOKIE等
	0x00,       // 根域名"."
	0x00, 0x29, // TYPE: OPT
	0x04, 0xd0, // CLASS:
	0x00, 0x00, 0x00, 0x00, // TTL
	0x00, 0x0c, // RDLEN
	0x00, 0x0a, //OPTION-CODE
	0x00, 0x08, // OPTION-LENGTH
	0x5a, 0x4e, 0x3d, 0xcb, // CLIENT COOKIE
	0x31, 0x59, 0xf2, 0x8b, // SERVER COOKIE
}

func TestParse(t *testing.T) {
	qInfo, err := Parser{}.Parse(testedPacket)
	if err != nil {
		t.Errorf("解析数据包失败: %s", err)
		return
	}
	t.Logf("%s", qInfo.String())
}

func TestParse2(t *testing.T) {
	qInfo, err := Parser{}.Parse(testedPacket2)
	if err != nil {
		t.Errorf("解析数据包失败: %s", err)
		return
	}
	t.Logf("%s", qInfo.String())
}
