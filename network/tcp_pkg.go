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
type PkgParser struct {
	byteLen      int    // 数据包长度len所占字节数
	minPkgLen    uint32 // 数据包最小长度
	maxPkgLen    uint32 // 数据包最大长度
	littleEndian bool   // 数据包长度len保存时的字节序顺序是否为小端序
}

func NewPkgParser() *PkgParser {
	return &PkgParser{
		byteLen:   2,
		minPkgLen: 1,
		maxPkgLen: 4096,
	}
}

func (p *PkgParser) SetPkgLen(byteLen int, minPkgLen, maxPkgLen uint32) {
	if byteLen == 1 || byteLen == 2 || byteLen == 4 {
		p.byteLen = byteLen
	}
	if minPkgLen != 0 {
		p.minPkgLen = minPkgLen
	}
	if maxPkgLen != 0 {
		p.maxPkgLen = maxPkgLen
	}

	var max uint32
	switch byteLen {
	case 1:
		max = math.MaxUint8
	case 2:
		max = math.MaxUint16
	case 4:
		max = math.MaxUint32
	}
	if p.minPkgLen > max {
		p.minPkgLen = max
	}
	if p.maxPkgLen > max {
		p.maxPkgLen = max
	}
	if p.maxPkgLen < p.minPkgLen {
		p.maxPkgLen, p.minPkgLen = p.minPkgLen, p.maxPkgLen
	}
}

func (p *PkgParser) SetEndian(isLittleEndian bool) {
	p.littleEndian = isLittleEndian
}

func (p *PkgParser) Read(r io.Reader) ([]byte, error) {
	// 读取数据长度
	data := make([]byte, 4)
	if _, err := io.ReadFull(r, data[:p.byteLen]); err != nil {
		return nil, err
	}
	var pkgLen uint32
	switch p.byteLen {
	case 1:
		pkgLen = uint32(data[0])
	case 2:
		if p.littleEndian {
			pkgLen = uint32(binary.LittleEndian.Uint16(data))
		} else {
			pkgLen = uint32(binary.BigEndian.Uint16(data))
		}
	case 4:
		if p.littleEndian {
			pkgLen = binary.LittleEndian.Uint32(data)
		} else {
			pkgLen = binary.BigEndian.Uint32(data)
		}
	}
	// 数据长度校验
	if pkgLen < p.minPkgLen {
		return nil, errors.New("message too short")
	} else if pkgLen > p.maxPkgLen {
		return nil, errors.New("message too long")
	}
	// 读取数据包
	data = make([]byte, pkgLen)
	_, err := io.ReadFull(r, data)
	return data, err
}

func (p *PkgParser) Write(w io.Writer, args ...[]byte) error {
	// 数据长度校验
	var pkgLen uint32
	for i := 0; i < len(args); i++ {
		pkgLen += uint32(len(args[i]))
	}
	if pkgLen < p.minPkgLen {
		return errors.New("message too short")
	} else if pkgLen > p.maxPkgLen {
		return errors.New("message too long")
	}

	data := make([]byte, uint32(p.byteLen)+pkgLen)
	// 写入数据长度
	switch p.byteLen {
	case 1:
		data[0] = byte(pkgLen)
	case 2:
		if p.littleEndian {
			binary.LittleEndian.PutUint16(data, uint16(pkgLen))
		} else {
			binary.BigEndian.PutUint16(data, uint16(pkgLen))
		}
	case 4:
		if p.littleEndian {
			binary.LittleEndian.PutUint32(data, pkgLen)
		} else {
			binary.BigEndian.PutUint32(data, pkgLen)
		}
	}
	// 写入数据
	l := p.byteLen
	for i := 0; i < len(args); i++ {
		copy(data[l:], args[i])
		l += len(args[i])
	}
	_, err := w.Write(data)
	return err
}
