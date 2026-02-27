// Package temperature 提供温度读取功能
package temperature

import (
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

	gpuVendor      string
	nvmlDevice     uintptr
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
	nvmlLoaded               bool
)

const nvmlTemperatureGPU = 0

// initNVMLWindows 通过syscall本地加载 nvml.dll
func (r *Reader) initNVMLWindows() {
	r.initVendorOnce.Do(func() {
		// 尝试直接加载nvml.dll
		nvmlDLL = syscall.NewLazyDLL("nvml.dll")
		if err := nvmlDLL.Load(); err != nil {
			// 降级策略：尝试从 NVIDIA 默认驱动安装路径硬加载
			nvmlDLL = syscall.NewLazyDLL("C:\\Program Files\\NVIDIA Corporation\\NVSMI\\nvml.dll")
			if err := nvmlDLL.Load(); err != nil {
				r.logger.Debug("未找到nvml.dll，可能未安装NVIDIA驱动")
				r.gpuVendor = "unknown"
				return
			}
		}

		// 获取所需的三个核心函数指针
		nvmlInit = nvmlDLL.NewProc("nvmlInit_v2")
		nvmlDeviceGetHandle = nvmlDLL.NewProc("nvmlDeviceGetHandleByIndex_v2")
		nvmlDeviceGetTemperature = nvmlDLL.NewProc("nvmlDeviceGetTemperature")

		// 调用nvmlInit_v2
		ret, _, _ := nvmlInit.Call()
		if ret != 0 { // 0代表NVML_SUCCESS
			r.logger.Debug("NVML初始化失败，返回码: %d", ret)
			r.gpuVendor = "unknown"
			return
		}

		// 获取并缓存显卡句柄
		var device uintptr
		ret, _, _ = nvmlDeviceGetHandle.Call(0, uintptr(unsafe.Pointer(&device)))
		if ret == 0 {
			r.nvmlDevice = device
			r.gpuVendor = "nvidia"
			nvmlLoaded = true
			r.logger.Debug("NVML本地DLL加载并初始化成功")
		} else {
			r.logger.Debug("NVML无法获取主显卡句柄，返回码: %d", ret)
			r.gpuVendor = "unknown"
		}
	})
}

// readGPUTemperature 读取GPU温度
func (r *Reader) readGPUTemperature() int {
	r.initNVMLWindows()

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
			// 将转速调整为100的整数倍
			roundedRPM := int((rpm+50)/100) * 100
			// 确保在有效范围内
			if roundedRPM < 1000 {
				return 1000
			}
			if roundedRPM > 4000 {
				return 4000
			}
			return roundedRPM
		}
	}

	return 0
}
