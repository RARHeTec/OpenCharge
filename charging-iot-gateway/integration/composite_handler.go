package integration

import ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"

// CompositeHandler fans one device event out to multiple handlers, preserving
// the order in which they are supplied (state before settlement by convention).
type CompositeHandler []ykcserver.Handler

func (c CompositeHandler) OnEvent(e ykcserver.Event) {
	for _, h := range c {
		h.OnEvent(e)
	}
}
