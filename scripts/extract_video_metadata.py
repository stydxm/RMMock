import socket

def udp_listener(port, buffer_size=1024):
    """
    监听指定的 UDP 端口，接收数据包并打印相关信息。

    Args:
        port (int): 要监听的端口号。
        buffer_size (int): 接收缓冲区的大小（字节）。
    """
    # 创建一个 UDP socket
    try:
        # socket.AF_INET 表示使用 IPv4
        # socket.SOCK_DGRAM 表示使用 UDP
        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    except Exception as e:
        print(f"❌ 错误：创建 socket 失败: {e}")
        return

    # 绑定到指定的端口和所有可用的接口
    server_address = ('0.0.0.0', port)
    try:
        sock.bind(server_address)
    except Exception as e:
        print(f"❌ 错误：绑定到端口 {port} 失败: {e}")
        # 尝试关闭 socket 以释放资源
        sock.close()
        return

    print(f"👂 正在监听 UDP 端口 {port}...")
    print("等待接收数据包... (按 Ctrl+C 停止)")

    try:
        while True:
            # 接收数据包。data 是接收到的数据，address 是发送方的地址。
            data, address = sock.recvfrom(buffer_size)

            packet_length = len(data)

            # 打印数据包长度
            print("-" * 30)
            print(f"📦 收到来自 {address} 的数据包。")
            print(f"📏 数据包总长度: {packet_length} 字节")

            # 提取并打印包头
            print(f"包头：{int.from_bytes(data[0:2],'big',signed=False)} {int.from_bytes(data[2:4],'big',signed=False)} {int.from_bytes(data[4:8],'big',signed=False)} ")

    except KeyboardInterrupt:
        # 用户按 Ctrl+C 停止程序
        print("\n✋ 程序停止。")
    except Exception as e:
        print(f"\n❌ 发生异常: {e}")
    finally:
        # 关闭 socket
        sock.close()
        print(f"端口 {port} 监听已关闭。")

# --- 程序入口 ---
if __name__ == "__main__":
    # 可以根据需要修改为你希望监听的端口号
    LISTEN_PORT = 3334
    udp_listener(LISTEN_PORT)