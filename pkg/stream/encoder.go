package stream

import "gocv.io/x/gocv"

// EncoderConfig 编码器配置
type EncoderConfig struct {
	Width         int
	Height        int
	FPS           float64
	Bitrate       int    // kbps
	Preset        string // https://trac.ffmpeg.org/wiki/Encode/H.265#ConstantRateFactorCRF
	Tune          string // https://trac.ffmpeg.org/wiki/Encode/H.265#ConstantRateFactorCRF
	RepeatHeaders bool   // 是否在每帧前重复发送SPS/PPS头
}

type Encoder interface {
	EncodeFrame(frame gocv.Mat) ([]byte, error)
	Flush() ([]byte, error)
	Close() error
	GetHeaders() ([]byte, error)
}
