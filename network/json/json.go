package json

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/skeletongo/leaf.v1/chanrpc"
	"github.com/skeletongo/leaf.v1/log"
)

// Processor 消息处理器
type Processor struct {
	msgInfo map[string]*MsgInfo
}

type MsgInfo struct {
	msgType       reflect.Type    // 消息类型
	msgRouter     *chanrpc.Server // 消息处理服务
	msgHandler    MsgHandler      // 处理方法，在当前协程中执行
	msgRawHandler MsgHandler      // 处理方法，在当前协程中执行，此方法执行后不会再执行msgHandler，msgRouter
}

type MsgHandler func([]interface{})

type MsgRaw struct {
	msgID      string
	msgRawData json.RawMessage
}

func NewProcessor() *Processor {
	return &Processor{
		msgInfo: make(map[string]*MsgInfo),
	}
}

func (p *Processor) Register(msg interface{}) {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		log.Fatal("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	if msgID == "" {
		log.Fatal("unnamed json message")
	}
	if _, ok := p.msgInfo[msgID]; ok {
		log.Fatal("message %s not registered", msgID)
	}

	p.msgInfo[msgID] = &MsgInfo{msgType: msgType}
}

func (p *Processor) SetRawHandler(msgID string, msgRawHandler MsgHandler) {
	i, ok := p.msgInfo[msgID]
	if !ok {
		log.Fatal("message %s not registered", msgID)
	}

	i.msgRawHandler = msgRawHandler
}

func (p *Processor) SetHandler(msg interface{}, msgHandler MsgHandler) {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		log.Fatal("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	i, ok := p.msgInfo[msgID]
	if !ok {
		log.Fatal("message %s not registered", msgID)
	}

	i.msgHandler = msgHandler
}

func (p *Processor) SetRouter(msg interface{}, msgRouter *chanrpc.Server) {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		log.Fatal("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	i, ok := p.msgInfo[msgID]
	if !ok {
		log.Fatal("message %s not registered", msgID)
	}

	i.msgRouter = msgRouter
}

func (p *Processor) Unmarshal(data []byte) (interface{}, error) {
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	if len(m) != 1 {
		return nil, errors.New("invalid json data")
	}

	for msgID, data := range m {
		i, ok := p.msgInfo[msgID]
		if !ok {
			return nil, fmt.Errorf("message %v not registered", msgID)
		}
		if i.msgRawHandler != nil {
			return &MsgRaw{msgID, data}, nil
		}
		msg := reflect.New(i.msgType.Elem()).Interface()
		return msg, json.Unmarshal(data, msg)
	}
	panic("bug")
}

func (p *Processor) Marshal(msg interface{}) ([][]byte, error) {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		return nil, errors.New("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	if _, ok := p.msgInfo[msgID]; !ok {
		return nil, fmt.Errorf("message %v not registered", msgID)
	}

	m := map[string]interface{}{msgID: msg}
	data, err := json.Marshal(&m)
	return [][]byte{data}, err
}

func (p *Processor) Route(msg interface{}, userData interface{}) error {
	if msgRaw, ok := msg.(*MsgRaw); ok {
		i, ok := p.msgInfo[msgRaw.msgID]
		if !ok {
			return fmt.Errorf("message %v not registered", msgRaw.msgID)
		}
		i.msgRawHandler([]interface{}{msgRaw.msgID, msgRaw.msgRawData, userData})
		return nil
	}

	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		return errors.New("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	i, ok := p.msgInfo[msgID]
	if !ok {
		return fmt.Errorf("message %v not registered", msgID)
	}
	if i.msgHandler != nil {
		i.msgHandler([]interface{}{msg, userData})
	}
	if i.msgRouter != nil {
		i.msgRouter.Go(msgType, msg, userData)
	}
	return nil
}
