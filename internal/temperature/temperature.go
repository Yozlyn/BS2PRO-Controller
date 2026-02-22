// Package temperature 提供温度读取功能
package temperature

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/TIANLI0/BS2PRO-Controller/internal/asus"
	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
	"github.com/shirou/gopsutil/v4/sensors"
)

// Reader 温度读取器
type Reader struct {
	asusClient *asus.Client
	logger     types.Logger

	// 缓存机制：防止无限制创建检测进程
	gpuVendor      string
	initVendorOnce sync.Once
}

// NewReader 创建新的温度读取器
func NewReader(asusClient *asus.Client, logger types.Logger) *Reader {
	return &Reader{
		asusClient: asusClient,
		logger:     logger,
	}
}

// Read 读取温度
func (r *Reader) Read() types.TemperatureData {
	temp := types.TemperatureData{
		UpdateTime: time.Now().Unix(),
		BridgeOk:   true,
	}

	// 使用 ASUS 接口读取 CPU 温度
	var cpuTemp int
	var err error

	if r.asusClient != nil {
		cpuTemp, err = r.asusClient.GetCPUTemperature()
	} else {
		err = fmt.Errorf("ASUS客户端未初始化")
	}

	if err == nil && cpuTemp > 0 && cpuTemp < 150 {
		temp.CPUTemp = cpuTemp
		temp.BridgeMsg = "使用ASUS ACPI接口"
	} else {
		// 降级方案
		temp.BridgeOk = false
		temp.BridgeMsg = "ASUS 接口异常或不支持，已切换至备用WMI/传感器读取模式"
		temp.CPUTemp = r.readCPUTemperature()
	}

	// 读取 GPU 温度
	temp.GPUTemp = r.readGPUTemperature()

	// 计算最高温度
	if temp.CPUTemp > temp.GPUTemp {
		temp.MaxTemp = temp.CPUTemp
	} else {
		temp.MaxTemp = temp.GPUTemp
	}

	return temp
}

// readCPUTemperature 读取CPU温度
func (r *Reader) readCPUTemperature() int {
	sensorTemps, err := sensors.SensorsTemperatures()
	if err == nil {
		for _, sensor := range sensorTemps {
			// 查找ACPI ThermalZone TZ00_0或类似的CPU温度传感器
			key := strings.ToLower(sensor.SensorKey)
			if strings.Contains(key, "tz00") || strings.Contains(key, "cpu") || strings.Contains(key, "core") {
				return int(sensor.Temperature)
			}
		}
	}

	// 如果传感器方式失败，尝试通过WMI (Windows)
	return r.readWindowsCPUTemp()
}

// getGPUVendor 仅检测一次GPU厂商并缓存
func (r *Reader) getGPUVendor() string {
	r.initVendorOnce.Do(func() {
		// 给予 2 秒超时，只检查一次
		if _, err := execCommandHidden(2*time.Second, "nvidia-smi", "--version"); err == nil {
			r.gpuVendor = "nvidia"
		} else {
			r.gpuVendor = "unknown"
		}
		r.logger.Debug("初始化检测 GPU 厂商完成: %s", r.gpuVendor)
	})
	return r.gpuVendor
}

// readGPUTemperature 读取GPU温度
func (r *Reader) readGPUTemperature() int {
	vendor := r.getGPUVendor()
	switch vendor {
	case "nvidia":
		return r.readNvidiaGPUTemp()
	default:
		return 0
	}
}

// readWindowsCPUTemp 通过WMI读取Windows CPU温度
func (r *Reader) readWindowsCPUTemp() int {
	// 增加 3 秒超时熔断，防止 WMI 服务挂起导致线程死锁
	output, err := execCommandHidden(3*time.Second, "wmic", "/namespace:\\\\root\\wmi", "PATH", "MSAcpi_ThermalZoneTemperature", "get", "CurrentTemperature", "/value")
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "CurrentTemperature="); ok {
			tempStr := strings.TrimSpace(after)
			if tempStr != "" {
				if temp, err := strconv.Atoi(tempStr); err == nil {
					celsius := (temp - 2732) / 10
					if celsius > 0 && celsius < 150 {
						return celsius
					}
				}
			}
		}
	}

	return 0
}

// readNvidiaGPUTemp 安全读取NVIDIA GPU温度
func (r *Reader) readNvidiaGPUTemp() int {
	// 增加 2 秒超时熔断，防止 nvidia-smi 驱动卡死
	output, err := execCommandHidden(2*time.Second, "nvidia-smi", "--query-gpu=temperature.gpu", "--format=csv,noheader,nounits")
	if err != nil {
		return 0
	}

	tempStr := strings.TrimSpace(string(output))
	lines := strings.Split(tempStr, "\n")

	if len(lines) > 0 && lines[0] != "" {
		if temp, err := strconv.Atoi(lines[0]); err == nil {
			return temp
		}
	}

	return 0
}

// execCommandHidden 执行命令并隐藏窗口，带严格的超时与僵尸进程防范
func execCommandHidden(timeout time.Duration, name string, args ...string) ([]byte, error) {
	// 创建一个带有超时取消的 Context
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	// 如果进程执行时间超过 timeout，ctx 会自动触发，强制 TerminateProcess 杀掉子进程
	return cmd.Output()
}

// CalculateTargetRPM 根据温度计算目标转速
func CalculateTargetRPM(temperature int, fanCurve []types.FanCurvePoint) int {
	if len(fanCurve) < 2 {
		return 0
	}

	if temperature <= fanCurve[0].Temperature {
		return fanCurve[0].RPM
	}

	lastPoint := fanCurve[len(fanCurve)-1]
	if temperature >= lastPoint.Temperature {
		return lastPoint.RPM
	}

	// 线性插值计算转速
	for i := 0; i < len(fanCurve)-1; i++ {
		p1 := fanCurve[i]
		p2 := fanCurve[i+1]

		if temperature >= p1.Temperature && temperature <= p2.Temperature {
			// 线性插值
			ratio := float64(temperature-p1.Temperature) / float64(p2.Temperature-p1.Temperature)
			rpm := float64(p1.RPM) + ratio*float64(p2.RPM-p1.RPM)
			return int(rpm)
		}
	}

	return 0
}
