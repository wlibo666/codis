#!/bin/bash
function show_msg()
{
	echo "$1 $2 $3 $4 $5 $6"
	return
}

function debug_msg()
{
#	echo "$1 $2 $3 $4 $5 $6"
	return
}

INSTALL_CMD="$1"
# INSTALL CONFIGURE FILE NAME
CUR_DIR="`pwd`"
INSTALL_CONFIG_FILE="$CUR_DIR/install.config"

# global variable
ETH_NAME=""
ETH_ADDR=""
DASHBOARD_IPADDR=""
REDIS_PORT=""
REDIS_LOG=""
REDIS_MEM=""
INSTALL_DIR=""
REPLACE_DIR=""

function load_conf()
{
	debug_msg "enter load_conf"
	ETH_NAME="`cat $INSTALL_CONFIG_FILE |grep "ETH_NAME"|awk -F= '{print $2}'`"
	REDIS_PORT="`cat $INSTALL_CONFIG_FILE |grep "REDIS_PORT"|awk -F= '{print $2}'`"
	REDIS_LOG="`cat $INSTALL_CONFIG_FILE |grep "REDIS_LOG"|awk -F= '{print $2}'`"
	DASHBOARD_IPADDR="`cat $INSTALL_CONFIG_FILE |grep "DASHBOARD_IPADDR"|awk -F= '{print $2}'`"
	REDIS_MEM="`cat $INSTALL_CONFIG_FILE |grep "REDIS_MEM"|awk -F= '{print $2}'`"
	INSTALL_DIR="`cat $INSTALL_CONFIG_FILE |grep "INSTALL_DIR"|awk -F= '{print $2}'`"
	ETH_ADDR="`ifconfig $ETH_NAME | grep "inet addr" | awk '{print $2}' | awk -F: '{print $2}'`"
	REPLACE_DIR="`cat $INSTALL_CONFIG_FILE |grep "REPLACE_DIR"|awk -F= '{print $2}'`"

	if [ -z "$ETH_NAME" ] || [ -z "$ETH_ADDR" ] || [ -z "$REDIS_PORT" ] || [ -z "$REDIS_LOG" ] || [ -z "$REDIS_MEM" ] || [ -z "$INSTALL_DIR" ] || [ -z "$REPLACE_DIR" ]; then
		show_msg "ETH_NAME=$ETH_NAME"
		show_msg "ETH_ADDR=$ETH_ADDR"
		show_msg "CUR_DIR=$CUR_DIR"
		show_msg "DASHBOARD_IPADDR=$DASHBOARD_IPADDR"
		show_msg "REDIS_PORT=$REDIS_PORT"
		show_msg "REDIS_LOG=$REDIS_LOG"
		show_msg "REDIS_MEM=$REDIS_MEM"
		show_msg "INSTALL_DIR=$INSTALL_DIR"
		show_msg "REPLACE_DIR=$REPLACE_DIR"
		show_msg "global config should be seted,now some of them are empty..."
		exit 0
	fi
	show_msg "global setting :"
	show_msg "ETH_NAME=$ETH_NAME"
	show_msg "ETH_ADDR=$ETH_ADDR"
	show_msg "CUR_DIR=$CUR_DIR"
	show_msg "DASHBOARD_IPADDR=$DASHBOARD_IPADDR"
	show_msg "REDIS_PORT=$REDIS_PORT"
	show_msg "REDIS_LOG=$REDIS_LOG"
	show_msg "REDIS_MEM=$REDIS_MEM"
	show_msg "INSTALL_DIR=$INSTALL_DIR"
	show_msg "REPLACE_DIR=$REPLACE_DIR"
	show_msg ""
	debug_msg "exit load_conf"
}

function create_base_dir()
{
	debug_msg "enter create_base_dir"
	mkdir -p $INSTALL_DIR/codis/
	mkdir -p $INSTALL_DIR/codis/bin/
	mkdir -p $INSTALL_DIR/codis/conf/
	debug_msg "exit create_base_dir"
}

function deploy_config_ini()
{
	debug_msg "enter deploy_config_ini"
	if [ -f $INSTALL_DIR/codis/conf/config.ini ] ; then
		mv $INSTALL_DIR/codis/conf/config.ini $INSTALL_DIR/codis/conf/config.ini".`date '+%s'`"
	fi
	cp $CUR_DIR/conf/config.ini $CUR_DIR/conf/config.ini.bak
	mv $CUR_DIR/conf/config.ini.bak $INSTALL_DIR/codis/conf/config.ini
	chmod 666 $INSTALL_DIR/codis/conf/config.ini
	sed -i "s/PROXY_IPADDR/$ETH_ADDR/g" $INSTALL_DIR/codis/conf/config.ini
	sed -i "s/DASHBOARD_IPADDR/$DASHBOARD_IPADDR/g" $INSTALL_DIR/codis/conf/config.ini
	debug_msg "exit deploy_config_ini"
}

# $1 bin_filename
function deploy_bin_file()
{
	debug_msg "enter deploy_bin_file"
	if [ -f $INSTALL_DIR/codis/bin/$1 ] ; then
		mv $INSTALL_DIR/codis/bin/$1 $INSTALL_DIR/codis/bin/$1".`date '+%s'`"
	fi
	cp $CUR_DIR/bin/$1 $INSTALL_DIR/codis/bin/
	chmod 755 $INSTALL_DIR/codis/bin/$1
	debug_msg "exit deploy_bin_file"
}

# $1 conf_filename
function deploy_conf_file()
{
	debug_msg "enter deploy_conf_file"
	if [ -f $INSTALL_DIR/codis/conf/$1 ] ; then
		mv $INSTALL_DIR/codis/conf/$1 $INSTALL_DIR/codis/conf/$1".`date '+%s'`"
	fi
	cp $CUR_DIR/conf/$1 $INSTALL_DIR/codis/conf/
	chmod 664 $INSTALL_DIR/codis/conf/$1
	debug_msg "exit deploy_conf_file"
}

# $1 control script filename
function deploy_control_script()
{
	debug_msg "enter deploy_control_script"
	if [ -f $INSTALL_DIR/codis/$1 ] ; then
		mv $INSTALL_DIR/codis/$1 $INSTALL_DIR/codis/$1".`date '+%s'`"
	fi
	cp $CUR_DIR/$1 $INSTALL_DIR/codis/
	chmod 775 $INSTALL_DIR/codis/$1
	sed -i "s/INSTALL_DIR/$REPLACE_DIR/g" $INSTALL_DIR/codis/$1
	debug_msg "exit deploy_control_script"
}

function install_codis_dashboard()
{
	debug_msg "enter install_codis_dashboard"
	mkdir -p $INSTALL_DIR/run/codis/dashboard/
	mkdir -p $INSTALL_DIR/logs/codis/dashboard/
	# assets
	if [ -d $INSTALL_DIR/codis/bin/assets ] ; then
		mv $INSTALL_DIR/codis/bin/assets $INSTALL_DIR/codis/bin/assets".`date '+%s'`"
	fi
	cp -rf $CUR_DIR/bin/assets $INSTALL_DIR/codis/bin
	# codis-config
	deploy_bin_file "codis-config"
	# delMgrtAllTask
	deploy_bin_file "delAllMgrtTask"
	# dashboard supervisor	
	deploy_conf_file "dashboard-supervisord.conf"
	sed -i "s/INSTALL_DIR/$REPLACE_DIR/g" $INSTALL_DIR/codis/conf/dashboard-supervisord.conf
	# dashboard.sh
	cp $CUR_DIR/dashboard.sh $INSTALL_DIR/codis/
	sed -i "s/INSTALL_DIR/$REPLACE_DIR/g" $INSTALL_DIR/codis/dashboard.sh
	debug_msg "exit install_codis_dashboard"
	show_msg "install codis-dashboard success."
	return 0
}

function install_codis_proxy()
{
	debug_msg "enter install_codis_proxy"
	mkdir -p $INSTALL_DIR/run/codis/proxy
	mkdir -p $INSTALL_DIR/logs/codis/proxy
	# codis-proxy
	deploy_bin_file "codis-proxy"	
	# proxy supervisor
	deploy_conf_file "proxy-supervisord.conf"
	sed -i "s/INSTALL_DIR/$REPLACE_DIR/g" $INSTALL_DIR/codis/conf/proxy-supervisord.conf
	sed -i "s/PROXY_IPADDR/$ETH_ADDR/g" $INSTALL_DIR/codis/conf/proxy-supervisord.conf
	# proxy.sh
	cp $CUR_DIR/proxy.sh $INSTALL_DIR/codis/
	sed -i "s/INSTALL_DIR/$REPLACE_DIR/g" $INSTALL_DIR/codis/proxy.sh
	debug_msg "exit install_codis_proxy"
	show_msg "install codis-proxy success."

	return 0
}

function install_codis_server()
{
	debug_msg "enter install_codis_server"
	mkdir -p $INSTALL_DIR/run/codis/server/
	mkdir -p $INSTALL_DIR/logs/codis/server/
	# codis-server
	deploy_bin_file "letv-redis"
	deploy_bin_file "redis-check-aof"
	deploy_bin_file "redis-check-dump"
	deploy_bin_file "redis-cli"
	deploy_bin_file "redis-sentinel"
	deploy_bin_file "checkSlot"
	# server supervisor
	deploy_conf_file "server-supervisord.conf"
	sed -i "s/INSTALL_DIR/$REPLACE_DIR/g" $INSTALL_DIR/codis/conf/server-supervisord.conf
	# server config
	deploy_conf_file "codis-server.conf"
	sed -i "s/INSTALL_DIR/$REPLACE_DIR/g" $INSTALL_DIR/codis/conf/codis-server.conf
	sed -i "s/REDIS_PORT/$REDIS_PORT/g" $INSTALL_DIR/codis/conf/codis-server.conf
	sed -i "s/REDIS_LOG/$REDIS_LOG/g" $INSTALL_DIR/codis/conf/codis-server.conf
	sed -i "s/REDIS_MEM/$REDIS_MEM/g" $INSTALL_DIR/codis/conf/codis-server.conf
	# server.sh
	cp $CUR_DIR/server.sh $INSTALL_DIR/codis/
	sed -i "s/INSTALL_DIR/$REPLACE_DIR/g" $INSTALL_DIR/codis/server.sh
	debug_msg "exit install_codis_server"
	show_msg "install letv-redis success."

	return 0
}

function install_codis_all()
{
	debug_msg "enter install_codis_all"
	install_codis_dashboard
	install_codis_proxy
	install_codis_server
	debug_msg "exit install_codis_all"
	return 0
}

function usage()
{
	echo "usage: $0 { dashboard | proxy | server | all }"
	exit 2
}

function prepare()
{
	debug_msg "enter prepare"
	create_base_dir
	deploy_config_ini
	debug_msg "exit prepare"
}

function main()
{
	debug_msg "enter main,cmd:$INSTALL_CMD"
	case $INSTALL_CMD in
		dashboard)
			prepare
			install_codis_dashboard
			;;
		proxy)
			prepare
			install_codis_proxy
			;;
		server)
			prepare
			install_codis_server
			;;
		all)
			prepare
			install_codis_dashboard
			install_codis_proxy
			install_codis_server
			;;
		*)
			usage
	esac
	debug_msg "exit main"
}

function check_supervisord()
{
	debug_msg "enter check_supervisord"
	RESULT="`which supervisord`"
	FLAG=`echo "$RESULT"|grep "/usr/bin/which: no"`
	if [ -z "$FLAG" ] ; then
		show_msg "supervisrod is installed,continue..."
	else
		show_msg "supervisord is not install or not in env:$PATH"
		exit 2
	fi
	debug_msg "exit check_supervisord"
}

check_supervisord
load_conf
main

