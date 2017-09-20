#!/usr/bin/env bash
curdir=`pwd`
prog=capture-redis-pkg
#netcard=eth3
netcard=lo
#dstip=10.110.60.53
dstip=127.0.0.1
#dstport=6383
dstport=6379,6380

rediscmd="DEL,db_apps;HDEL,db_apps;DEL,db_packages;HDEL,db_packages;DEL,db_appuser;HDEL,db_appuser;DEL,db_internal_apps_0;SPOP,db_internal_apps_0;DEL,db_internal_apps_1;SPOP,db_internal_apps_1;DEL,db_black_devices;SPOP,db_black_devices"


chmod +x $curdir/$prog
while [ 1 ]
do
    progid=`pidof $prog`
    if [ -z "$progid" ] ; then
        $curdir/$prog -dev "$netcard"  -dstip "$dstip" -dstport $dstport -rediscmd "$rediscmd" -minlen 16 -log $curdir/capture.redis.log
    fi
    sleep 5
done
