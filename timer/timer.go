package timer

import (
	"github.com/name5566/leaf/conf"
	"github.com/name5566/leaf/log"
	"runtime"
	"time"
)

//--------
// Timer主要是提供一个Cron功能的定时器服务，其中Timer是time.AfterFunc的封装，是为了方便居合道Skeleton中。
//

// one dispatcher per goroutine (goroutine not safe)
// 封装一个*Timer类型的通道
type Dispatcher struct {
	ChanTimer chan *Timer
}
// 创建一个Dispatcher对象，l为chan的缓存容量
func NewDispatcher(l int) *Dispatcher {
	disp := new(Dispatcher)
	disp.ChanTimer = make(chan *Timer, l)
	return disp
}

// Timer
// 封装了标准库中的Timer，加上了一个回调函数cb
type Timer struct {
	t  *time.Timer
	cb func()
}
// 停止t timer，并将cb置空
func (t *Timer) Stop() {
	t.t.Stop()
	t.cb = nil
}

// 封装cb
func (t *Timer) Cb() {
	defer func() {
		t.cb = nil
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

	if t.cb != nil {
		t.cb()
	}
}
// 定时功能仍然是通过标准库的time.AfterFunc方法实现，只是在定时完成之后的处理是将t发送给Dispathcher的chan中，在skeleton
// 中会接收这个cb回调，并处理cb回调。
func (disp *Dispatcher) AfterFunc(d time.Duration, cb func()) *Timer {
	t := new(Timer)
	t.cb = cb
	t.t = time.AfterFunc(d, func() {
		disp.ChanTimer <- t
	})
	return t
}

// Cron
type Cron struct {
	t *Timer
}

func (c *Cron) Stop() {
	if c.t != nil {
		c.t.Stop()
	}
}

func (disp *Dispatcher) CronFunc(cronExpr *CronExpr, _cb func()) *Cron {
	c := new(Cron)

	now := time.Now()
	nextTime := cronExpr.Next(now)
	if nextTime.IsZero() {
		return c
	}

	// callback
	// 启动一个定时器，当定时器时间到了，触发cb回调，此时，会找下一个时间点，然后启动一个新的定时器，定时器启动
	// 完成后（启动完成并不是定时器时间到了）会执行_cb回调，然后再等到定时器时间到，再触发cb回调，这样一直循环下
	// 去，直到没有下一个时间点，然后执行最后一次_cb回调，可以理解为cb回调只是为了重复启动定时器，真正控制的是_cb的内容
	var cb func()
	cb = func() {
		defer _cb()

		now := time.Now()
		nextTime := cronExpr.Next(now)
		if nextTime.IsZero() {
			return
		}
		c.t = disp.AfterFunc(nextTime.Sub(now), cb)
	}

	c.t = disp.AfterFunc(nextTime.Sub(now), cb)
	return c
}
