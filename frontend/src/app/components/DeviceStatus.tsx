'use client';

import React, { memo } from 'react';
import { 
  ExclamationTriangleIcon,
  CpuChipIcon,
  BoltIcon,
  ArrowPathIcon,
  ComputerDesktopIcon,
  WifiIcon,
  SignalIcon,
  CogIcon,
} from '@heroicons/react/24/outline';
import {
  CheckCircleIcon,
} from '@heroicons/react/24/solid';
import { types } from '../../../wailsjs/go/models';
import { apiService } from '../services/api';
import { ToggleSwitch, Card, Badge, Button } from './ui';
import clsx from 'clsx';

interface DeviceStatusProps {
  isConnected: boolean;
  fanData: types.FanData | null;
  temperature: types.TemperatureData | null;
  config: types.AppConfig;
  onConnect: () => void;
  onDisconnect: () => void;
  onConfigChange: (config: types.AppConfig) => void;
}

// 温度状态判断
const getTempStatus = (temp: number) => {
  if (temp > 85) return { color: 'text-red-500', bg: 'bg-red-500', label: '过热' };
  if (temp > 75) return { color: 'text-orange-500', bg: 'bg-orange-500', label: '偏高' };
  if (temp > 60) return { color: 'text-yellow-500', bg: 'bg-yellow-500', label: '正常' };
  return { color: 'text-green-500', bg: 'bg-green-500', label: '良好' };
};

// ============ 独立的实时数据组件 - 避免触发父组件重绘 ============

// CPU 温度显示组件
const CpuTempDisplay = memo(function CpuTempDisplay({ 
  temp 
}: { 
  temp: number | undefined 
}) {
  const status = getTempStatus(temp || 0);
  return (
    <div className="flex flex-col items-center h-full">
      <div className="flex-1 flex flex-col justify-center">
        <div className="flex items-baseline gap-0.5 justify-center">
          <span className={clsx('text-3xl font-bold tabular-nums', status.color)}>
            {temp ?? '--'}
          </span>
          <span className="text-sm text-gray-400">°C</span>
        </div>
        <div className="text-xs text-gray-500 dark:text-gray-400 mt-1 text-center h-4">
          {status.label}
        </div>
      </div>
      <div className="w-full h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden mt-2">
        <div 
          className={clsx('h-full rounded-full transition-all duration-500', status.bg)}
          style={{ width: `${Math.min(100, ((temp || 0) / 100) * 100)}%` }}
        />
      </div>
    </div>
  );
});

// GPU 温度显示组件
const GpuTempDisplay = memo(function GpuTempDisplay({ 
  temp 
}: { 
  temp: number | undefined 
}) {
  const status = getTempStatus(temp || 0);
  return (
    <div className="flex flex-col items-center h-full">
      <div className="flex-1 flex flex-col justify-center">
        <div className="flex items-baseline gap-0.5 justify-center">
          <span className={clsx('text-3xl font-bold tabular-nums', status.color)}>
            {temp ?? '--'}
          </span>
          <span className="text-sm text-gray-400">°C</span>
        </div>
        <div className="text-xs text-gray-500 dark:text-gray-400 mt-1 text-center h-4">
          {status.label}
        </div>
      </div>
      <div className="w-full h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden mt-2">
        <div 
          className={clsx('h-full rounded-full transition-all duration-500', status.bg)}
          style={{ width: `${Math.min(100, ((temp || 0) / 100) * 100)}%` }}
        />
      </div>
    </div>
  );
});

// 风扇转速显示组件
const FanRpmDisplay = memo(function FanRpmDisplay({ 
  currentRpm,
  targetRpm,
  setGear,
}: { 
  currentRpm: number | undefined;
  targetRpm: number | undefined;
  setGear: string | undefined;
}) {
  const rpmPercentage = Math.min(100, ((currentRpm || 0) / 4000) * 100);
  return (
    <div className="flex flex-col items-center h-full">
      <div className="flex-1 flex flex-col justify-center">
        <div className="flex items-baseline gap-0.5 justify-center">
          <span className="text-3xl font-bold tabular-nums text-blue-600 dark:text-blue-400">
            {currentRpm ?? '--'}
          </span>
          <span className="text-sm text-gray-400">RPM</span>
        </div>
        <div className="text-xs text-gray-500 dark:text-gray-400 mt-1 text-center h-4">
          目标 {targetRpm ?? '--'} · {setGear || '--'}
        </div>
      </div>
      <div className="w-full h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden mt-2">
        <div 
          className="h-full bg-blue-500 rounded-full transition-all duration-300"
          style={{ width: `${rpmPercentage}%` }}
        />
      </div>
    </div>
  );
});

// 最高温度显示组件
const MaxTempDisplay = memo(function MaxTempDisplay({ 
  temp 
}: { 
  temp: number | undefined 
}) {
  const status = getTempStatus(temp || 0);
  return (
    <span className={clsx('text-sm font-semibold tabular-nums', status.color)}>
      {temp ?? '--'}°C
    </span>
  );
});

// ============ 主组件 ============

export default function DeviceStatus({ 
  isConnected, 
  fanData, 
  temperature, 
  config,
  onConnect, 
  onDisconnect,
  onConfigChange
}: DeviceStatusProps) {
  
  const handleAutoControlChange = async (enabled: boolean) => {
    try {
      await apiService.setAutoControl(enabled);
      const newConfig = types.AppConfig.createFrom({ ...config, autoControl: enabled });
      onConfigChange(newConfig);
    } catch (error) {
      console.error('设置智能变频失败:', error);
    }
  };

  // 设备型号判断
  const deviceModel = fanData?.maxGear === '超频' ? 'BS2 PRO' : 'BS2';

  return (
    <div className="space-y-4">
      {/* 核心数据仪表盘 */}
      {isConnected ? (
        <div className="grid grid-cols-3 gap-4">
          {/* CPU 温度卡片 */}
          <Card className="p-5" hover>
            <div className="flex items-center gap-2 mb-4">
              <div className="p-2 rounded-lg bg-orange-50 dark:bg-orange-900/20">
                <CpuChipIcon className="w-5 h-5 text-orange-500" />
              </div>
              <span className="text-sm font-medium text-gray-600 dark:text-gray-400">CPU 温度</span>
            </div>
            
            <div className="h-20">
              <CpuTempDisplay temp={temperature?.cpuTemp} />
            </div>
          </Card>

          {/* GPU 温度卡片 */}
          <Card className="p-5" hover>
            <div className="flex items-center gap-2 mb-4">
              <div className="p-2 rounded-lg bg-purple-50 dark:bg-purple-900/20">
                <svg className="w-5 h-5 text-purple-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                  <rect x="4" y="4" width="16" height="16" rx="2" />
                  <rect x="8" y="8" width="8" height="8" rx="1" />
                  <path d="M4 9h1M4 15h1M19 9h1M19 15h1M9 4v1M15 4v1M9 19v1M15 19v1" strokeLinecap="round" />
                </svg>
              </div>
              <span className="text-sm font-medium text-gray-600 dark:text-gray-400">GPU 温度</span>
            </div>
            
            <div className="h-20">
              <GpuTempDisplay temp={temperature?.gpuTemp} />
            </div>
          </Card>

          {/* 风扇转速卡片 */}
          <Card className="p-5" hover>
            <div className="flex items-center gap-2 mb-4">
              <div className="p-2 rounded-lg bg-blue-50 dark:bg-blue-900/20">
                <SignalIcon className="w-5 h-5 text-blue-500" />
              </div>
              <span className="text-sm font-medium text-gray-600 dark:text-gray-400">风扇转速</span>
            </div>
            
            <div className="h-20">
              <FanRpmDisplay 
                currentRpm={fanData?.currentRpm}
                targetRpm={fanData?.targetRpm}
                setGear={fanData?.setGear}
              />
            </div>
          </Card>
        </div>
      ) : (
        /* 未连接提示 */
        <Card className="p-8">
          <div className="text-center">
            <div className="w-16 h-16 mx-auto mb-4 rounded-xl bg-gray-100 dark:bg-gray-700 flex items-center justify-center">
              <WifiIcon className="w-8 h-8 text-gray-400" />
            </div>
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-2">
              设备未连接
            </h3>
            <p className="text-sm text-gray-500 dark:text-gray-400 mb-6 max-w-xs mx-auto">
              请将 BS2/BS2PRO 散热器通过 蓝牙 连接到电脑
            </p>
            <Button onClick={onConnect} icon={<ArrowPathIcon className="w-4 h-4" />}>
              连接设备
            </Button>
          </div>
        </Card>
      )}

    </div>
  );
}