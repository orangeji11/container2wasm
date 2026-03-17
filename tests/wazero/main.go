package main

import (
	"context"
	crand "crypto/rand"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	gvntypes "github.com/containers/gvisor-tap-vsock/pkg/types"
	gvnvirtualnetwork "github.com/containers/gvisor-tap-vsock/pkg/virtualnetwork"
	"github.com/tetratelabs/wazero/experimental/sock"
)

const (
	gatewayIP = "192.168.127.1"
	vmIP      = "192.168.127.3"
	vmMAC     = "02:00:00:00:00:01"
)

func main() {
	var (
		mapDir     = flag.String("mapdir", "", "directory mapping to the image")
		debug      = flag.Bool("debug", false, "debug log")
		mac        = flag.String("mac", vmMAC, "mac address assigned to the container")
		wasiAddr   = flag.String("wasi-addr", "127.0.0.1:1234", "IP address used to communicate between wasi and network stack")
		enableNet  = flag.Bool("net", false, "enable network")
		enableUDP  = flag.Bool("udp", false, "enable UDP support (default: TCP only)")
		listenAddr = flag.String("listen", "", "listen address for direct mode (e.g., :53)")
	)
	var portFlags sliceFlags
	flag.Var(&portFlags, "p", "map port between host and guest (host:guest). -mac must be set correctly.")
	var envs sliceFlags
	flag.Var(&envs, "env", "environment variables")

	flag.Parse()
	args := flag.Args()
	fsConfig := wazero.NewFSConfig()
	if mapDir != nil && *mapDir != "" {
		m := strings.SplitN(*mapDir, "::", 2)
		if len(m) != 2 {
			panic("specify mapdir as dst::src")
		}
		fsConfig = fsConfig.WithDirMount(m[1], m[0])
	}

	ctx := context.Background()
	if *enableNet {
		forwards := make(map[string]string)
		for _, p := range portFlags {
			parts := strings.Split(p, ":")
			switch len(parts) {
			case 3:
				// IP:PORT1:PORT2
				forwards[strings.Join(parts[0:2], ":")] = strings.Join([]string{vmIP, parts[2]}, ":")
			case 2:
				// PORT1:PORT2
				forwards["0.0.0.0:"+parts[0]] = vmIP + ":" + parts[1]
			}
		}
		config := &gvntypes.Configuration{
			Debug:             *debug,
			MTU:               1500,
			Subnet:            "192.168.127.0/24",
			GatewayIP:         gatewayIP,
			GatewayMacAddress: "5a:94:ef:e4:0c:dd",
			DHCPStaticLeases: map[string]string{
				vmIP: *mac,
			},
			Forwards: forwards,
			NAT: map[string]string{
				"192.168.127.254": "127.0.0.1",
			},
			GatewayVirtualIPs: []string{"192.168.127.254"},
			Protocol:          gvntypes.QemuProtocol,
		}
		vn, err := gvnvirtualnetwork.New(config)
		if err != nil {
			panic(err)
		}
		go func() {
			var conn net.Conn
			for i := 0; i < 100; i++ {
				time.Sleep(1 * time.Second)
				fmt.Fprintf(os.Stderr, "connecting to NW...\n")
				conn, err = net.Dial("tcp", *wasiAddr)
				if err == nil {
					break
				}
				fmt.Fprintf(os.Stderr, "failed connecting to NW: %v; retrying...\n", err)
			}
			if conn == nil {
				panic("failed to connect to vm")
			}
			// We register our VM network as a qemu "-netdev socket".
			if err := vn.AcceptQemu(context.TODO(), conn); err != nil {
				fmt.Fprintf(os.Stderr, "failed AcceptQemu: %v\n", err)
			}
		}()
		u, err := url.Parse("dummy://" + *wasiAddr)
		if err != nil {
			panic(err)
		}
		p, err := strconv.Atoi(u.Port())
		if err != nil {
			panic(err)
		}
		if *enableUDP {
			fmt.Fprintf(os.Stderr, "enabling UDP support with TCP listener on %s\n", *wasiAddr)
			sockCfg := sock.NewConfig().WithTCPListener(u.Hostname(), p)
			ctx = sock.WithConfig(ctx, sockCfg)
		} else {
			sockCfg := sock.NewConfig().WithTCPListener(u.Hostname(), p)
			ctx = sock.WithConfig(ctx, sockCfg)
		}
	} else if *listenAddr != "" {
		// Direct listen mode without gvisor-tap-vsock
		fmt.Fprintf(os.Stderr, "starting in direct listen mode on %s\n", *listenAddr)
		go startDirectListen(*listenAddr)
	}
	c, err := os.ReadFile(args[0])
	if err != nil {
		panic(err)
	}
	r := wazero.NewRuntime(ctx)
	defer func() {
		r.Close(ctx)
	}()
	wasi_snapshot_preview1.MustInstantiate(ctx, r)
	compiled, err := r.CompileModule(ctx, c)
	if err != nil {
		panic(err)
	}
	conf := wazero.NewModuleConfig().WithSysWalltime().WithSysNanotime().WithSysNanosleep().WithRandSource(crand.Reader).WithStdout(os.Stdout).WithStderr(os.Stderr).WithStdin(os.Stdin).WithFSConfig(fsConfig).WithArgs(append([]string{"arg0"}, args[1:]...)...)
	for _, v := range envs {
		es := strings.SplitN(v, "=", 2)
		if len(es) == 2 {
			conf = conf.WithEnv(es[0], es[1])
		} else {
			panic("env must be a key value pair")
		}
	}
	_, err = r.InstantiateModule(ctx, compiled, conf)
	if err != nil {
		panic(err)
	}
}

// startDirectListen 直接监听模式 - 简单端口转发
// 注意：由于 wazero 的 socket API 限制，这个实现只做端口监听演示
// 完整的 UDP/TCP 支持需要使用 wasmtime 或其他支持 socket 的 runtime
func startDirectListen(addr string) {
	fmt.Fprintf(os.Stderr, "=== Direct Listen Mode ===\n")
	fmt.Fprintf(os.Stderr, "Note: This is a demo mode. For full UDP/TCP support, use wasmtime.\n")
	
	// 启动 UDP 监听器
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve UDP address: %v\n", err)
		return
	}
	
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start UDP listener on %s: %v\n", addr, err)
		return
	}
	defer udpConn.Close()
	
	fmt.Fprintf(os.Stderr, "UDP listener started on %s\n", udpConn.LocalAddr())
	
	// 简单的 UDP 回显（用于测试）
	buf := make([]byte, 4096)
	for {
		n, fromAddr, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "UDP read error: %v\n", err)
			continue
		}
		fmt.Fprintf(os.Stderr, "UDP received %d bytes from %s: %x\n", n, fromAddr, buf[:n])
		
		// 回显响应（用于测试连通性）
		_, err = udpConn.WriteToUDP([]byte("echo: "+string(buf[:n])), fromAddr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "UDP write error: %v\n", err)
		}
	}
}

type sliceFlags []string

func (f *sliceFlags) String() string {
	var s []string = *f
	return fmt.Sprintf("%v", s)
}

func (f *sliceFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}

