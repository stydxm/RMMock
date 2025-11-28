# -*- coding: utf-8 -*-

import socket
import struct
import os
import time
import sys
import subprocess

# ================= ⚙️ 配置区域 =================
UDP_IP = "0.0.0.0"
UDP_PORT = 3334
VIDEO_FILENAME = "video_record.hevc"

# 申请 20MB 缓冲区
REQUEST_BUF_SIZE = 20 * 1024 * 1024 
# ===============================================

def get_nal_type(payload):
    start_offset = -1
    if payload.startswith(b'\x00\x00\x00\x01'):
        start_offset = 4
    elif payload.startswith(b'\x00\x00\x01'):
        start_offset = 3
    if start_offset == -1 or len(payload) <= start_offset: return -1
    return (payload[start_offset] >> 1) & 0x3F

def main():
    # 1. 设置网络
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, REQUEST_BUF_SIZE)
    actual_buf = sock.getsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF)
    
    print(f"\n✅ 接收端启动 (Live View)")
    print(f"   Buffer: {actual_buf/1024/1024:.2f} MB")
    
    if actual_buf < 5 * 1024 * 1024:
        print("⚠️ 警告: 缓冲区过小，请执行 sudo sysctl 命令！")

    try:
        sock.bind((UDP_IP, UDP_PORT))
    except OSError:
        print(f"❌ 端口 {UDP_PORT} 被占用")
        return

    f_video = open(VIDEO_FILENAME, 'wb')

    # 2. 启动 ffplay 子进程 (管道模式)
    # 我们把 Python 收到的数据，直接塞进 ffplay 的嘴里
    ffplay_cmd = [
        'ffplay',
        '-window_title', 'Real-time Robot Stream', # 窗口标题
        '-f', 'hevc',           # 强制 H.265 格式
        '-fflags', 'nobuffer',  # 关闭输入缓冲 (低延迟关键)
        '-flags', 'low_delay',  # 低延迟标志
        '-probesize', '32',     # 极速探测
        '-analyzeduration', '0',
        '-sync', 'ext',         # 外部时钟同步
        '-i', '-'               # 从标准输入读取
    ]
    
    print("📺 正在启动播放器窗口...")
    try:
        # stdin=subprocess.PIPE 允许我们写入数据
        player = subprocess.Popen(ffplay_cmd, stdin=subprocess.PIPE, stderr=subprocess.DEVNULL)
    except FileNotFoundError:
        print("❌ 未找到 ffplay，无法播放。请安装 ffmpeg。")
        player = None

    # 状态变量
    current_frame_id = -1
    current_shards = {}
    
    # 核心：必须等到 IDR 才开始往管道里塞数据，否则一开始就是花的
    stream_ready = False 
    
    # 缓存参数头 (VPS/SPS/PPS)
    headers_cache = [] 

    stats_total = 0
    stats_ok = 0
    start_time = time.time()
    last_log = time.time()

    print(f"🚀 正在接收数据流... (按 Ctrl+C 停止)")

    try:
        while True:
            data, _ = sock.recvfrom(65535)
            if len(data) <= 8: continue

            header = data[:8]
            # 解析头
            frame_id, shard_id, _ = struct.unpack('>HHI', header)
            payload = data[8:]

            if frame_id != current_frame_id:
                # 结算上一帧
                if current_frame_id != -1 and len(current_shards) > 0:
                    stats_total += 1
                    
                    # 完整性检查
                    indices = sorted(current_shards.keys())
                    is_complete = (indices[0] == 0) and ((indices[-1] - indices[0] + 1) == len(indices))

                    if is_complete:
                        full_frame = b''.join([current_shards[k] for k in indices])
                        nal_type = get_nal_type(full_frame)

                        # --- 逻辑处理 ---
                        is_param = nal_type in [32, 33, 34]
                        is_idr = nal_type in [19, 20, 21]

                        # 1. 永远缓存最新的参数包
                        if is_param:
                            headers_cache.append(full_frame)
                            # 限制缓存大小，防止无限增长
                            if len(headers_cache) > 10: headers_cache.pop(0)

                        # 2. 门卫：遇到 IDR 才开门
                        # if is_idr:
                        #     stream_ready = True
                        if len(headers_cache) > 0:
                            stream_ready = True
                        
                        # 3. 数据分发
                        if stream_ready or is_param:
                            # 写入文件
                            f_video.write(full_frame)
                            stats_ok += 1
                            
                            # 写入播放器 (如果活着)
                            if player and player.poll() is None:
                                try:
                                    # 如果是 IDR，先把缓存的 SPS/PPS 塞进去，防止播放器忘记参数
                                    if is_idr:
                                        for h in headers_cache:
                                            player.stdin.write(h)
                                    
                                    player.stdin.write(full_frame)
                                    player.stdin.flush()
                                except BrokenPipeError:
                                    print("\n⚠️ 播放器窗口已关闭")
                                    player = None

                # 打印日志 (每秒)
                now = time.time()
                if now - last_log >= 1.0:
                    loss = (1 - stats_ok/stats_total)*100 if stats_total > 0 else 0
                    sys.stdout.write(f"\r⏱️ {now-start_time:.0f}s | 帧数:{stats_total} | 完整:{stats_ok} | 丢包:{loss:.2f}% | 状态:{'播放中' if stream_ready else '等待IDR'} ")
                    sys.stdout.flush()
                    last_log = now

                current_frame_id = frame_id
                current_shards = {}

            current_shards[shard_id] = payload

    except KeyboardInterrupt:
        print("\n🛑 停止")
    finally:
        f_video.close()
        sock.close()
        if player:
            player.stdin.close()
            player.terminate()

if __name__ == "__main__":
    main()
