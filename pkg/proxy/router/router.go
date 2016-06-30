// Copyright 2014 Wandoujia Inc. All Rights Reserved.
// Licensed under the MIT (MIT-LICENSE.txt) license.

package router

import (
	"github.com/wlibo666/codis/pkg/models"
	"github.com/wlibo666/codis/pkg/utils/errors"
	"github.com/wlibo666/codis/pkg/utils/log"
	"github.com/wlibo666/codis/pkg/proxy/redis"
	"strings"
	"sync"
)

const MaxSlotNum = models.DEFAULT_SLOT_NUM

type Router struct {
	mu sync.Mutex

	auth string
	Pool map[string]*SharedBackendConn

	GSlots [MaxSlotNum]*Slot

	closed bool
}

func New() *Router {
	return NewWithAuth("")
}

func NewWithAuth(auth string) *Router {
	s := &Router{
		auth: auth,
		Pool: make(map[string]*SharedBackendConn),
	}
	for i := 0; i < len(s.GSlots); i++ {
		s.GSlots[i] = &Slot{Id: i}
	}
	return s
}

func (s *Router) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	for i := 0; i < len(s.GSlots); i++ {
		s.resetSlot(i)
	}
	s.closed = true
	return nil
}

var errClosedRouter = errors.New("use of closed router")

func (s *Router) ResetSlot(i int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errClosedRouter
	}
	s.resetSlot(i)
	return nil
}

func (s *Router) FillSlot(i int, Addr, From string, lock bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errClosedRouter
	}
	s.fillSlot(i, Addr, From, lock)
	return nil
}

func (s *Router) KeepAlive() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errClosedRouter
	}
	for _, bc := range s.Pool {
		bc.KeepAlive()
	}
	return nil
}

func (s *Router) Dispatch(r *Request) error {
	hkey := getHashKey(r.Resp, r.OpStr)
	slot := s.GSlots[hashSlot(hkey)]
	return slot.forward(r, hkey)
}

// Add by WangChunya
func (s *Router) GetRedisAddrByRequest(r *Request) string {
	hkey := getHashKey(r.Resp, r.OpStr)
	slot := s.GSlots[hashSlot(hkey)]
	return slot.Backend.Addr
}

func (s *Router) GetRedisAddrByRespOP(resp *redis.Resp, opstr string) string {
	hkey := getHashKey(resp, opstr)
	slot := s.GSlots[hashSlot(hkey)]
	return slot.Backend.Addr
}

func (s *Router) getBackendConn(Addr string) *SharedBackendConn {
	bc := s.Pool[Addr]
	if bc != nil {
		bc.IncrRefcnt()
	} else {
		bc = NewSharedBackendConn(Addr, s.auth)
		s.Pool[Addr] = bc
	}
	return bc
}

func (s *Router) putBackendConn(bc *SharedBackendConn) {
	if bc != nil && bc.Close() {
		delete(s.Pool, bc.Addr())
	}
}

func (s *Router) isValidSlot(i int) bool {
	return i >= 0 && i < len(s.GSlots)
}

func (s *Router) resetSlot(i int) {
	if !s.isValidSlot(i) {
		return
	}
	slot := s.GSlots[i]
	slot.blockAndWait()

	s.putBackendConn(slot.Backend.bc)
	s.putBackendConn(slot.Migrate.bc)
	slot.reset()

	slot.unblock()
}

func (s *Router) fillSlot(i int, Addr, From string, lock bool) {
	if !s.isValidSlot(i) {
		return
	}
	slot := s.GSlots[i]
	slot.blockAndWait()

	s.putBackendConn(slot.Backend.bc)
	s.putBackendConn(slot.Migrate.bc)
	slot.reset()

	if len(Addr) != 0 {
		xx := strings.Split(Addr, ":")
		if len(xx) >= 1 {
			slot.Backend.host = []byte(xx[0])
		}
		if len(xx) >= 2 {
			slot.Backend.port = []byte(xx[1])
		}
		slot.Backend.Addr = Addr
		slot.Backend.bc = s.getBackendConn(Addr)
	}
	if len(From) != 0 {
		slot.Migrate.From = From
		slot.Migrate.bc = s.getBackendConn(From)
	}

	if !lock {
		slot.unblock()
	}

	if slot.Migrate.bc != nil {
		log.Infof("fill slot %04d, Backend.Addr = %s, Migrate.From = %s",
			i, slot.Backend.Addr, slot.Migrate.From)
	} else {
		log.Infof("fill slot %04d, Backend.Addr = %s",
			i, slot.Backend.Addr)
	}
}
