package proxycore

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatedier/frp/client"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/config/v1/validation"
)

var (
	mu          sync.Mutex
	socksServer *Server
	frpService  *client.Service
	cancelFrp   context.CancelFunc
	isRunning   bool

	logMu   sync.Mutex
	logList []string
)

func AddLog(msg string) {
	logMu.Lock()
	defer logMu.Unlock()
	t := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", t, msg)
	logList = append(logList, entry)
	if len(logList) > 200 {
		logList = logList[1:]
	}
}

// GetRecentLogs returns the collected debug logs joined by newline
func GetRecentLogs() string {
	logMu.Lock()
	defer logMu.Unlock()
	if len(logList) == 0 {
		return "No logs yet..."
	}
	res := ""
	for _, l := range logList {
		res += l + "\n"
	}
	return res
}

func ClearLogs() {
	logMu.Lock()
	defer logMu.Unlock()
	logList = nil
}

type Server struct {
	listenPort  int
	username    string
	password    string
	running     int32
	listener    net.Listener
	udpListener net.PacketConn
	udpSessions sync.Map // map[string]*udpSession
}

type udpSession struct {
	clientAddr net.Addr
	outConn    net.PacketConn
	lastActive    int64
	active        int32
	lastTargetDNS string
}

func (s *udpSession) close() {
	if atomic.CompareAndSwapInt32(&s.active, 1, 0) {
		if s.outConn != nil {
			_ = s.outConn.Close()
		}
	}
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", s.listenPort))
	if err != nil {
		return fmt.Errorf("tcp listen error: %w", err)
	}
	s.listener = ln

	udpLn, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%d", s.listenPort))
	if err != nil {
		_ = ln.Close()
		return fmt.Errorf("udp listen error: %w", err)
	}
	if pconn, ok := udpLn.(*net.UDPConn); ok {
		_ = pconn.SetReadBuffer(4 * 1024 * 1024)
		_ = pconn.SetWriteBuffer(4 * 1024 * 1024)
	}
	s.udpListener = udpLn

	atomic.StoreInt32(&s.running, 1)
	go s.acceptLoop()
	go s.udpListenLoop()
	return nil
}

func (s *Server) Stop() {
	atomic.StoreInt32(&s.running, 0)
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.udpListener != nil {
		_ = s.udpListener.Close()
	}
	s.udpSessions.Range(func(key, value interface{}) bool {
		sess := value.(*udpSession)
		sess.close()
		return true
	})
}

func (s *Server) acceptLoop() {
	for atomic.LoadInt32(&s.running) == 1 {
		conn, err := s.listener.Accept()
		if err != nil {
			break
		}
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetNoDelay(true)
			_ = tcpConn.SetKeepAlive(true)
			_ = tcpConn.SetKeepAlivePeriod(30 * time.Second)
		}
		go s.handleClient(conn)
	}
}

func (s *Server) handleClient(conn net.Conn) {
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	reader := bufio.NewReader(conn)

	// SOCKS Version Negotiation
	version, err := reader.ReadByte()
	if err != nil || version != 0x05 {
		return
	}

	nMethods, err := reader.ReadByte()
	if err != nil || nMethods == 0 {
		return
	}

	methods := make([]byte, nMethods)
	if _, err = io.ReadFull(reader, methods); err != nil {
		return
	}

	// Auth Check
	if s.username != "" && s.password != "" {
		hasUserPass := false
		for _, m := range methods {
			if m == 0x02 {
				hasUserPass = true
				break
			}
		}
		if !hasUserPass {
			_, _ = conn.Write([]byte{0x05, 0xFF}) // No acceptable methods
			return
		}

		_, _ = conn.Write([]byte{0x05, 0x02}) // USER/PASS auth method

		authVer, err := reader.ReadByte()
		if err != nil || authVer != 0x01 {
			return
		}
		uLen, err := reader.ReadByte()
		if err != nil {
			return
		}
		userBuf := make([]byte, uLen)
		if _, err = io.ReadFull(reader, userBuf); err != nil {
			return
		}
		pLen, err := reader.ReadByte()
		if err != nil {
			return
		}
		passBuf := make([]byte, pLen)
		if _, err = io.ReadFull(reader, passBuf); err != nil {
			return
		}

		if string(userBuf) != s.username || string(passBuf) != s.password {
			_, _ = conn.Write([]byte{0x01, 0x01}) // Auth failure
			return
		}
		_, _ = conn.Write([]byte{0x01, 0x00}) // Auth success
	} else {
		_, _ = conn.Write([]byte{0x05, 0x00}) // NO AUTH
	}

	// Read Request
	header := make([]byte, 4)
	if _, err = io.ReadFull(reader, header); err != nil {
		return
	}
	if header[0] != 0x05 || header[2] != 0x00 {
		return
	}

	cmd := header[1]
	atyp := header[3]

	var host string
	switch atyp {
	case 0x01: // IPv4
		ipBuf := make([]byte, 4)
		if _, err = io.ReadFull(reader, ipBuf); err != nil {
			return
		}
		host = net.IP(ipBuf).String()
	case 0x03: // Domain
		dLen, err := reader.ReadByte()
		if err != nil {
			return
		}
		domainBuf := make([]byte, dLen)
		if _, err = io.ReadFull(reader, domainBuf); err != nil {
			return
		}
		host = string(domainBuf)
	case 0x04: // IPv6
		ipBuf := make([]byte, 16)
		if _, err = io.ReadFull(reader, ipBuf); err != nil {
			return
		}
		host = net.IP(ipBuf).String()
	default:
		_, _ = conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	portBuf := make([]byte, 2)
	if _, err = io.ReadFull(reader, portBuf); err != nil {
		return
	}
	port := int(binary.BigEndian.Uint16(portBuf))

	if cmd == 0x03 { // UDP ASSOCIATE
		s.handleUDPAssociate(conn)
		return
	}

	if cmd != 0x01 { // Only TCP CONNECT supported
		_, _ = conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	// Handle TCP CONNECT
	targetAddr := fmt.Sprintf("%s:%d", host, port)
	remote, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer remote.Close()

	if tcpRemote, ok := remote.(*net.TCPConn); ok {
		_ = tcpRemote.SetNoDelay(true)
		_ = tcpRemote.SetKeepAlive(true)
	}

	// SOCKS5 reply Success
	_, _ = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	_ = conn.SetDeadline(time.Time{})

	var wg sync.WaitGroup
	wg.Add(2)
	relay := func(dst io.Writer, src io.Reader) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		_, _ = io.CopyBuffer(dst, src, buf)
	}
	go relay(remote, reader)
	go relay(conn, remote)
	wg.Wait()
}

func (s *Server) handleUDPAssociate(client net.Conn) {
	if s.udpListener == nil {
		_, _ = client.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	reply := []byte{0x05, 0x00, 0x00}
	bindIP := "0.0.0.0"

	// Kembalikan IP server tempat klien terkoneksi (LocalAddr) agar klien bisa mengirim UDP ke IP yang benar
	if tcpAddr, ok := client.LocalAddr().(*net.TCPAddr); ok {
		ip := tcpAddr.IP
		if !ip.IsUnspecified() {
			bindIP = ip.String()
		}
	}

	parsedIP := net.ParseIP(bindIP)
	if parsedIP != nil && parsedIP.To4() != nil {
		reply = append(reply, 0x01)
		reply = append(reply, parsedIP.To4()...)
	} else if parsedIP != nil {
		reply = append(reply, 0x04)
		reply = append(reply, parsedIP.To16()...)
	} else {
		reply = append(reply, 0x01, 0, 0, 0, 0)
	}

	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(s.listenPort))
	reply = append(reply, portBytes...)

	_, _ = client.Write(reply)

	// RFC 1928: Keep TCP connection alive; when TCP closes, UDP association terminates
	tmp := make([]byte, 512)
	for {
		_, err := client.Read(tmp)
		if err != nil {
			break
		}
	}
}

func (s *Server) udpListenLoop() {
	buf := make([]byte, 65536)
	for atomic.LoadInt32(&s.running) == 1 {
		n, clientAddr, err := s.udpListener.ReadFrom(buf)
		if err != nil {
			break
		}
		if n < 10 || buf[2] != 0x00 { // RSV must be 0x00
			continue
		}

		atype := buf[3]
		idx := 4
		var dstIP string

		switch atype {
		case 0x01: // IPv4
			if idx+4 > n {
				continue
			}
			dstIP = fmt.Sprintf("%d.%d.%d.%d", buf[idx], buf[idx+1], buf[idx+2], buf[idx+3])
			idx += 4
		case 0x03: // Domain
			length := int(buf[idx])
			idx++
			if idx+length > n {
				continue
			}
			dstIP = string(buf[idx : idx+length])
			idx += length
		case 0x04: // IPv6
			if idx+16 > n {
				continue
			}
			dstIP = net.IP(buf[idx : idx+16]).String()
			idx += 16
		default:
			continue
		}

		if idx+2 > n {
			continue
		}
		dstPort := int(binary.BigEndian.Uint16(buf[idx : idx+2]))
		payload := buf[idx+2 : n]

		clientKey := clientAddr.String()
		var sess *udpSession
		if val, ok := s.udpSessions.Load(clientKey); ok {
			sess = val.(*udpSession)
		} else {
			outConn, bindErr := net.ListenPacket("udp4", "0.0.0.0:0")
			if bindErr != nil || outConn == nil {
				continue
			}
			sess = &udpSession{
				clientAddr: clientAddr,
				outConn:    outConn,
				lastActive: time.Now().Unix(),
				active:     1,
			}
			s.udpSessions.Store(clientKey, sess)

			// Reader goroutine for UDP responses from internet
			go func(session *udpSession, key string) {
				tBuf := make([]byte, 65536)
				for atomic.LoadInt32(&session.active) == 1 && atomic.LoadInt32(&s.running) == 1 {
					_ = session.outConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
					nt, tAddr, er := session.outConn.ReadFrom(tBuf)
					if er != nil {
						if netErr, ok := er.(net.Error); ok && netErr.Timeout() {
							continue
						}
						break
					}
					atomic.StoreInt64(&session.lastActive, time.Now().Unix())
					udpAddr, ok := tAddr.(*net.UDPAddr)
					if !ok {
						continue
					}

					respIP := udpAddr.IP.String()
					// Jika IP tadi di-fallback dari 1.1.1.1 ke 8.8.8.8, kembalikan 1.1.1.1 agar client (Brook) tidak menganggap 'invalid answer'
					if udpAddr.Port == 53 && session.lastTargetDNS != "" {
						respIP = session.lastTargetDNS
					}

					packed := packUDPHeader(respIP, udpAddr.Port, tBuf[:nt])
					if packed != nil {
						_, _ = s.udpListener.WriteTo(packed, session.clientAddr)
					}
				}
				session.close()
				s.udpSessions.Delete(key)
			}(sess, clientKey)

			// Reaper for inactive sessions
			go func(session *udpSession, key string) {
				for atomic.LoadInt32(&session.active) == 1 && atomic.LoadInt32(&s.running) == 1 {
					time.Sleep(5 * time.Second)
					if time.Now().Unix()-atomic.LoadInt64(&session.lastActive) > 60 {
						break
					}
				}
				session.close()
				s.udpSessions.Delete(key)
			}(sess, clientKey)
		}

		atomic.StoreInt64(&sess.lastActive, time.Now().Unix())

		// Jika klien (seperti Brook) mencoba query ke DNS yang diblokir/rto oleh ISP lokal (seperti 1.1.1.1 / 9.9.9.9),
		// secara cerdas forward ke Google DNS (8.8.8.8) yang terbukti tembus dan simpan IP aslinya
		if dstPort == 53 {
			sess.lastTargetDNS = dstIP
			if dstIP == "1.1.1.1" || dstIP == "1.0.0.1" || dstIP == "9.9.9.9" || dstIP == "208.67.222.222" {
				dstIP = "8.8.8.8"
			}
		}

		targetAddrStr := fmt.Sprintf("%s:%d", dstIP, dstPort)
		rAddr, err := net.ResolveUDPAddr("udp", targetAddrStr)
		if err == nil {
			_, _ = sess.outConn.WriteTo(payload, rAddr)
		}
	}
}

func packUDPHeader(dstIP string, dstPort int, payload []byte) []byte {
	header := []byte{0x00, 0x00, 0x00}
	ip := net.ParseIP(dstIP)
	if ip != nil && ip.To4() != nil {
		header = append(header, 0x01)
		header = append(header, ip.To4()...)
	} else if ip != nil {
		header = append(header, 0x04)
		header = append(header, ip.To16()...)
	} else {
		header = append(header, 0x03, byte(len(dstIP)))
		header = append(header, []byte(dstIP)...)
	}

	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(dstPort))
	header = append(header, portBytes...)
	return append(header, payload...)
}

// StartServer starts high-performance native SOCKS5 server (TCP+UDP) and optional FRP client.
func StartServer(localPort int, serverAddr string, serverPort int, token string, remotePort int, socksUser, socksPass string) string {
	mu.Lock()
	defer mu.Unlock()

	if isRunning {
		return "ERROR: Server is already running"
	}

	// 1. Inisialisasi Native High-Performance SOCKS5 Server
	s := &Server{
		listenPort: localPort,
		username:   socksUser,
		password:   socksPass,
	}

	if err := s.Start(); err != nil {
		return fmt.Sprintf("ERROR socks5 start: %v", err)
	}
	socksServer = s

	AddLog(fmt.Sprintf("SOCKS5 Server started on 0.0.0.0:%d (TCP+UDP)", localPort))

	// 2. Inisialisasi FRP Client (Hanya jika serverAddr diisi)
	if serverAddr != "" && remotePort > 0 {
		AddLog(fmt.Sprintf("Initiating FRP tunnel to VPS %s:%d -> RemotePort %d", serverAddr, serverPort, remotePort))
		common := &v1.ClientCommonConfig{}
		common.Complete()
		common.ServerAddr = serverAddr
		common.ServerPort = serverPort
		if token != "" {
			common.Auth.Method = v1.AuthMethodToken
			common.Auth.Token = token
		}

		tcpProxy := &v1.TCPProxyConfig{
			ProxyBaseConfig: v1.ProxyBaseConfig{
				Name:      fmt.Sprintf("socks5-tcp-%d", remotePort),
				Type:      string(v1.ProxyTypeTCP),
				LocalIP:   "127.0.0.1",
				LocalPort: localPort,
			},
			RemotePort: remotePort,
		}
		tcpProxy.Complete("")

		udpProxy := &v1.UDPProxyConfig{
			ProxyBaseConfig: v1.ProxyBaseConfig{
				Name:      fmt.Sprintf("socks5-udp-%d", remotePort),
				Type:      string(v1.ProxyTypeUDP),
				LocalIP:   "127.0.0.1",
				LocalPort: localPort,
			},
			RemotePort: remotePort,
		}
		udpProxy.Complete("")

		proxyCfgs := []v1.ProxyConfigurer{tcpProxy, udpProxy}
		warning, err := validation.ValidateAllClientConfig(common, proxyCfgs, nil)
		if err != nil {
			AddLog(fmt.Sprintf("FRP Config validation error: %v", err))
		} else {
			if warning != nil {
				AddLog(fmt.Sprintf("FRP Config warning: %v", warning))
			}
			frpSvr, err := client.NewService(client.ServiceOptions{
				Common:    common,
				ProxyCfgs: proxyCfgs,
			})
			if err != nil {
				AddLog(fmt.Sprintf("FRP NewService error: %v", err))
			} else {
				frpService = frpSvr
				ctx, cancel := context.WithCancel(context.Background())
				cancelFrp = cancel
				go func() {
					AddLog("FRP Client service is running in background...")
					runErr := frpService.Run(ctx)
					if runErr != nil {
						AddLog(fmt.Sprintf("FRP Service exited: %v", runErr))
					} else {
						AddLog("FRP Service closed normally.")
					}
				}()
			}
		}
	} else {
		AddLog("FRP Tunnel is disabled (no VPS address specified). SOCKS5 local only.")
	}

	isRunning = true
	return "OK"
}

// StopServer cleanly stops everything.
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
		socksServer.Stop()
		socksServer = nil
	}

	AddLog("Proxy & FRP stopped cleanly.")
	isRunning = false
	return "OK"
}

func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return isRunning
}
