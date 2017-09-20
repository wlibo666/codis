package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"minerva/scloud/stargazer-base-lib/log"
	"strconv"
)

const (
	snapShotLen int32 = 1024
)

var (
	deviceName  = flag.String("dev", "eth0", "-dev eth0")
	promiscuous = flag.Bool("promis", false, "-promis false(network card promiscuous mode)")
	logfile     = flag.String("log", "./capture.log", "-log capture.log")
	lognum      = flag.Int("lognum", 4, "-lognum 3")
	debug       = flag.Bool("debug", false, "-debug true|false")

	srcip   = flag.String("srcip", "", "-srcip 127.0.0.1,10.58.80.254")
	dstip   = flag.String("dstip", "", "-dstip 127.0.0.1,10.58.80.254")
	srcport = flag.String("srcport", "", "-srcport 80,8080")
	dstport = flag.String("dstport", "", "-dstport 80,8080")
	minlen  = flag.Int("minlen", 16, "-minlen 32")

	redisCmd  = flag.String("rediscmd", "DEL,db_apps;HDEL,db_apps", "-rediscmd DEL,db_apps;HDEL,db_apps")
	recordCnt = flag.Int64("count", -1, "-count 100")

	cmds     [][]string
	srcips   []string
	dstips   []string
	srcports []uint16
	dstports []uint16
	count    int64 = 0
)

func recordCmd(ipv4 *layers.IPv4, tcp *layers.TCP, data []byte) {
	if *recordCnt > 0 && count >= *recordCnt {
		log.Logger.Info("set count:%d,record count:%d,exit", *recordCnt, count)
		os.Exit(0)
		return
	}
	reqCmd := GetReqCmd(data)
	if len(reqCmd) < 2 {
		return
	}

	for _, cmd := range cmds {
		if len(cmd) != 2 {
			continue
		}
		if strings.ToUpper(cmd[0]) == strings.ToUpper(reqCmd[0]) &&
			strings.ToUpper(cmd[1]) == strings.ToUpper(reqCmd[1]) {
			count++
			log.Logger.Info("record srcip:[%s],srcport:[%d],cmd:%v", ipv4.SrcIP.String(), tcp.SrcPort, reqCmd)
		}
	}
}

func CapPkg() {
	log.SetFileLogger(*logfile, *lognum)
	if *debug {
		log.SetLoggerDebug()
	}
	log.Logger.Info("program start,dev:[%s],srcip:[%s],dstip:[%s],srcport:[%d],dstport:[%d],rediscmd:[%s],count:[%d]",
		*deviceName, *srcip, *dstip, *srcport, *dstport, *redisCmd, *recordCnt)
	for _, cmd := range strings.Split(*redisCmd, ";") {
		perCmd := make([]string, 0)
		for _, c := range strings.Split(cmd, ",") {
			perCmd = append(perCmd, c)
		}
		cmds = append(cmds, perCmd)
	}
	log.Logger.Info("redis cmd:%v", cmds)

	for _, ip := range strings.Split(*srcip, ",") {
		srcips = append(srcips, ip)
	}
	log.Logger.Info("srcips:%v", srcips)
	for _, ip := range strings.Split(*dstip, ",") {
		dstips = append(dstips, ip)
	}
	log.Logger.Info("dstips:%v", dstips)
	for _, port := range strings.Split(*srcport, ",") {
		p, err := strconv.Atoi(port)
		if err == nil {
			srcports = append(srcports, uint16(p))
		}
	}
	log.Logger.Info("srcports:%v", srcports)
	for _, port := range strings.Split(*dstport, ",") {
		p, err := strconv.Atoi(port)
		if err == nil {
			dstports = append(dstports, uint16(p))
		}
	}
	log.Logger.Info("dstports:%v", dstports)

	handler, err := pcap.OpenLive(*deviceName, snapShotLen, *promiscuous, 2*time.Second)
	if err != nil {
		log.Logger.Warn("OpenLive failed,err:%s", err.Error())
		return
	}
	defer handler.Close()

	pkgSource := gopacket.NewPacketSource(handler, handler.LinkType())
	for pkg := range pkgSource.Packets() {
		tcpData, ok, ipv4, tcp, _ := GetTcpData(pkg, srcips, dstips, srcports, dstports, uint16(*minlen))
		if !ok {
			continue
		}
		recordCmd(ipv4, tcp, tcpData)
	}
}

func main() {
	flag.Parse()

	CapPkg()
}
