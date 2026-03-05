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

// acpiRequest 映射底层的内存结构 (16 bytes)
type acpiRequest struct {
	Signature [4]byte
	Size      uint32
	Arg       uint32
	Padding   uint32
}

type Client struct {
	handle syscall.Handle
}

// NewClient 初始化并连接到 ATKACPI 设备
func NewClient() (*Client, error) {
	devicePath, err := syscall.UTF16PtrFromString(`\\.\ATKACPI`)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(
		devicePath,
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
	if err := c.init(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

// init 发送 INIT 指令初始化 ACPI 设备
func (c *Client) init() error {
	req := acpiRequest{
		Size: 8,
	}
	copy(req.Signature[:], "INIT")

	var out [16]byte
	var ret uint32

	r1, _, err := procDeviceIoCtrl.Call(
		uintptr(c.handle),
		uintptr(IOCTL_ASUS_ACPI),
		uintptr(unsafe.Pointer(&req)),
		unsafe.Sizeof(req),
		uintptr(unsafe.Pointer(&out[0])),
		uintptr(len(out)),
		uintptr(unsafe.Pointer(&ret)),
		0,
	)

	if r1 == 0 {
		return err
	}
	return nil
}

// GetCPUTemperature 获取 CPU 实时温度
func (c *Client) GetCPUTemperature() (int, error) {
	req := acpiRequest{
		Size: 8,
		Arg:  ID_CPU_TEMP,
	}
	copy(req.Signature[:], "DSTS")

	var out [16]byte
	var ret uint32

	r1, _, err := procDeviceIoCtrl.Call(
		uintptr(c.handle),
		uintptr(IOCTL_ASUS_ACPI),
		uintptr(unsafe.Pointer(&req)),
		unsafe.Sizeof(req),
		uintptr(unsafe.Pointer(&out[0])),
		uintptr(len(out)),
		uintptr(unsafe.Pointer(&ret)),
		0,
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

	return 0, syscall.Errno(0x1F)
}

// Close 关闭设备句柄
func (c *Client) Close() {
	if c != nil && c.handle != 0 && c.handle != syscall.InvalidHandle {
		syscall.CloseHandle(c.handle)
		c.handle = 0
	}
}
