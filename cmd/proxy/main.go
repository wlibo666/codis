// Copyright 2014 Wandoujia Inc. All Rights Reserved.
// Licensed under the MIT (MIT-LICENSE.txt) license.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/docopt/docopt-go"
	//"github.com/ngaut/gostats"
	"github.com/wlibo666/codis/pkg/proxy"
	"github.com/wlibo666/codis/pkg/proxy/router"
	"github.com/wlibo666/codis/pkg/utils"
	"github.com/wlibo666/codis/pkg/utils/bytesize"
	"github.com/wlibo666/codis/pkg/utils/log"
)

var (
	cpus       = 2
	addr       = ":9000"
	httpAddr   = ":9001"
	configFile = "config.ini"
)

var usage = `usage: proxy [-c <config_file>] [-L <log_file>] [--log-level=<loglevel>] [--log-filesize=<filesize>] [--cpu=<cpu_num>] [--addr=<proxy_listen_addr>] [--http-addr=<debug_http_server_addr>]

options:
   -c	set config file
   -L	set output log file, default is stdout
   --log-level=<loglevel>	set log level: info, warn, error, debug [default: info]
   --log-filesize=<maxsize>  set max log file size, suffixes "KB", "MB", "GB" are allowed, 1KB=1024 bytes, etc. Default is 1GB.
   --cpu=<cpu_num>		num of cpu cores that proxy can use
   --addr=<proxy_listen_addr>		proxy listen address, example: 0.0.0.0:9000
   --http-addr=<debug_http_server_addr>		debug vars http server
`

const banner string = `
  _____  ____    ____/ /  (_)  _____
 / ___/ / __ \  / __  /  / /  / ___/
/ /__  / /_/ / / /_/ /  / /  (__  )
\___/  \____/  \__,_/  /_/  /____/

`

func init() {
	log.SetLevel(log.LEVEL_INFO)
}

func setLogLevel(level string) {
	level = strings.ToLower(level)
	var l = log.LEVEL_INFO
	switch level {
	case "error":
		l = log.LEVEL_ERROR
	case "warn", "warning":
		l = log.LEVEL_WARN
	case "debug":
		l = log.LEVEL_DEBUG
	case "info":
		fallthrough
	default:
		level = "info"
		l = log.LEVEL_INFO
	}
	log.SetLevel(l)
	log.Infof("set log level to <%s>", level)
}

func setCrashLog(file string) {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.InfoErrorf(err, "cannot open crash log file: %s", file)
	} else {
		syscall.Dup2(int(f.Fd()), 2)
	}
}

func handleSetLogLevel(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	setLogLevel(r.Form.Get("level"))
}

// Add by WangChunyan,for test,should not use
/*func printAllRedisStats() {
	allredisop := router.GetAllRedisOpStats()
	var i int = len(allredisop)
	log.Infof("state redis len:%d", i)

	for _, redisop := range allredisop {
		log.Infof("redisop addr:%s", redisop.RedisAddr)
		for _, cmdop := range redisop.Cmdmap {
			log.Infof("cmd:%s,call:%d", cmdop.OpStr(), cmdop.Calls())
		}
	}
}*/

func handleDebugVars(w http.ResponseWriter, r *http.Request) {
	//r.ParseForm()
	var m = make(map[string]interface{})
	m["ops"] = router.OpCounts()
	//printAllRedisStats()
	b, _ := json.Marshal(m)
	str := fmt.Sprintf("{ \"router\": %s}", string(b))
	fmt.Fprintf(w, str)
}

func handleMoniData(w http.ResponseWriter, r *http.Request) {
	//r.ParseForm()
	var m = make(map[string]interface{})
	m["conn_num"] = router.AllConns()
	m["conn_fail_num"] = router.FailConns()
	m["op_num"] = router.AllReqs()
	m["op_fail_num"] = router.FailReqs()
	m["op_succ_num"] = router.OpCounts()
	m["cmd_proxy"] = router.GetAllOpStats()
	proxy.CleanInvalidRedis()
	m["cmd_redis"] = router.GetAllRedisOpStats()
	b, _ := json.Marshal(m)
	str := fmt.Sprintf("{ \"router\": %s}", string(b))
	fmt.Fprintf(w, str)
}

/*
func printSlotsInfo() {
	sinfo := proxy.GetProxySlots()

	var slen int = len(sinfo)
	log.Infof("i slot len :%d", slen)

	for i := 0; i < slen; i++ {
		si := sinfo[i]
		if si != nil {
			log.Infof("id:%d,master:%s,ismgrt:%d,mgrtfrom:%s", si.ID, si.Master, si.IsMgrt, si.MgrtFrom)
		} else {
			log.Info("si[%d] is nil", i)
		}
	}
}
*/
func handleSlotsInfo(w http.ResponseWriter, r *http.Request) {
	//printSlotsInfo()
	var m = make(map[string]interface{})
	m["sinfo"] = proxy.GetProxySlots()
	b, _ := json.Marshal(m)
	str := fmt.Sprintf("{\"slots\": %s}", string(b))
	fmt.Fprintf(w, str)
}

func checkUlimit(min int) {
	ulimitN, err := exec.Command("/bin/sh", "-c", "ulimit -n").Output()
	if err != nil {
		log.WarnErrorf(err, "get ulimit failed")
	}

	n, err := strconv.Atoi(strings.TrimSpace(string(ulimitN)))
	if err != nil || n < min {
		log.Panicf("ulimit too small: %d, should be at least %d", n, min)
	}
}

func main() {
	args, err := docopt.Parse(usage, nil, true, utils.Version, true)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Print(banner)

	// set config file
	if args["-c"] != nil {
		configFile = args["-c"].(string)
	}

	var maxFileFrag = 10
	var maxFragSize int64 = bytesize.MB * 128
	if s, ok := args["--log-filesize"].(string); ok && s != "" {
		v, err := bytesize.Parse(s)
		if err != nil {
			log.PanicErrorf(err, "invalid max log file size = %s", s)
		}
		maxFragSize = v
	}

	// set output log file
	if s, ok := args["-L"].(string); ok && s != "" {
		f, err := log.NewRollingFile(s, maxFileFrag, maxFragSize)
		if err != nil {
			log.PanicErrorf(err, "open rolling log file failed: %s", s)
		} else {
			defer f.Close()
			log.StdLog = log.New(f, "")
		}
	}
	log.SetLevel(log.LEVEL_INFO)
	log.SetFlags(log.Flags() | log.Lshortfile)

	// set log level
	if s, ok := args["--log-level"].(string); ok && s != "" {
		setLogLevel(s)
	}
	cpus = runtime.NumCPU()
	// set cpu
	if args["--cpu"] != nil {
		cpus, err = strconv.Atoi(args["--cpu"].(string))
		if err != nil {
			log.PanicErrorf(err, "parse cpu number failed")
		}
	}

	// set addr
	if args["--addr"] != nil {
		addr = args["--addr"].(string)
	}

	// set http addr
	if args["--http-addr"] != nil {
		httpAddr = args["--http-addr"].(string)
	}

	checkUlimit(1024)
	runtime.GOMAXPROCS(cpus)

	http.HandleFunc("/setloglevel", handleSetLogLevel)
	http.HandleFunc("/debug/vars", handleDebugVars)
	http.HandleFunc("/MoniData", handleMoniData)
	http.HandleFunc("/slotsinfo", handleSlotsInfo)
	go func() {
		err := http.ListenAndServe(httpAddr, nil)
		log.PanicError(err, "http debug server quit")
	}()
	log.Info("running on ", addr)
	conf, err := proxy.LoadConf(configFile)
	if err != nil {
		log.PanicErrorf(err, "load config failed")
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, os.Kill)

	s := proxy.New(addr, httpAddr, conf)
	defer s.Close()
	proxy.StoreGlobalProxy(s)
	/*
		stats.PublishJSONFunc("router", func() string {
			var m = make(map[string]interface{})
			m["ops"] = router.OpCounts()
			m["cmds"] = router.GetAllOpStats()
			m["info"] = s.Info()
			m["build"] = map[string]interface{}{
				"version": utils.Version,
				"compile": utils.Compile,
			}
			b, _ := json.Marshal(m)
			return string(b)
		})*/

	go func() {
		<-c
		log.Info("ctrl-c or SIGTERM found, bye bye...")
		s.Close()
	}()

	time.Sleep(time.Second)
	if err := s.SetMyselfOnline(); err != nil {
		log.WarnError(err, "mark myself online fail, you need mark online manually by dashboard")
	}

	s.Join()
	log.Infof("proxy exit!! :(")
}
