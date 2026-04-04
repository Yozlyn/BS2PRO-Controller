// Package temperature 提供温度读取功能
package temperature

import (
	"math"
	"path/filepath"
	"runtime/debug"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/TIANLI0/BS2PRO-Controller/internal/asus"
	"github.com/TIANLI0/BS2PRO-Controller/internal/types"
)

// Reader 温度读取器
type Reader struct {
	asusClient *asus.Client
	logger     types.Logger

	gpuVendor  string
	nvmlDevice uintptr
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
	if r.asusClient != nil {
		cpuTemp, err := r.asusClient.GetCPUTemperature()
		if err == nil && cpuTemp > 0 && cpuTemp < 150 {
			temp.CPUTemp = cpuTemp
			temp.BridgeMsg = "使用ASUS ACPI接口"
		} else {
			temp.BridgeOk = false
			temp.BridgeMsg = "ASUS ACPI内核驱动未就绪，读取失败"
			temp.CPUTemp = 0
		}
	} else {
		temp.BridgeOk = false
		temp.BridgeMsg = "ASUS 客户端未初始化"
		temp.CPUTemp = 0
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

// NVML Windows Native绑定
var (
	nvmlDLL                  *syscall.LazyDLL
	nvmlInit                 *syscall.LazyProc
	nvmlDeviceGetHandle      *syscall.LazyProc
	nvmlDeviceGetTemperature *syscall.LazyProc

	nvmlLoaded       bool
	globalNvmlDevice uintptr
	nvmlOnce         sync.Once
)

const nvmlTemperatureGPU = 0

// initNVMLWindows 通过syscall本地加载 nvml.dll
func (r *Reader) initNVMLWindows() {
	if nvmlLoaded {
		r.gpuVendor = "nvidia"
		r.nvmlDevice = globalNvmlDevice
		return
	}
	nvmlOnce.Do(func() {
		var possiblePaths []string
		driverStorePattern := "C:\\Windows\\System32\\DriverStore\\FileRepository\\nv*\\nvml.dll"
		matches, err := filepath.Glob(driverStorePattern)
		if err == nil && len(matches) > 0 {
			possiblePaths = append(possiblePaths, matches...)
		}

		possiblePaths = append(possiblePaths,
			"C:\\Windows\\System32\\nvml.dll",
		)

		var loaded bool
		var lastErr error
		for _, path := range possiblePaths {
			nvmlDLL = syscall.NewLazyDLL(path)
			err := nvmlDLL.Load()
			if err == nil {
				r.logger.Debug("成功加载 nvml.dll", "path", path, "source", "nvml")
				loaded = true
				break
			}
			lastErr = err
		}

		if !loaded {
			r.logger.Warn("未找到 nvml.dll，可能未安装 NVIDIA 驱动", "source", "nvml", "error", lastErr)
			return
		}

		nvmlInit = nvmlDLL.NewProc("nvmlInit_v2")
		nvmlDeviceGetHandle = nvmlDLL.NewProc("nvmlDeviceGetHandleByIndex_v2")
		nvmlDeviceGetTemperature = nvmlDLL.NewProc("nvmlDeviceGetTemperature")

		ret, _, _ := nvmlInit.Call()
		if ret != 0 {
			r.logger.Warn("NVML 初始化失败", "source", "nvml", "state", ret)
			return
		}

		// 获取并缓存全局显卡句柄
		var device uintptr
		ret, _, _ = nvmlDeviceGetHandle.Call(0, uintptr(unsafe.Pointer(&device)))
		if ret == 0 {
			globalNvmlDevice = device // 存入全局缓存
			nvmlLoaded = true
			r.logger.Debug("NVML 本地 DLL 加载并初始化成功", "source", "nvml")
			// NVML初始化占用的临时内存归还OS
			debug.FreeOSMemory()
			kernel32 := syscall.NewLazyDLL("kernel32.dll")
			setWorkingSet := kernel32.NewProc("SetProcessWorkingSetSize")
			proc, _ := syscall.GetCurrentProcess()
			_, _, _ = setWorkingSet.Call(uintptr(proc), ^uintptr(0), ^uintptr(0))
		} else {
			r.logger.Warn("NVML 无法获取主显卡句柄", "source", "nvml", "state", ret)
		}
	})

	if nvmlLoaded {
		r.gpuVendor = "nvidia"
		r.nvmlDevice = globalNvmlDevice
	} else {
		r.gpuVendor = "unknown"
	}
}

// readGPUTemperature 读取GPU温度
func (r *Reader) readGPUTemperature() int {
	if r.gpuVendor == "" {
		r.initNVMLWindows()
	}

	if r.gpuVendor == "nvidia" && nvmlLoaded {
		return r.readNvidiaGPUTemp()
	}
	return 0
}

// readNvidiaGPUTemp 安全读取NVIDIA GPU温度
func (r *Reader) readNvidiaGPUTemp() int {
	if r.nvmlDevice == 0 {
		return 0
	}

	var temp uint32
	// 直接通过缓存读取温度
	ret, _, _ := nvmlDeviceGetTemperature.Call(r.nvmlDevice, nvmlTemperatureGPU, uintptr(unsafe.Pointer(&temp)))
	if ret != 0 {
		return 0
	}

	return int(temp)
}

// CalculateTargetRPM 根据温度线性插值计算目标转速
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
			roundedRPM := int(math.Round(rpm/100)) * 100
			if roundedRPM < 500 {
				return 500
			}
			if roundedRPM > 4000 {
				return 4000
			}
			return roundedRPM
		}
	}

	return 0
}

// CalculateOffset 根据温度线性插值计算风扇曲线偏移量
func CalculateOffset(temperature int, fanCurve []types.FanCurvePoint) int {
	if len(fanCurve) < 2 {
		return 0
	}

	if temperature <= fanCurve[0].Temperature {
		return fanCurve[0].Offset
	}

	lastPoint := fanCurve[len(fanCurve)-1]
	if temperature >= lastPoint.Temperature {
		return lastPoint.Offset
	}

	for i := 0; i < len(fanCurve)-1; i++ {
		p1 := fanCurve[i]
		p2 := fanCurve[i+1]

		if temperature >= p1.Temperature && temperature <= p2.Temperature {
			ratio := float64(temperature-p1.Temperature) / float64(p2.Temperature-p1.Temperature)
			offset := float64(p1.Offset) + ratio*float64(p2.Offset-p1.Offset)
			return int(math.Round(offset/100)) * 100
		}
	}

	return 0
}

// ApplyOffset 将偏移量应用到基础转速上，结果钳位到 500-4000 且为100的倍数
func ApplyOffset(baseRPM, offset int) int {
	rpm := baseRPM + offset
	rpm = int((float64(rpm)+50)/100) * 100
	if rpm < 500 {
		return 500
	}
	if rpm > 4000 {
		return 4000
	}
	return rpm
}
