package network

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

// --------------
// | len | data |
// --------------
type MsgParser struct {
	lenMsgLen    int		// len的字节长度
	minMsgLen    uint32		// 最小msg长度
	maxMsgLen    uint32		// 最大msg长度
	littleEndian bool		// 大小端
}
// 创建一个消息解析器
func NewMsgParser() *MsgParser {
	p := new(MsgParser)
	p.lenMsgLen = 2
	p.minMsgLen = 1
	p.maxMsgLen = 4096
	p.littleEndian = false

	return p
}

// 在读写时调用该方法是危险的，lenMsgLen只能是 1 或者 2 或者 4
// 设置消息解析器的长度参数（len,minLen,maxLen）
func (p *MsgParser) SetMsgLen(lenMsgLen int, minMsgLen uint32, maxMsgLen uint32) {
	if lenMsgLen == 1 || lenMsgLen == 2 || lenMsgLen == 4 {
		p.lenMsgLen = lenMsgLen
	}
	if minMsgLen != 0 {
		p.minMsgLen = minMsgLen
	}
	if maxMsgLen != 0 {
		p.maxMsgLen = maxMsgLen
	}

	var max uint32
	switch p.lenMsgLen {
	case 1:
		max = math.MaxUint8
	case 2:
		max = math.MaxUint16
	case 4:
		max = math.MaxUint32
	}
	if p.minMsgLen > max {
		p.minMsgLen = max
	}
	if p.maxMsgLen > max {
		p.maxMsgLen = max
	}
}

// It's dangerous to call the method on reading or writing
// 设置大小端（true 代表小端， false 代表大端）
func (p *MsgParser) SetByteOrder(littleEndian bool) {
	p.littleEndian = littleEndian
}

// goroutine safe
// 因为conn实现了Read方法，所以Read方法会通过io:ReadFull来读取conn的数据,io:ReadFull最终会调用conn.Read来获取数据,参考io:ReadFull源码
func (p *MsgParser) Read(conn *TCPConn) ([]byte, error) {
	var b [4]byte
	bufMsgLen := b[:p.lenMsgLen]

	// read len  读取len大小的字节
	if _, err := io.ReadFull(conn, bufMsgLen); err != nil {
		return nil, err
	}

	// parse len 通过解析获取包的长度len
	var msgLen uint32
	switch p.lenMsgLen {
	case 1:
		msgLen = uint32(bufMsgLen[0])
	case 2:
		if p.littleEndian {
			msgLen = uint32(binary.LittleEndian.Uint16(bufMsgLen))
		} else {
			msgLen = uint32(binary.BigEndian.Uint16(bufMsgLen))
		}
	case 4:
		if p.littleEndian {
			msgLen = binary.LittleEndian.Uint32(bufMsgLen)
		} else {
			msgLen = binary.BigEndian.Uint32(bufMsgLen)
		}
	}

	// check len 检查len大小，是不是在规定的min到max之间
	if msgLen > p.maxMsgLen {
		return nil, errors.New("message too long")
	} else if msgLen < p.minMsgLen {
		return nil, errors.New("message too short")
	}

	// data 读取len大小的数据
	msgData := make([]byte, msgLen)
	if _, err := io.ReadFull(conn, msgData); err != nil {
		return nil, err
	}

	return msgData, nil
}

// goroutine safe
func (p *MsgParser) Write(conn *TCPConn, args ...[]byte) error {
	// get len  获取msg的len
	var msgLen uint32
	for i := 0; i < len(args); i++ {
		msgLen += uint32(len(args[i]))
	}

	// check len 检测len是否在min和max之间
	if msgLen > p.maxMsgLen {
		return errors.New("message too long")
	} else if msgLen < p.minMsgLen {
		return errors.New("message too short")
	}
	// 根据len和data大小申请msg buf
	msg := make([]byte, uint32(p.lenMsgLen)+msgLen)

	// write len 根据大小端和len的字节长度，向msg buf中写入len
	switch p.lenMsgLen {
	case 1:
		msg[0] = byte(msgLen)
	case 2:
		if p.littleEndian {
			binary.LittleEndian.PutUint16(msg, uint16(msgLen))
		} else {
			binary.BigEndian.PutUint16(msg, uint16(msgLen))
		}
	case 4:
		if p.littleEndian {
			binary.LittleEndian.PutUint32(msg, msgLen)
		} else {
			binary.BigEndian.PutUint32(msg, msgLen)
		}
	}

	// write data 将data写入到msg buf中
	l := p.lenMsgLen
	for i := 0; i < len(args); i++ {
		copy(msg[l:], args[i])
		l += len(args[i])
	}

	conn.Write(msg)

	return nil
}
