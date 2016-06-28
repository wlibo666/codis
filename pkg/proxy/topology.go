// Copyright 2014 Wandoujia Inc. All Rights Reserved.
// Licensed under the MIT (MIT-LICENSE.txt) license.

package proxy

import (
	"encoding/json"
	"path"
	"strings"
	//"sync"
	"time"

	topo "github.com/wandoulabs/go-zookeeper/zk"

	"github.com/wlibo666/codis/pkg/models"
	"github.com/wlibo666/codis/pkg/utils/errors"
	"github.com/wlibo666/codis/pkg/utils/log"
	"github.com/wandoulabs/zkhelper"
)

type TopoUpdate interface {
	OnGroupChange(groupId int)
	OnSlotChange(slotId int)
}

type ZkFactory func(zkAddr string, zkSessionTimeout int) (zkhelper.Conn, error)

type Topology struct {
	ProductName      string
	zkAddr           string
	zkConn           zkhelper.Conn
	fact             ZkFactory
	provider         string
	zkSessionTimeout int
	proxyServer      *Server // changed WangChunyan 此处记录代理的地址
}

func (top *Topology) GetGroup(groupId int) (*models.ServerGroup, error) {
	return models.GetGroup(top.zkConn, top.ProductName, groupId)
}

func (top *Topology) Exist(path string) (bool, error) {
	return zkhelper.NodeExists(top.zkConn, path)
}

func (top *Topology) GetSlotByIndex(i int) (*models.Slot, *models.ServerGroup, error) {
	slot, err := models.GetSlot(top.zkConn, top.ProductName, i)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	groupServer, err := models.GetGroup(top.zkConn, top.ProductName, slot.GroupId)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	return slot, groupServer, nil
}

func NewTopo(ProductName string, zkAddr string, f ZkFactory, provider string, zkSessionTimeout int) *Topology {
	t := &Topology{zkAddr: zkAddr, ProductName: ProductName, fact: f, provider: provider, zkSessionTimeout: zkSessionTimeout}
	if t.fact == nil {
		switch t.provider {
		case "etcd":
			t.fact = zkhelper.NewEtcdConn
		case "zookeeper":
			t.fact = zkhelper.ConnectToZk
		default:
			log.Panicf("coordinator not found in config")
		}
	}
	t.InitZkConn()
	return t
}

func (top *Topology) InitZkConn() {
	var err error
	var retryZkConnTimes int = 30
	// changed WangChunyan
	for i := 0; i < retryZkConnTimes; i++ {
		top.zkConn, err = top.fact(top.zkAddr, top.zkSessionTimeout)
		if err != nil {
			log.Warn("InitZkConn failed at times [%d],error [%v]", i, err.Error())
			time.Sleep(time.Second)
			continue
			//log.PanicErrorf(err, "InitZkConn init failed")
		}
		/*ZkMutexLock.Lock()
		ZkClosedFlag = false
		ZkMutexLock.Unlock()*/

		break
	}
}

func (top *Topology) GetActionWithSeq(seq int64) (*models.Action, error) {
	return models.GetActionWithSeq(top.zkConn, top.ProductName, seq, top.provider)
}

func (top *Topology) GetActionWithSeqObject(seq int64, act *models.Action) error {
	return models.GetActionObject(top.zkConn, top.ProductName, seq, act, top.provider)
}

func (top *Topology) GetActionSeqList(productName string) ([]int, error) {
	return models.GetActionSeqList(top.zkConn, productName)
}

func (top *Topology) IsChildrenChangedEvent(e interface{}) bool {
	return e.(topo.Event).Type == topo.EventNodeChildrenChanged
}

func (top *Topology) CreateProxyInfo(pi *models.ProxyInfo) (string, error) {
	return models.CreateProxyInfo(top.zkConn, top.ProductName, pi)
}

func (top *Topology) CreateProxyFenceNode(pi *models.ProxyInfo) (string, error) {
	return models.CreateProxyFenceNode(top.zkConn, top.ProductName, pi)
}

func (top *Topology) GetProxyInfo(proxyName string) (*models.ProxyInfo, error) {
	return models.GetProxyInfo(top.zkConn, top.ProductName, proxyName)
}

func (top *Topology) GetActionResponsePath(seq int) string {
	return path.Join(models.GetActionResponsePath(top.ProductName), top.zkConn.Seq2Str(int64(seq)))
}

func (top *Topology) SetProxyStatus(proxyName string, status string) error {
	return models.SetProxyStatus(top.zkConn, top.ProductName, proxyName, status)
}

func (top *Topology) Close(proxyName string) {
	// delete fence znode
	pi, err := models.GetProxyInfo(top.zkConn, top.ProductName, proxyName)
	if err != nil {
		log.Errorf("killing fence error, proxy %s is not exists", proxyName)
	} else {
		zkhelper.DeleteRecursive(top.zkConn, path.Join(models.GetProxyFencePath(top.ProductName), pi.Addr), -1)
	}
	// delete ephemeral znode
	zkhelper.DeleteRecursive(top.zkConn, path.Join(models.GetProxyPath(top.ProductName), proxyName), -1)
	top.zkConn.Close()
}

func (top *Topology) DoResponse(seq int, pi *models.ProxyInfo) error {
	//create response node
	actionPath := top.GetActionResponsePath(seq)
	//log.Debug("actionPath:", actionPath)
	data, err := json.Marshal(pi)
	if err != nil {
		return errors.Trace(err)
	}

	_, err = top.zkConn.Create(path.Join(actionPath, pi.Id), data,
		0, zkhelper.DefaultFileACLs())

	return err
}

//var ZkClosedFlag bool = false
//var ZkMutexLock sync.Mutex

// Add WangChunyan
func (top *Topology) reConnZk() {
	log.Warn("some errors happened, now will exec Topology reConnZk()")
	/*ZkMutexLock.Lock()
	if ZkClosedFlag == false {
		top.zkConn.Close()
	}
	ZkClosedFlag = true
	ZkMutexLock.Unlock()*/
	top.InitZkConn()
	log.Info("now will register proxy again")
	top.proxyServer.register()
}

func (top *Topology) doWatch(evtch <-chan topo.Event, evtbus chan interface{}) {
	e := <-evtch

	// changed WangChunyan
	// date: 2015.12.02 13:38:00
	// 从zookeeper收到session超时消息
	// 可以尝试让代理重连zookeeper，而不是代理退出
	if e.State == topo.StateExpired || e.Type == topo.EventNotWatching {
		//log.Panicf("session expired: %+v", e)
		log.Warn("doWatch() event state is StateExpired,now will reconnect to zookeeper...")
		top.reConnZk()
		return
	}

	log.Warnf("topo event %+v", e)

	switch e.Type {
	//case topo.EventNodeCreated:
	//case topo.EventNodeDataChanged:
	case topo.EventNodeChildrenChanged: //only care children changed
		//todo:get changed node and decode event
	// changed WangChunyan
	// zk删除自身节点时重新创建proxy节点
	case topo.EventNodeDeleted:
		if strings.Contains(e.Path, top.proxyServer.conf.proxyId) {
			log.Warnf("now will create proxy info [%s] again", e.Path)
			top.CreateProxyInfo(&top.proxyServer.info)
		}
	default:
		log.Warnf("%+v", e)
	}

	evtbus <- e
}

func (top *Topology) WatchChildren(path string, evtbus chan interface{}) ([]string, error) {
	content, _, evtch, err := top.zkConn.ChildrenW(path)
	if err != nil {
		return nil, errors.Trace(err)
	}

	go top.doWatch(evtch, evtbus)
	return content, nil
}

func (top *Topology) WatchNode(path string, evtbus chan interface{}) ([]byte, error) {
	content, _, evtch, err := top.zkConn.GetW(path)
	if err != nil {
		return nil, errors.Trace(err)
	}

	go top.doWatch(evtch, evtbus)
	return content, nil
}
