// Package asus 提供华硕 ACPI 设备接口
package asus

import (
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procDeviceIoCtrl = kernel32.NewProc("DeviceIoControl")
)

const (
	// IOCTL_ASUS_ACPI 华硕 ACPI 设备控制码
	IOCTL_ASUS_ACPI = 0x0022240C
	// ID_CPU_TEMP CPU 温度传感器设备 ID
	ID_CPU_TEMP = 0x00120094
)

// Client 华硕 ACPI 设备客户端
type Client struct {
	handle syscall.Handle
}

// NewClient 初始化并连接到 ATKACPI 设备
func NewClient() (*Client, error) {
	h, err := syscall.CreateFile(
		syscall.StringToUTF16Ptr(`\\.\ATKACPI`),
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}

	c := &Client{handle: h}
	c.init()
	return c, nil
}

// init 发送 INIT 指令初始化 ACPI 设备
func (c *Client) init() {
	in := make([]byte, 16)
	copy(in[0:4], []byte("INIT"))
	*(*uint32)(unsafe.Pointer(&in[4])) = 8

	// 分配输出缓冲区，即使返回值不重要，也避免访问违规
	out := make([]byte, 16)
	var ret uint32
	procDeviceIoCtrl.Call(
		uintptr(c.handle),
		uintptr(IOCTL_ASUS_ACPI),
		uintptr(unsafe.Pointer(&in[0])),
		uintptr(16),
		uintptr(unsafe.Pointer(&out[0])),
		uintptr(16),
		uintptr(unsafe.Pointer(&ret)),
		uintptr(0),
	)
}

// GetCPUTemperature 获取 CPU 实时温度
func (c *Client) GetCPUTemperature() (int, error) {
	in := make([]byte, 16)
	copy(in[0:4], []byte("DSTS"))
	*(*uint32)(unsafe.Pointer(&in[4])) = 8
	*(*uint32)(unsafe.Pointer(&in[8])) = ID_CPU_TEMP

	out := make([]byte, 16)
	var ret uint32

	r1, _, err := procDeviceIoCtrl.Call(
		uintptr(c.handle),
		uintptr(IOCTL_ASUS_ACPI),
		uintptr(unsafe.Pointer(&in[0])),
		uintptr(16),
		uintptr(unsafe.Pointer(&out[0])),
		uintptr(16),
		uintptr(unsafe.Pointer(&ret)),
		uintptr(0),
	)

	if r1 == 0 {
		return 0, err
	}

	if ret >= 4 {
		val := *(*uint32)(unsafe.Pointer(&out[0]))
		// 华硕算法：原始值 - 65536
		temperature := int(val) - 65536

		// 验证温度值在合理范围内
		if temperature >= 0 && temperature <= 150 {
			return temperature, nil
		}
	}

	return 0, syscall.Errno(0x1F) // ERROR_READ_FAULT
}

// Close 关闭设备句柄
func (c *Client) Close() {
	if c != nil && c.handle != 0 && c.handle != syscall.InvalidHandle {
		syscall.CloseHandle(c.handle)
		c.handle = 0
	}
}
