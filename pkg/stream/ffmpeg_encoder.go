package stream

/*
#cgo pkg-config: libavcodec libavutil libswscale
#include <libavcodec/avcodec.h>
#include <libavutil/opt.h>
#include <libavutil/imgutils.h>
#include <libswscale/swscale.h>
#include <stdlib.h>
#include <string.h>

// 辅助函数来处理AVERROR宏
static inline int get_averror_eagain() {
    return AVERROR(EAGAIN);
}

static inline int get_averror_eof() {
    return AVERROR_EOF;
}
*/
import "C"
import (
	"fmt"
	"github.com/sirupsen/logrus"
	"gocv.io/x/gocv"
	"image"
	"unsafe"
)

// FFmpegEncoder 使用FFmpeg libavcodec的HEVC编码器
type FFmpegEncoder struct {
	config     EncoderConfig
	codec      *C.AVCodec
	codecCtx   *C.AVCodecContext
	frame      *C.AVFrame
	packet     *C.AVPacket
	swsCtx     *C.struct_SwsContext
	frameCount int64
	headers    []byte // SPS/PPS头
	gopSize    int    // GOP大小（关键帧间隔）
}

func FFmpegEncoderFactory(config EncoderConfig) (*FFmpegEncoder, error) {
	encoder := &FFmpegEncoder{
		config:  config,
		gopSize: 30,
	}

	codecName := C.CString("libx265")
	defer C.free(unsafe.Pointer(codecName))

	encoder.codec = C.avcodec_find_encoder_by_name(codecName)
	if encoder.codec == nil {
		return nil, fmt.Errorf("无法找到libx265编码器")
	}

	// 分配编码器上下文
	encoder.codecCtx = C.avcodec_alloc_context3(encoder.codec)
	if encoder.codecCtx == nil {
		return nil, fmt.Errorf("无法分配编码器上下文")
	}

	// 设置编码参数
	encoder.codecCtx.width = C.int(config.Width)
	encoder.codecCtx.height = C.int(config.Height)
	encoder.codecCtx.time_base = C.AVRational{num: 1, den: C.int(config.FPS)}
	encoder.codecCtx.framerate = C.AVRational{num: C.int(config.FPS), den: 1}
	encoder.codecCtx.pix_fmt = C.AV_PIX_FMT_YUV420P
	encoder.codecCtx.gop_size = C.int(encoder.gopSize)
	encoder.codecCtx.max_b_frames = 0

	if config.Bitrate > 0 {
		encoder.codecCtx.bit_rate = C.int64_t(config.Bitrate * 1000) // 转换为 bps
	}

	// 设置编码器预设和调优
	preset := config.Preset
	if preset == "" {
		preset = "medium"
	}
	tune := config.Tune
	if tune == "" {
		tune = "zerolatency"
	}
	defer C.free(unsafe.Pointer(C.CString(preset)))
	defer C.free(unsafe.Pointer(C.CString(tune)))

	C.av_opt_set(unsafe.Pointer(encoder.codecCtx.priv_data), C.CString("preset"), C.CString(preset), 0)
	C.av_opt_set(unsafe.Pointer(encoder.codecCtx.priv_data), C.CString("tune"), C.CString(tune), 0)

	// 添加全局头标志（用于流传输）
	encoder.codecCtx.flags |= C.AV_CODEC_FLAG_GLOBAL_HEADER

	// 打开编码器
	if C.avcodec_open2(encoder.codecCtx, encoder.codec, nil) < 0 {
		C.avcodec_free_context(&encoder.codecCtx)
		return nil, fmt.Errorf("无法打开编码器")
	}

	// 分配帧
	encoder.frame = C.av_frame_alloc()
	if encoder.frame == nil {
		C.avcodec_free_context(&encoder.codecCtx)
		return nil, fmt.Errorf("无法分配帧")
	}

	encoder.frame.format = C.int(encoder.codecCtx.pix_fmt)
	encoder.frame.width = encoder.codecCtx.width
	encoder.frame.height = encoder.codecCtx.height

	// 分配帧缓冲区
	if C.av_frame_get_buffer(encoder.frame, 0) < 0 {
		C.av_frame_free(&encoder.frame)
		C.avcodec_free_context(&encoder.codecCtx)
		return nil, fmt.Errorf("无法分配帧缓冲区")
	}

	// 分配数据包
	encoder.packet = C.av_packet_alloc()
	if encoder.packet == nil {
		C.av_frame_free(&encoder.frame)
		C.avcodec_free_context(&encoder.codecCtx)
		return nil, fmt.Errorf("无法分配数据包")
	}

	// 初始化swscale上下文（用于BGR到YUV转换）
	encoder.swsCtx = C.sws_getContext(
		encoder.codecCtx.width,
		encoder.codecCtx.height,
		C.AV_PIX_FMT_BGR24,
		encoder.codecCtx.width,
		encoder.codecCtx.height,
		C.AV_PIX_FMT_YUV420P,
		C.SWS_BILINEAR,
		nil, nil, nil,
	)
	if encoder.swsCtx == nil {
		C.av_packet_free(&encoder.packet)
		C.av_frame_free(&encoder.frame)
		C.avcodec_free_context(&encoder.codecCtx)
		return nil, fmt.Errorf("无法创建swscale上下文")
	}

	logrus.Debugf("FFmpeg编码器初始化成功: %dx%d @ %.2f fps, bitrate: %d kbps, preset: %s, tune: %s",
		config.Width, config.Height, config.FPS, config.Bitrate, preset, tune)

	if config.RepeatHeaders {
		if encoder.codecCtx.extradata_size > 0 {
			encoder.headers = C.GoBytes(unsafe.Pointer(encoder.codecCtx.extradata), encoder.codecCtx.extradata_size)
			logrus.Debugf("成功缓存编码器头信息，大小: %d 字节", len(encoder.headers))
		}
	}

	return encoder, nil
}

// EncodeFrame 编码单个帧并返回HEVC NAL单元
func (e *FFmpegEncoder) EncodeFrame(frame gocv.Mat) ([]byte, error) {
	if frame.Empty() {
		return nil, fmt.Errorf("空帧无法编码")
	}

	// 调整帧大小（如果需要）
	if frame.Cols() != e.config.Width || frame.Rows() != e.config.Height {
		resized := gocv.NewMat()
		defer resized.Close()
		gocv.Resize(frame, &resized, image.Point{X: e.config.Width, Y: e.config.Height}, 0, 0, gocv.InterpolationLinear)
		frame = resized
	}

	// 获取BGR数据
	bgrData := frame.ToBytes()

	// 确保帧可写
	if C.av_frame_make_writable(e.frame) < 0 {
		return nil, fmt.Errorf("无法使帧可写")
	}

	// 将BGR数据复制到C内存（避免cgo指针规则问题）
	cBgrData := C.CBytes(bgrData)
	defer C.free(cBgrData)

	// 在C内存中创建指针数组和linesize数组
	srcData := (**C.uint8_t)(C.malloc(C.size_t(unsafe.Sizeof(uintptr(0))) * 4))
	defer C.free(unsafe.Pointer(srcData))
	srcLinesize := (*C.int)(C.malloc(C.size_t(unsafe.Sizeof(C.int(0))) * 4))
	defer C.free(unsafe.Pointer(srcLinesize))

	// 设置第一个平面的数据和linesize
	*srcData = (*C.uint8_t)(cBgrData)
	*srcLinesize = C.int(e.config.Width * 3)

	// BGR转YUV420P
	C.sws_scale(
		e.swsCtx,
		srcData,
		srcLinesize,
		0,
		e.codecCtx.height,
		(**C.uint8_t)(unsafe.Pointer(&e.frame.data[0])),
		(*C.int)(unsafe.Pointer(&e.frame.linesize[0])),
	)

	// 设置帧的PTS
	e.frame.pts = C.int64_t(e.frameCount)
	e.frameCount++

	// 发送帧到编码器
	ret := C.avcodec_send_frame(e.codecCtx, e.frame)
	if ret < 0 {
		return nil, fmt.Errorf("发送帧到编码器失败")
	}

	// 接收编码后的数据包
	ret = C.avcodec_receive_packet(e.codecCtx, e.packet)
	if ret == C.get_averror_eagain() || ret == C.get_averror_eof() {
		return []byte{}, nil // 需要更多数据或结束
	} else if ret < 0 {
		return nil, fmt.Errorf("接收数据包失败")
	}

	defer C.av_packet_unref(e.packet)

	// 复制编码后的数据
	encodedData := C.GoBytes(unsafe.Pointer(e.packet.data), e.packet.size)

	// 如果配置了重复发送headers，则在关键帧前添加headers
	if e.config.RepeatHeaders && len(e.headers) > 0 {
		// 检查是否为关键帧
		isKeyframe := (e.packet.flags & C.AV_PKT_FLAG_KEY) != 0

		if isKeyframe {
			// 将headers添加到编码数据前面
			result := make([]byte, 0, len(e.headers)+len(encodedData))
			result = append(result, e.headers...)
			result = append(result, encodedData...)
			return result, nil
		}
	}

	return encodedData, nil
}

// Flush 刷新编码器缓冲区
func (e *FFmpegEncoder) Flush() ([]byte, error) {
	var allData []byte

	// 发送NULL帧表示刷新
	C.avcodec_send_frame(e.codecCtx, nil)

	for {
		ret := C.avcodec_receive_packet(e.codecCtx, e.packet)
		if ret == C.get_averror_eof() || ret == C.get_averror_eagain() {
			break
		} else if ret < 0 {
			return nil, fmt.Errorf("刷新时接收数据包失败")
		}

		encodedData := C.GoBytes(unsafe.Pointer(e.packet.data), e.packet.size)
		allData = append(allData, encodedData...)
		C.av_packet_unref(e.packet)
	}

	return allData, nil
}

// Close 关闭编码器并释放资源
func (e *FFmpegEncoder) Close() error {
	if e.swsCtx != nil {
		C.sws_freeContext(e.swsCtx)
		e.swsCtx = nil
	}

	if e.packet != nil {
		C.av_packet_free(&e.packet)
		e.packet = nil
	}

	if e.frame != nil {
		C.av_frame_free(&e.frame)
		e.frame = nil
	}

	if e.codecCtx != nil {
		C.avcodec_free_context(&e.codecCtx)
		e.codecCtx = nil
	}

	logrus.Debug("FFmpeg编码器已关闭")
	return nil
}

// GetHeaders 获取SPS/PPS头信息（extradata）
func (e *FFmpegEncoder) GetHeaders() ([]byte, error) {
	if e.codecCtx.extradata_size > 0 {
		return C.GoBytes(unsafe.Pointer(e.codecCtx.extradata), e.codecCtx.extradata_size), nil
	}
	return []byte{}, nil
}
