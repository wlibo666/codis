#!/usr/bin/python2.7
import pexpect
import os
import string
import sys

def ssh_scp(ip, user, passwd, filename, dstpath):
	passwd_key = '.*assword.*'
	if os.path.isdir(filename):
		cmdline = 'scp -r %s %s@%s:%s' % (filename, user, ip, dstpath)
	else:
		cmdline = 'scp %s %s@%s:%s' % (filename, user, ip, dstpath)
	print "scp cmdline:%s" % cmdline
	try:
		child = pexpect.spawn(cmdline)
		child.expect(passwd_key)
		child.sendline(passwd)
		print "Uploading file [%s]" % filename
		child.expect(pexpect.EOF)
		print "Upload file [%s] success." % filename
		return 0
	except:
		print "upload [%s] faild!" % filename
		return -1

def ssh_cmd(username, ip, password, cmd):
	ret = -1
	sshcmd = 'ssh %s@%s "%s"' % (Username, ip, cmd)
	#print "sshcmd[%s]" % sshcmd
	ssh = pexpect.spawn(sshcmd)
	try:
		i = ssh.expect(['assword:', 'continue connect'], timeout=5)
		if i == 0:
			ssh.sendline(password)
		elif i == 1:
			ssh.sendline("yes\n")
			ssh.expect('assword:')
			ssh.sendline(password)
		ssh.sendline(cmd)
		res = ssh.read()
		if string.find(res, "closed by remote") != -1:
			ret = -3
		else:
			ret = 0
			#print "    res[%s]" % (res)
	except pexpect.EOF:
		ssh.close()
		ret = -1
	except pexpect.TIMEOUT:
		ssh.close()
		ret = -2
	return ret

def run_cmds(username, ip, password, cmds):
	print "will run on ip [%s]" % ip
	for cmd in cmds:
		print "    will run cmd [%s]" % cmd
		ret = ssh_cmd(username, ip, password, cmd)
		if ret != 0:
			print "    [%s] ssh_cmd[%s] failed,ret[%d]." % (ip, cmd, ret)
			break

def check_codis(modulename, ip, user, pwd):
	cmd = 'ssh %s@%s ' % (user, ip)
	if modulename == "proxy":
		cmd += "sudo /usr/sbin/ss -l | grep 16379"
	elif modulename == "server":
		cmd += "sudo /usr/sbin/ss -l | grep 10240"
	else:
		cmd += "sudo /usr/sbin/ss -l | grep 18087"
	ssh = pexpect.spawn(cmd)
	try:
		i = ssh.expect(['assword:', 'continue connect'], timeout=15)
		if i == 0:
			ssh.sendline(pwd)
		elif i == 1:
			ssh.sendline("yes\n")
			ssh.expect('assword:')
			ssh.sendline(password)
		ssh.sendline(cmd)
		res = ssh.read()
		return res
	except:
		ssh.close()
		return "error:%s %s" % (sys.exc_info()[0], sys.exc_info()[1])

def install_codis(modulename):
	for ip in Ips:
		Cmds = []
		ret = ssh_scp(ip, Username, Pwd, "%s/%s" % (os.getcwd(), Pkgname), "/tmp")
		if ret != 0:
			break
		Cmds.append('mkdir -p %s' % Pkgdir)
		Cmds.append('rm -rf %s/*' % Pkgdir)
		Cmds.append('mv /tmp/%s %s/' % (Pkgname, Pkgdir))
		Cmds.append('cd %s; tar zxvf %s' % (Pkgdir, Pkgname))
		if modulename == "proxy":
			Cmds.append('cd %s%s; sudo ./install.sh proxy' % (Pkgdir, Tardir))
#			Cmds.append("sudo /letv/codis/proxy.sh start")
		elif modulename == "dashboard":
			Cmds.append('cd %s%s; sudo ./install.sh dashboard' % (Pkgdir, Tardir))
#			Cmds.append("sudo /letv/codis/dashboard.sh start")
		elif modulename == "server":
			Cmds.append('cd %s%s; sudo ./install.sh server' % (Pkgdir, Tardir))
			#Cmds.append("sudo /letv/codis/server.sh start")
		else:
			print "invalid module [%s]" % modulename
			os._exit(0)
		run_cmds(Username, ip, Pwd, Cmds)
		print "Check codis [%s],res:" % modulename
		print " %s" % check_codis(modulename, ip, Username, Pwd)

def update_codis(modulename):
	for ip in Ips:
		Cmds = []
		ret = ssh_scp(ip, Username, Pwd, "%s/%s" % (os.getcwd(), ProgName), "/tmp")
		if ret != 0:
			break
		if modulename == "proxy":
			Cmds.append("sudo mv /letv/codis/bin/codis-proxy /letv/codis/bin/codis-proxy.`date +%s`")
			Cmds.append("sudo mv /tmp/codis-proxy /letv/codis/bin/codis-proxy")
			Cmds.append("sudo /letv/codis/proxy.sh restart")
		else:
			print "not support other module update except proxy"
			os._exit(0)
		run_cmds(Username, ip, Pwd, Cmds)
		print "Check codis [%s],res:" % modulename
		print " %s" % check_codis(modulename, ip, Username, Pwd)

def Usage(progname):
	print "Usage: %s [install | update ] [proxy|dashboard|server]" % progname
	os._exit(0)

Pkgname = "codis-pkg.tar.gz"
ProgName = "codis-proxy"
Tardir = "/codis"
Pkgdir = "/home/wangchunyan/codis-tools"
#Ips = ['10.112.29.22','10.112.29.23','10.112.28.96','10.112.28.97','10.112.28.98','10.112.28.109','10.112.28.216','10.112.28.224',
#		'10.154.34.82','10.154.34.84','10.154.34.85','10.154.34.86','10.154.34.89','10.154.34.98','10.154.34.113','10.154.34.114',
#		'10.154.34.116','10.154.34.146']
Ips = ['10.135.29.171']
Username = "wangchunyan"
Pwd = "wlibo666@126.com"

if __name__ == '__main__':
	if len(sys.argv) != 3:
		Usage(sys.argv[0])
	if sys.argv[1] != "install" and sys.argv[1] != "update":
		Usage(sys.argv[0])
	if sys.argv[2] != "dashboard" and sys.argv[2] != "proxy" and sys.argv[2] != "server":
		Usage()
	if sys.argv[1] == "install":
		Pkgname = "codis-pkg.tar.gz"
		install_codis(sys.argv[2])
	else:
		ProgName = "codis-proxy"
		update_codis(sys.argv[2])



