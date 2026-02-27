// Wails API 服务封装
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime';
import { 
  ConnectDevice, 
  DisconnectDevice, 
  GetDeviceStatus,
  GetConfig,
  UpdateConfig,
  SetFanCurve,
  GetFanCurve,
  SetAutoControl,
  GetAppVersion,
  SetManualGear,
  GetAvailableGears,
  SetGearLight,
  SetPowerOnStart,
  SetSmartStartStop,
  SetBrightness,
  GetTemperature,
  GetCurrentFanData,
  TestTemperatureReading,
  GetDebugInfo,
  SetDebugMode,
  UpdateGuiResponseTime,
  SetCustomSpeed,
  SetRGBMode,
  CheckWindowsAutoStart,
  SetWindowsAutoStart,
} from '../../../wailsjs/go/main/App';

import { types } from '../../../wailsjs/go/models';

import type { 
  DeviceInfo 
} from '../types/app';

class ApiService {
  // 设备连接
  async connectDevice(): Promise<boolean> {
    return await ConnectDevice();
  }

  async disconnectDevice(): Promise<void> {
    return await DisconnectDevice();
  }

  async getDeviceStatus(): Promise<any> {
    return await GetDeviceStatus();
  }

  // 配置管理
  async getConfig(): Promise<types.AppConfig> {
    return await GetConfig();
  }

  async getAppVersion(): Promise<string> {
    return await GetAppVersion();
  }

  async updateConfig(config: types.AppConfig): Promise<void> {
    return await UpdateConfig(config);
  }

  // 风扇曲线
  async setFanCurve(curve: types.FanCurvePoint[]): Promise<void> {
    return await SetFanCurve(curve);
  }

  async getFanCurve(): Promise<types.FanCurvePoint[]> {
    return await GetFanCurve();
  }

  // 智能变频
  async setAutoControl(enabled: boolean): Promise<void> {
    return await SetAutoControl(enabled);
  }

  // 自定义转速
  async setCustomSpeed(enabled: boolean, rpm: number): Promise<void> {
    return await SetCustomSpeed(enabled, rpm);
  }

  // 手动挡位控制
  async setManualGear(gear: string, level: string): Promise<boolean> {
    return await SetManualGear(gear, level);
  }

  // 获取可用挡位
  async getAvailableGears(): Promise<any> {
    return await GetAvailableGears();
  }

  // 设备设置
  async setGearLight(enabled: boolean): Promise<boolean> {
    return await SetGearLight(enabled);
  }

  async setPowerOnStart(enabled: boolean): Promise<boolean> {
    return await SetPowerOnStart(enabled);
  }

  async setSmartStartStop(mode: string): Promise<boolean> {
    return await SetSmartStartStop(mode);
  }

  async setBrightness(percentage: number): Promise<boolean> {
    return await SetBrightness(percentage);
  }

  async setRGBMode(params: { mode: string; colors: { r: number; g: number; b: number }[]; speed: string; brightness: number }): Promise<boolean> {
    return await SetRGBMode(params as any);
  }

  // Windows自启动相关
  async checkWindowsAutoStart(): Promise<boolean> {
    return await CheckWindowsAutoStart();
  }

  async setWindowsAutoStart(enabled: boolean): Promise<void> {
    return await SetWindowsAutoStart(enabled);
  }

  // 数据获取
  async getTemperature(): Promise<types.TemperatureData> {
    return await GetTemperature();
  }

  async getCurrentFanData(): Promise<types.FanData | null> {
    return await GetCurrentFanData();
  }

  async testTemperatureReading(): Promise<types.TemperatureData> {
    return await TestTemperatureReading();
  }

  // 桥接程序相关
  async getBridgeProgramStatus(): Promise<any> {
    return await (window as any).go?.main?.App?.GetBridgeProgramStatus();
  }

  async testBridgeProgram(): Promise<any> {
    return await (window as any).go?.main?.App?.TestBridgeProgram();
  }

  // 事件监听
  onDeviceConnected(callback: (data: DeviceInfo) => void): () => void {
    EventsOn('device-connected', callback);
    return () => EventsOff('device-connected');
  }

  onDeviceDisconnected(callback: () => void): () => void {
    EventsOn('device-disconnected', callback);
    return () => EventsOff('device-disconnected');
  }

  onDeviceError(callback: (error: string) => void): () => void {
    EventsOn('device-error', callback);
    return () => EventsOff('device-error');
  }

  onFanDataUpdate(callback: (data: types.FanData) => void): () => void {
    EventsOn('fan-data-update', callback);
    return () => EventsOff('fan-data-update');
  }

  onTemperatureUpdate(callback: (data: types.TemperatureData) => void): () => void {
    EventsOn('temperature-update', callback);
    return () => EventsOff('temperature-update');
  }

  onConfigUpdate(callback: (config: types.AppConfig) => void): () => void {
    EventsOn('config-update', callback);
    return () => EventsOff('config-update');
  }

  // 核心服务连接状态事件
  onCoreServiceError(callback: (msg: string) => void): () => void {
    EventsOn('core-service-error', callback);
    return () => EventsOff('core-service-error');
  }

  onCoreServiceConnected(callback: () => void): () => void {
    EventsOn('core-service-connected', callback);
    return () => EventsOff('core-service-connected');
  }

  // 调试相关方法
  async getDebugInfo(): Promise<any> {
    return await GetDebugInfo();
  }

  async setDebugMode(enabled: boolean): Promise<void> {
    return await SetDebugMode(enabled);
  }

  async updateGuiResponseTime(): Promise<void> {
    return await UpdateGuiResponseTime();
  }
}

export const apiService = new ApiService();
