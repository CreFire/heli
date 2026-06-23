package xsync

import (
	"game/deps/basal"
	"game/deps/xlog"
	"runtime"
	"time"
)

const (
	spinningTimeout = time.Second * 5 //超时
	spinMax         = 1000            //自旋次数大于此值, 休息1秒输出错误
	spinMs          = 100             //自旋次数大于此值休息1毫秒
	spinCpu         = 10              //自旋次数大于10主动让出cpu
)

func spinning(spin uint16, now time.Time) uint16 {
	if spin > spinMax {
		if d := time.Since(now); d > spinningTimeout {
			xlog.Errorf("please check for deadlock, 获取锁超时(重复加锁,递归加锁,未释放锁,锁下逻辑阻塞等): %s, %v", basal.StackLine(3), d)
			time.Sleep(time.Second)
			return spin
		}
		time.Sleep(time.Millisecond)
	} else if spin > spinMs {
		time.Sleep(time.Millisecond)
		spin++
	} else if spin > spinCpu {
		runtime.Gosched()
		spin++
	} else {
		spin++
	}
	return spin
}
