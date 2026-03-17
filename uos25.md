
本地构建部署。


## 编译

### 环境准备
```bash
[root@localhost ~]# uosinfo
#################################################
Release:  UOS Server release 25 (qianlai)
Kernel :  6.6.0-25.02.2500.027.uos25.x86_64
Build  :  UOS Server 25 20250807 amd64
#################################################

## 安装软件包
[root@localhost ~]# dnf install golang docker git make nodejs org.deepin.browser -y

## 安装 docker buildx 插件
[root@localhost ~]# wget https://ghfast.top/github.com/docker/buildx/releases/download/v0.22.0/buildx-v0.22.0.linux-amd64
[root@localhost ~]# mkdir -p ~/.docker/cli-plugins
[root@localhost ~]# mv buildx-v0.22.0.linux-amd64 ~/.docker/cli-plugins/docker-buildx
[root@localhost ~]# chmod +x ~/.docker/cli-plugins/docker-buildx
[root@localhost ~]# docker buildx version
github.com/docker/buildx v0.22.0 18ccba072076ddbfb0aeedd6746d7719b0729b58
```

### 代码拉取及修改

```bash
[root@localhost ~]# git clone https://github.com/container2wasm/container2wasm && cd container2wasm && git reset a36dff606e8fcbde13160157987f0fc37adb5181 --hard
// uos25 docker 版本 build 命令不支持 --progress --output 选项，先注释掉。
cmd/c2w/main.go#L187
// "--progress=plain",
cmd/c2w/main.go#L259
// "--progress=plain",
cmd/c2w/main.go#L228
// buildxArgs = append(buildxArgs, "--output",
cmd/c2w/main.go#L293
// buildArgs = append(buildArgs, "--output",
Dockerfile#L91
FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/golang:1.24-bullseye AS golang-base
```

### 编译

```bash
[root@localhost container2wasm]# go env -w GO111MODULE=on && go env -w GOPROXY="https://goproxy.cn" && make build
```
## 使用

### 配置国内镜像加速器

这是最直接有效的方法。你需要修改 Docker 守护进程的配置文件（通常是 `/etc/docker/daemon.json`），添加国内的镜像源。

**1. 编辑配置文件**

```bash
mkdir /etc/docker && vim /etc/docker/daemon.json
```

**2. 添加以下内容（如果没有该文件，则新建一个）** 目前比较稳定的国内源（如阿里云、腾讯云等），你可以使用下面的通用配置：

```json
{
  "registry-mirrors": [
    "https://docker.m.daocloud.io",
    "https://docker.1panel.live",
    "https://dockerhub.icu"
  ]
}
```

_(注：镜像源地址可能会随时间失效，如果上述不可用，建议搜索最新的“Docker 国内镜像源”)_

**3. 重启 Docker 服务**

```bash
sudo systemctl daemon-reload
sudo systemctl restart docker
```


### 下载 wasmtime

```bash
[root@localhost ~]# wget https://ghfast.top/github.com/bytecodealliance/wasmtime/releases/download/v42.0.1/wasmtime-v42.0.1-x86_64-linux.tar.xz
[root@localhost ~]# mv wasmtime-v42.0.1-x86_64-linux/wasmtime* /usr/local/bin/
[root@localhost ~]# wasmtime --version
wasmtime 42.0.1 (6844a83b5 2026-02-25)
```

### 镜像转换

生成wasm文件

```bash
[root@localhost container2wasm]# DOCKER_BUILDKIT=1 out/c2w ubuntu:22.04 out.wasm
[root@localhost container2wasm]# ls out.wasm 
out.wasm
```

执行命令

```bash
[root@localhost container2wasm]# wasmtime out.wasm uname -a
Linux localhost 6.1.0 #1 PREEMPT_DYNAMIC Mon Mar 16 09:43:31 UTC 2026 x86_64 x86_64 x86_64 GNU/Linux
[root@localhost container2wasm]# uname -a
Linux localhost.localdomain 6.6.0-25.02.2500.027.uos25.x86_64 #1 SMP Fri Sep 26 17:39:11 CST 2025 x86_64 x86_64 x86_64 GNU/Linux
[root@localhost container2wasm]# wasmtime out.wasm ls /
bin   dev  home  lib32  libx32  mnt  proc  run   srv  tmp  var
boot  etc  lib   lib64  media   opt  root  sbin  sys  usr
[root@localhost container2wasm]# mkdir -p /tmp/share/ && echo hi > /tmp/share/from-host
[root@localhost container2wasm]# wasmtime --dir /tmp/share out.wasm cat /tmp/share/from-host
hi
```

### 浏览器容器

```bash
[root@localhost container2wasm]# mkdir /tmp/out-js2/
[root@localhost container2wasm]# mkdir -p /tmp/out-js2/htdocs
[root@localhost container2wasm]# cp out.wasm /tmp/out-js2/htdocs
[root@localhost container2wasm]# cp -R ./examples/wasi-browser/* /tmp/out-js2/
[root@localhost container2wasm]# chmod 755 /tmp/out-js2/htdocs
[root@localhost container2wasm]# docker run --rm -p 8080:80 \
         -v "/tmp/out-js2/htdocs:/usr/local/apache2/htdocs/:ro" \
         -v "/tmp/out-js2/xterm-pty.conf:/usr/local/apache2/conf/extra/xterm-pty.conf:ro" \
         --entrypoint=/bin/sh httpd -c 'echo "Include conf/extra/xterm-pty.conf" >> /usr/local/apache2/conf/httpd.conf && httpd-foreground'
```

打开浏览器访问本地 8080 端口。

![](../../images/Pasted%20image%2020260316215339.png)

此时容器中没有curl 命令。
### 热补丁带curl 

新开一个终端，重建一个带有curl命令的容器并替换wasm文件。

```bash
## 构建镜像
[root@localhost ~]# cat <<EOF | docker build -t debian-curl -
FROM debian:sid-slim
RUN apt-get update && apt-get install -y curl
EOF
## 生成 wasm 并替换
[root@localhost ~]# container2wasm/out/c2w debian-curl /tmp/out-js2/htdocs/out.wasm
[root@localhost ~]# wget -O /tmp/out-js2/htdocs/c2w-net-proxy.wasm https://ghfast.top/github.com/ktock/container2wasm/releases/download/v0.5.0/c2w-net-proxy.wasm
```

由于之前的httpd容器还在跑，可以直接访问 http://localhost:8080/?net=browser

![](../../images/Pasted%20image%2020260316222016.png)

有curl命令同时可以访问网络。

本地命令行启动

```bash
[root@localhost ~]# container2wasm/out/c2w-net -listen-ws localhost:8888
```

浏览器访问地址 localhost:8080/?net=delegate=ws://localhost:8888 。

![](../../images/Pasted%20image%2020260317094832.png)


### Emscripten 启动方式


```bash
[root@localhost ~]# container2wasm/out/c2w --to-js alpine:3.20 /tmp/out-js/htdocs/
[root@localhost ~]# npm config set registry https://registry.npmmirror.com && curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
[root@localhost ~]# export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"  # This loads nvm
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"
[root@localhost ~]# nvm install 14
[root@localhost container2wasm]# ( cd ./examples/emscripten/htdocs/ && npx webpack && cp -R index.html dist vendor/xterm.css /tmp/out-js/htdocs/ )
[root@localhost container2wasm]# wget -O /tmp/c2w-net-proxy.wasm https://ghfast.top/github.com/ktock/container2wasm/releases/download/v0.5.0/c2w-net-proxy.wasm
[root@localhost container2wasm]# cat /tmp/c2w-net-proxy.wasm | gzip > /tmp/out-js/htdocs/c2w-net-proxy.wasm.gzip
[root@localhost container2wasm]# cp ./examples/emscripten/xterm-pty.conf /tmp/out-js/
[root@localhost container2wasm]# chmod 755 /tmp/out-js/htdocs
[root@localhost container2wasm]# chmod 644 /tmp/out-js/htdocs/*html
[root@localhost container2wasm]# docker run --rm -p 127.0.0.1:8080:80 \
         -v "/tmp/out-js/htdocs:/usr/local/apache2/htdocs/:ro" \
         -v "/tmp/out-js/xterm-pty.conf:/usr/local/apache2/conf/extra/xterm-pty.conf:ro" \
         --entrypoint=/bin/sh httpd -c 'echo "Include conf/extra/xterm-pty.conf" >> /usr/local/apache2/conf/httpd.conf && httpd-foreground'

```

浏览器访问

![](../../images/Pasted%20image%2020260317112954.png)


### coredns 中的尝试

```bash
## 执行命令构建一个 my-coredns 容器镜像
[root@localhost container2wasm]# cat <<EOF | docker build -t my-coredns -
FROM coredns/coredns:latest
# 使用 heredoc 语法直接写入配置内容
COPY <<EOTXT /etc/coredns/Corefile
.:1053 {
    hosts {
        127.0.0.1 controller
        fallthrough
    }
    forward . 8.8.8.8
    cache
    log
}
EOTXT

# 覆盖默认启动命令，指定配置文件路径
CMD ["-conf", "/etc/coredns/Corefile"]
EOF
[root@localhost container2wasm]# out/c2w my-coredns coredns1.wasm
[root@localhost container2wasm]# ./out/wazero-test -debug -udp -net -p 53:53 coredns1.wasm 
enabling UDP support with TCP listener on 127.0.0.1:1234
connecting to NW...
failed connecting to NW: dial tcp 127.0.0.1:1234: connect: connection refused; retrying...
connecting to NW...
failed connecting to NW: dial tcp 127.0.0.1:1234: connect: connection refused; retrying...
connecting to NW...
failed connecting to NW: dial tcp 127.0.0.1:1234: connect: connection refused; retrying...
connecting to NW...
maxprocs: Leaving GOMAXPROCS=1: CPU quota undefined
.:1053
CoreDNS-1.14.2
linux/amd64, go1.26.1, dd1df4f
INFO[0026] PACKET: 42 bytes
- Layer 1 (14 bytes) = Ethernet {Contents=[..14..] Payload=[..28..] SrcMAC=5a:94:ef:e4:0c:dd DstMAC=ff:ff:ff:ff:ff:ff EthernetType=ARP Length=0}
- Layer 2 (28 bytes) = ARP   {Contents=[..28..] Payload=[] AddrType=Ethernet Protocol=IPv4 HwAddressSize=6 ProtAddressSize=4 Operation=1 SourceHwAddress=[..6..] SourceProtAddress=[192, 168, 127, 1] DstHwAddress=[..6..] DstProtAddress=[192, 168, 127, 3]} 
INFO[0027] PACKET: 42 bytes
```

```bash
## 无法正常访问
[root@localhost ~]# dig  controller @127.0.0.1 -p 53 +short
;; communications error to 127.0.0.1#53: connection refused
;; communications error to 127.0.0.1#53: connection refused
;; communications error to 127.0.0.1#53: connection refused

; <<>> DiG 9.20.16 <<>> controller @127.0.0.1 -p 53 +short
;; global options: +cmd
;; no servers could be reached
[root@localhost ~]# dig +tcp  controller @127.0.0.1 -p 53 +short
;; communications error to 127.0.0.1#53: connection reset
;; communications error to 127.0.0.1#53: connection reset
;; communications error to 127.0.0.1#53: connection reset

; <<>> DiG 9.20.16 <<>> +tcp controller @127.0.0.1 -p 53 +short
;; global options: +cmd
;; no servers could be reached

```
https://github.com/wazero/wazero/issues/2187
https://github.com/dispatchrun/net/tree/main/wasip1
https://github.com/coredns/coredns/issues/6562
https://github.com/satrobit/coredns-wasm.git
https://github.com/WasmFunction/wasm-faas

wasm 可以挂载目录，理论上是可以通过修改 Corefile 实现动态更新解析。
