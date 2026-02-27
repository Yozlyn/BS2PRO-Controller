'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  PresentationChartLineIcon,
  Cog6ToothIcon,
  ExclamationTriangleIcon,
  XMarkIcon,
  SwatchIcon,
  PowerIcon,
  InformationCircleIcon,
  SparklesIcon
} from '@heroicons/react/24/outline';

import FanCurve from './components/FanCurve';
import ControlPanel from './components/ControlPanel';
import RGBControl from './components/RGBControl';
import AboutPanel from './components/AboutPanel';
import { apiService } from './services/api';
import { types } from '../../wailsjs/go/models';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';

const BRIDGE_WARNING_MESSAGE = 'CPU/GPU 温度读取失败，可能被 Windows Defender 拦截，请将 TempBridge.exe 加入白名单或尝试重新安装后再试。';
const FanHex = ({ className }: { className?: string }) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" className={className}>
    <polygon points="12 2 20.66 7 20.66 17 12 22 3.34 17 3.34 7" strokeOpacity="0.4" />
    <circle cx="12" cy="12" r="3" />
    <path d="M12 8L12 2M16 12L22 12M12 16L12 22M8 12L2 12" />
    <path d="M14.8 9.2L18.5 5.5M14.8 14.8L18.5 18.5M9.2 14.8L5.5 18.5M9.2 9.2L5.5 5.5" strokeOpacity="0.4"/>
  </svg>
);

const getTempStatus = (temp: number) => {
  if (temp > 85) return { color: 'text-red-500', bg: 'bg-red-500', label: '过热' };
  if (temp > 75) return { color: 'text-orange-500', bg: 'bg-orange-500', label: '偏高' };
  if (temp > 60) return { color: 'text-yellow-500', bg: 'bg-yellow-500', label: '正常' };
  return { color: 'text-emerald-500', bg: 'bg-emerald-500', label: '良好' };
};


export default function Home() {
  const [isConnected, setIsConnected] = useState(false);
  const [config, setConfig] = useState<types.AppConfig | null>(null);
  const [fanData, setFanData] = useState<types.FanData | null>(null);
  const [temperature, setTemperature] = useState<types.TemperatureData | null>(null);
  const [bridgeWarning, setBridgeWarning] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  const [activeTab, setActiveTab] = useState<'curve' | 'rgb' | 'control' | 'about'>('curve'); 

  const handleTemperaturePayload = useCallback((data: types.TemperatureData | null) => {
    setTemperature(data);
    if (data && data.bridgeOk === false) {
      setBridgeWarning((data.bridgeMessage || '').trim() || BRIDGE_WARNING_MESSAGE);
    } else {
      setBridgeWarning(null);
    }
  }, []);

  const initializeApp = useCallback(async () => {
    setIsLoading(true);
    try {
      setConfig(await apiService.getConfig());
      setError(null);
    } catch (err) {}

    try {
      const deviceStatus = await apiService.getDeviceStatus();
      setIsConnected(deviceStatus.connected || false);
      setFanData(deviceStatus.currentData || null);
      handleTemperaturePayload(deviceStatus.temperature || null);
    } catch (err) {
      setIsConnected(false);
    }
    setIsLoading(false);
  }, [handleTemperaturePayload]);

  const handleConnect = useCallback(async () => {
    try {
      if (await apiService.connectDevice()) {
        setIsConnected(true);
        setError(null);
      }
    } catch (err) {
      setError('设备连接失败');
    }
  }, []);

  const handleDisconnect = useCallback(async () => {
    try {
      await apiService.disconnectDevice();
      setIsConnected(false);
      setFanData(null);
    } catch (err) {}
  }, []);

  const handleConfigChange = useCallback(async (newConfig: types.AppConfig) => {
    setConfig(newConfig);
    try { await apiService.updateConfig(newConfig); } catch (err) {}
  }, []);

  useEffect(() => {
    const unsubscribers = [
      apiService.onDeviceConnected(() => { setIsConnected(true); setError(null); }),
      apiService.onDeviceDisconnected(() => { setIsConnected(false); setFanData(null); }),
      apiService.onDeviceError((errorMsg: string) => { setError(errorMsg); }),
      apiService.onFanDataUpdate((data: types.FanData) => { setFanData(data); }),
      apiService.onTemperatureUpdate((data: types.TemperatureData) => { handleTemperaturePayload(data); }),
      apiService.onConfigUpdate((updatedConfig: types.AppConfig) => { setConfig(updatedConfig); }),
      apiService.onCoreServiceError((msg: string) => {
        setError(msg);
        setIsConnected(false);
      }),
      
      apiService.onCoreServiceConnected(() => {
        setError(null);
        initializeApp();
      })
    ];
    return () => { unsubscribers.forEach(unsub => unsub()); };
  }, [handleTemperaturePayload, initializeApp]);

  useEffect(() => {
    const interval = setInterval(() => { apiService.updateGuiResponseTime().catch(() => {}); }, 10000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => { initializeApp(); }, [initializeApp]);

  const getMaxGearStatus = () => {
    if (!fanData || !fanData.maxGear) return null;
    let displayText = fanData.maxGear;
    let colorClass = 'text-slate-600 bg-slate-100 border-slate-200 dark:text-slate-300 dark:bg-slate-800 dark:border-slate-700';
    let dotColor = 'bg-slate-400';

    if (typeof fanData.maxGear === 'string' && fanData.maxGear.startsWith('未知(')) {
      const match = fanData.maxGear.match(/未知\(0x([0-9A-Fa-f]+)\)/);
      if (match) {
        const hex = parseInt(match[1], 16);
        if ([0x03, 0x0B, 0x00, 0x01].includes(hex)) displayText = '标准';
        else if ([0x04, 0x0C, 0x0D].includes(hex)) displayText = '强劲';
        else if ([0x06, 0x0E, 0x0F].includes(hex)) displayText = '超频';
      }
    }

    if (displayText.includes('超频')) { 
      colorClass = 'text-purple-700 bg-purple-50 border-purple-200 dark:text-purple-400 dark:bg-purple-900/30 dark:border-purple-800'; 
      dotColor = 'bg-purple-500'; 
    } else if (displayText.includes('强劲')) { 
      colorClass = 'text-orange-700 bg-orange-50 border-orange-200 dark:text-orange-400 dark:bg-orange-900/30 dark:border-orange-800'; 
      dotColor = 'bg-orange-500'; 
    } else if (displayText.includes('标准')) { 
      colorClass = 'text-blue-700 bg-blue-50 border-blue-200 dark:text-blue-400 dark:bg-blue-900/30 dark:border-blue-800'; 
      dotColor = 'bg-blue-500'; 
    }

    return { displayText, colorClass, dotColor };
  };

  const gearStatus = getMaxGearStatus();
  
  // 获取当前温度状态
  const cpuStatus = getTempStatus(temperature?.cpuTemp || 0);
  const gpuStatus = getTempStatus(temperature?.gpuTemp || 0);

  if (isLoading) {
    return (
      <div className="min-h-screen bg-slate-50 dark:bg-[#0b0c10] flex items-center justify-center">
        <div className="text-center space-y-4">
          <div className="w-10 h-10 border-2 border-blue-200 border-t-blue-600 rounded-full animate-spin mx-auto"></div>
          <div className="font-medium text-slate-600 dark:text-slate-400 text-sm">系统初始化中...</div>
        </div>
      </div>
    );
  }

  if (error && !config) {
    return (
      <div className="min-h-screen bg-slate-50 dark:bg-[#0b0c10] flex items-center justify-center">
        <div className="text-center p-8 rounded-2xl border border-red-200 dark:border-red-900/50 bg-red-50 dark:bg-red-900/20">
          <ExclamationTriangleIcon className="w-12 h-12 text-red-500 mx-auto mb-4" />
          <div className="font-bold text-slate-800 dark:text-slate-300 mb-6">{error}</div>
          <button onClick={() => initializeApp()} className="px-6 py-2 bg-red-600 text-white hover:bg-red-700 font-medium text-sm rounded-lg transition-all">
            重试连接
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen p-4 md:p-6 lg:p-8 font-sans bg-slate-100 dark:bg-[#0b0c10] text-slate-900 dark:text-slate-200 transition-colors duration-300">

      <div className="w-full max-w-[1024px] mx-auto flex flex-col gap-4">
        
        {bridgeWarning && (
          <div className="absolute top-0 left-1/2 -translate-x-1/2 -translate-y-[120%] z-50 w-full max-w-2xl animate-in slide-in-from-top-4 duration-500">
            <div className="bg-amber-50 dark:bg-amber-950/80 px-4 py-3 rounded-xl flex items-start gap-4 border border-amber-200 dark:border-amber-800 shadow-md">
              <ExclamationTriangleIcon className="w-5 h-5 text-amber-500 shrink-0 mt-0.5" />
              <div className="flex-1">
                <h4 className="font-bold text-amber-800 dark:text-amber-500 text-sm mb-1">温度读取受阻</h4>
                <p className="text-xs text-amber-700 dark:text-amber-400 leading-relaxed">{bridgeWarning}</p>
              </div>
              <button onClick={() => setBridgeWarning(null)} className="text-amber-500 hover:text-amber-700 transition-colors">
                <XMarkIcon className="w-5 h-5" />
              </button>
            </div>
          </div>
        )}

        <div className="h-[72px] rounded-2xl shrink-0 shadow-sm border border-slate-200 dark:border-slate-800 bg-white dark:bg-[#161922] z-10 transition-colors duration-300">
          <div className="w-full h-full px-6 flex items-center justify-between">
            <div className="flex items-center gap-6">
              <div className="flex items-center gap-3 group cursor-pointer">
                <FanHex className={`w-8 h-8 transition-all duration-500 ${isConnected ? 'text-blue-600 dark:text-blue-400 animate-[spin_4s_linear_infinite] group-hover:animate-[spin_1s_linear_infinite]' : 'text-slate-500 animate-[spin_12s_linear_infinite] group-hover:animate-[spin_3s_linear_infinite]'}`} />
                <div>
                  <h1 className="font-bold text-lg text-slate-800 dark:text-slate-300 tracking-wide">BS2PRO</h1>
                  <p className="text-[10px] font-medium text-slate-500 mt-0.5 uppercase tracking-wider">轻量控制台</p>
                </div>
              </div>
              
              <div className="w-px h-8 bg-slate-200 dark:bg-slate-700"></div>
              
              <div className="flex items-center gap-3">
                <span className={`px-2.5 py-1 rounded-full text-xs font-semibold flex items-center gap-1.5 ${isConnected ? 'bg-emerald-50 text-emerald-600 border border-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-400 dark:border-emerald-500/20' : 'bg-slate-100 text-slate-500 border border-slate-200 dark:bg-slate-800 dark:border-slate-700'}`}>
                  <span className={`w-1.5 h-1.5 rounded-full ${isConnected ? 'bg-emerald-500' : 'bg-slate-400'}`}></span>
                  {isConnected ? '已连接' : '未连接'}
                </span>
                {isConnected && gearStatus && (
                  <span className={`text-xs font-semibold px-2.5 py-1 rounded-full border flex items-center gap-1.5 ${gearStatus.colorClass}`}>
                    <span className={`w-1.5 h-1.5 rounded-full ${gearStatus.dotColor}`}></span>
                    {gearStatus.displayText}
                  </span>
                )}
              </div>
            </div>

            <div className="flex items-center gap-6">
              {isConnected && config && (
                <div className="flex items-center gap-3 px-4 py-2 rounded-xl border border-slate-300 dark:border-slate-600">
                  <span className={`text-sm font-semibold ${config.autoControl ? 'text-slate-800 dark:text-slate-300' : 'text-slate-500'}`}>智能变频</span>
                  <div
                    onClick={async () => {
                      try {
                        await apiService.setAutoControl(!config.autoControl);
                        setConfig(types.AppConfig.createFrom({ ...config, autoControl: !config.autoControl }));
                      } catch (err) {}
                    }}
                    className={`w-10 h-5 rounded-full border transition-all relative cursor-pointer ${config.autoControl ? 'bg-blue-600 border-blue-600 dark:bg-blue-500 dark:border-blue-500' : 'bg-slate-200 border-slate-300 dark:bg-slate-700 dark:border-slate-600'}`}
                  >
                    <div className={`absolute top-1/2 -translate-y-1/2 w-4 h-4 rounded-full bg-white transition-all duration-300 shadow-sm ${config.autoControl ? 'right-0.5' : 'left-0.5'}`}></div>
                  </div>
                </div>
              )}
              <button
                onClick={() => isConnected ? handleDisconnect() : handleConnect()}
                className={`flex items-center gap-2 text-sm font-semibold transition-colors px-4 py-2 rounded-xl ${isConnected ? 'bg-slate-100 text-slate-600 hover:bg-red-50 hover:text-red-600 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-red-900/30 dark:hover:text-red-400' : 'bg-blue-600 text-white hover:bg-blue-700'}`}
              >
                <PowerIcon className="w-4 h-4" /> {isConnected ? '断开连接' : '连接设备'}
              </button>
            </div>
          </div>
        </div>

        <div className="h-[110px] grid grid-cols-3 gap-4 shrink-0 z-10">
          
          {/* 1. CPU 温度 */}
          <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-[#161922] shadow-sm flex flex-col justify-between p-5 relative overflow-hidden transition-colors duration-300">
            <div className="flex justify-between items-start z-10">
              <div className="flex items-center gap-2">
                <div className="w-8 h-8 rounded-lg bg-orange-50 dark:bg-orange-900/20 flex items-center justify-center">
                  <svg className="w-4 h-4 text-orange-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"/></svg>
                </div>
                <span className="text-sm font-semibold text-slate-600 dark:text-slate-400">CPU 温度</span>
              </div>
              <span className={`text-xs font-bold ${isConnected ? cpuStatus.color : 'text-slate-400 dark:text-slate-500'}`}>
                {isConnected ? cpuStatus.label : '离线'}
              </span>
            </div>
            <div className="z-10 flex items-baseline gap-1 mt-2">
              <span className={`text-3xl font-black tabular-nums tracking-tight ${isConnected ? cpuStatus.color : 'text-slate-400 dark:text-slate-600'}`}>
                {isConnected ? (temperature?.cpuTemp ?? 0) : '--'}
              </span>
              <span className="text-xs font-semibold text-slate-500">°C</span>
            </div>
            <div className="absolute bottom-0 left-0 right-0 h-1 bg-slate-100 dark:bg-slate-800">
              <div 
                className={`h-full ${isConnected ? cpuStatus.bg : 'bg-slate-300 dark:bg-slate-700'} transition-all duration-500`} 
                style={{ width: isConnected ? `${Math.min(100, ((temperature?.cpuTemp || 0) / 100) * 100)}%` : '0%' }}
              />
            </div>
          </div>

          {/* 2. GPU 温度 */}
          <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-[#161922] shadow-sm flex flex-col justify-between p-5 relative overflow-hidden transition-colors duration-300">
            <div className="flex justify-between items-start z-10">
              <div className="flex items-center gap-2">
                <div className="w-8 h-8 rounded-lg bg-purple-50 dark:bg-purple-900/20 flex items-center justify-center">
                  <svg className="w-4 h-4 text-purple-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z M8 10h8v4H8z"/></svg>
                </div>
                <span className="text-sm font-semibold text-slate-600 dark:text-slate-400">GPU 温度</span>
              </div>
              <span className={`text-xs font-bold ${isConnected ? gpuStatus.color : 'text-slate-400 dark:text-slate-500'}`}>
                {isConnected ? gpuStatus.label : '离线'}
              </span>
            </div>
            <div className="z-10 flex items-baseline gap-1 mt-2">
              <span className={`text-3xl font-black tabular-nums tracking-tight ${isConnected ? gpuStatus.color : 'text-slate-400 dark:text-slate-600'}`}>
                {isConnected ? (temperature?.gpuTemp ?? 0) : '--'}
              </span>
              <span className="text-xs font-semibold text-slate-500">°C</span>
            </div>
            <div className="absolute bottom-0 left-0 right-0 h-1 bg-slate-100 dark:bg-slate-800">
              <div 
                className={`h-full ${isConnected ? gpuStatus.bg : 'bg-slate-300 dark:bg-slate-700'} transition-all duration-500`} 
                style={{ width: isConnected ? `${Math.min(100, ((temperature?.gpuTemp || 0) / 100) * 100)}%` : '0%' }}
              />
            </div>
          </div>

          {/* 3. 风扇转速 */}
          <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-[#161922] shadow-sm flex flex-col justify-between p-5 relative overflow-hidden transition-colors duration-300">
            <div className="flex justify-between items-start z-10">
              <div className="flex items-center gap-2">
                <div className="w-8 h-8 rounded-lg bg-blue-50 dark:bg-blue-900/20 flex items-center justify-center">
                  <svg className="w-4 h-4 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M12 22C6.477 22 2 17.523 2 12S6.477 2 12 2s10 4.477 10 10-4.477 10-10 10zm0-2a8 8 0 100-16 8 8 0 000 16zm0-5a3 3 0 110-6 3 3 0 010 6zm0-2a1 1 0 100-2 1 1 0 000 2z"/></svg>
                </div>
                <span className="text-sm font-semibold text-slate-600 dark:text-slate-400">风扇转速</span>
              </div>
              <span className={`text-xs font-bold ${isConnected ? 'text-blue-600 dark:text-blue-400' : 'text-slate-400 dark:text-slate-500'}`}>
                {isConnected ? `目标 ${fanData?.targetRpm || 0} · ${fanData?.setGear || '未知'}` : '离线'}
              </span>
            </div>
            <div className="z-10 flex items-baseline gap-1 mt-2">
              <span className={`text-3xl font-black tabular-nums tracking-tight ${isConnected ? 'text-blue-600 dark:text-blue-400' : 'text-slate-400 dark:text-slate-600'}`}>
                {isConnected ? (fanData?.currentRpm ?? 0) : '--'}
              </span>
              <span className="text-xs font-semibold text-slate-500">RPM</span>
            </div>
            <div className="absolute bottom-0 left-0 right-0 h-1 bg-slate-100 dark:bg-slate-800">
              <div 
                className={`h-full ${isConnected ? 'bg-blue-500' : 'bg-slate-300 dark:bg-slate-700'} transition-all duration-500`} 
                style={{ width: isConnected ? `${Math.min(100, ((fanData?.currentRpm || 0) / 4000) * 100)}%` : '0%' }}
              />
            </div>
          </div>

        </div>

        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-[#161922] shadow-sm flex flex-col z-10 transition-colors duration-300">
          
          {/* 内嵌菜单切换区 */}
          <div className="px-6 pt-5 pb-3 flex justify-between items-end border-b border-slate-100 dark:border-slate-800 shrink-0">
            <div className="flex gap-6">
              {[
                { id: 'curve', label: '风扇曲线', icon: <PresentationChartLineIcon className="w-4 h-4" /> },
                { id: 'rgb', label: 'RGB 灯效', icon: <SparklesIcon className="w-4 h-4" /> },
                { id: 'control', label: '控制面板', icon: <Cog6ToothIcon className="w-4 h-4" /> },
                { id: 'about', label: '关于软件', icon: <InformationCircleIcon className="w-4 h-4" /> },
              ].map(tab => {
                const isActive = activeTab === tab.id;
                return (
                  <button key={tab.id} onClick={() => setActiveTab(tab.id as typeof activeTab)} className={`relative pb-3 flex items-center gap-2 transition-colors duration-200 ${isActive ? 'text-blue-600 dark:text-blue-400' : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-300'}`}>
                    {tab.icon}
                    <span className={`text-sm font-bold tracking-wide`}>{tab.label}</span>
                    {isActive && <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-blue-600 dark:bg-blue-500 rounded-t-full"></div>}
                  </button>
                )
              })}
            </div>
            <div className="pb-3 cursor-pointer group" onClick={() => BrowserOpenURL('https://www.tianli0.top/')}>
              <span className="text-xs font-semibold text-slate-400 group-hover:text-blue-500 transition-colors uppercase tracking-wider">BS2PRO Controller · By Tianli</span>
            </div>
          </div>

          <div className="p-6">
            {activeTab === 'curve' && config && (
              <FanCurve
                config={config}
                onConfigChange={handleConfigChange}
                isConnected={isConnected}
                fanData={fanData}
                temperature={temperature}
              />
            )}
            
            {activeTab === 'rgb' && (
              <RGBControl
                isConnected={isConnected}
                savedConfig={config?.rgbConfig}
                onSetRGBMode={async (params) => {
                  try { return await apiService.setRGBMode(params); } 
                  catch { return false; }
                }}
              />
            )}
            
            {activeTab === 'control' && config && (
              <ControlPanel
                config={config}
                onConfigChange={handleConfigChange}
                isConnected={isConnected}
              />
            )}

            {activeTab === 'about' && <AboutPanel />}
          </div>

        </div>
      </div>
    </div>
  );
}