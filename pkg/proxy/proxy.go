// Copyright 2014 Wandoujia Inc. All Rights Reserved.
// Licensed under the MIT (MIT-LICENSE.txt) license.

package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wandoulabs/go-zookeeper/zk"
	topo "github.com/wandoulabs/go-zookeeper/zk"
	"github.com/wlibo666/codis/pkg/models"
	"github.com/wlibo666/codis/pkg/proxy/router"
	"github.com/wlibo666/codis/pkg/utils/log"
)

type Server struct {
	conf   *Config
	topo   *Topology
	info   models.ProxyInfo
	groups map[int]int

	lastActionSeq int

	evtbus   chan interface{}
	GRouter  *router.Router
	listener net.Listener

	kill chan interface{}
	wait sync.WaitGroup
	stop sync.Once
}

var GlobalProxy *Server

func StoreGlobalProxy(s *Server) {
	GlobalProxy = s
}

func CleanInvalidRedis() {
	for key, _ := range router.CmdStats.RedisOpMap {
		//if key is not master redis,delete it
		if GlobalProxy.GRouter.Pool[key] == nil {
			log.Warnf("redis [%s] is not master,delete its StatInfo", key)
			delete(router.CmdStats.RedisOpMap, key)
		}
	}
}

// 获取代理的slot状态
// 用于向监控程序提供slot信息，若与zk上不一致时报警
// 原因是网络不好时proxy可能收不到zk的通知
type MoniSlot struct {
	ID       int    `json:"slotid"`
	Master   string `json:"master"`
	IsMgrt   int    `json:"ismgrt"`
	MgrtFrom string `json:"mgrtfrom"`
}

func GetProxySlots() []*MoniSlot {
	slots := make([]*MoniSlot, 0, router.MaxSlotNum)

	for _, s := range GlobalProxy.GRouter.GSlots {
		var tmpIsMgrt int = 0
		var tmpMgrtFrom string = ""
		if s.Migrate.From == "" {
			tmpIsMgrt = 0
		} else {
			tmpIsMgrt = 1
			tmpMgrtFrom = s.Migrate.From
		}
		tmps := &MoniSlot{
			ID:       s.Id,
			Master:   s.Backend.Addr,
			IsMgrt:   tmpIsMgrt,
			MgrtFrom: tmpMgrtFrom,
		}
		slots = append(slots, tmps)
	}
	return slots
}

// changed WangChunyan
// addr 地址为指定的IP地址端口，发送给zookeeper，但是默认监听所有端口
func New(addr string, debugVarAddr string, conf *Config) *Server {
	log.Infof("create proxy with config: %+v", conf)

	proxyHost := strings.Split(addr, ":")[0]
	debugHost := strings.Split(debugVarAddr, ":")[0]

	hostname, err := os.Hostname()
	if err != nil {
		log.PanicErrorf(err, "get host name failed")
	}
	if proxyHost == "0.0.0.0" || strings.HasPrefix(proxyHost, "127.0.0.") || proxyHost == "" {
		proxyHost = hostname
	}
	if debugHost == "0.0.0.0" || strings.HasPrefix(debugHost, "127.0.0.") || debugHost == "" {
		debugHost = hostname
	}

	s := &Server{conf: conf, lastActionSeq: -1, groups: make(map[int]int)}
	s.topo = NewTopo(conf.productName, conf.zkAddr, conf.fact, conf.provider, conf.zkSessionTimeout)
	s.topo.proxyServer = s // changed WangChunyan
	s.info.Id = conf.proxyId
	s.info.State = models.PROXY_STATE_OFFLINE
	s.info.Addr = proxyHost + ":" + strings.Split(addr, ":")[1]
	s.info.DebugVarAddr = debugHost + ":" + strings.Split(debugVarAddr, ":")[1]
	s.info.Pid = os.Getpid()
	s.info.StartAt = time.Now().String()
	s.kill = make(chan interface{})

	log.Infof("proxy info = %+v", s.info)

	// changed WangChunyan
	// addr为指定的IP地址，但是需要监控所有的网卡(Listen不支持指定多IP参数)
	if l, err := net.Listen(conf.proto, ":"+strings.Split(addr, ":")[1]); err != nil {
		log.PanicErrorf(err, "open listener failed")
	} else {
		s.listener = l
	}
	s.GRouter = router.NewWithAuth(conf.passwd)
	s.evtbus = make(chan interface{}, 1024)

	s.register()

	s.wait.Add(1)
	go func() {
		defer s.wait.Done()
		s.serve()
	}()
	return s
}

func (s *Server) SetMyselfOnline() error {
	log.Info("mark myself online")
	info := models.ProxyInfo{
		Id:    s.conf.proxyId,
		State: models.PROXY_STATE_ONLINE,
	}
	b, _ := json.Marshal(info)
	url := "http://" + s.conf.dashboardAddr + "/api/proxy"
	res, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	if res.StatusCode != 200 {
		return errors.New("response code is not 200")
	}
	return nil
}

func (s *Server) serve() {
	defer s.close()

	if !s.waitOnline() {
		return
	}

	s.rewatchNodes()

	for i := 0; i < router.MaxSlotNum; i++ {
		s.fillSlot(i)
	}
	log.Info("proxy is serving")
	go func() {
		defer s.close()
		s.handleConns()
	}()

	s.loopEvents()
}

func (s *Server) handleConns() {
	ch := make(chan net.Conn, 4096)
	defer close(ch)

	go func() {
		for c := range ch {
			x := router.NewSessionSize(s.GRouter, c, s.conf.passwd, s.conf.maxBufSize, s.conf.maxTimeout)
			go x.Serve(s.GRouter, s.conf.maxPipeline)
		}
	}()
	var acceptFailTimes int64 = 0
	for {
		c, err := s.listener.Accept()
		if err != nil {
			acceptFailTimes++
			// changed WangChunyan
			// date: 2015.12.01 15:23:00
			// 接受外部连接出错时起始不应该退出,可以打印警告信息让管理员知道
			if acceptFailTimes%10000 == 0 {
				log.Warnf("listener.Accept failed times [%v],error [%v],will sllep 1s.", acceptFailTimes, err.Error())
				time.Sleep(time.Second)
			}
		} else {
			ch <- c
		}
	}
}

func (s *Server) Info() models.ProxyInfo {
	return s.info
}

func (s *Server) Join() {
	s.wait.Wait()
}

func (s *Server) Close() error {
	s.close()
	s.wait.Wait()
	return nil
}

func (s *Server) close() {
	s.stop.Do(func() {
		s.listener.Close()
		if s.GRouter != nil {
			s.GRouter.Close()
		}
		close(s.kill)
	})
}

func (s *Server) rewatchProxy() {
	_, err := s.topo.WatchNode(path.Join(models.GetProxyPath(s.topo.ProductName), s.info.Id), s.evtbus)
	if err != nil {
		// changed WangChunyan
		//log.PanicErrorf(err, "watch node failed")
		log.Warn("Server rewatchProxy() failed,err [%v]", err.Error())
		s.topo.reConnZk()
	}
}

func (s *Server) rewatchNodes() []string {
	nodes, err := s.topo.WatchChildren(models.GetWatchActionPath(s.topo.ProductName), s.evtbus)
	if err != nil {
		var empty []string
		// changed WangChunyan
		// log.PanicErrorf(err, "watch children failed")
		log.Warn("Server rewatchNodes() failed,err:[%v]", err.Error())
		s.topo.reConnZk()
		return empty
	}
	return nodes
}

func (s *Server) register() {
	// changed WangChunyan
	if _, err := s.topo.CreateProxyInfo(&s.info); err != nil {
		log.Warnf("Server register() create proxy node failed,err: %v", err.Error())
		//log.PanicErrorf(err, "create proxy node failed")
	}
	// changed WangChunyan
	if _, err := s.topo.CreateProxyFenceNode(&s.info); err != nil && err != zk.ErrNodeExists {
		//log.PanicErrorf(err, "create fence node failed")
		log.Warn("Server register() create fence node failed,err: %v", err.Error())
	}
	log.Warn("********** Attention **********")
	log.Warn("You should use `kill {pid}` rather than `kill -9 {pid}` to stop me,")
	log.Warn("or the node resisted on zk will not be cleaned when I'm quiting and you must remove it manually")
	log.Warn("*******************************")
}

func (s *Server) markOffline() {
	s.topo.Close(s.info.Id)
	s.info.State = models.PROXY_STATE_MARK_OFFLINE
}

func (s *Server) waitOnline() bool {
	var trytimes int = 0
	for {
		// changed WangChunyan
		info, err := s.topo.GetProxyInfo(s.info.Id)
		if err != nil {
			trytimes++
			//log.PanicErrorf(err, "waitOnline:get proxy info failed: %s", s.info.Id)
			log.Warnf("Server waitOnline() failed,err: %v", err.Error())
			if trytimes >= 60 {
				log.Errorf("Server waitOnline [%d] times,will exit.", trytimes)
				os.Exit(0)
			} else {
				time.Sleep(1 * time.Second)
			}
			continue
		}
		trytimes = 0
		switch info.State {
		case models.PROXY_STATE_MARK_OFFLINE:
			log.Infof("waitOnline mark offline, proxy got offline event: %s", s.info.Id)
			s.markOffline()
			return false
		case models.PROXY_STATE_ONLINE:
			s.info.State = info.State
			log.Infof("we are online: %s", s.info.Id)
			s.rewatchProxy()
			return true
		}
		select {
		case <-s.kill:
			log.Infof("mark offline, proxy is killed: %s", s.info.Id)
			s.markOffline()
			return false
		default:
		}
		log.Infof("wait to be online: %s", s.info.Id)
		time.Sleep(3 * time.Second)
	}
}

func getEventPath(evt interface{}) string {
	return evt.(topo.Event).Path
}

func needResponse(receivers []string, self models.ProxyInfo) bool {
	var info models.ProxyInfo
	for _, v := range receivers {
		err := json.Unmarshal([]byte(v), &info)
		if err != nil {
			if v == self.Id {
				return true
			}
			return false
		}
		if info.Id == self.Id && info.Pid == self.Pid && info.StartAt == self.StartAt {
			return true
		}
	}
	return false
}

func groupMaster(groupInfo models.ServerGroup) string {
	var master string
	for _, server := range groupInfo.Servers {
		if server.Type == models.SERVER_TYPE_MASTER {
			if master != "" {
				log.Panicf("two master not allowed: %+v", groupInfo)
			}
			master = server.Addr
		}
	}
	if master == "" {
		log.Panicf("master not found: %+v", groupInfo)
	}
	return master
}

func (s *Server) resetSlot(i int) {
	s.GRouter.ResetSlot(i)
}

func (s *Server) fillSlot(i int) {
	slotInfo, slotGroup, err := s.topo.GetSlotByIndex(i)
	if err != nil {
		// changed WangChunyan
		// fillSlot失败应该退出
		log.PanicErrorf(err, "Server fillSlot() get slot by index failed", i)
	}

	var from string
	var addr = groupMaster(*slotGroup)
	if slotInfo.State.Status == models.SLOT_STATUS_MIGRATE {
		fromGroup, err := s.topo.GetGroup(slotInfo.State.MigrateStatus.From)
		if err != nil {
			// changed WangChunyan
			// fillSlot失败应该退出
			log.PanicErrorf(err, "get migrate from failed")
		}
		from = groupMaster(*fromGroup)
		if from == addr {
			// changed WangChunyan
			// fillSlot失败应该退出
			log.Panicf("set slot %04d migrate from %s to %s", i, from, addr)
		}
	}

	s.groups[i] = slotInfo.GroupId
	s.GRouter.FillSlot(i, addr, from,
		slotInfo.State.Status == models.SLOT_STATUS_PRE_MIGRATE)
}

func (s *Server) onSlotRangeChange(param *models.SlotMultiSetParam) {
	log.Infof("slotRangeChange %+v", param)
	for i := param.From; i <= param.To; i++ {
		switch param.Status {
		case models.SLOT_STATUS_OFFLINE:
			s.resetSlot(i)
		case models.SLOT_STATUS_ONLINE:
			s.fillSlot(i)
		default:
			// changed WangChunyan
			log.Panicf("can not handle status %v", param.Status)
		}
	}
}

func (s *Server) onGroupChange(groupId int) {
	log.Infof("group changed %d", groupId)
	for i, g := range s.groups {
		if g == groupId {
			s.fillSlot(i)
		}
	}
}

func (s *Server) responseAction(seq int64) {
	log.Infof("send response seq = %d", seq)
	err := s.topo.DoResponse(int(seq), &s.info)
	if err != nil {
		log.InfoErrorf(err, "send response seq = %d failed", seq)
	}
}

func (s *Server) getActionObject(seq int, target interface{}) {
	act := &models.Action{Target: target}
	// changed WangChunyan
	err := s.topo.GetActionWithSeqObject(int64(seq), act)
	if err != nil {
		//log.PanicErrorf(err, "get action object failed, seq = %d", seq)
		log.Warnf("Server getActionObject failed,err: %v", err.Error())
		s.topo.reConnZk()
	}
	log.Infof("action %+v", act)
}

func (s *Server) checkAndDoTopoChange(seq int) bool {
	act, err := s.topo.GetActionWithSeq(int64(seq))
	// changed WangChunyan
	if err != nil { //todo: error is not "not exist"
		//log.PanicErrorf(err, "action failed, seq = %d", seq)
		log.Warnf("checkAndDoTopoChange failed,err :%v", err.Error())
		return false
	}

	if !needResponse(act.Receivers, s.info) { //no need to response
		return false
	}

	log.Warnf("action %v receivers %v", seq, act.Receivers)

	switch act.Type {
	case models.ACTION_TYPE_SLOT_MIGRATE, models.ACTION_TYPE_SLOT_CHANGED,
		models.ACTION_TYPE_SLOT_PREMIGRATE:
		slot := &models.Slot{}
		s.getActionObject(seq, slot)
		s.fillSlot(slot.Id)
	case models.ACTION_TYPE_SERVER_GROUP_CHANGED:
		serverGroup := &models.ServerGroup{}
		s.getActionObject(seq, serverGroup)
		s.onGroupChange(serverGroup.Id)
	case models.ACTION_TYPE_SERVER_GROUP_REMOVE:
	//do not care
	case models.ACTION_TYPE_MULTI_SLOT_CHANGED:
		param := &models.SlotMultiSetParam{}
		s.getActionObject(seq, param)
		s.onSlotRangeChange(param)
	default:
		// changed WangChunyan
		log.Panicf("unknown action %+v", act)
	}
	return true
}

func (s *Server) processAction(e interface{}) {
	if strings.Index(getEventPath(e), models.GetProxyPath(s.topo.ProductName)) == 0 {
		info, err := s.topo.GetProxyInfo(s.info.Id)
		if err != nil {
			// changed WangChunyan
			// date: 2015.12.02 18:34:00
			// 本来获取代理信息出错直接就退出了，此处改为记录信息不退出
			//log.PanicErrorf(err, "processAction :get proxy info failed: %s", s.info.Id)
			log.Errorf("processAction :get proxy info failed: %s,err %v", s.info.Id, err.Error())
			return
		}
		switch info.State {
		case models.PROXY_STATE_MARK_OFFLINE:
			log.Infof("processAction mark offline, proxy got offline event: %s", s.info.Id)
			s.markOffline()
		case models.PROXY_STATE_ONLINE:
			s.rewatchProxy()
		default:
			// changed WangChunyan
			log.Panicf("unknown proxy state %v", info)
		}
		return
	}

	//re-watch
	nodes := s.rewatchNodes()

	seqs, err := models.ExtraSeqList(nodes)
	if err != nil {
		// changed WangChunyan
		log.Warnf("processAction failed at ExtraSeqList,err: %v", err.Error())
		return
		//log.PanicErrorf(err, "get seq list failed")
	}

	if len(seqs) == 0 || !s.topo.IsChildrenChangedEvent(e) {
		return
	}

	//get last pos
	index := -1
	for i, seq := range seqs {
		if s.lastActionSeq < seq {
			index = i
			//break
			//only handle latest action
		}
	}

	if index < 0 {
		return
	}

	actions := seqs[index:]
	for _, seq := range actions {
		// changed WangChunyan
		exist, err := s.topo.Exist(path.Join(s.topo.GetActionResponsePath(seq), s.info.Id))
		if err != nil {
			//log.PanicErrorf(err, "get action failed")
			log.Warnf("get action failed,err: %v", err.Error())
			return
		}
		if exist {
			continue
		}
		if s.checkAndDoTopoChange(seq) {
			s.responseAction(int64(seq))
		}
	}

	s.lastActionSeq = seqs[len(seqs)-1]
}

func (s *Server) loopEvents() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var tick int = 0
	for s.info.State == models.PROXY_STATE_ONLINE {
		select {
		case <-s.kill:
			log.Infof("mark offline, proxy is killed: %s", s.info.Id)
			s.markOffline()
		case e := <-s.evtbus:
			evtPath := getEventPath(e)
			log.Infof("got event %s, %v, lastActionSeq %d", s.info.Id, e, s.lastActionSeq)
			if strings.Index(evtPath, models.GetActionResponsePath(s.conf.productName)) == 0 {
				seq, err := strconv.Atoi(path.Base(evtPath))
				if err != nil {
					log.ErrorErrorf(err, "parse action seq failed")
				} else {
					if seq < s.lastActionSeq {
						log.Infof("ignore seq = %d", seq)
						continue
					}
				}
			}
			s.processAction(e)
		case <-ticker.C:
			if maxTick := s.conf.pingPeriod; maxTick != 0 {
				if tick++; tick >= maxTick {
					s.GRouter.KeepAlive()
					tick = 0
				}
			}
		}
	}
}
