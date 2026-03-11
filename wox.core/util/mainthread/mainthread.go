package mainthread

import "runtime"

type callRequest struct {
	fn   func()
	done chan struct{}
}

var funcQ = make(chan callRequest)

func init() {
	runtime.LockOSThread()
}

func Init(main func()) {
	go main()

	for f := range funcQ {
		if f.fn != nil {
			f.fn()
		}
		close(f.done)
	}
}

func Call(f func()) {
	done := make(chan struct{})
	funcQ <- callRequest{fn: f, done: done}
	<-done
}
