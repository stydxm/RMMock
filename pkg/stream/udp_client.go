package stream

import (
	"encoding/binary"
	"github.com/sirupsen/logrus"
	"net"
)

func GetUDPConn() *net.UDPConn {
	serverAddr, err := net.ResolveUDPAddr("udp", "0.0.0.0:3334")
	if err != nil {
		logrus.Fatalf("无法解析服务器地址: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		logrus.Fatalf("无法连接到UDP服务器: %v", err)
	}
	return conn
}

func PacketFactory(frameID, sliceID uint16, sliceData []byte) []byte {
	packet := make([]byte, 8+len(sliceData))
	sliceSize := uint32(len(sliceData))

	binary.LittleEndian.PutUint16(packet[0:2], frameID)
	binary.LittleEndian.PutUint16(packet[2:4], sliceID)
	binary.LittleEndian.PutUint32(packet[4:8], sliceSize)
	copy(packet[8:], sliceData)

	return packet
}

func SendPacket(conn *net.UDPConn, encodedData []byte, frameID uint16) {
	// 计算需要多少个切片
	packetSize := 1000 // UDP包最大推荐大小，留出头部空间
	totalSlices := len(encodedData) / packetSize
	if len(encodedData)%packetSize != 0 {
		totalSlices++
	}

	// 发送每个切片
	for sliceID := uint16(0); sliceID < uint16(totalSlices); sliceID++ {
		start := int(sliceID) * packetSize
		end := start + packetSize
		if end > len(encodedData) {
			end = len(encodedData)
		}

		// 通过UDP发送数据包到服务器
		packet := PacketFactory(frameID, sliceID, encodedData[start:end])
		_, err := conn.Write(packet)
		if err != nil {
			logrus.Errorf("发送切片失败: %v", err)
			continue
		}
		logrus.Debugf("发送切片%d", sliceID)
	}
}
