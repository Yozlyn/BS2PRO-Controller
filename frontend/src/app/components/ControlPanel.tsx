import React, { useState, useCallback, useEffect } from 'react';
import { AlertTriangle, BarChart2, Bug, CheckCircle, ChevronDown, Clock, Eye, EyeOff, Flame, Lightbulb, Monitor, Pause, Play, Power, SlidersHorizontal, Zap } from 'lucide-react'
import { apiService } from '../services/api';
import { logger } from '../services/logger';
import { types } from '../../../wailsjs/go/models';
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime';
import { DebugInfo } from '../types/app';
import { ToggleSwitch, RadioGroup, Card, Badge, Button, Select } from './ui';
import clsx from 'clsx';

interface ControlPanelProps {
  config: types.AppConfig;
  onConfigChange: (config: types.AppConfig) => void;
  isConnected: boolean;
}

interface SettingItemProps {
  icon: React.ReactNode;
  iconBgActive: string;
  iconBgInactive: string;
  title: string;
  description: string;
  enabled: boolean;
  onChange: (enabled: boolean) => void;
  disabled?: boolean;
  loading?: boolean;
  color?: 'blue' | 'green' | 'purple' | 'orange';
}

function SettingItem({ 
  icon, 
  iconBgActive, 
  iconBgInactive, 
  title, 
  description, 
  enabled, 
  onChange, 
  disabled = false,
  loading = false,
  color = 'blue'
}: SettingItemProps) {
  return (
    <div className={clsx(
      'flex items-center justify-between py-4 px-4 -mx-4 rounded-xl transition-all duration-200',
      'hover:bg-gray-50 dark:hover:bg-gray-700/50',
      disabled && 'opacity-60'
    )}>
      <div className="flex items-center gap-4">
        <div className={clsx(
          'p-2.5 rounded-xl transition-all duration-300',
          enabled ? iconBgActive : iconBgInactive,
          enabled && 'scale-105 shadow-sm'
        )}>
          {icon}
        </div>
        <div>
          <div className="font-medium text-gray-900 dark:text-gray-300">{title}</div>
          <div className="text-sm text-gray-500 dark:text-gray-400">{description}</div>
        </div>
      </div>
      <ToggleSwitch
        enabled={enabled}
        onChange={onChange}
        disabled={disabled}
        loading={loading}
        color={color}
      />
    </div>
  );
}


interface DebugPanelProps {
  config: types.AppConfig
  toggleDebugMode: () => void
  toggleGuiMonitoring: () => void
  fetchDebugInfo: () => void
  debugInfo: DebugInfo | null
  debugInfoLoading: boolean
}

function DebugPanel({ config, toggleDebugMode, toggleGuiMonitoring, fetchDebugInfo, debugInfo, debugInfoLoading }: DebugPanelProps) {
  const [open, setOpen] = React.useState(false)
  return (
    <div className="mt-6 rounded-2xl border border-gray-200 dark:border-gray-700 overflow-hidden">
      <button
        onClick={() => setOpen(o => !o)}
        className="w-full px-4 py-3 flex items-center justify-between bg-gray-50 dark:bg-gray-700/50 hover:bg-gray-100 dark:hover:bg-gray-600/50 transition-colors"
      >
        <div className="flex items-center gap-3">
          <Bug className="w-5 h-5 text-indigo-600 dark:text-indigo-400" />
          <span className="font-medium text-gray-900 dark:text-gray-300">调试面板</span>
        </div>
        <ChevronDown className={clsx('w-5 h-5 text-gray-500 transition-transform duration-200', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="p-4 space-y-4">
          <div className="flex items-center justify-between p-3 rounded-xl bg-gray-50 dark:bg-gray-700/50">
            <div className="flex items-center gap-3">
              <Bug className="w-5 h-5 text-gray-600 dark:text-gray-400" />
              <div>
                <div className="font-medium text-gray-900 dark:text-gray-300">调试模式</div>
                <div className="text-xs text-gray-500 dark:text-gray-400">启用详细日志输出</div>
              </div>
            </div>
            <ToggleSwitch enabled={config.debugMode} onChange={toggleDebugMode} color="purple" />
          </div>
          <div className="flex items-center justify-between p-3 rounded-xl bg-gray-50 dark:bg-gray-700/50">
            <div className="flex items-center gap-3">
              {config.guiMonitoring ? (
                <Eye className="w-5 h-5 text-gray-600 dark:text-gray-400" />
              ) : (
                <EyeOff className="w-5 h-5 text-gray-600 dark:text-gray-400" />
              )}
              <div>
                <div className="font-medium text-gray-900 dark:text-gray-300">GUI 监控</div>
                <div className="text-xs text-gray-500 dark:text-gray-400">监控 GUI 响应状态</div>
              </div>
            </div>
            <ToggleSwitch enabled={config.guiMonitoring} onChange={toggleGuiMonitoring} color="purple" />
          </div>
          <Button variant="secondary" onClick={fetchDebugInfo} loading={debugInfoLoading} className="w-full">
            刷新调试信息
          </Button>
          {debugInfo && (
            <pre className="p-3 rounded-xl bg-gray-900 text-green-400 text-xs overflow-auto max-h-60">
              {JSON.stringify(debugInfo, null, 2)}
            </pre>
          )}
        </div>
      )}
    </div>
  )
}

export default function ControlPanel({ config, onConfigChange, isConnected }: ControlPanelProps) {
  const [loadingStates, setLoadingStates] = useState<Record<string, boolean>>({});
  const [debugInfo, setDebugInfo] = useState<DebugInfo | null>(null);
  const [debugInfoLoading, setDebugInfoLoading] = useState(false);
  const [customSpeedInput, setCustomSpeedInput] = useState<number>((config as any).customSpeedRPM || 2000);

  const setLoading = (key: string, value: boolean) => {
    setLoadingStates(prev => ({ ...prev, [key]: value }));
  };

  const handleOpenUrl = useCallback((url: string) => {
    try {
      BrowserOpenURL(url);
    } catch (error) {
      logger.error('打开链接失败', 'ControlPanel', error);
    }
  }, []);

  const handleAutoControlChange = useCallback(async (enabled: boolean) => {
    setLoading('autoControl', true);
    try {
      await apiService.setAutoControl(enabled);
      onConfigChange(types.AppConfig.createFrom({ ...config, autoControl: enabled }));
    } catch (error) {
      logger.error('设置智能变频失败', 'ControlPanel', error);
    } finally {
      setLoading('autoControl', false);
    }
  }, [config, onConfigChange]);

  const handleCustomSpeedApply = useCallback(async (enabled: boolean, rpm: number) => {
    setLoading('customSpeed', true);
    try {
      await apiService.setCustomSpeed(enabled, rpm);
      onConfigChange(types.AppConfig.createFrom({ 
        ...config, 
        customSpeedEnabled: enabled,
        customSpeedRPM: rpm,
        autoControl: enabled ? false : config.autoControl
      }));
    } catch (error) {
      logger.error('设置自定义转速失败', 'ControlPanel', error);
    } finally {
      setLoading('customSpeed', false);
    }
  }, [config, onConfigChange]);

  const handleCustomSpeedToggle = useCallback((enabled: boolean) => {
    if (enabled) {
      handleCustomSpeedApply(true, customSpeedInput);
    } else {
      handleCustomSpeedApply(false, customSpeedInput);
    }
  }, [customSpeedInput, handleCustomSpeedApply]);

  const handleGearLightChange = useCallback(async (enabled: boolean) => {
    if (!isConnected) return;
    setLoading('gearLight', true);
    try {
      const success = await apiService.setGearLight(enabled);
      if (success) {
        onConfigChange(types.AppConfig.createFrom({ ...config, gearLight: enabled }));
      }
    } catch (error) {
      logger.error('设置挡位灯失败', 'ControlPanel', error);
    } finally {
      setLoading('gearLight', false);
    }
  }, [config, onConfigChange, isConnected]);

  const handlePowerOnStartChange = useCallback(async (enabled: boolean) => {
    if (!isConnected) return;
    setLoading('powerOnStart', true);
    try {
      const success = await apiService.setPowerOnStart(enabled);
      if (success) {
        onConfigChange(types.AppConfig.createFrom({ ...config, powerOnStart: enabled }));
      }
    } catch (error) {
      logger.error('设置通电自启动失败', 'ControlPanel', error);
    } finally {
      setLoading('powerOnStart', false);
    }
  }, [config, onConfigChange, isConnected]);

  // Windows 开机自启动
  const handleWindowsAutoStartChange = useCallback(async (enabled: boolean) => {
    setLoading('windowsAutoStart', true);
    try {
      await apiService.setWindowsAutoStart(enabled);
      onConfigChange(types.AppConfig.createFrom({ ...config, windowsAutoStart: enabled }));
    } catch (error) {
      logger.error('设置开机自启动失败', 'ControlPanel', error);
      alert(`设置自启动失败: ${error}`);
    } finally {
      setLoading('windowsAutoStart', false);
    }
  }, [config, onConfigChange]);

  const handleIgnoreDeviceOnReconnectChange = useCallback(async (enabled: boolean) => {
    try {
      const newConfig = types.AppConfig.createFrom({ ...config, ignoreDeviceOnReconnect: enabled });
      await apiService.updateConfig(newConfig);
      onConfigChange(newConfig);
    } catch (error) {
      logger.error('设置断连保持配置模式失败', 'ControlPanel', error);
    }
  }, [config, onConfigChange]);

  const handleSmartStartStopChange = useCallback(async (mode: string) => {
    if (!isConnected) return;
    try {
      const success = await apiService.setSmartStartStop(mode);
      if (success) {
        onConfigChange(types.AppConfig.createFrom({ ...config, smartStartStop: mode }));
      }
    } catch (error) {
      logger.error('设置智能启停失败', 'ControlPanel', error);
    }
  }, [config, onConfigChange, isConnected]);

  const toggleDebugMode = useCallback(async () => {
    try {
      await apiService.setDebugMode(!config.debugMode);
      onConfigChange(types.AppConfig.createFrom({ ...config, debugMode: !config.debugMode }));
    } catch (error) {
      logger.error('设置调试模式失败', 'ControlPanel', error);
    }
  }, [config, onConfigChange]);

  const toggleGuiMonitoring = useCallback(async () => {
    try {
      const newConfig = types.AppConfig.createFrom({ ...config, guiMonitoring: !config.guiMonitoring });
      await apiService.updateConfig(newConfig);
      onConfigChange(newConfig);
    } catch (error) {
      logger.error('设置GUI监控失败', 'ControlPanel', error);
    }
  }, [config, onConfigChange]);

  const fetchDebugInfo = useCallback(async () => {
    try {
      setDebugInfoLoading(true);
      const info = await apiService.getDebugInfo();
      setDebugInfo(info);
    } catch (error) {
      logger.error('获取调试信息失败', 'ControlPanel', error);
    } finally {
      setDebugInfoLoading(false);
    }
  }, []);



  const smartStartStopOptions = [
    { value: 'off', label: '关闭', description: '禁用智能启停功能' },
    { value: 'immediate', label: '即时', description: '立即响应系统负载变化' },
    { value: 'delayed', label: '延时', description: '延时响应，避免频繁启停' },
  ];

  const allSampleCountOptions = [
    { value: 1, label: '1次 (即时响应)' },
    { value: 2, label: '2次 (2秒平均)' },
    { value: 3, label: '3次 (3秒平均)' },
    { value: 5, label: '5次 (5秒平均)' },
    { value: 10, label: '10次 (10秒平均)' },
  ];
  // 偏移模式开启时，采样间隔被强制为至少 3 秒，1次/2次 选项无实际意义，隐藏避免误导
  const sampleCountOptions = config.fanCurveOffsetEnabled
    ? allSampleCountOptions.filter(o => o.value >= 3)
    : allSampleCountOptions;

  const handleSampleCountChange = useCallback(async (count: number) => {
    try {
      const newConfig = types.AppConfig.createFrom({ ...config, tempSampleCount: count });
      await apiService.updateConfig(newConfig);
      onConfigChange(newConfig);
    } catch (error) {
      logger.error('设置温度采样次数失败', 'ControlPanel', error);
    }
  }, [config, onConfigChange]);

  const handleFanCurveOffsetChange = useCallback(async (enabled: boolean) => {
    try {
      // 开启偏移时确保采样数至少为 3，与后端强制逻辑保持一致
      const currentSampleCount = (config as any).tempSampleCount || 1;
      const safeSampleCount = enabled && currentSampleCount < 3 ? 3 : currentSampleCount;
      const newConfig = types.AppConfig.createFrom({ ...config, fanCurveOffsetEnabled: enabled, tempSampleCount: safeSampleCount });
      await apiService.updateConfig(newConfig);
      onConfigChange(newConfig);
    } catch (error) {
      logger.error('设置风扇曲线偏移失败', 'ControlPanel', error);
    }
  }, [config, onConfigChange]);

  return (
    <>
      <Card className="p-6">
        <div className="divide-y divide-gray-100 dark:divide-gray-700/50">
          <SettingItem
            icon={config.autoControl ? 
              <Play className="w-5 h-5 text-green-600 dark:text-green-400" /> : 
              <Pause className="w-5 h-5 text-gray-500 dark:text-gray-400" />
            }
            iconBgActive="bg-green-100 dark:bg-green-900/30"
            iconBgInactive="bg-gray-100 dark:bg-gray-700"
            title="自动温度控制"
            description="根据温度曲线自动调节风扇转速"
            enabled={config.autoControl}
            onChange={handleAutoControlChange}
            disabled={!isConnected || (config as any).customSpeedEnabled}
            loading={loadingStates.autoControl}
            color="green"
          />

          {config.autoControl && (
            <div className="py-4 px-4 -mx-4 rounded-xl hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-all duration-200">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-4">
                  <div className="p-2.5 rounded-xl bg-cyan-100 dark:bg-cyan-900/30">
                    <BarChart2 className="w-5 h-5 text-cyan-600 dark:text-cyan-400" />
                  </div>
                  <div>
                    <div className="font-medium text-gray-900 dark:text-gray-300">平滑曲线模式</div>
                    <div className="text-sm text-gray-500 dark:text-gray-400">
                      通过多次采样取平均值，减少温度波动对风扇转速的影响，防止频繁调整噪音
                    </div>
                  </div>
                </div>
                <Select
                  value={config.fanCurveOffsetEnabled
                    ? Math.max((config as any).tempSampleCount || 1, 3)
                    : ((config as any).tempSampleCount || 1)}
                  onChange={(val) => handleSampleCountChange(val as number)}
                  options={sampleCountOptions}
                  size="sm"
                />
              </div>
            </div>
          )}

          {config.autoControl && (
            <SettingItem
              icon={<SlidersHorizontal className="w-5 h-5 text-amber-600 dark:text-amber-400" />}
              iconBgActive="bg-amber-100 dark:bg-amber-900/30"
              iconBgInactive="bg-gray-100 dark:bg-gray-700"
              title="自动曲线偏移"
              description="根据温度趋势自动调整风扇偏移量，温度上升加速、稳定/下降节能降速"
              enabled={config.fanCurveOffsetEnabled ?? false}
              onChange={handleFanCurveOffsetChange}
              disabled={!isConnected}
              color="orange"
            />
          )}

          <div className="py-4 px-4 -mx-4 rounded-xl hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-all duration-200">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-4">
                <div className={clsx(
                  'p-2.5 rounded-xl transition-all duration-300',
                  (config as any).customSpeedEnabled
                    ? 'bg-blue-100 dark:bg-blue-900/30 scale-105'
                    : 'bg-gray-100 dark:bg-gray-700'
                )}>
                  <Flame className={clsx(
                    'w-5 h-5 transition-colors duration-300',
                    (config as any).customSpeedEnabled
                      ? 'text-blue-600 dark:text-blue-400'
                      : 'text-gray-500 dark:text-gray-400'
                  )} />
                </div>
                <div>
                  <div className="font-medium text-gray-900 dark:text-gray-300">自定义转速</div>
                  <div className="text-sm text-gray-500 dark:text-gray-400">
                    固定风扇转速，适合特殊场景使用
                  </div>
                </div>
              </div>
              <ToggleSwitch
                enabled={(config as any).customSpeedEnabled || false}
                onChange={handleCustomSpeedToggle}
                disabled={!isConnected}
                loading={loadingStates.customSpeed}
                color="blue"
              />
            </div>
            
            {(config as any).customSpeedEnabled && (
              <div className="pt-4 border-t border-gray-200 dark:border-gray-700 mt-4">
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  目标转速 (RPM)
                </label>
                <div className="flex items-center gap-3">
                  <input
                    type="number"
                    value={customSpeedInput}
                    onChange={(e) => setCustomSpeedInput(Number(e.target.value))}
                    className="flex-1 px-4 py-2.5 rounded-xl border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-300 focus:ring-2 focus:ring-blue-500 focus:border-transparent transition-all duration-200"
                    min={500}
                    max={4000}
                    step={50}
                  />
                  <Button
                    variant="primary"
                    onClick={() => handleCustomSpeedApply(true, customSpeedInput)}
                  >
                    应用
                  </Button>
                </div>
                <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
                  启用自定义转速时会暂时禁用智能温控功能
                </p>
              </div>
            )}
          </div>

          <SettingItem
            icon={<Lightbulb className={clsx(
              'w-5 h-5 transition-colors duration-300',
              config.gearLight ? 'text-yellow-500' : 'text-gray-500 dark:text-gray-400'
            )} />}
            iconBgActive="bg-yellow-100 dark:bg-yellow-900/30"
            iconBgInactive="bg-gray-100 dark:bg-gray-700"
            title="挡位灯"
            description="控制设备上的挡位指示灯"
            enabled={config.gearLight}
            onChange={handleGearLightChange}
            disabled={!isConnected}
            loading={loadingStates.gearLight}
            color="blue"
          />

          <SettingItem
            icon={<Power className={clsx(
              'w-5 h-5 transition-colors duration-300',
              config.powerOnStart ? 'text-blue-600 dark:text-blue-400' : 'text-gray-500 dark:text-gray-400'
            )} />}
            iconBgActive="bg-blue-100 dark:bg-blue-900/30"
            iconBgInactive="bg-gray-100 dark:bg-gray-700"
            title="通电自启动"
            description="设备通电后自动开始运行"
            enabled={config.powerOnStart}
            onChange={handlePowerOnStartChange}
            disabled={!isConnected}
            loading={loadingStates.powerOnStart}
            color="blue"
          />

          <div className="py-4 px-4 -mx-4 rounded-xl hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-all duration-200">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-4">
                <div className={clsx(
                  'p-2.5 rounded-xl transition-all duration-300',
                  config.windowsAutoStart 
                    ? 'bg-green-100 dark:bg-green-900/30 scale-105' 
                    : 'bg-gray-100 dark:bg-gray-700'
                )}>
                  <Monitor className={clsx(
                    'w-5 h-5 transition-colors duration-300',
                    config.windowsAutoStart 
                      ? 'text-green-600 dark:text-green-400' 
                      : 'text-gray-500 dark:text-gray-400'
                  )} />
                </div>
                <div>
                  <div className="font-medium text-gray-900 dark:text-gray-300">开机自启动</div>
                  <div className="text-sm text-gray-500 dark:text-gray-400">
                    Windows 启动时自动启动本程序
                  </div>
                  <div className="text-xs text-green-600 dark:text-green-400 mt-0.5">
                    💡 静默启动控制台托盘程序
                  </div>
                </div>
              </div>
              <ToggleSwitch
                enabled={config.windowsAutoStart}
                onChange={handleWindowsAutoStartChange}
                loading={loadingStates.windowsAutoStart}
                color="green"
              />
            </div>
          </div>

          <div className="py-4 px-4 -mx-4 rounded-xl hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-all duration-200">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-4">
                <div className={clsx(
                  'p-2.5 rounded-xl transition-all duration-300',
                  (config as any).ignoreDeviceOnReconnect 
                    ? 'bg-emerald-100 dark:bg-emerald-900/30 scale-105' 
                    : 'bg-gray-100 dark:bg-gray-700'
                )}>
                  <Clock className={clsx(
                    'w-5 h-5 transition-colors duration-300',
                    (config as any).ignoreDeviceOnReconnect 
                      ? 'text-emerald-600 dark:text-emerald-400' 
                      : 'text-gray-500 dark:text-gray-400'
                  )} />
                </div>
                <div>
                  <div className="font-medium text-gray-900 dark:text-gray-300">断连保持配置</div>
                  <div className="text-sm text-gray-500 dark:text-gray-400">
                    设备断开重连后继续使用APP配置，而不是设备默认状态
                  </div>
                  <div className="text-xs text-emerald-600 dark:text-emerald-400 mt-0.5">
                    推荐开启，防止设备异常断连导致进入手动模式
                  </div>
                </div>
              </div>
              <ToggleSwitch
                enabled={(config as any).ignoreDeviceOnReconnect ?? true}
                onChange={handleIgnoreDeviceOnReconnectChange}
                color="green"
              />
            </div>
          </div>

          <div className="py-4">
            <div className="flex items-center gap-4 mb-4">
              <div className="p-2.5 rounded-xl bg-purple-100 dark:bg-purple-900/30">
                <Zap className="w-5 h-5 text-purple-600 dark:text-purple-400" />
              </div>
              <div>
                <div className="font-medium text-gray-900 dark:text-gray-300">智能启停</div>
                <div className="text-sm text-gray-500 dark:text-gray-400">
                  根据系统负载智能控制风扇启停
                </div>
              </div>
            </div>
            <div className="ml-14">
              <RadioGroup
                value={config.smartStartStop || 'off'}
                onChange={handleSmartStartStopChange}
                options={smartStartStopOptions}
                disabled={!isConnected}
                orientation="horizontal"
              />
            </div>
          </div>
        </div>

        {!isConnected && (
          <div className="mt-6 p-4 rounded-xl bg-gray-100 dark:bg-gray-700/50 border border-gray-200 dark:border-gray-600">
            <div className="flex items-center gap-3 text-gray-600 dark:text-gray-400">
              <AlertTriangle className="w-5 h-5" />
              <span className="text-sm">设备未连接，部分功能不可用</span>
            </div>
          </div>
        )}

        {/* 调试面板 */}
        <DebugPanel config={config} toggleDebugMode={toggleDebugMode} toggleGuiMonitoring={toggleGuiMonitoring} fetchDebugInfo={fetchDebugInfo} debugInfo={debugInfo} debugInfoLoading={debugInfoLoading} />

      </Card>
    </>
  );
}