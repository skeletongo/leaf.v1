package module

import (
	"runtime"
	"sync"

	"github.com/skeletongo/leaf.v1/conf"
	"github.com/skeletongo/leaf.v1/log"
)

// Module 模块接口
type Module interface {
	OnInit()
	OnDestroy()
	Run(closeSig chan struct{})
}

type module struct {
	mi       Module         // 模块
	closeSig chan struct{}  // 通知模块关闭
	wg       sync.WaitGroup // 模块启动标记
}

var mods []*module

// Register 模块注册
func Register(mi Module) {
	m := new(module)
	m.mi = mi
	m.closeSig = make(chan struct{}, 1)
	mods = append(mods, m)
}

func Init() {
	for i := 0; i < len(mods); i++ {
		mods[i].mi.OnInit()
	}
	// 模块初始化成功后再执行模块任务
	var m *module
	for i := 0; i < len(mods); i++ {
		m = mods[i]
		m.wg.Add(1) // 模块启动
		go run(m)
	}
}

func run(m *module) {
	// 模块启动时抛异常不处理，程序启动失败
	m.mi.Run(m.closeSig)
	m.wg.Done()
}

// Destroy 模块关闭
// 模块关闭的顺序和模块注册顺序相反
// 模块启动后才能关闭，否则等待模块启动完成
func Destroy() {
	for i := len(mods) - 1; i >= 0; i-- {
		m := mods[i]
		m.closeSig <- struct{}{} // 发送关闭信号
		m.wg.Wait()              // 等待模块线程关闭
		destroy(m)
	}
}

func destroy(m *module) {
	// 模块关闭出现异常，需要恢复，为了不影响其它模块的正常关闭
	defer func() {
		if r := recover(); r != nil {
			if conf.LenStackBuf > 0 {
				buf := make([]byte, conf.LenStackBuf)
				l := runtime.Stack(buf, false)
				log.Error("%v: %s", r, buf[:l])
			} else {
				log.Error("%v", r)
			}
		}
	}()

	m.mi.OnDestroy()
}
