package worker

import (
	"time"
)

func LongRunningMemoryWorker(wconf WorkerIntConfig) Worker {
	wk := NewInMemoryWorker(wconf.name, wconf.poolName, wconf.ch, 30*time.Second, 0, 30*time.Second)
	wk.Lifespan = wconf.lifespan
	wk.Memory = wconf.memory
	wk.RestartsCounter = wconf.restarts
	return &wk
}

func ShortRunningMemoryWorker(wconf WorkerIntConfig) Worker {
	wk := NewInMemoryWorker(wconf.name, wconf.poolName, wconf.ch, 100*time.Millisecond, 0, 30*time.Second)
	wk.Lifespan = wconf.lifespan
	wk.Memory = wconf.memory
	wk.RestartsCounter = wconf.restarts
	return &wk
}

func MediumRunningMemoryWorker(wconf WorkerIntConfig) Worker {
	wk := NewInMemoryWorker(wconf.name, wconf.poolName, wconf.ch, 10*time.Second, 0, 30*time.Second)
	wk.Lifespan = wconf.lifespan
	wk.Memory = wconf.memory
	wk.RestartsCounter = wconf.restarts
	return &wk
}

func IgnoresInterruptMemoryWorker(wconf WorkerIntConfig) Worker {
	wk := NewInMemoryWorker(wconf.name, wconf.poolName, wconf.ch, 30*time.Second, 0, 30*time.Second)
	wk.Lifespan = wconf.lifespan
	wk.Memory = wconf.memory
	wk.RestartsCounter = wconf.restarts
	wk.IgnoreTerm = true
	return &wk
}

func EmptyMemoryWorker(wconf WorkerIntConfig) Worker {
	return &InMemoryWorker{}
}
