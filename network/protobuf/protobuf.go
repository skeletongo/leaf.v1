package protobuf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/skeletongo/leaf.v1/chanrpc"
	"github.com/skeletongo/leaf.v1/log"
	"github.com/golang/protobuf/proto"
)

// -------------------------
// | id | protobuf message |
// -------------------------
type Processor struct {
	littleEndian bool                    // id部分，是否使用小端序
	msgInfo      []*MsgInfo              // 消息列表
	msgID        map[reflect.Type]uint16 // 索引
}

type MsgInfo struct {
	msgType       reflect.Type    // 消息类型
	msgRouter     *chanrpc.Server // 消息处理服务
	msgHandler    MsgHandler      // 处理方法，在当前线程中执行
	msgRawHandler MsgHandler      // 处理方法，在当前线程中执行，此方法执行后不会再执行msgHandler，msgRouter
}

type MsgHandler func([]interface{})

type MsgRaw struct {
	msgID      uint16
	msgRawData []byte
}

func NewProcessor() *Processor {
	return &Processor{
		msgID: make(map[reflect.Type]uint16),
	}
}

func (p *Processor) SetEndian(isLittleEndian bool) {
	p.littleEndian = isLittleEndian
}

func (p *Processor) Register(msg proto.Message) uint16 {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		log.Fatal("json message pointer required")
	}
	if _, ok := p.msgID[msgType]; ok {
		log.Fatal("message %s is already registered", msgType)
	}
	if len(p.msgInfo) >= math.MaxUint16 {
		log.Fatal("too many protobuf messages (max = %v)", math.MaxUint16)
	}

	id := uint16(len(p.msgInfo) + 1)
	p.msgID[msgType] = id
	p.msgInfo = append(p.msgInfo, &MsgInfo{msgType: msgType})
	return id
}

func (p *Processor) SetRawHandler(id uint16, msgRawHandler MsgHandler) {
	if id >= uint16(len(p.msgInfo)) {
		log.Fatal("message id %v not registered", id)
	}
	p.msgInfo[id].msgRawHandler = msgRawHandler
}

func (p *Processor) SetHandler(msg proto.Message, msgHandler MsgHandler) {
	msgType := reflect.TypeOf(msg)
	id, ok := p.msgID[msgType]
	if !ok {
		log.Fatal("message %s not registered", msgType)
	}

	p.msgInfo[id].msgHandler = msgHandler
}

func (p *Processor) SetRouter(msg proto.Message, msgRouter *chanrpc.Server) {
	msgType := reflect.TypeOf(msg)
	id, ok := p.msgID[msgType]
	if !ok {
		log.Fatal("message %s not registered", msgType)
	}

	p.msgInfo[id].msgRouter = msgRouter
}

func (p *Processor) Unmarshal(data []byte) (interface{}, error) {
	if len(data) < 2 { // id占两个字节
		return nil, errors.New("protobuf data too short")
	}

	var id uint16
	if p.littleEndian {
		id = binary.LittleEndian.Uint16(data[:2])
	} else {
		id = binary.BigEndian.Uint16(data[:2])
	}
	if id >= uint16(len(p.msgInfo)) {
		return nil, fmt.Errorf("message id %v not registered", id)
	}

	i := p.msgInfo[id]
	if i.msgRawHandler != nil {
		return &MsgRaw{id, data[2:]}, nil
	}
	msg := reflect.New(i.msgType.Elem()).Interface()
	return msg, proto.UnmarshalMerge(data[2:], msg.(proto.Message))
}

func (p *Processor) Marshal(msg interface{}) ([][]byte, error) {
	msgType := reflect.TypeOf(msg)
	id, ok := p.msgID[msgType]
	if !ok {
		log.Fatal("message %s not registered", msgType)
	}

	_id := make([]byte, 2)
	if p.littleEndian {
		binary.LittleEndian.PutUint16(_id, id)
	} else {
		binary.BigEndian.PutUint16(_id, id)
	}
	data, err := proto.Marshal(msg.(proto.Message))
	return [][]byte{_id, data}, err
}

func (p *Processor) Router(msg interface{}, userData interface{}) error {
	if msgRaw, ok := msg.(*MsgRaw); ok {
		if msgRaw.msgID >= uint16(len(p.msgInfo)) {
			return fmt.Errorf("message id %v not registered", msgRaw.msgID)
		}
		i := p.msgInfo[msgRaw.msgID]
		if i.msgRawHandler != nil {
			i.msgRawHandler([]interface{}{msgRaw.msgID, msgRaw.msgRawData, userData})
		}
		return nil
	}

	msgType := reflect.TypeOf(msg)
	id, ok := p.msgID[msgType]
	if !ok {
		return fmt.Errorf("message %s not registered", msgType)
	}
	i := p.msgInfo[id]
	if i.msgHandler != nil {
		i.msgHandler([]interface{}{msg, userData})
	}
	if i.msgRouter != nil {
		i.msgRouter.Go(msgType, msg, userData)
	}
	return nil
}

func (p *Processor) Range(f func(id uint16, msgType reflect.Type)) {
	for k, v := range p.msgInfo {
		f(uint16(k), v.msgType)
	}
}
