package g

import (
	"container/list"
	"github.com/name5566/leaf/conf"
	"github.com/name5566/leaf/log"
	"runtime"
	"sync"
)

// one Go per goroutine (goroutine not safe)
type Go struct {
	ChanCb    chan func()		// 用于传送callback函数
	pendingGo int				// 用于记录正在处理的go的计数器
}

type LinearGo struct {
	f  func()
	cb func()
}
// LinearContext提供顺序调用功能，其中利用list来顺序保存执行函数和回调函数 可以认为是一个顺序执行的Go
type LinearContext struct {
	g              *Go
	linearGo       *list.List	// 用来顺序保存执行函数和回调函数
	mutexLinearGo  sync.Mutex
	mutexExecution sync.Mutex
}

func New(l int) *Go {
	g := new(Go)
	g.ChanCb = make(chan func(), l)	// 创建用于传送callback函数的通道，l为通道容量
	return g
}

func (g *Go) Go(f func(), cb func()) {
	g.pendingGo++		// 计数+1，记录调用计数

	// 开一个协程执行f
	go func() {
		// 将回调函数cb发送到ChanCb中
		defer func() {
			g.ChanCb <- cb
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

		f()
	}()
}

func (g *Go) Cb(cb func()) {
	// 执行完cb之后对pengdingo减1，表示整个过程执行完成
	defer func() {
		g.pendingGo--
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
	// 执行cb函数
	if cb != nil {
		cb()
	}
}

func (g *Go) Close() {
	// 等待所有的go调用的Cb，并执行
	for g.pendingGo > 0 {
		g.Cb(<-g.ChanCb)
	}
}

func (g *Go) Idle() bool {
	return g.pendingGo == 0
}

// 生成LinearContext
func (g *Go) NewLinearContext() *LinearContext {
	c := new(LinearContext)
	c.g = g
	c.linearGo = list.New()
	return c
}


func (c *LinearContext) Go(f func(), cb func()) {
	c.g.pendingGo++
	// 整体的过程和*GO的Go方法一致，只是因为在向LinearGo中写入f函数和回调函数是增加了锁，保证加入队列的顺序和调用顺序一致。
	// 将需要执行的f函数和cb回调函数，放入linearGo中
	c.mutexLinearGo.Lock()//  加锁
	c.linearGo.PushBack(&LinearGo{f: f, cb: cb})
	c.mutexLinearGo.Unlock()// 解锁

	go func() {
		// 在启动协程时加了锁，保证协程执行完成后才会解锁，保证调用的执行顺序
		c.mutexExecution.Lock()
		// 协程执行完，解锁
		defer c.mutexExecution.Unlock()

		// 取数据是同样加锁
		c.mutexLinearGo.Lock()	//  加锁
		e := c.linearGo.Remove(c.linearGo.Front()).(*LinearGo)
		c.mutexLinearGo.Unlock() // 解锁
		// 执行完f函数后，向ChanCb中发送回调函数
		defer func() {
			c.g.ChanCb <- e.cb
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
		// 执行f函数
		e.f()
	}()
}
