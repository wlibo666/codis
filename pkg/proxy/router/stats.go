// Copyright 2014 Wandoujia Inc. All Rights Reserved.
// Licensed under the MIT (MIT-LICENSE.txt) license.

package router

import (
	"../../utils/atomic2"
	"encoding/json"
	"sync"
)

type OpStats struct {
	opstr string
	calls atomic2.Int64 // one operation call number
	usecs atomic2.Int64 // one operation used time
	// add by WangChunyan
	failCalls atomic2.Int64 // one operation failed number
	failUsecs atomic2.Int64 // one operation failed time
}

func (s *OpStats) OpStr() string {
	return s.opstr
}

func (s *OpStats) Calls() int64 {
	return s.calls.Get()
}

func (s *OpStats) USecs() int64 {
	return s.usecs.Get()
}

// Add by WangChunyan
func (s *OpStats) FailCalls() int64 {
	return s.failCalls.Get()
}

func (s *OpStats) FailUsess() int64 {
	return s.failUsecs.Get()
}

// this function not used now
func (s *OpStats) MarshalJSON() ([]byte, error) {
	var m = make(map[string]interface{})
	var calls = s.calls.Get()
	var usecs = s.usecs.Get()
	var fail_calls = s.failCalls.Get()
	var fail_usecs = s.failUsecs.Get()
	var perusecs int64 = 0
	if calls != 0 {
		perusecs = usecs / calls
	}

	m["cmd"] = s.opstr
	m["calls"] = calls
	m["usecs"] = usecs
	m["usecs_percall"] = perusecs
	m["fail_calls"] = fail_calls
	m["fail_usecs"] = fail_usecs
	return json.Marshal(m)
}

// Add by WangChunyan,op stat of every redis
type RedisOpStats struct {
	redisAddr string
	calls     atomic2.Int64
	failcalls atomic2.Int64
	cmdmap    map[string]*OpStats
}

func (s *RedisOpStats) RedisOpCalls() int64 {
	return s.calls.Get()
}

func (s *RedisOpStats) RedisOpFailCalls() int64 {
	return s.failcalls.Get()
}

type StateRedisOp struct {
	RedisAddr string
	Calls     int64
	FailCalls int64
	Cmdmap    []*OpStats
}

var cmdstats struct {
	requests atomic2.Int64 // all success redis command
	// Add by WangChunyan
	allRequests     atomic2.Int64 // all redis command
	failRequests    atomic2.Int64 // all fail redis command
	timeoutRequests atomic2.Int64 // timeout fail redis command
	connReqs        atomic2.Int64 // all connections
	failConnReqs    atomic2.Int64 // all fail connections
	opmap           map[string]*OpStats
	rwlck           sync.RWMutex

	redisopmap map[string]*RedisOpStats
}

func init() {
	cmdstats.opmap = make(map[string]*OpStats)
	cmdstats.redisopmap = make(map[string]*RedisOpStats)
}

func OpCounts() int64 {
	return cmdstats.requests.Get()
}

// Add by WangChunyan
func AllReqs() int64 {
	return cmdstats.allRequests.Get()
}

func FailReqs() int64 {
	return cmdstats.failRequests.Get()
}

func TimeoutReqs() int64 {
	return cmdstats.timeoutRequests.Get()
}

func AllConns() int64 {
	return cmdstats.connReqs.Get()
}

func FailConns() int64 {
	return cmdstats.failConnReqs.Get()
}

func GetOpStats(opstr string, create bool) *OpStats {
	cmdstats.rwlck.RLock()
	s := cmdstats.opmap[opstr]
	cmdstats.rwlck.RUnlock()

	if s != nil || !create {
		return s
	}

	cmdstats.rwlck.Lock()
	s = cmdstats.opmap[opstr]
	if s == nil {
		s = &OpStats{opstr: opstr}
		cmdstats.opmap[opstr] = s
	}
	cmdstats.rwlck.Unlock()
	return s
}

// Add by WangChunyan
func GetRedisOpStats(redisAddr string, opstr string) (*RedisOpStats, *OpStats) {
	cmdstats.rwlck.RLock()
	s := cmdstats.redisopmap[redisAddr]
	cmdstats.rwlck.RUnlock()
	// find redis map
	if s != nil {
		cmdstats.rwlck.RLock()
		rs := s.cmdmap[opstr]
		cmdstats.rwlck.RUnlock()
		// find op map
		if rs != nil {
			return s, rs
		} else {
			cmdstats.rwlck.Lock()
			rs = &OpStats{opstr: opstr}
			s.cmdmap[opstr] = rs
			cmdstats.rwlck.Unlock()
			return s, rs
		}
	}
	s = &RedisOpStats{redisAddr: redisAddr}
	s.cmdmap = make(map[string]*OpStats)
	rs := &OpStats{opstr: opstr}
	s.cmdmap[opstr] = rs
	cmdstats.rwlck.Lock()
	cmdstats.redisopmap[redisAddr] = s
	cmdstats.rwlck.Unlock()
	return s, rs
}

func GetAllOpStats() []*OpStats {
	var all = make([]*OpStats, 0, 128)
	cmdstats.rwlck.RLock()
	for _, s := range cmdstats.opmap {
		all = append(all, s)
	}
	cmdstats.rwlck.RUnlock()
	return all
}

func GetAllRedisOpStats() []*StateRedisOp {
	var maxkeynum int = 0
	var all = make([]*StateRedisOp, 0, 64)
	cmdstats.rwlck.RLock()
	for key, s := range cmdstats.redisopmap {
		maxkeynum++
		var rediscmdmap = &StateRedisOp{
			RedisAddr: key,
			Calls:     s.RedisOpCalls(),
			FailCalls: s.RedisOpFailCalls(),
			Cmdmap:    make([]*OpStats, 0, 128),
		}
		var maxrs int = 0
		for _, rs := range s.cmdmap {
			maxrs++
			rediscmdmap.Cmdmap = append(rediscmdmap.Cmdmap, rs)
		}
		all = append(all, rediscmdmap)
	}
	cmdstats.rwlck.RUnlock()
	return all
}

func incrOpStats(opstr string, usecs int64) {
	s := GetOpStats(opstr, true)
	s.calls.Incr()
	s.usecs.Add(usecs)
	cmdstats.requests.Incr()
}

func incrRedisOpStats(redisAddr string, opstr string, usecs int64) {
	rs, s := GetRedisOpStats(redisAddr, opstr)
	s.calls.Incr()
	s.usecs.Add(usecs)
	rs.calls.Incr()
}

func incrRedisFailOpStats(redisAddr string, opstr string, usecs int64) {
	rs, s := GetRedisOpStats(redisAddr, opstr)
	s.failCalls.Incr()
	s.failUsecs.Add(usecs)
	rs.failcalls.Incr()
}

// Add by WangChunyan
func incrAllRequests() {
	cmdstats.allRequests.Incr()
}

func incrFailRequests() {
	cmdstats.failRequests.Incr()
}

func incrFailOpStats(opstr string, usecs int64) {
	s := GetOpStats(opstr, true)
	s.failCalls.Incr()
	s.failUsecs.Add(usecs)
}

func incrTimeoutOpStats() {
	cmdstats.timeoutRequests.Incr()
}

func incrConnReqs() {
	cmdstats.connReqs.Incr()
}

func incrFailConnReqs() {
	cmdstats.failConnReqs.Incr()
}
