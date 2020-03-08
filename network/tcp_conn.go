package network

import (
	"github.com/name5566/leaf/log"
	"net"
	"sync"
)

type ConnSet map[net.Conn]struct{}	// 定义一个ConnSet类型实际为一个net.Conn到空对象的映射

type TCPConn struct {
	sync.Mutex					// 互斥锁
	conn      net.Conn			// 连接
	writeChan chan []byte		// 写通道
	closeFlag bool				// 关闭标识符
	msgParser *MsgParser		// 消息解释器
}

// 初始化一个TCPConn，pendingWriteNum为写通道的容量
func newTCPConn(conn net.Conn, pendingWriteNum int, msgParser *MsgParser) *TCPConn {
	tcpConn := new(TCPConn)
	tcpConn.conn = conn
	tcpConn.writeChan = make(chan []byte, pendingWriteNum)
	tcpConn.msgParser = msgParser
	// 开启一个协程，从writechan中不断的接受数据，然后发送出去
	go func() {
		// 当tcpConn关闭时会向tcpConn.writeChan中发送一个nil
		for b := range tcpConn.writeChan {
			if b == nil {
				break
			}

			_, err := conn.Write(b)
			if err != nil {
				break
			}
		}
		// 当发送完成后，关闭连接，并且将tcpConn的closeFlag设置为true，表示writeChan已关闭
		conn.Close()
		tcpConn.Lock()
		tcpConn.closeFlag = true
		tcpConn.Unlock()
	}()

	return tcpConn
}

func (tcpConn *TCPConn) doDestroy() {
	tcpConn.conn.(*net.TCPConn).SetLinger(0)
	tcpConn.conn.Close()

	if !tcpConn.closeFlag {
		close(tcpConn.writeChan)
		tcpConn.closeFlag = true
	}
}
// tcpConn销毁，如果再销毁时发现writeChan通道未关闭，则关闭通道
func (tcpConn *TCPConn) Destroy() {
	tcpConn.Lock()
	defer tcpConn.Unlock()

	tcpConn.doDestroy()
}
// 关闭tcpConn，如果writeChan通道未关闭，则会向通道发送nil,这里直接走的doWrite并没有走Write,因为Write会将nil过滤掉，保证其他地方不会传入nil
func (tcpConn *TCPConn) Close() {
	tcpConn.Lock()
	defer tcpConn.Unlock()
	if tcpConn.closeFlag {
		return
	}

	tcpConn.doWrite(nil)
	tcpConn.closeFlag = true
}

func (tcpConn *TCPConn) doWrite(b []byte) {
	if len(tcpConn.writeChan) == cap(tcpConn.writeChan) {
		log.Debug("close conn: channel full")
		tcpConn.doDestroy()
		return
	}

	tcpConn.writeChan <- b
}

// b must not be modified by the others goroutines
// 通过调用do_write,将信息b发送到writeChan中
func (tcpConn *TCPConn) Write(b []byte) {
	tcpConn.Lock()
	defer tcpConn.Unlock()
	if tcpConn.closeFlag || b == nil {
		return
	}

	tcpConn.doWrite(b)
}

//
func (tcpConn *TCPConn) Read(b []byte) (int, error) {
	return tcpConn.conn.Read(b)
}
// 获取本地网络地址
func (tcpConn *TCPConn) LocalAddr() net.Addr {
	return tcpConn.conn.LocalAddr()
}
// 获取远程网络地址
func (tcpConn *TCPConn) RemoteAddr() net.Addr {
	return tcpConn.conn.RemoteAddr()
}
// 通过调用MsgParser的Read方法，读取数据。
func (tcpConn *TCPConn) ReadMsg() ([]byte, error) {
	return tcpConn.msgParser.Read(tcpConn)
}

// 先通过msgParser的Write将信息按照协议封装，在msgParser.Wirte中会调用TCPConn的Write，最终实现将封装好的信息发送到
// writeChan中，详细参考tcp_msg中MsgParser的Write方法
func (tcpConn *TCPConn) WriteMsg(args ...[]byte) error {
	return tcpConn.msgParser.Write(tcpConn, args...)
}
