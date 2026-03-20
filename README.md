# BS2PRO-Controller

BS2PRO-Controller 是一个面向飞智空间站 BS2 / BS2PRO 的第三方桌面控制程序，用于提供设备连接、风扇控制、温度监控、RGB控制等功能。

本项目依赖 ASUS System Control Interface v3，使用前请先确认系统已正确安装该驱动。

## 功能概览

- 设备连接、状态检测与运行概览
- 风扇曲线编辑与自动温控
- 手动挡位、自定义转速与温控参数设置
- RGB 灯效模式、速度与颜色序列调整
- 基于前台进程的风扇策略联动

## 系统架构

项目主要由以下进程组成：

- `BS2PRO-Controller.exe`：图形界面主程序
- `BS2PRO-CoreService.exe`：后台核心服务
- `BS2PRO-Monitor.exe`：监控相关辅助进程

## 技术栈

- Go 1.26.1
- Wails v2
- Vue 3
- TypeScript
- Vite
- Tailwind CSS 4

## 开发环境

建议在 Windows 环境下进行完整开发与构建。

必需组件：

- Go 1.26.1 或更高版本
- Node.js 18 或更高版本
- Bun
- Wails CLI  
- go-winres  

可选组件：

- NSIS 3.x，用于生成安装程序

## 快速开始

```bash
git clone https://github.com/Yozlyn/BS2PRO-Controller.git
cd BS2PRO-Controller
go mod tidy

cd frontend
bun install
cd ..

wails dev
```

生产构建：

```bash
build.bat
```

构建输出位于 `build/bin/` 目录，包括：

- `BS2PRO-Controller.exe`
- `BS2PRO-CoreService.exe`
- `BS2PRO-Monitor.exe`

## 项目结构

```text
BS2PRO-Controller/
├── main.go
├── app.go
├── wails.json
├── build.bat
├── build_debug.bat
├── scripts/
├── cmd/
│   ├── core/
│   └── bs2pro-monitor/
├── internal/
├── frontend/
├── build/
└── LICENSE
```

## 配置与日志

配置与运行数据位于：

```text
%APPDATA%\BS2PRO-Controller\
```

程序会写入运行日志，并在异常时生成崩溃报告，便于排查问题。

## 构建说明

版本号定义于 `wails.json`，构建时会注入到可执行文件中。  
`build.bat` 会依次构建核心服务、监控进程与图形界面程序；如已安装 NSIS，还会生成安装程序。

## 贡献

欢迎通过 Issue 和 Pull Request 参与改进。

## 开源许可

本项目基于 MIT License 发布。详见 [LICENSE](LICENSE)。

## 作者

- TIANLI0 - [GitHub](https://github.com/TIANLI0)
- Email: wutianli@tianli0.top

## 致谢

- [Wails](https://wails.io/)
- [Vue](https://vuejs.org/)
- [Vite](https://vite.dev/)
- [GHelper](https://github.com/seerge/g-helper)
- 飞智 BS2 / BS2PRO 硬件设备

## 免责声明

本项目为第三方开源项目，与飞智官方无关。用户因使用本软件而产生的风险与后果需自行承担。

