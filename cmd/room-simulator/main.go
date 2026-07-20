package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/server"
)

func main() {
	var (
		ip         = flag.String("ip", "", "本地绑定 IPv4（推荐填与 YABE 同网段的地址，如 192.168.3.115；空则 0.0.0.0）")
		iface      = flag.String("iface", "", "网卡名（可选，与 -ip 二选一，如 eth0 / Ethernet）")
		port       = flag.Int("port", 47810, "BACnet/IP UDP 端口（默认 47810，与 YABE 的 47808 分离）")
		subnet     = flag.Int("subnet", 24, "子网 CIDR（与 -ip 配合，如 24 表示 /24）")
		deviceID   = flag.Uint("device-id", 1234, "BACnet Device 实例号")
		deviceName = flag.String("device-name", "Room Simulator", "Device Object_Name")
		vendorID   = flag.Uint("vendor-id", 999, "Vendor Identifier")
		dynamic    = flag.Bool("dynamic", false, "缓慢变化 Space Temperature，便于观察刷新/COV")
	)
	flag.Parse()

	cfg := &server.DeviceConfig{
		DeviceID:   btypes.ObjectInstance(*deviceID),
		DeviceName: *deviceName,
		VendorID:   uint32(*vendorID),
		Ip:         *ip,
		Interface:  *iface,
		Port:       *port,
		SubnetCIDR: *subnet,
		MaxPDU:     btypes.MaxAPDU,
	}
	if cfg.Ip == "" && cfg.Interface == "" {
		cfg.Ip = "0.0.0.0"
	}

	srv, err := server.NewServer(cfg)
	if err != nil {
		log.Fatalf("创建服务端失败: %v", err)
	}
	defer srv.Close()

	if err := SeedRoomObjects(srv); err != nil {
		log.Fatalf("加载房间对象失败: %v", err)
	}

	go func() {
		if err := srv.Serve(); err != nil {
			log.Printf("服务端退出: %v", err)
		}
	}()

	printBanner(cfg)
	if *dynamic {
		go simulateSpaceTemperature(srv)
		log.Println("已启用 -dynamic：Space Temperature 会缓慢变化")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("正在停止 Room Simulator...")
}

func printBanner(cfg *server.DeviceConfig) {
	bind := cfg.Ip
	if cfg.Interface != "" {
		bind = "iface=" + cfg.Interface
	}
	fmt.Println("========================================")
	fmt.Println(" BACnet-Go Room Simulator")
	fmt.Println("========================================")
	fmt.Printf(" Device ID   : %d\n", cfg.DeviceID)
	fmt.Printf(" Device Name : %s\n", cfg.DeviceName)
	fmt.Printf(" Bind        : %s  port %d  /%d\n", bind, cfg.Port, cfg.SubnetCIDR)
	fmt.Println(" Objects     :")
	for _, o := range DefaultRoomObjects() {
		fmt.Printf("   - %-18s %-4d  %s\n", o.Type.String(), o.Instance, o.Name)
	}
	fmt.Println("----------------------------------------")
	fmt.Println(" YABE：与本机同网段 → Who-Is → 添加设备")
	fmt.Println(" 防火墙请放行 UDP", cfg.Port)
	if cfg.Ip == "" || cfg.Ip == "0.0.0.0" {
		fmt.Println(" 提示：同机运行 YABE 时建议指定本机具体 IP，例如：")
		fmt.Println("   -ip 192.168.3.115 -subnet 24")
		fmt.Println("   这样单播 ReadProperty 会正确路由到 Room Simulator")
	}
	fmt.Println(" 详见 ROOM_SIMULATOR.md")
	fmt.Println("========================================")
	fmt.Println("按 Ctrl+C 停止")
}

func simulateSpaceTemperature(srv server.Server) {
	const base = 22.5
	t0 := time.Now()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		// ±0.8°C slow sine wave
		offset := 0.8 * math.Sin(time.Since(t0).Seconds()/30.0)
		_ = srv.SetProperty(btypes.AnalogInput, 1, btypes.PROP_PRESENT_VALUE, base+offset)
	}
}
