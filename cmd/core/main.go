package main

import (
	"fmt"
	"log"
	"os"

	"github.com/kardianos/service"
)

// program 实现 service.Interface 接口
type program struct {
	app *CoreApp
}

// Start 服务启动时的回调
func (p *program) Start(s service.Service) error {
	// 检测命令行参数
	debugMode := false
	for _, arg := range os.Args {
		if arg == "--debug" || arg == "/debug" || arg == "-debug" {
			debugMode = true
		}
	}

	// 实例化核心应用
	p.app = NewCoreApp(debugMode)

	// 在后台协程中启动核心，防止阻塞系统服务管理器
	go func() {
		if err := p.app.Start(); err != nil {
			svcLogger, loggerErr := s.Logger(nil)
			if loggerErr == nil {
				svcLogger.Errorf("启动核心服务失败: %v", err)
			}
		}
	}()

	return nil
}

// Stop 服务停止时的回调
func (p *program) Stop(s service.Service) error {
	if p.app != nil {
		p.app.Stop()
	}
	return nil
}

func main() {
	// 配置 Windows 服务属性
	svcConfig := &service.Config{
		Name:        "BS2PRO_CoreService",
		DisplayName: "BS2 PRO 控制器核心服务",
		Description: "后台守护运行，负责 BS2 PRO 散热器的底层HID通信、硬件温控以及灯效管理。",
		Option: service.KeyValue{
			"RunAtLoad": true,
		},
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	// 处理服务相关的命令行指令 (install, uninstall, start, stop 等)
	if len(os.Args) > 1 {
		cmd := os.Args[1]
		if cmd == "install" || cmd == "uninstall" || cmd == "start" || cmd == "stop" || cmd == "restart" {
			err = service.Control(s, cmd)
			if err != nil {
				log.Fatalf("执行服务命令 '%s' 失败: %v", cmd, err)
			}
			fmt.Printf("成功执行服务命令: %s\n", cmd)
			return
		}
	}

	// 默认运行（如果通过系统服务管理器启动，则进入服务运行模式）
	err = s.Run()
	if err != nil {
		log.Fatal(err)
	}
}
