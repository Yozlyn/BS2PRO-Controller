'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  ChartBarIcon,
  PresentationChartLineIcon,
  CogIcon,
  ExclamationTriangleIcon,
  XMarkIcon,
  ComputerDesktopIcon,
  ArrowPathIcon,
} from '@heroicons/react/24/outline';
import { CheckCircleIcon } from '@heroicons/react/24/solid';
import DeviceStatus from './components/DeviceStatus';
import FanCurve from './components/FanCurve';
import ControlPanel from './components/ControlPanel';
import RGBControl from './components/RGBControl';
import { ToggleSwitch, Button } from './components/ui';
import { apiService } from './services/api';
import { types } from '../../wailsjs/go/models';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';
import clsx from 'clsx';

const BRIDGE_WARNING_MESSAGE = 'CPU/GPU 温度读取失败，可能被 Windows Defender 拦截，请将 TempBridge.exe 加入白名单或尝试重新安装后再试。';

export default function Home() {
  // 状态管理
  const [isConnected, setIsConnected] = useState(false);
  const [config, setConfig] = useState<types.AppConfig | null>(null);
  const [fanData, setFanData] = useState<types.FanData | null>(null);
  const [temperature, setTemperature] = useState<types.TemperatureData | null>(null);
  const [bridgeWarning, setBridgeWarning] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'status' | 'curve' | 'control' | 'debug'>('status');

  // 温度数据处理，附带桥接异常的全局提示
  const handleTemperaturePayload = useCallback((data: types.TemperatureData | null) => {
    setTemperature(data);

    if (data && data.bridgeOk === false) {
      const msg = (data.bridgeMessage || '').trim();
      setBridgeWarning(msg || BRIDGE_WARNING_MESSAGE);
    } else {
      setBridgeWarning(null);
    }
  }, []);

  // 初始化应用
  const initializeApp = useCallback(async () => {
    try {
      setIsLoading(true);
      
      // 获取配置
      const appConfig = await apiService.getConfig();
      setConfig(appConfig);
      
      // 获取设备状态
      const deviceStatus = await apiService.getDeviceStatus();
      setIsConnected(deviceStatus.connected || false);
      setFanData(deviceStatus.currentData || null);
      handleTemperaturePayload(deviceStatus.temperature || null);
      
      setError(null);
    } catch (err) {
      console.error('初始化失败:', err);
      setError('应用初始化失败');
    } finally {
      setIsLoading(false);
    }
  }, []);

  // 连接设备
  const handleConnect = useCallback(async () => {
    try {
      const success = await apiService.connectDevice();
      if (success) {
        setIsConnected(true);
        setError(null);
      }
    } catch (err) {
      console.error('连接失败:', err);
      setError('设备连接失败');
    }
  }, []);

  // 断开设备
  const handleDisconnect = useCallback(async () => {
    try {
      await apiService.disconnectDevice();
      setIsConnected(false);
      setFanData(null);
    } catch (err) {
      console.error('断开连接失败:', err);
    }
  }, []);

  // 更新配置
  const handleConfigChange = useCallback(async (newConfig: types.AppConfig) => {
    try {
      await apiService.updateConfig(newConfig);
      setConfig(newConfig);
    } catch (err) {
      console.error('配置更新失败:', err);
      setError('配置保存失败');
    }
  }, []);

  // 设置事件监听器
  useEffect(() => {
    const unsubscribers: (() => void)[] = [];

    // 设备连接事件
    unsubscribers.push(
      apiService.onDeviceConnected((deviceInfo) => {
        console.log('设备已连接:', deviceInfo);
        setIsConnected(true);
        setError(null);
      })
    );

    // 设备断开事件
    unsubscribers.push(
      apiService.onDeviceDisconnected(() => {
        console.log('设备已断开');
        setIsConnected(false);
        setFanData(null);
      })
    );

    // 设备错误事件
    unsubscribers.push(
      apiService.onDeviceError((errorMsg) => {
        console.error('设备错误:', errorMsg);
        setError(errorMsg);
      })
    );

    // 风扇数据更新事件
    unsubscribers.push(
      apiService.onFanDataUpdate((data) => {
        setFanData(data);
      })
    );

    // 温度数据更新事件
    unsubscribers.push(
      apiService.onTemperatureUpdate((data) => {
        handleTemperaturePayload(data);
      })
    );

    // 配置更新事件
    unsubscribers.push(
      apiService.onConfigUpdate((updatedConfig) => {
        setConfig(updatedConfig);
      })
    );

    // 清理函数
    return () => {
      unsubscribers.forEach(unsub => unsub());
    };
  }, [handleTemperaturePayload]);

  // 组件挂载时初始化
  useEffect(() => {
    initializeApp();
  }, [initializeApp]);

  // 加载状态
  if (isLoading) {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto mb-4"></div>
          <div className="text-gray-600 dark:text-gray-400">正在加载...</div>
        </div>
      </div>
    );
  }

  // 错误状态
  if (error && !config) {
    return (
      <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex items-center justify-center">
        <div className="text-center">
          <div className="text-red-600 dark:text-red-400 mb-4">
            <svg className="w-12 h-12 mx-auto mb-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.729-.833-2.5 0L4.232 15.5c-.77.833.192 2.5 1.732 2.5z" />
            </svg>
            {error}
          </div>
          <button
            onClick={initializeApp}
            className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 transition-colors"
          >
            重试
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex flex-col bg-gradient-to-br from-gray-50 to-gray-100 dark:from-gray-900 dark:to-gray-800">
      {/* 头部 */}
      <header className="sticky top-0 z-50 backdrop-blur-sm bg-white/80 dark:bg-gray-900/80 border-b border-gray-200/50 dark:border-gray-700/50">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4">
          <div className={clsx(
            'bg-white dark:bg-gray-800 rounded-2xl shadow-lg border border-gray-200 dark:border-gray-700 p-5 relative overflow-hidden',
            isConnected && 'bg-gradient-to-r from-white via-white to-blue-50/50 dark:from-gray-800 dark:via-gray-800 dark:to-blue-900/20'
          )}>
            {/* 背景装饰 */}
            {isConnected && (
              <div className="absolute top-0 right-0 w-32 h-32 bg-gradient-to-br from-blue-500/10 to-purple-500/10 rounded-full blur-2xl -translate-y-1/2 translate-x-1/2" />
            )}
            
            <div className="flex items-center justify-between relative">
              {/* 左侧：设备信息 */}
              <div className="flex items-center gap-4">
                {/* 设备图标 */}
                <div className="relative">
                  <div className={clsx(
                    'w-14 h-14 rounded-2xl flex items-center justify-center transition-all duration-300',
                    isConnected
                      ? 'bg-gradient-to-br from-blue-500 to-indigo-600 shadow-lg shadow-blue-500/30'
                      : 'bg-gray-100 dark:bg-gray-700/50'
                  )}>
                    <ComputerDesktopIcon className={clsx(
                      'w-7 h-7 transition-colors',
                      isConnected ? 'text-white' : 'text-gray-500 dark:text-gray-400'
                    )} />
                  </div>
                  {/* 连接状态指示点 */}
                  <div className={clsx(
                    'absolute -bottom-1 -right-1 w-5 h-5 rounded-full flex items-center justify-center ring-2 ring-white dark:ring-gray-800 transition-all duration-300',
                    isConnected
                      ? 'bg-green-500 shadow-lg shadow-green-500/50'
                      : 'bg-gray-400 dark:bg-gray-500'
                  )}>
                    {isConnected ? (
                      <CheckCircleIcon className="w-5 h-5 text-white" />
                    ) : (
                      <span className="w-2 h-2 rounded-full bg-white" />
                    )}
                  </div>
                </div>

                {/* 设备名称和状态 */}
                <div>
                  <h1 className="text-xl font-bold text-gray-900 dark:text-white">
                    BS2PRO 压风控制器
                  </h1>
                  {/* 已连接状态 */}
                  <div className="mt-1 flex items-center gap-2">
                    <div className={`inline-flex items-center space-x-1 px-2 py-1 rounded-full text-xs font-medium transition-all duration-200 ${
                      isConnected
                        ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400 border border-green-200 dark:border-green-800'
                        : 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300 border border-gray-200 dark:border-gray-600'
                    }`}>
                      <div className={`w-1.5 h-1.5 rounded-full transition-all duration-200 ${
                        isConnected ? 'bg-green-500' : 'bg-gray-400'
                      }`}></div>
                      <span>{isConnected ? '已连接' : '未连接'}</span>
                    </div>
                    
                    {/* 最高功率状态 */}
                    {isConnected && fanData && fanData.maxGear && (
                      (() => {
                        const maxGear = fanData.maxGear;
                        let bgColor = '';
                        let dotColor = '';
                        let displayText = maxGear;
                        
                        if (typeof maxGear === 'string' && maxGear.startsWith('未知(')) {
                          const match = maxGear.match(/未知\(0x([0-9A-Fa-f]+)\)/);
                          if (match) {
                            const hexValue = parseInt(match[1], 16);
                            if (hexValue === 0x03 || hexValue === 0x0B || hexValue === 0x00 || hexValue === 0x01) {
                              displayText = '标准';
                            } else if (hexValue === 0x04 || hexValue === 0x0C || hexValue === 0x0D) {
                              displayText = '强劲';
                            } else if (hexValue === 0x06 || hexValue === 0x0E || hexValue === 0x0F) {
                              displayText = '超频';
                            }
                            // 其他情况保持原样
                          }
                        }
                        
                        // 根据显示文本设置不同的颜色
                        if (displayText === '超频') {
                          bgColor = 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400 border border-purple-200 dark:border-purple-800';
                          dotColor = 'bg-purple-500';
                        } else if (displayText === '强劲') {
                          bgColor = 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400 border border-orange-200 dark:border-orange-800';
                          dotColor = 'bg-orange-500';
                        } else if (displayText === '标准') {
                          bgColor = 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300 border border-gray-200 dark:border-gray-600';
                          dotColor = 'bg-gray-400';
                        } else {
                          // 未知状态
                          bgColor = 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300 border border-gray-200 dark:border-gray-600';
                          dotColor = 'bg-gray-400';
                        }
                        
                        return (
                          <div className={`inline-flex items-center space-x-1 px-2 py-1 rounded-full text-xs font-medium transition-all duration-200 ${bgColor}`}>
                            <div className={`w-1.5 h-1.5 rounded-full transition-all duration-200 ${dotColor}`}></div>
                            <span>{displayText}</span>
                          </div>
                        );
                      })()
                    )}
                  </div>
                  {!isConnected && (
                    <div className="mt-1">
                      <span className="text-sm text-gray-500 dark:text-gray-400">
                        等待设备连接...
                      </span>
                    </div>
                  )}
                </div>
              </div>

              {/* 右侧：操作区 */}
              <div className="flex items-center gap-4">
                {isConnected && (
                  <div className="flex items-center gap-3 px-4 py-2 bg-gray-50 dark:bg-gray-700/50 rounded-xl border border-gray-200 dark:border-gray-600">
                    <ToggleSwitch
                      enabled={config?.autoControl || false}
                      onChange={async (enabled: boolean) => {
                        try {
                          await apiService.setAutoControl(enabled);
                          const newConfig = types.AppConfig.createFrom({ ...config, autoControl: enabled });
                          // 更新本地状态
                          setConfig(newConfig);
                        } catch (error) {
                          console.error('设置智能变频失败:', error);
                        }
                      }}
                      label="智能变频"
                      color="blue"
                    />
                  </div>
                )}
                <Button
                  variant={isConnected ? 'secondary' : 'primary'}
                  size="sm"
                  onClick={isConnected ? handleDisconnect : handleConnect}
                  icon={isConnected ? undefined : <ArrowPathIcon className="w-4 h-4" />}
                >
                  {isConnected ? '断开连接' : '连接设备'}
                </Button>
              </div>
            </div>
          </div>
        </div>
      </header>

      {/* 桥接程序异常的全局提醒 */}
      {bridgeWarning && (
        <div className="sticky top-16 z-40">
          <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 pt-3">
            <div className="relative rounded-2xl border border-amber-200 bg-amber-50 text-amber-900 shadow-md">
              <div className="flex items-start gap-3 px-4 py-3">
                <div className="mt-0.5">
                  <ExclamationTriangleIcon className="w-5 h-5 text-amber-500" />
                </div>
                <div className="flex-1">
                  <div className="text-sm font-semibold">温度读取受阻</div>
                  <p className="text-sm leading-relaxed text-amber-800">{bridgeWarning}</p>
                </div>
                <button
                  type="button"
                  onClick={() => setBridgeWarning(null)}
                  className="p-1 text-amber-700 hover:text-amber-900 hover:bg-amber-100 rounded-lg transition-colors"
                  aria-label="关闭提示"
                >
                  <XMarkIcon className="w-5 h-5" />
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* 主内容 */}
      <main className="flex-1 max-w-7xl mx-auto w-full px-4 sm:px-6 lg:px-8 py-4 md:py-8">
        {/* 标签页导航 */}
        <div className="mb-6 md:mb-8">
          <div className="bg-white dark:bg-gray-800 rounded-2xl shadow-lg border border-gray-200 dark:border-gray-700 p-1 md:p-2">
            <nav className="flex space-x-0.5 md:space-x-1" aria-label="Tabs">
              {[
                { id: 'status', name: '设备状态', icon: 'ChartBarIcon', desc: '实时监控' },
                { id: 'curve', name: '风扇曲线', icon: 'PresentationChartLineIcon', desc: '温度控制' },
                { id: 'control', name: '控制面板', icon: 'CogIcon', desc: '手动调节' },
              ].map((tab) => (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id as typeof activeTab)}
                  className={`${
                    activeTab === tab.id
                      ? 'bg-gradient-to-r from-blue-500 to-indigo-600 text-white shadow-lg shadow-blue-500/30'
                      : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-gray-700'
                  } group relative rounded-xl px-3 md:px-6 py-2 md:py-3 font-medium text-sm transition-all duration-200 flex-1 flex flex-col items-center space-y-0.5 md:space-y-1`}
                >
                  <div className="flex items-center space-x-1 md:space-x-2">
                    {tab.icon === 'ChartBarIcon' && <ChartBarIcon className="w-4 h-4 md:w-5 md:h-5" />}
                    {tab.icon === 'PresentationChartLineIcon' && <PresentationChartLineIcon className="w-4 h-4 md:w-5 md:h-5" />}
                    {tab.icon === 'CogIcon' && <CogIcon className="w-4 h-4 md:w-5 md:h-5" />}
                    <span className="font-semibold text-xs md:text-sm">{tab.name}</span>
                  </div>
                  <span className={`text-xs hidden md:block ${
                    activeTab === tab.id 
                      ? 'text-blue-100' 
                      : 'text-gray-500 dark:text-gray-500 group-hover:text-gray-600 dark:group-hover:text-gray-400'
                  }`}>
                    {tab.desc}
                  </span>
                </button>
              ))}
            </nav>
          </div>
        </div>

        {/* 标签页内容 */}
        <div className="space-y-4 md:space-y-8">
          {activeTab === 'status' && (
            <div className="space-y-4">
              <DeviceStatus
                isConnected={isConnected}
                fanData={fanData}
                temperature={temperature}
                config={config || new types.AppConfig()}
                onConnect={handleConnect}
                onDisconnect={handleDisconnect}
                onConfigChange={handleConfigChange}
              />
              <RGBControl
                isConnected={isConnected}
                savedConfig={config?.rgbConfig}
                onSetRGBMode={async (params) => {
                  try {
                    return await apiService.setRGBMode(params);
                  } catch {
                    return false;
                  }
                }}
              />
            </div>
          )}

          {activeTab === 'curve' && config && (
            <FanCurve
              config={config}
              onConfigChange={handleConfigChange}
              isConnected={isConnected}
              fanData={fanData}
              temperature={temperature}
            />
          )}

          {activeTab === 'control' && config && (
            <ControlPanel
              config={config}
              onConfigChange={handleConfigChange}
              isConnected={isConnected}
              fanData={fanData}
              temperature={temperature}
            />
          )}


        </div>
      </main>

      {/* 底部信息 */}
      <footer className="mt-auto border-t border-gray-200 dark:border-gray-700">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4">
          <div className="text-center text-sm text-gray-500 dark:text-gray-400">
            BS2PRO Controller - By{' '}
            <span 
              className="cursor-pointer hover:text-blue-600 dark:hover:text-blue-400 transition-colors group relative"
              title="点击访问开发者主页"
              onClick={() => BrowserOpenURL('https://www.tianli0.top/')}
            >
              Tianli
              {/* Tooltip */}
              <div className="absolute bottom-full left-1/2 transform -translate-x-1/2 mb-2 px-3 py-2 bg-gray-900 dark:bg-gray-700 text-white text-xs rounded-lg opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none whitespace-nowrap z-50">
                <div className="text-center">
                  <div className="font-medium">Tianli</div>
                  <div className="text-gray-300 dark:text-gray-400">www.tianli0.top</div>
                </div>
                {/* 箭头 */}
                <div className="absolute top-full left-1/2 transform -translate-x-1/2 w-0 h-0 border-l-4 border-r-4 border-t-4 border-transparent border-t-gray-900 dark:border-t-gray-700"></div>
              </div>
            </span>
          </div>
        </div>
      </footer>
    </div>
  );
}
