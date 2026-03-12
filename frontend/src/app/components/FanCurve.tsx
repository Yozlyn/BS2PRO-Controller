'use client';

import React, { useState, useEffect, useCallback, memo, useMemo, useRef } from 'react';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import {
  ArrowPathIcon,
  CheckIcon,
  InformationCircleIcon,
  ArrowDownTrayIcon,
  ArrowUpTrayIcon,
  AdjustmentsHorizontalIcon,
} from '@heroicons/react/24/outline';
import { apiService } from '../services/api';
import { logger } from '../services/logger';
import { types } from '../../../wailsjs/go/models';
import { ToggleSwitch, Select, Button, Badge, Card } from './ui';
import clsx from 'clsx';

interface FanCurveProps {
  config: types.AppConfig;
  onConfigChange: (config: types.AppConfig) => void;
  isConnected: boolean;
  fanData: types.FanData | null;
  temperature: types.TemperatureData | null;
}

// 独立的温度指示线组件 - 不会触发图表重绘
const TemperatureIndicator = memo(function TemperatureIndicator({
  temperature,
  chartRef,
  temperatureRange,
}: {
  temperature: number | null;
  chartRef: React.RefObject<HTMLDivElement | null>;
  temperatureRange: { min: number; max: number };
}) {
  const [position, setPosition] = useState<{ x: number; top: number; height: number } | null>(null);

  useEffect(() => {
    if (temperature === null || !chartRef.current) {
      setPosition(null);
      return;
    }

    const updatePosition = () => {
      const chartArea = chartRef.current?.querySelector('.recharts-cartesian-grid');
      if (!chartArea) return;

      const rect = chartArea.getBoundingClientRect();
      const containerRect = chartRef.current!.querySelector('.recharts-responsive-container')?.getBoundingClientRect();
      if (!containerRect) return;

      const chartWidth = rect.width;
      const chartLeft = rect.left - containerRect.left;
      
      // 计算温度对应的 X 位置
      const tempPercent = (temperature - temperatureRange.min) / (temperatureRange.max - temperatureRange.min);
      const x = chartLeft + tempPercent * chartWidth;
      
      setPosition({
        x,
        top: rect.top - containerRect.top,
        height: rect.height
      });
    };

    updatePosition();
    
    // 监听窗口大小变化
    window.addEventListener('resize', updatePosition);
    return () => window.removeEventListener('resize', updatePosition);
  }, [temperature, chartRef, temperatureRange]);

  if (!position || temperature === null) return null;

  return (
    <svg 
      className="absolute inset-0 pointer-events-none overflow-visible"
      style={{ width: '100%', height: '100%' }}
    >
      {/* 虚线 */}
      <line
        x1={position.x}
        y1={position.top}
        x2={position.x}
        y2={position.top + position.height}
        stroke="#ef4444"
        strokeWidth={2}
        strokeDasharray="5 5"
      />
      {/* 标签背景 */}
      <rect
        x={position.x - 45}
        y={position.top - 22}
        width={90}
        height={20}
        rx={4}
        fill="#ef4444"
      />
      {/* 标签文字 */}
      <text
        x={position.x}
        y={position.top - 8}
        textAnchor="middle"
        fill="white"
        fontSize={11}
        fontWeight={500}
      >
        当前 {temperature}°C
      </text>
    </svg>
  );
});


// 自定义可拖拽的点组件
const DraggablePoint = memo(function DraggablePoint({
  cx,
  cy,
  index,
  rpm,
  onDragStart,
  isActive,
}: {
  cx: number;
  cy: number;
  index: number;
  temperature: number;
  rpm: number;
  onDragStart: (index: number) => void;
  isActive: boolean;
}) {
  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    onDragStart(index);
  }, [index, onDragStart]);

  const handleTouchStart = useCallback((e: React.TouchEvent) => {
    e.preventDefault();
    e.stopPropagation();
    onDragStart(index);
  }, [index, onDragStart]);

  return (
    <g>
      {/* 外圈 - 交互区域 */}
      <circle
        cx={cx}
        cy={cy}
        r={isActive ? 14 : 10}
        fill="transparent"
        stroke="transparent"
        style={{ cursor: 'ns-resize' }}
        onMouseDown={handleMouseDown}
        onTouchStart={handleTouchStart}
      />
      {/* 主点 */}
      <circle
        cx={cx}
        cy={cy}
        r={isActive ? 8 : 6}
        fill={isActive ? '#1d4ed8' : '#3b82f6'}
        stroke="white"
        strokeWidth={2}
        style={{ 
          cursor: 'ns-resize',
          transition: isActive ? 'none' : 'all 0.2s ease',
          filter: isActive ? 'drop-shadow(0 4px 8px rgba(59, 130, 246, 0.5))' : 'drop-shadow(0 2px 4px rgba(0, 0, 0, 0.1))'
        }}
        onMouseDown={handleMouseDown}
        onTouchStart={handleTouchStart}
      />
      {/* 活动状态时显示数值 */}
      {isActive && (
        <g>
          <rect
            x={cx - 35}
            y={cy - 35}
            width={70}
            height={24}
            rx={4}
            fill="#1e40af"
            opacity={0.95}
          />
          <text
            x={cx}
            y={cy - 19}
            textAnchor="middle"
            fill="white"
            fontSize={12}
            fontWeight={600}
          >
            {rpm} RPM
          </text>
        </g>
      )}
    </g>
  );
});

const FanCurve = memo(function FanCurve({ config, onConfigChange, isConnected, fanData, temperature }: FanCurveProps) {
  // 本地编辑状态 - 完全独立于外部配置
  const [localCurve, setLocalCurve] = useState<types.FanCurvePoint[]>([]);
  const [hasUnsavedChanges, setHasUnsavedChanges] = useState(false);
  const [isInitialized, setIsInitialized] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [isDarkMode, setIsDarkMode] = useState(false);
  
  // 拖拽状态
  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [isInteracting, setIsInteracting] = useState(false);
  
  // Chart ref for coordinate calculations
  const chartRef = useRef<HTMLDivElement>(null);
  const chartBoundsRef = useRef<{ top: number; bottom: number; left: number; right: number; yMin: number; yMax: number } | null>(null);

  // 静态 RPM 范围 - 仅在首次初始化时计算
  const [rpmRange, setRpmRange] = useState({ min: 500, max: 4000, ticks: [500, 1000, 1500, 2000, 2500, 3000, 3500, 4000] });
  
  // 温度范围
  const temperatureRange = useMemo(() => ({
    min: 30,
    max: 100,
    ticks: Array.from({ length: 15 }, (_, i) => 30 + i * 5)
  }), []);

  useEffect(() => {
    const root = document.documentElement;

    const updateTheme = () => {
      setIsDarkMode(root.classList.contains('dark'));
    };

    updateTheme();

    const observer = new MutationObserver(updateTheme);
    observer.observe(root, { attributes: true, attributeFilter: ['class'] });

    return () => observer.disconnect();
  }, []);

  // 仅在组件首次加载时初始化
  useEffect(() => {
    if (!isInitialized && config.fanCurve && config.fanCurve.length > 0) {
      // 迁移兼容：若曲线最高温度点 < 100°C，自动追加 100°C 点
      const curve = [...config.fanCurve];
      const lastPoint = curve[curve.length - 1];
      if (lastPoint.temperature < 100) {
        curve.push({ temperature: 100, rpm: lastPoint.rpm, offset: 0 });
      }
      setLocalCurve(curve);
      setIsInitialized(true);
      
      // 初始化 RPM 范围
      if (fanData?.maxGear) {
        let maxRpm = 4000;
        switch (fanData.maxGear) {
          case '标准': maxRpm = 2760; break;
          case '强劲': maxRpm = 3300; break;
          case '超频': maxRpm = 4000; break;
        }
        const step = 500;
        const ticks = Array.from({ length: Math.floor((maxRpm - 500) / step) + 1 }, (_, i) => 500 + i * step);
        setRpmRange({ min: 500, max: maxRpm, ticks });
      }
    }
  }, [config.fanCurve, fanData?.maxGear, isInitialized]);

  // 同步后端自动偏移变更到本地曲线（不影响 RPM 编辑状态）
  useEffect(() => {
    if (!isInitialized || !config.fanCurveOffsetEnabled) return;
    setLocalCurve(prev => {
      let changed = false;
      const next = prev.map((p, i) => {
        const cfgOffset = config.fanCurve[i]?.offset ?? 0;
        if (p.offset !== cfgOffset) {
          changed = true;
          return { ...p, offset: cfgOffset };
        }
        return p;
      });
      return changed ? next : prev;
    });
  }, [config.fanCurve, config.fanCurveOffsetEnabled, isInitialized]);

  // 图表数据 - 使用本地状态
  const chartData = useMemo(() => {
    return localCurve.map((point, index) => {
      // 当自动偏移开启时，使用每个控制点自身的偏移值
      const offset = config.fanCurveOffsetEnabled ? (point.offset || 0) : 0;
      const effectiveRpm = Math.min(rpmRange.max, Math.max(rpmRange.min, Math.round((point.rpm + offset) / 100) * 100));
      return {
        temperature: point.temperature,
        rpm: point.rpm,
        effectiveRpm: config.fanCurveOffsetEnabled ? effectiveRpm : undefined,
        index
      };
    });
  }, [localCurve, config.fanCurveOffsetEnabled, rpmRange]);

  // 更新单个点
  const updatePoint = useCallback((index: number, newRpm: number) => {
    const clampedRpm = Math.max(rpmRange.min, Math.min(rpmRange.max, Math.round(newRpm / 100) * 100));
    
    setLocalCurve(prev => {
      if (prev[index]?.rpm === clampedRpm) return prev;
      const newCurve = [...prev];
      newCurve[index] = { ...newCurve[index], rpm: clampedRpm };
      return newCurve;
    });
    setHasUnsavedChanges(true);
  }, [rpmRange]);

  // 更新单个点的偏移量
  const updateOffset = useCallback((index: number, newOffset: number) => {
    const clampedOffset = Math.round(newOffset / 100) * 100;
    
    setLocalCurve(prev => {
      if (prev[index]?.offset === clampedOffset) return prev;
      const newCurve = [...prev];
      newCurve[index] = { ...newCurve[index], offset: clampedOffset };
      return newCurve;
    });
    setHasUnsavedChanges(true);
  }, []);

  // 拖拽处理
  const handleDragStart = useCallback((index: number) => {
    setDragIndex(index);
    setIsInteracting(true);
    
    // 计算图表边界
    if (chartRef.current) {
      const chartArea = chartRef.current.querySelector('.recharts-cartesian-grid');
      if (chartArea) {
        const rect = chartArea.getBoundingClientRect();
        chartBoundsRef.current = {
          top: rect.top,
          bottom: rect.bottom,
          left: rect.left,
          right: rect.right,
          yMin: rpmRange.min,
          yMax: rpmRange.max
        };
      }
    }
  }, [rpmRange]);

  const handleDrag = useCallback((clientY: number) => {
    if (dragIndex === null || !chartBoundsRef.current) return;
    
    const bounds = chartBoundsRef.current;
    const chartHeight = bounds.bottom - bounds.top;
    const relativeY = Math.max(0, Math.min(1, (bounds.bottom - clientY) / chartHeight));
    const newRpm = bounds.yMin + relativeY * (bounds.yMax - bounds.yMin);
    
    updatePoint(dragIndex, newRpm);
  }, [dragIndex, updatePoint]);

  const handleDragEnd = useCallback(() => {
    setDragIndex(null);
    setTimeout(() => setIsInteracting(false), 100);
  }, []);

  // 全局拖拽事件监听
  useEffect(() => {
    if (dragIndex === null) return;

    const handleMouseMove = (e: MouseEvent) => {
      e.preventDefault();
      handleDrag(e.clientY);
    };

    const handleTouchMove = (e: TouchEvent) => {
      if (e.touches.length > 0) {
        handleDrag(e.touches[0].clientY);
      }
    };

    const handleEnd = () => handleDragEnd();

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleEnd);
    document.addEventListener('touchmove', handleTouchMove, { passive: false });
    document.addEventListener('touchend', handleEnd);

    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleEnd);
      document.removeEventListener('touchmove', handleTouchMove);
      document.removeEventListener('touchend', handleEnd);
    };
  }, [dragIndex, handleDrag, handleDragEnd]);

  // 保存曲线
  const saveCurve = useCallback(async () => {
    if (isSaving) return;
    
    try {
      setIsSaving(true);
      await apiService.setFanCurve(localCurve);
      const newConfig = types.AppConfig.createFrom({ ...config, fanCurve: localCurve });
      onConfigChange(newConfig);
      setHasUnsavedChanges(false);
    } catch (error) {
      logger.error('保存风扇曲线失败', 'FanCurve', error);
    } finally {
      setIsSaving(false);
    }
  }, [localCurve, config, onConfigChange, isSaving]);

  // 重置曲线
  const resetCurve = useCallback(() => {
    const defaultCurve: types.FanCurvePoint[] = [
      { temperature: 30, rpm: 500, offset: 0 },
      { temperature: 35, rpm: 1200, offset: 0 },
      { temperature: 40, rpm: 1400, offset: 0 },
      { temperature: 45, rpm: 1600, offset: 0 },
      { temperature: 50, rpm: 1800, offset: 100 },
      { temperature: 55, rpm: 2000, offset: 100 },
      { temperature: 60, rpm: Math.min(2300, rpmRange.max), offset: 100 },
      { temperature: 65, rpm: Math.min(2600, rpmRange.max), offset: 200 },
      { temperature: 70, rpm: Math.min(2900, rpmRange.max), offset: 200 },
      { temperature: 75, rpm: Math.min(3200, rpmRange.max), offset: 200 },
      { temperature: 80, rpm: Math.min(3500, rpmRange.max), offset: 300 },
      { temperature: 85, rpm: Math.min(3800, rpmRange.max), offset: 200 },
      { temperature: 90, rpm: rpmRange.max, offset: 0 },
      { temperature: 95, rpm: rpmRange.max, offset: 0 },
      { temperature: 100, rpm: rpmRange.max, offset: 0 },
    ];
    
    setLocalCurve(defaultCurve);
    setHasUnsavedChanges(true);
  }, [rpmRange.max]);

  // 导出风扇配置
  const exportFanConfig = useCallback(() => {
    const fanConfig = {
      fanCurve: localCurve,
      autoControl: config.autoControl,
      manualGear: config.manualGear,
      manualLevel: config.manualLevel,
      exportDate: new Date().toISOString(),
      version: '1.0'
    };
    
    const jsonStr = JSON.stringify(fanConfig, null, 2);
    const blob = new Blob([jsonStr], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `bs2pro-fan-config-${new Date().toISOString().split('T')[0]}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }, [localCurve, config]);

  // 导入风扇配置
  const importFanConfig = useCallback(() => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    
    input.onchange = async (e) => {
      const file = (e.target as HTMLInputElement).files?.[0];
      if (!file) return;
      
      const reader = new FileReader();
      reader.onload = async (event) => {
        try {
          const content = event.target?.result as string;
          const fanConfig = JSON.parse(content);
          
          // 验证导入的数据结构
          if (!fanConfig.fanCurve || !Array.isArray(fanConfig.fanCurve)) {
            throw new Error('无效的风扇配置文件');
          }
          
          // 更新本地曲线
          setLocalCurve(fanConfig.fanCurve);
          setHasUnsavedChanges(true);
          
          // 可选：更新其他配置
          if (fanConfig.autoControl !== undefined) {
            try {
              await apiService.setAutoControl(fanConfig.autoControl);
              const newConfig = types.AppConfig.createFrom({ ...config, autoControl: fanConfig.autoControl });
              onConfigChange(newConfig);
            } catch (error) {
              logger.error('设置智能变频失败', 'FanCurve', error);
            }
          }
          
          alert('风扇配置导入成功！');
        } catch (error) {
          logger.error('导入风扇配置失败', 'FanCurve', error);
          alert('导入失败：文件格式无效');
        }
      };
      reader.readAsText(file);
    };
    
    input.click();
  }, [config, onConfigChange]);

  // 智能变频切换
  const handleAutoControlChange = useCallback(async (enabled: boolean) => {
    try {
      await apiService.setAutoControl(enabled);
      const newConfig = types.AppConfig.createFrom({ ...config, autoControl: enabled });
      onConfigChange(newConfig);
    } catch (error) {
      logger.error('设置智能变频失败', 'FanCurve', error);
    }
  }, [config, onConfigChange]);

  // 风扇曲线偏移开关切换
  const handleOffsetToggle = useCallback(async (enabled: boolean) => {
    try {
      const newConfig = types.AppConfig.createFrom({ ...config, fanCurveOffsetEnabled: enabled });
      await apiService.updateConfig(newConfig);
      onConfigChange(newConfig);
    } catch (error) {
      logger.error('设置风扇偏移失败', 'FanCurve', error);
    }
  }, [config, onConfigChange]);

  // 将当前偏移量烘焙进基础转速
  const handleApplyOffsetToCurve = useCallback(async () => {
    try {
      await apiService.applyOffsetToCurve();
    } catch (error) {
      logger.error('应用偏移到曲线失败', 'FanCurve', error);
    }
  }, []);

  // 手动挡位选项
  const gearOptions = [
    { value: '静音', label: '静音', description: '低噪音模式' },
    { value: '标准', label: '标准', description: '平衡模式' },
    { value: '强劲', label: '强劲', description: '高性能模式' },
    { value: '超频', label: '超频', description: '极限模式' },
  ];

  const levelOptions = [
    { value: '低', label: '低' },
    { value: '中', label: '中' },
    { value: '高', label: '高' },
  ];

  // 手动挡位切换
  const handleGearChange = useCallback(async (gear: string) => {
    try {
      await apiService.setManualGear(gear, config.manualLevel || '中');
      const newConfig = types.AppConfig.createFrom({ ...config, manualGear: gear });
      onConfigChange(newConfig);
    } catch (error) {
      logger.error('设置手动挡位失败', 'FanCurve', error);
    }
  }, [config, onConfigChange]);

  const handleLevelChange = useCallback(async (level: string) => {
    try {
      await apiService.setManualGear(config.manualGear || '标准', level);
      const newConfig = types.AppConfig.createFrom({ ...config, manualLevel: level });
      onConfigChange(newConfig);
    } catch (error) {
      logger.error('设置挡位级别失败', 'FanCurve', error);
    }
  }, [config, onConfigChange]);

  // 自定义点渲染
  const CustomDot = useCallback((props: any): React.ReactElement<SVGElement> => {
    const { cx, cy, index, payload } = props;
    // 如果坐标无效，返回一个空的 g 元素而不是 null
    if (cx === undefined || cy === undefined) {
      return <g />;
    }
    
    return (
      <DraggablePoint
        key={`dot-${index}`}
        cx={cx}
        cy={cy}
        index={index}
        temperature={payload.temperature}
        rpm={payload.rpm}
        onDragStart={handleDragStart}
        isActive={dragIndex === index}
      />
    );
  }, [dragIndex, handleDragStart]);

  return (
    <Card className="p-3">
      {/* 头部 - 状态徽章 */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-1.5">
          {hasUnsavedChanges && (
            <Badge variant="warning" size="sm">未保存</Badge>
          )}
          {isInteracting && (
            <Badge variant="info" size="sm">编辑中</Badge>
          )}
        </div>
        
        {/* 手动挡位控制（仅在关闭智能变频时显示） */}
        {!config.autoControl && isConnected && (
          <div className="flex items-center gap-1.5">
            <span className="text-xs text-gray-500 dark:text-gray-400">手动挡位</span>
            <Select
              value={config.manualGear || '标准'}
              onChange={handleGearChange}
              options={gearOptions}
              size="sm"
            />
            <Select
              value={config.manualLevel || '中'}
              onChange={handleLevelChange}
              options={levelOptions}
              size="sm"
            />
          </div>
        )}
      </div>

      {/* 图表区域 */}
      <div
        ref={chartRef}
        className={clsx(
          'relative rounded-xl border bg-white dark:bg-gray-800 p-2 mb-3',
          'border-gray-200 dark:border-gray-700',
          dragIndex !== null && 'ring-2 ring-blue-500 ring-opacity-50'
        )}
      >
        <div className="h-68 md:h-76 relative">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart
              data={chartData}
              margin={{ top: 10, right: 20, left: 10, bottom: 15 }}
            >
              <CartesianGrid 
                strokeDasharray="3 3" 
                stroke="#e5e7eb"
                className="dark:stroke-gray-600"
              />
              
              <XAxis 
                dataKey="temperature" 
                type="number"
                domain={[temperatureRange.min, temperatureRange.max]}
                ticks={temperatureRange.ticks}
                tickLine={false}
                axisLine={{ stroke: '#d1d5db' }}
                tick={{ fill: '#6b7280', fontSize: 11 }}
                allowDataOverflow={true}
                label={{ 
                  value: '温度 (°C)', 
                  position: 'insideBottom', 
                  offset: -10,
                  fill: '#6b7280',
                  fontSize: 12
                }}
              />
              <YAxis 
                type="number"
                domain={[rpmRange.min, rpmRange.max]}
                ticks={rpmRange.ticks}
                tickLine={false}
                axisLine={{ stroke: '#d1d5db' }}
                tick={{ fill: '#6b7280', fontSize: 11 }}
                allowDataOverflow={true}
                label={{ 
                  value: '转速 (RPM)', 
                  angle: -90, 
                  position: 'insideLeft',
                  fill: '#6b7280',
                  fontSize: 12
                }}
              />
              <Tooltip 
                formatter={(value: number, name: string) => {
                  if (name === 'effectiveRpm') return [`${value} RPM`, '实际转速(含偏移)'];
                  return [`${value} RPM`, '基础转速'];
                }}
                labelFormatter={(value) => `温度: ${value}°C`}
                contentStyle={{
                  backgroundColor: isDarkMode ? 'rgba(17, 24, 39, 0.95)' : 'rgba(255, 255, 255, 0.95)',
                  border: '1px solid',
                  borderColor: isDarkMode ? '#374151' : 'transparent',
                  borderRadius: '8px',
                  boxShadow: isDarkMode
                    ? '0 10px 25px -5px rgba(0, 0, 0, 0.6)'
                    : '0 10px 25px -5px rgba(0, 0, 0, 0.1)',
                  padding: '8px 12px',
                  color: isDarkMode ? '#e5e7eb' : '#111827'
                }}
                labelStyle={{
                  color: isDarkMode ? '#e5e7eb' : '#111827',
                  fontWeight: 600
                }}
                itemStyle={{
                  color: isDarkMode ? '#e5e7eb' : '#111827'
                }}
              />
              <Line 
                type="monotone" 
                dataKey="rpm" 
                stroke="#3b82f6" 
                strokeWidth={3}
                dot={CustomDot}
                activeDot={false}
                isAnimationActive={false}
                name="rpm"
              />
              {config.fanCurveOffsetEnabled && (
                <Line 
                  type="monotone" 
                  dataKey="effectiveRpm" 
                  stroke="#f59e0b" 
                  strokeWidth={2}
                  strokeDasharray="6 3"
                  dot={false}
                  activeDot={false}
                  isAnimationActive={false}
                  name="effectiveRpm"
                />
              )}
            </LineChart>
          </ResponsiveContainer>
          
          {/* 独立的温度指示线覆盖层 - 不触发图表重绘 */}
          <TemperatureIndicator 
            temperature={temperature?.maxTemp ?? null}
            chartRef={chartRef}
            temperatureRange={temperatureRange}
          />
        </div>
      </div>

      {/* 按钮组 */}
      <div className="flex flex-wrap items-center justify-center gap-1.5 mb-2">
        <Button
          variant="secondary"
          size="sm"
          onClick={resetCurve}
          icon={<ArrowPathIcon className="w-3 h-3" />}
        >
          重置
        </Button>
        <Button
          variant="primary"
          size="sm"
          onClick={saveCurve}
          disabled={!hasUnsavedChanges}
          loading={isSaving}
          icon={<CheckIcon className="w-3 h-3" />}
        >
          保存
        </Button>
        <Button
          variant="secondary"
          size="sm"
          onClick={exportFanConfig}
          icon={<ArrowDownTrayIcon className="w-3 h-3" />}
        >
          导出
        </Button>
        <Button
          variant="secondary"
          size="sm"
          onClick={importFanConfig}
          icon={<ArrowUpTrayIcon className="w-3 h-3" />}
        >
          导入
        </Button>
      </div>

      {/* 偏移开关 */}
      <div className="flex items-center justify-between mb-2 px-1">
        <div className="flex items-center gap-1.5">
          <AdjustmentsHorizontalIcon className="w-3.5 h-3.5 text-amber-500" />
          <span className="text-xs font-medium text-gray-700 dark:text-gray-300">自动曲线偏移</span>
          {config.fanCurveOffsetEnabled && (
            <Badge variant="warning" size="sm">
              {(temperature?.autoOffset ?? 0) >= 0 ? '+' : ''}{temperature?.autoOffset ?? 0} RPM
            </Badge>
          )}
        </div>
        <ToggleSwitch
          enabled={config.fanCurveOffsetEnabled ?? false}
          onChange={(val) => { handleOffsetToggle(val); }}
        />
      </div>
      {config.fanCurveOffsetEnabled && (
        <div className="mb-2 px-1">
          <div className="flex items-center gap-1.5 text-xs text-amber-600 dark:text-amber-400 mb-1">
            <span className="inline-block w-4 border-t-2 border-dashed border-amber-500" />
            <span>橙色虚线 = 基础转速 + 各点偏移量（实际生效转速）</span>
          </div>
          <div className="text-xs text-gray-500 dark:text-gray-400 mb-2">
            系统按温度区间独立调节偏移量，收敛后自动锁定；温度突变时重新调整
          </div>
          <button
            onClick={handleApplyOffsetToCurve}
            className="w-full text-xs py-1 px-2 rounded border border-amber-400 text-amber-600 dark:text-amber-400 hover:bg-amber-50 dark:hover:bg-amber-900/20 transition-colors"
          >
            应用偏移到曲线（将偏移量烘焙入基础 RPM）
          </button>
        </div>
      )}

      {/* 拖拽提示 */}
      <div className="text-center mb-2">
        <span className="text-xs text-gray-400 dark:text-gray-500 px-1.5 py-0.5 rounded-full bg-gray-100 dark:bg-gray-700/50">
          💡 拖拽图表上的蓝色圆点可直接调整转速
        </span>
      </div>

      {/* 控制点网格 */}
      <div className="mb-3">
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-xs font-semibold text-gray-900 dark:text-gray-300">控制点调节</h3>
          <span className="text-xs text-gray-500 dark:text-gray-400">
            转速范围: {rpmRange.min} - {rpmRange.max} RPM
          </span>
        </div>
        
        <div className="grid grid-cols-3 sm:grid-cols-5 md:grid-cols-5 lg:grid-cols-5 gap-1.5">
          {localCurve.map((point, index) => {
            const pointOffset = config.fanCurveOffsetEnabled ? (point.offset || 0) : 0;
            const effectiveRpm = config.fanCurveOffsetEnabled
              ? Math.min(rpmRange.max, Math.max(rpmRange.min, Math.round((point.rpm + pointOffset) / 100) * 100))
              : point.rpm;
            return (
            <div
              key={`control-${point.temperature}`}
              className={clsx(
                'p-1.5 rounded-md border transition-all duration-200',
                'bg-gray-50 dark:bg-gray-700/50',
                dragIndex === index
                  ? 'border-blue-500 ring-1 ring-blue-500/20'
                  : 'border-gray-200 dark:border-gray-600 hover:border-blue-300 dark:hover:border-blue-500'
              )}
            >
              <div className="text-center mb-0.5">
                <span className="text-xs font-medium text-gray-500 dark:text-gray-400">
                  {point.temperature}°C
                </span>
              </div>
              
              <input
                type="number"
                value={point.rpm}
                onChange={(e) => updatePoint(index, Number(e.target.value))}
                onFocus={() => setIsInteracting(true)}
                onBlur={() => setTimeout(() => setIsInteracting(false), 100)}
                min={rpmRange.min}
                max={rpmRange.max}
                step={100}
                className={clsx(
                  'w-full px-1 py-0.5 text-center text-xs font-medium rounded',
                  'bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-600',
                  'focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-transparent',
                  'transition-all duration-200'
                )}
              />
              
              <div className="mt-0.5">
                <input
                  type="range"
                  value={point.rpm}
                  onChange={(e) => updatePoint(index, Number(e.target.value))}
                  onMouseDown={() => setIsInteracting(true)}
                  onMouseUp={() => setTimeout(() => setIsInteracting(false), 100)}
                  onTouchStart={() => setIsInteracting(true)}
                  onTouchEnd={() => setTimeout(() => setIsInteracting(false), 100)}
                  min={rpmRange.min}
                  max={rpmRange.max}
                  step={100}
                  className="w-full h-1 rounded-full appearance-none cursor-pointer slider-thumb"
                  style={{
                    background: `linear-gradient(to right, #3b82f6 0%, #3b82f6 ${((point.rpm - rpmRange.min) / (rpmRange.max - rpmRange.min)) * 100}%, #e5e7eb ${((point.rpm - rpmRange.min) / (rpmRange.max - rpmRange.min)) * 100}%, #e5e7eb 100%)`
                  }}
                />
              </div>

              {/* 自动偏移信息展示 */}
              {config.fanCurveOffsetEnabled && (
                <div className="mt-1 pt-1 border-t border-gray-200 dark:border-gray-600">
                  <div className="flex items-center justify-between">
                    <span className="text-[10px] text-amber-600 dark:text-amber-400">偏移</span>
                    <span className="text-[10px] text-gray-400">
                      ={effectiveRpm}
                    </span>
                  </div>
                </div>
              )}
            </div>
            );
          })}
        </div>
      </div>

      {/* 说明卡片 */}
      <div className="p-2 rounded-md bg-gradient-to-r from-blue-50 to-indigo-50 dark:from-blue-900/20 dark:to-indigo-900/20 border border-blue-200 dark:border-blue-800">
        <div className="flex gap-1.5">
          <InformationCircleIcon className="w-3.5 h-3.5 text-blue-600 dark:text-blue-400 flex-shrink-0 mt-0.5" />
          <div className="text-xs text-blue-800 dark:text-blue-200 space-y-0.5">
            <p className="font-medium">使用说明</p>
            <ul className="space-y-0.5 text-blue-700 dark:text-blue-300">
              <li className="text-xs">• <strong>拖拽图表点：</strong>直接在图表上拖拽蓝色圆点调整转速</li>
              <li className="text-xs">• <strong>数值输入：</strong>在下方控制点卡片中直接输入精确值</li>
              <li className="text-xs">• <strong>滑块调节：</strong>使用滑块快速微调</li>
              <li className="text-xs">• <strong>自动偏移：</strong>开启后系统根据温度趋势自动调整偏移量，无需手动设置</li>
              <li className="text-xs">• <strong>保存设置：</strong>修改后点击保存按钮应用更改</li>
            </ul>
            <p className="text-xs text-blue-600 dark:text-blue-400 pt-0.5 border-t border-blue-200 dark:border-blue-700">
              挡位限制：静音 ≤2000 | 标准 ≤2760 | 强劲 ≤3300 | 超频 ≤4000 RPM
            </p>
          </div>
        </div>
      </div>
    </Card>
  );
});

export default FanCurve;
