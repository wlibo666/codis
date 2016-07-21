// Copyright 2014 Wandoujia Inc. All Rights Reserved.
// Licensed under the MIT (MIT-LICENSE.txt) license.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"time"

	"github.com/wandoulabs/go-zookeeper/zk"
	"github.com/wandoulabs/zkhelper"

	REDIS "github.com/garyburd/redigo/redis"
	"github.com/wlibo666/codis/pkg/models"
	"github.com/wlibo666/codis/pkg/utils/log"
)

const (
	MAX_LOCK_TIMEOUT = 10 * time.Second
)

const (
	MIGRATE_TASK_PENDING   string = "pending"
	MIGRATE_TASK_MIGRATING string = "migrating"
	MIGRATE_TASK_FINISHED  string = "finished"
	MIGRATE_TASK_ERR       string = "error"
)

var PerSlotMigrateNum int

// check if migrate task is valid
type MigrateTaskCheckFunc func(t *MigrateTask) (bool, error)

// migrate task will store on zk
type MigrateManager struct {
	runningTask *MigrateTask
	zkConn      zkhelper.Conn
	productName string
}

func getMigrateTasksPath(product string) string {
	return fmt.Sprintf("/zk/codis/db_%s/migrate_tasks", product)
}

func NewMigrateManager(zkConn zkhelper.Conn, pn string) *MigrateManager {
	m := &MigrateManager{
		zkConn:      zkConn,
		productName: pn,
	}
	zkhelper.CreateRecursive(m.zkConn, getMigrateTasksPath(m.productName), "", 0, zkhelper.DefaultDirACLs())
	m.mayRecover()
	go m.loop()
	return m
}

// if there are tasks that is not pending, process them.
func (m *MigrateManager) mayRecover() error {
	// It may be not need to do anything now.
	return nil
}

//add a new task to zk
func (m *MigrateManager) PostTask(info *MigrateTaskInfo) {
	b, _ := json.Marshal(info)
	p, _ := safeZkConn.Create(getMigrateTasksPath(m.productName)+"/", b, zk.FlagSequence, zkhelper.DefaultFileACLs())
	_, info.Id = path.Split(p)
}

func (m *MigrateManager) loop() error {
	for {
		PerSlotMigrateNum = 0
		time.Sleep(time.Second)
		info := m.NextTask()
		if info == nil {
			// no migrate task,sleep 5 seconds
			time.Sleep(time.Second * 5)
			continue
		}
		t := GetMigrateTask(*info)
		err := t.preMigrateCheck()
		if err != nil {
			log.ErrorErrorf(err, "pre migrate check failed,taskId:%d,slotId:%d", t.Id, t.SlotId)
		}
		// if can not connect redis,skip this task
		err = m.checkProxyAndServer()
		if err != nil {
			log.ErrorErrorf(err, "some redis is not alive,delete task[%s],slotId[%d]", t.Id, t.SlotId)
			t.UpdateFinish()
			continue
		}

		err = t.run()
		if err != nil {
			t.UpdateFinish()
			log.ErrorErrorf(err, "migrate failed,now will delete this task Id[%s],slotid[%d] and sleep 15s,error:%s", t.Id, t.SlotId, err.Error())
			time.Sleep(time.Second * 15)
		}
		log.Infof("migrate slot [%d],success num [%d]", t.SlotId, PerSlotMigrateNum)
	}
}

func (m *MigrateManager) NextTask() *MigrateTaskInfo {
	ts := m.Tasks()
	if len(ts) == 0 {
		return nil
	}
	return &ts[0]
}

func (m *MigrateManager) Tasks() []MigrateTaskInfo {
	res := Tasks{}
	tasks, _, _ := safeZkConn.Children(getMigrateTasksPath(m.productName))
	for _, id := range tasks {
		data, _, _ := safeZkConn.Get(getMigrateTasksPath(m.productName) + "/" + id)
		info := new(MigrateTaskInfo)
		json.Unmarshal(data, info)
		info.Id = id
		res = append(res, *info)
	}
	sort.Sort(res)
	return res
}

type Tasks []MigrateTaskInfo

func (t Tasks) Len() int {
	return len(t)
}

func (t Tasks) Less(i, j int) bool {
	return t[i].Id <= t[j].Id
}

func (t Tasks) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func getProxyPath(product string) string {
	return fmt.Sprintf("/zk/codis/db_%s/proxy", product)
}

type Proxys []models.ProxyInfo

func (m *MigrateManager) allProxy() []models.ProxyInfo {
	res := Proxys{}
	pPath := getProxyPath(m.productName)
	ps, _, _ := safeZkConn.Children(pPath)
	for _, p := range ps {
		data, _, _ := safeZkConn.Get(pPath + "/" + p)
		tmpP := new(models.ProxyInfo)
		json.Unmarshal(data, tmpP)
		res = append(res, *tmpP)
	}
	return res
}

func getServerPath(product string) string {
	return fmt.Sprintf("/zk/codis/db_%s/servers", product)
}

type Servers []models.Server

func (m *MigrateManager) allServer() []models.Server {
	res := Servers{}
	gPath := getServerPath(m.productName)
	groups, _, _ := safeZkConn.Children(gPath)
	for _, group := range groups {
		sers, _, _ := safeZkConn.Children(gPath + "/" + group)
		for _, ser := range sers {
			data, _, _ := safeZkConn.Get(gPath + "/" + group + "/" + ser)
			tmpS := new(models.Server)
			json.Unmarshal(data, tmpS)
			res = append(res, *tmpS)
		}
	}
	return res
}

func pingRedis(addr string) error {
	maxTry := 3
	tryTime := 0
	for tryTime < maxTry {
		conn, err := REDIS.DialTimeout("tcp", addr, 3*time.Second, 3*time.Second, 3*time.Second)
		if err != nil {
			tryTime += 1
			log.Infof("dial redis [%s] failed,trytimes:[%d], err:%s", addr, tryTime, err.Error())
			time.Sleep(time.Second)
			continue
		}
		tryTime += 1
		_, err = conn.Do("PING")
		if err != nil {
			log.Infof("ping from redis [%s] failed,err:%s", addr, err.Error())
			conn.Close()
			time.Sleep(time.Second)
			continue
		}

		conn.Close()
		return nil
	}
	return errors.New(fmt.Sprintf("connect redis[%s] timeout", addr))
}

func (m *MigrateManager) checkProxyAndServer() error {
	servers := m.allServer()
	for _, s := range servers {
		//log.Warnf("server:[%s],role[%s],group[%d]", s.Addr, s.Type, s.GroupId)
		if s.Type == models.SERVER_TYPE_MASTER {
			err := pingRedis(s.Addr)
			if err != nil {
				return err
			}
		}
	}
	// one proxy is down,should not affect service
	//proxys := m.allProxy()
	/*for _, p := range proxys {
		//log.Warnf("proxy:[%s],addr[%s],debugaddr[%s],status[%s]", p.Id, p.Addr, p.DebugVarAddr, p.State)
		if p.State == models.PROXY_STATE_ONLINE {

		}
	}*/
	return nil
}
