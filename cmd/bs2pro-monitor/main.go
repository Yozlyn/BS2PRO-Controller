package main

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/ipc"
	"github.com/TIANLI0/BS2PRO-Controller/internal/notification"
)

func main() {
	client := ipc.NewClient(nil)
	client.SetRole(ipc.RoleMonitorAgent)
	client.SetEventHandler(handleEvent)
	serviceDownNotified := false

	for {
		if err := client.Connect(); err != nil {
			if !serviceDownNotified {
				_ = notification.Send("", "BS2PRO 后台服务已断开", "控制功能暂时不可用，系统正在等待恢复")
				serviceDownNotified = true
			}
			log.Printf("connect core failed: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		serviceDownNotified = false
		if _, err := client.SendRequest(ipc.ReqRegisterClient, ipc.RegisterClientParams{Role: ipc.RoleMonitorAgent}); err != nil {
			log.Printf("register monitor failed: %v", err)
		} else {
			log.Printf("register monitor success")
		}

		for client.IsConnected() {
			processName := strings.TrimSpace(getForegroundProcessName())
			if processName != "" {
				if _, err := client.SendRequest(ipc.ReqReportForegroundProcess, ipc.ReportForegroundProcessParams{
					ProcessName: processName,
					ReportedAt:  time.Now().Format(time.RFC3339),
				}); err != nil {
					log.Printf("report foreground process failed: %v", err)
					break
				}
			}
			time.Sleep(2 * time.Second)
		}

		client.Close()
		time.Sleep(1 * time.Second)
	}
}

func handleEvent(event ipc.Event) {
	if event.Type != ipc.EventNotificationRequest {
		return
	}

	var req notification.Request
	if err := json.Unmarshal(event.Data, &req); err != nil {
		log.Printf("parse notification failed: %v", err)
		return
	}
	log.Printf("received notification event: type=%s title=%s", req.Type, req.Title)
	if err := notification.Send("", req.Title, req.Message); err != nil {
		log.Printf("show notification failed: %v", err)
		return
	}
	log.Printf("notification shown: type=%s", req.Type)
}
