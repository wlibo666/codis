autoinstall
├── autoinstall.py
├── codis
│   ├── bin
│   │   ├── assets
│   │   │   ├── statics
│   │   │   │   ├── admin
│   │   │   │   │   ├── 6df2b309.favicon.ico
│   │   │   │   │   ├── fonts
│   │   │   │   │   │   ├── glyphicons-halflings-regular.eot
│   │   │   │   │   │   ├── glyphicons-halflings-regular.svg
│   │   │   │   │   │   ├── glyphicons-halflings-regular.ttf
│   │   │   │   │   │   ├── glyphicons-halflings-regular.woff
│   │   │   │   │   │   └── glyphicons-halflings-regular.woff2
│   │   │   │   │   ├── index.html
│   │   │   │   │   ├── robots.txt
│   │   │   │   │   ├── scripts
│   │   │   │   │   │   ├── 1e5b706e.vendor.js
│   │   │   │   │   │   ├── d41d8cd9.plugins.js
│   │   │   │   │   │   └── dea5e6a4.main.js
│   │   │   │   │   └── styles
│   │   │   │   │       ├── a1b6bacd.main.css
│   │   │   │   │       └── b5495e1f.vendor.css
│   │   │   │   ├── bootstrap.min.css
│   │   │   │   ├── bootstrap.min.js
│   │   │   │   ├── jquery.min.js
│   │   │   │   ├── md5.js
│   │   │   │   ├── underscore-min.js
│   │   │   │   └── utils.js
│   │   │   └── template
│   │   │       └── slots.html
│   │   ├── CheckRedis
│   │   ├── codis-config
│   │   ├── codis-proxy
│   │   ├── DelAllMigrtTask
│   │   ├── GetSlotGroup
│   │   ├── letv-redis
│   │   ├── MigrateBigKey
│   │   ├── redis-check-aof
│   │   ├── redis-check-dump
│   │   ├── redis-cli
│   │   ├── redis-sentinel
│   │   └── SetSlotGroup
│   ├── conf
│   │   ├── codis-server.conf
│   │   ├── config.ini
│   │   ├── dashboard-supervisord.conf
│   │   ├── proxy-supervisord.conf
│   │   └── server-supervisord.conf
│   ├── dashboard.sh
│   ├── install.config
│   ├── install.sh
│   ├── proxy.sh
│   └── server.sh
└── genpkg.sh

说明:
autoinstall 目录是自动安装目录脚本根目录
autoinstall.py 是自动安装脚本，实际使用时需要修改里面 Ips变量中的IP地址
bin 目录下是所有的二进制文件
conf 目录下是所有配置文件
genpkg.sh 是生成安装包的脚本

使用说明参见 http://www.cnblogs.com/wlibo666/p/5235641.html
注:由于二进制文件太大，此处将 autoinstall/codis/bin
目录下的二进制文件都删除了，实际使用时参考上面的文件树 放入即可。


10 directories, 44 files
