// Copyright 2024 The BACnet Authors. All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package server

import (
	"fmt"

	"github.com/anviod/bacnet/btypes"
	log "github.com/anviod/bacnet/helpers/log"
	"go.uber.org/zap"
)

// PropertyWriteEvent describes a single property write applied to an object.
// It is delivered to hooks registered via Server.OnPropertyWrite after the
// write has been committed to the object store.
//
// 中文说明：PropertyWriteEvent 描述一次已应用到对象的属性写入。
// 在写入成功提交到对象存储后，投递给通过 Server.OnPropertyWrite 注册的钩子。
type PropertyWriteEvent struct {
	ObjectType     btypes.ObjectType     // target object type / 目标对象类型
	ObjectInstance btypes.ObjectInstance // target object instance / 目标对象实例
	PropertyType   btypes.PropertyType   // written property / 被写入的属性
	ArrayIndex     uint32                // requested array index (ArrayAll if none) / 请求的数组索引
	OldValue       interface{}           // value before the write; nil if it did not exist / 写入前的值，属性原本不存在时为 nil
	NewValue       interface{}           // value written by the client / 客户端写入的新值
	Priority       btypes.NPDUPriority   // write priority from the request / 请求携带的写优先级
	Source         *btypes.Address       // client address; nil for local writes via Server.SetProperty / 客户端地址，本地调用 Server.SetProperty 时为 nil
}

// PropertyWriteHook is the callback signature for property write notifications.
// Hooks run on the message-handling goroutine after the SimpleAck has been
// sent, so they must not block for a long time.
//
// 中文说明：PropertyWriteHook 是属性写入通知的回调签名。
// 钩子在 SimpleAck 发送之后、于消息处理 goroutine 中运行，不应长时间阻塞。
type PropertyWriteHook func(evt PropertyWriteEvent)

// OnPropertyWrite registers a hook invoked whenever an object property is
// written — either by a remote BACnet client (WriteProperty /
// WritePropertyMultiple) or locally through Server.SetProperty.
// It returns an unsubscribe function that removes the hook.
//
// 中文说明：OnPropertyWrite 注册属性写入钩子。当远端 BACnet 客户端
// （WriteProperty / WritePropertyMultiple）或本地通过 Server.SetProperty
// 写入对象属性时触发。返回的函数用于取消注册。
func (s *server) OnPropertyWrite(hook PropertyWriteHook) func() {
	if hook == nil {
		return func() {}
	}

	s.writeHooksMu.Lock()
	s.writeHookSeq++
	id := s.writeHookSeq
	s.writeHooks = append(s.writeHooks, writeHookEntry{id: id, hook: hook})
	s.writeHooksMu.Unlock()

	return func() {
		s.writeHooksMu.Lock()
		defer s.writeHooksMu.Unlock()
		for i, e := range s.writeHooks {
			if e.id == id {
				s.writeHooks = append(s.writeHooks[:i], s.writeHooks[i+1:]...)
				return
			}
		}
	}
}

// fireWriteHooks delivers a write event to all registered hooks.
// A panicking hook is recovered and logged so it cannot break the server.
//
// 中文说明：fireWriteHooks 向所有已注册钩子投递写入事件。
// 钩子 panic 会被捕获并记录日志，不会影响服务端运行。
func (s *server) fireWriteHooks(evt PropertyWriteEvent) {
	s.writeHooksMu.Lock()
	if len(s.writeHooks) == 0 {
		s.writeHooksMu.Unlock()
		return
	}
	hooks := make([]PropertyWriteHook, 0, len(s.writeHooks))
	for _, e := range s.writeHooks {
		hooks = append(hooks, e.hook)
	}
	s.writeHooksMu.Unlock()

	for _, h := range hooks {
		invokeWriteHook(h, evt)
	}
}

func invokeWriteHook(h PropertyWriteHook, evt PropertyWriteEvent) {
	defer func() {
		if r := recover(); r != nil {
			log.Logger.Error("property write hook panicked",
				zap.String("panic", fmt.Sprint(r)),
				zap.Stringer("object", evt.ObjectType),
				zap.Uint32("instance", uint32(evt.ObjectInstance)),
				zap.Uint32("property", uint32(evt.PropertyType)),
			)
		}
	}()
	h(evt)
}
