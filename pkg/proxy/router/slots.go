// Copyright 2014 Wandoujia Inc. All Rights Reserved.
// Licensed under the MIT (MIT-LICENSE.txt) license.

package router

import (
	"github.com/wlibo666/codis/pkg/utils/errors"
	"github.com/wlibo666/codis/pkg/utils/log"
	"github.com/wlibo666/codis/pkg/proxy/redis"
	"fmt"
	"sync"
)

type Slot struct {
	Id int

	Backend struct {
		Addr string
		host []byte
		port []byte
		bc   *SharedBackendConn
	}
	Migrate struct {
		From string
		bc   *SharedBackendConn
	}

	wait sync.WaitGroup
	lock struct {
		hold bool
		sync.RWMutex
	}
}

func (s *Slot) blockAndWait() {
	if !s.lock.hold {
		s.lock.hold = true
		s.lock.Lock()
	}
	s.wait.Wait()
}

func (s *Slot) unblock() {
	if !s.lock.hold {
		return
	}
	s.lock.hold = false
	s.lock.Unlock()
}

func (s *Slot) reset() {
	s.Backend.Addr = ""
	s.Backend.host = nil
	s.Backend.port = nil
	s.Backend.bc = nil
	s.Migrate.From = ""
	s.Migrate.bc = nil
}

func (s *Slot) forward(r *Request, key []byte) error {
	s.lock.RLock()
	bc, err := s.prepare(r, key)
	s.lock.RUnlock()
	if err != nil {
		return err
	} else {
		bc.PushBack(r)
		return nil
	}
}

var ErrSlotIsNotReady = errors.New("slot is not ready, may be offline")

func (s *Slot) prepare(r *Request, key []byte) (*SharedBackendConn, error) {
	if s.Backend.bc == nil {
		log.Infof("slot-%04d is not ready: key = %s", s.Id, key)
		return nil, ErrSlotIsNotReady
	}
	if err := s.slotsmgrt(r, key); err != nil {
		log.Warnf("slot-%04d Migrate From = %s to %s failed: key = %s, error = %s",
			s.Id, s.Migrate.From, s.Backend.Addr, key, err)
		return nil, err
	} else {
		r.slot = &s.wait
		r.slot.Add(1)
		return s.Backend.bc, nil
	}
}

//func microseconds() int64 {
//	return time.Now().UnixNano() / int64(time.Microsecond)
//}

func (s *Slot) slotsmgrt(r *Request, key []byte) error {
	if len(key) == 0 || s.Migrate.bc == nil {
		return nil
	}
	m := &Request{
		Resp: redis.NewArray([]*redis.Resp{
			redis.NewBulkBytes([]byte("SLOTSMGRTTAGONE")),
			redis.NewBulkBytes(s.Backend.host),
			redis.NewBulkBytes(s.Backend.port),
			redis.NewBulkBytes([]byte("3000")),
			redis.NewBulkBytes(key),
		}),
		Wait: &sync.WaitGroup{},
	}
	s.Migrate.bc.PushBack(m)

	m.Wait.Wait()

	resp, err := m.Response.Resp, m.Response.Err
	if err != nil {
		incrFailOpStats("slotsmgrttagone", 1000000)
		incrRedisFailOpStats(r.RedisAddr, r.OpStr, 1000000)
		return err
	}
	if resp == nil {
		return ErrRespIsRequired
	}
	if resp.IsError() {
		incrFailOpStats("slotsmgrttagone", 1000000)
		incrRedisFailOpStats(r.RedisAddr, r.OpStr, 1000000)
		return errors.New(fmt.Sprintf("error resp: %s", resp.Value))
	}
	if resp.IsInt() {
		log.Debugf("slot-%04d Migrate From %s to %s: key = %s, resp = %s",
			s.Id, s.Migrate.From, s.Backend.Addr, key, resp.Value)
		return nil
	} else {
		return errors.New(fmt.Sprintf("error resp: should be integer, but got %s", resp.Type))
	}
}
