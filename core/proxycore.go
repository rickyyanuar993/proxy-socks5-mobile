package proxycore

import (
	"context"
	"fmt"
	"sync"

	"github.com/fatedier/frp/client"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/config/v1/validation"
	"github.com/txthinking/socks5"
)

var (
	mu          sync.Mutex
	socksServer *socks5.Server
	frpService  *client.Service
	cancelFrp   context.CancelFunc
	isRunning   bool
)

// StartServer starts both high-performance SOCKS5 server (TCP+UDP) and embedded FRP client.
func StartServer(localPort int, serverAddr string, serverPort int, token string, remotePort int, socksUser, socksPass string) string {
	mu.Lock()
	defer mu.Unlock()

	if isRunning {
		return "ERROR: Server is already running"
	}

	// 1. Inisialisasi SOCKS5 Server (Listen 0.0.0.0, Support Full TCP + UDP Associate)
	listenAddr := fmt.Sprintf("0.0.0.0:%d", localPort)
	server, err := socks5.NewClassicServer(listenAddr, "127.0.0.1", socksUser, socksPass, 300, 300)
	if err != nil {
		return fmt.Sprintf("ERROR socks5 init: %v", err)
	}
	socksServer = server

	// Jalankan SOCKS5 di background
	go func() {
		if err := socksServer.ListenAndServe(nil); err != nil {
			fmt.Printf("SOCKS5 stopped: %v\n", err)
		}
	}()

	// 2. Inisialisasi Embedded FRP Client (frpc)
	common := &v1.ClientCommonConfig{}
	common.ServerAddr = serverAddr
	common.ServerPort = serverPort
	if token != "" {
		common.Auth.Method = v1.AuthMethodToken
		common.Auth.Token = token
	}

	// Tunnel TCP Proxy
	tcpProxy := &v1.TCPProxyConfig{
		ProxyBaseConfig: v1.ProxyBaseConfig{
			Name:       fmt.Sprintf("socks5-tcp-%d", remotePort),
			Type:       string(v1.ProxyTypeTCP),
			LocalIP:    "127.0.0.1",
			LocalPort:  localPort,
		},
		RemotePort: remotePort,
	}

	// Tunnel UDP Proxy
	udpProxy := &v1.UDPProxyConfig{
		ProxyBaseConfig: v1.ProxyBaseConfig{
			Name:       fmt.Sprintf("socks5-udp-%d", remotePort),
			Type:       string(v1.ProxyTypeUDP),
			LocalIP:    "127.0.0.1",
			LocalPort:  localPort,
		},
		RemotePort: remotePort,
	}

	proxyCfgs := []v1.ProxyConfigurer{tcpProxy, udpProxy}
	_, err = validation.ValidateAllClientConfig(common, proxyCfgs, nil)
	if err != nil {
		_ = socksServer.Shutdown()
		return fmt.Sprintf("ERROR frp config validation: %v", err)
	}

	frpSvr, err := client.NewService(client.ServiceOptions{
		Common:    common,
		ProxyCfgs: proxyCfgs,
	})
	if err != nil {
		_ = socksServer.Shutdown()
		return fmt.Sprintf("ERROR frp service creation: %v", err)
	}
	frpService = frpSvr

	ctx, cancel := context.WithCancel(context.Background())
	cancelFrp = cancel

	go func() {
		_ = frpService.Run(ctx)
	}()

	isRunning = true
	return "OK"
}

// StopServer stops both FRP client and SOCKS5 server cleanly.
func StopServer() string {
	mu.Lock()
	defer mu.Unlock()

	if !isRunning {
		return "OK"
	}

	if cancelFrp != nil {
		cancelFrp()
	}
	if frpService != nil {
		frpService.Close()
		frpService = nil
	}
	if socksServer != nil {
		_ = socksServer.Shutdown()
		socksServer = nil
	}

	isRunning = false
	return "OK"
}

// IsRunning returns true if the server is active.
func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return isRunning
}
