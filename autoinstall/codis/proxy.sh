#!/bin/bash

. /etc/rc.d/init.d/functions

prog=proxy
THIS_DIR=$(dirname $(readlink -f $0) )
cmd=$1

supervisor_pidfile=INSTALL_DIR/run/codis/proxy/supervisord.pid
supervisor_config=$THIS_DIR/conf/proxy-supervisord.conf
SUPERVISORD=/usr/local/python2.7/bin/supervisord
SUPERVISORCTL=/usr/local/python2.7/bin/supervisorctl

function start()
{
	ulimit -n 100000 
	mkdir -p INSTALL_DIR/run/codis/proxy
	mkdir -p INSTALL_DIR/logs/codis/proxy

	THIS_IP=$(ifconfig eth0|grep inet| sed -n '1p'|awk '{print $2}'|awk -F ':' '{print $2}')
	[ "$THIS_IP" = "" ] && { echo "can't get local IP"; return 1; }
	PROXY_NAME="proxy_${THIS_IP}"

	sed -i "s/product.*/product=pusher/g" $THIS_DIR/conf/config.ini
	sed -i "s/proxy_id.*/proxy_id=${PROXY_NAME}/g" $THIS_DIR/conf/config.ini

	status $prog > /dev/null
	[ $? -eq 0 ] && { echo "$prog is already running"; return 1; }

	$SUPERVISORD -c $supervisor_config
	[ $? -ne 0 ] && { echo "start supervisord failed"; exit 1; }
	retry=0
	while [ $retry -lt 5 ]; do
		$SUPERVISORCTL -c $supervisor_config status $prog |grep RUNNING >/dev/null
		[ $? -eq 0 ] && { break; }
		retry=$(($retry+1))
		sleep 1
	done
	[ $? -ge 5 ] && { echo "$prog not in running status"; return 1; }
	sleep 1
	$THIS_DIR/bin/codis-config -c $THIS_DIR/conf/config.ini proxy online $PROXY_NAME
	[ $? -ne 0 ] && { echo "set proxy online failed"; }
	success
	return 0
}

function stop()
{
	$SUPERVISORCTL -c $supervisor_config stop $prog >/dev/null 2>&1
	$SUPERVISORCTL -c $supervisor_config shutdown >/dev/null 2>&1

	status -p $supervisor_pidfile supervisord > /dev/null
	[ $? -eq 0 ] && { killproc -p $supervisor_pidfile $prog; }
	
	status $prog > /dev/null
	[ $? -eq 0 ] && { killproc $prog; }
	
	success
	return 0	
}

function restart()
{
	stop
	start
}

rh_status() {
	status -p $supervisor_pidfile supervisord
	status $prog
}

case $cmd in
	start)
		start
		;;
	stop)
		stop
		;;	
	status)
		rh_status
		;;	
	restart)
		restart
		;;
	*)
		echo $"Usage: $0 {start|stop|status|restart}"
		RET=2	
esac
exit $RET

