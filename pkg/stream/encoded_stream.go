package stream

import (
	"github.com/sirupsen/logrus"
	"gocv.io/x/gocv"
	"sync"
)

// StartEncodedStream 启动带HEVC编码的视频流
func StartEncodedStream[T interface{ int | string }](source T, wg *sync.WaitGroup) {
	defer wg.Done()

	conn := GetUDPConn()
	logrus.Info("成功连接到UDP服务器")
	defer conn.Close()

	stream := GetOpencvVideoStream(source)
	defer stream.Close()
	streamParam := GetOpenCVCaptureParam(stream)
	logrus.Debugf("视频分辨率: %dx%d, FPS: %.2f", streamParam.frameWidth, streamParam.frameHeight, streamParam.fps)

	// 创建HEVC编码器
	encoder, err := FFmpegEncoderFactory(EncoderConfig{
		Width:         streamParam.frameWidth,
		Height:        streamParam.frameHeight,
		FPS:           streamParam.fps,
		Bitrate:       2000,
		Preset:        "veryfast",
		Tune:          "zerolatency",
		RepeatHeaders: true,
	})
	if err != nil {
		logrus.Fatalf("无法创建编码器: %v", err)
	}
	defer encoder.Close()

	// 获取并发送SPS/PPS头
	headers, err := encoder.GetHeaders()
	if err != nil {
		logrus.Warnf("无法获取编码器头: %v", err)
	} else if len(headers) > 0 {
		logrus.Debugf("发送编码器头 (%d 字节)", len(headers))
	}

	// 创建Mat对象用于存储帧
	frame := gocv.NewMat()
	defer frame.Close()

	frameID := uint16(0)
	for {
		if ok := stream.Read(&frame); !ok {
			logrus.Error("无法读取摄像头帧")
			break
		}

		if frame.Empty() {
			logrus.Warn("空帧")
			continue
		}

		// 编码帧
		encodedData, err := encoder.EncodeFrame(frame)
		if err != nil {
			logrus.Fatalf("编码失败: %v", err)
			continue
		}

		// 如果编码器缓冲中，跳过
		if len(encodedData) == 0 {
			continue
		}

		logrus.Debugf("帧 %d 编码完成，大小: %d 字节（原始: %d 字节）",
			frameID, len(encodedData), len(frame.ToBytes()))

		SendPacket(conn, encodedData, frameID)
		frameID = (frameID + 1) % 65535

		window := gocv.NewWindow("Encoded Camera Feed")
		window.IMShow(frame)
		if window.WaitKey(1) >= 0 {
			break
		}
	}

	// 刷新编码器
	logrus.Debug("刷新编码器缓冲区")
	flushedData, _ := encoder.Flush()
	if len(flushedData) > 0 {
		conn.Write(flushedData)
		logrus.Debug("发送刷新数据: %d 字节", len(flushedData))
	}
	logrus.Debug("编码流传输完成")
}
