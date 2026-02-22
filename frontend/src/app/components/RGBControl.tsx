'use client';

import React, { useState, useRef, useEffect, useCallback } from 'react';
import clsx from 'clsx';
import { RGBConfig } from '../types';
import {
  CpuChipIcon,
  ArrowPathIcon,
  HeartIcon,
  LightBulbIcon,
  SwatchIcon,
  SparklesIcon,
  XMarkIcon,
} from '@heroicons/react/24/outline';

// ============ Types ============
export interface RGBColor {
  r: number;
  g: number;
  b: number;
}

export interface RGBModeParams {
  mode: string;
  colors: RGBColor[];
  speed: string;
  brightness: number;
}

type LightMode =
  | 'smart'
  | 'rotation'
  | 'breathing'
  | 'static_single'
  | 'static_multi'
  | 'flowing'
  | 'off';

const MODES: { id: LightMode; label: string; icon: React.ReactNode; maxColors?: number; minColors?: number; hasSpeed?: boolean; hasColors?: boolean }[] = [
  { id: 'smart',         label: '智能温控', icon: <CpuChipIcon className="w-5 h-5" /> },
  { id: 'rotation',      label: '旋转',     icon: <ArrowPathIcon className="w-5 h-5" />, hasSpeed: true, hasColors: true, minColors: 1, maxColors: 6 },
  { id: 'breathing',     label: '呼吸',     icon: <HeartIcon className="w-5 h-5" />, hasSpeed: true, hasColors: true, minColors: 1, maxColors: 5 },
  { id: 'static_single', label: '单色常亮', icon: <LightBulbIcon className="w-5 h-5" />, hasColors: true, minColors: 1, maxColors: 1 },
  { id: 'static_multi',  label: '多色常亮', icon: <SwatchIcon className="w-5 h-5" />, hasColors: true, minColors: 3, maxColors: 3 },
  { id: 'flowing',       label: '流光',     icon: <SparklesIcon className="w-5 h-5" />, hasSpeed: true },
  { id: 'off',           label: '关闭',     icon: <XMarkIcon className="w-5 h-5" /> },
];

const SPEED_OPTIONS = [
  { id: 'fast',   label: '快' },
  { id: 'medium', label: '中' },
  { id: 'slow',   label: '慢' },
];

const PRESETS: { label: string; color: RGBColor }[] = [
  { label: '最蓝', color: { r: 0,   g: 0,   b: 255 } },
  { label: '最红', color: { r: 255, g: 0,   b: 0   } },
  { label: '最绿', color: { r: 0,   g: 255, b: 0   } },
];

// ============ Color Strip Picker ============
function ColorStrip({
  value,
  onChange,
}: {
  value: RGBColor;
  onChange: (c: RGBColor) => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const isDragging = useRef(false);

  const drawStrip = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const w = canvas.width;
    const h = canvas.height;

    // Draw rainbow hue strip
    const gradient = ctx.createLinearGradient(0, 0, w, 0);
    for (let i = 0; i <= 360; i += 30) {
      gradient.addColorStop(i / 360, `hsl(${i}, 100%, 50%)`);
    }
    ctx.fillStyle = gradient;
    ctx.fillRect(0, 0, w, h);

    // White overlay on left
    const whiteGrad = ctx.createLinearGradient(0, 0, 0, h);
    whiteGrad.addColorStop(0, 'rgba(255,255,255,0.8)');
    whiteGrad.addColorStop(0.5, 'rgba(255,255,255,0)');
    ctx.fillStyle = whiteGrad;
    ctx.fillRect(0, 0, w, h);

    // Black overlay on right
    const blackGrad = ctx.createLinearGradient(w * 0.6, 0, w, 0);
    blackGrad.addColorStop(0, 'rgba(0,0,0,0)');
    blackGrad.addColorStop(1, 'rgba(0,0,0,0.8)');
    ctx.fillStyle = blackGrad;
    ctx.fillRect(0, 0, w, h);
  }, []);

  useEffect(() => {
    drawStrip();
  }, [drawStrip]);

  const pickColor = useCallback((clientX: number) => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    const x = Math.max(0, Math.min(clientX - rect.left, rect.width - 1));
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const scaleX = canvas.width / rect.width;
    const pixel = ctx.getImageData(Math.floor(x * scaleX), Math.floor(canvas.height / 2), 1, 1).data;
    onChange({ r: pixel[0], g: pixel[1], b: pixel[2] });
  }, [onChange]);

  return (
    <div className="relative">
      <canvas
        ref={canvasRef}
        width={400}
        height={24}
        className="w-full h-6 rounded-lg cursor-crosshair border border-gray-200 dark:border-gray-700"
        onMouseDown={(e) => { isDragging.current = true; pickColor(e.clientX); }}
        onMouseMove={(e) => { if (isDragging.current) pickColor(e.clientX); }}
        onMouseUp={() => { isDragging.current = false; }}
        onMouseLeave={() => { isDragging.current = false; }}
        onTouchStart={(e) => { isDragging.current = true; pickColor(e.touches[0].clientX); }}
        onTouchMove={(e) => { if (isDragging.current) pickColor(e.touches[0].clientX); }}
        onTouchEnd={() => { isDragging.current = false; }}
      />
    </div>
  );
}

// ============ Color Swatch with Picker ============
function ColorSwatch({
  color,
  onChange,
  onRemove,
  canRemove = false,
}: {
  color: RGBColor;
  onChange: (c: RGBColor) => void;
  onRemove?: () => void;
  canRemove?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [popupPosition, setPopupPosition] = useState<'top' | 'bottom'>('bottom');
  const ref = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  const [popupOffset, setPopupOffset] = useState<number>(0);
  const [popupStyle, setPopupStyle] = useState<React.CSSProperties>({});

  useEffect(() => {
    if (!open) return;
    
    // 计算弹出窗口最佳位置
    const updatePopupPosition = () => {
      if (buttonRef.current) {
        const buttonRect = buttonRef.current.getBoundingClientRect();
        const spaceBelow = window.innerHeight - buttonRect.bottom;
        const spaceAbove = buttonRect.top;
        const spaceLeft = buttonRect.left;
        const spaceRight = window.innerWidth - buttonRect.right;
        let top = 0;
        if (spaceBelow < 320 && spaceAbove > spaceBelow) {
          setPopupPosition('top');
          top = buttonRect.top - 320;
        } else {
          setPopupPosition('bottom');
          top = buttonRect.bottom + 8;
        }
        
        let left = buttonRect.left + buttonRect.width / 2;
        let offset = 0;
        
        if (spaceLeft < 20) {
          offset = 10;
        }
        else if (spaceRight < 20) {
          offset = -10;
        }
        
        setPopupOffset(offset);
        left += offset;
        
        const popupWidth = 224;
        if (left < popupWidth / 2) {
          left = popupWidth / 2;
        } else if (left > window.innerWidth - popupWidth / 2) {
          left = window.innerWidth - popupWidth / 2;
        }
        
        setPopupStyle({
          position: 'fixed',
          top: `${Math.max(8, Math.min(window.innerHeight - 320, top))}px`,
          left: `${left}px`,
          transform: 'translateX(-50%)',
          zIndex: 9999,
        });
      }
    };
    
    updatePopupPosition();
    window.addEventListener('resize', updatePopupPosition);
    window.addEventListener('scroll', updatePopupPosition);
    
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    
    return () => {
      document.removeEventListener('mousedown', handler);
      window.removeEventListener('resize', updatePopupPosition);
      window.removeEventListener('scroll', updatePopupPosition);
    };
  }, [open]);

  const css = `rgb(${color.r},${color.g},${color.b})`;

  return (
    <div className="relative" ref={ref}>
      <div className="flex flex-col items-center gap-2">
        <button
          ref={buttonRef}
          onClick={() => setOpen(!open)}
          className="w-10 h-10 rounded-lg border-2 border-white dark:border-gray-800 shadow-md hover:scale-105 transition-all duration-200 ring-2 ring-gray-300 dark:ring-gray-700 hover:ring-blue-400 dark:hover:ring-blue-500"
          style={{ backgroundColor: css }}
          title={`RGB(${color.r}, ${color.g}, ${color.b})`}
        />
        <div className="flex flex-col items-center gap-1.5 w-full">
          <div className="flex items-center gap-1.5 w-full justify-center">
            <input
              type="text"
              value={`${color.r},${color.g},${color.b}`}
              onChange={(e) => {
                const text = e.target.value;
                const parts = text.split(',').map(part => parseInt(part.trim()) || 0);
                if (parts.length === 3) {
                  const [r, g, b] = parts;
                  if (r >= 0 && r <= 255 && g >= 0 && g <= 255 && b >= 0 && b <= 255) {
                    onChange({ r, g, b });
                  }
                }
              }}
              onBlur={(e) => {
                const text = e.target.value;
                const parts = text.split(',').map(part => parseInt(part.trim()) || 0);
                if (parts.length === 3) {
                  const [r, g, b] = parts;
                  onChange({
                    r: Math.min(255, Math.max(0, r)),
                    g: Math.min(255, Math.max(0, g)),
                    b: Math.min(255, Math.max(0, b))
                  });
                }
              }}
              className="text-xs font-medium text-gray-600 dark:text-gray-400 px-1.5 py-0.5 bg-gray-100 dark:bg-gray-700 rounded w-20 text-center"
              placeholder="R,G,B"
            />
            {canRemove && onRemove && (
              <button
                onClick={onRemove}
                className="w-5 h-5 rounded-md bg-gray-300 dark:bg-gray-600 text-gray-700 dark:text-gray-300 hover:bg-red-500 hover:text-white text-xs flex items-center justify-center transition-all duration-200 hover:scale-110"
                title="移除颜色"
              >
                <svg className="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            )}
          </div>
        </div>
      </div>

      {open && (
        <div
          className="fixed bg-white dark:bg-gray-800 rounded-xl shadow-2xl border border-gray-200 dark:border-gray-700 p-4 w-56 z-[9999]"
          style={popupStyle}
        >
          <div className="flex items-center justify-between mb-3">
            <div className="text-sm font-semibold text-gray-700 dark:text-gray-300">颜色选择器</div>
            <button
              onClick={() => setOpen(false)}
              className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
            >
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          <div className="mb-4">
            <div className="text-xs text-gray-500 dark:text-gray-400 mb-2 font-medium">拖动滑块选择颜色</div>
            <ColorStrip value={color} onChange={(c) => onChange(c)} />
          </div>
          
          <div className="flex gap-2 mt-4 pt-3 border-t border-gray-200 dark:border-gray-700">
            <button
              onClick={() => {
                const randomColor = {
                  r: Math.floor(Math.random() * 256),
                  g: Math.floor(Math.random() * 256),
                  b: Math.floor(Math.random() * 256)
                };
                onChange(randomColor);
              }}
              className="flex-1 py-1.5 text-xs bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-400 rounded-md font-medium hover:bg-purple-200 dark:hover:bg-purple-900/50 transition-colors"
            >
              随机
            </button>
          </div>

          {/* RGB sliders */}
          <div className="mt-3 space-y-2">
            {(['r', 'g', 'b'] as const).map((ch) => (
              <div key={ch} className="flex items-center gap-2">
                <span className="text-xs w-3 font-bold uppercase" style={{ color: ch === 'r' ? '#ef4444' : ch === 'g' ? '#22c55e' : '#3b82f6' }}>
                  {ch}
                </span>
                <input
                  type="range"
                  min={0}
                  max={255}
                  value={color[ch]}
                  onChange={(e) => onChange({ ...color, [ch]: Number(e.target.value) })}
                  className="flex-1 h-1.5 rounded-full appearance-none cursor-pointer slider-thumb"
                  style={{
                    background: `linear-gradient(to right, ${
                      ch === 'r' ? '#ef4444' : ch === 'g' ? '#22c55e' : '#3b82f6'
                    } 0%, ${ch === 'r' ? '#ef4444' : ch === 'g' ? '#22c55e' : '#3b82f6'} ${(color[ch] / 255) * 100}%, #e5e7eb ${(color[ch] / 255) * 100}%)`
                  }}
                />
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// ============ Main RGBControl Component ============
interface RGBControlProps {
  isConnected: boolean;
  savedConfig?: RGBConfig | null;
  onSetRGBMode: (params: RGBModeParams) => Promise<boolean>;
}

export default function RGBControl({ isConnected, savedConfig, onSetRGBMode }: RGBControlProps) {
  const [activeMode, setActiveMode] = useState<LightMode>('smart');
  const [speed, setSpeed] = useState<string>('slow');
  const [brightness, setBrightness] = useState(100);
  const [colors, setColors] = useState<RGBColor[]>([
    { r: 0, g: 0, b: 255 },
    { r: 255, g: 0, b: 0 },
    { r: 0, g: 255, b: 0 },
  ]);
  const [applying, setApplying] = useState(false);
  const [lastResult, setLastResult] = useState<boolean | null>(null);
  const applyTimeout = useRef<ReturnType<typeof setTimeout> | null>(null);

  const modeConfig = MODES.find((m) => m.id === activeMode)!;

  const apply = useCallback(
    async (mode: LightMode, colorsArg: RGBColor[], speedArg: string, brightnessArg: number) => {
      if (!isConnected) return;
      if (applyTimeout.current) {
        clearTimeout(applyTimeout.current);
      }
      setApplying(true);
      try {
        const result = await onSetRGBMode({
          mode,
          colors: colorsArg,
          speed: speedArg,
          brightness: brightnessArg,
        });
        setLastResult(result);
      } catch {
        setLastResult(false);
      } finally {
        setApplying(false);
      }
    },
    [isConnected, onSetRGBMode]
  );

  // Debounced apply for sliders
  const debouncedApply = useCallback(
    (mode: LightMode, colorsArg: RGBColor[], speedArg: string, brightnessArg: number) => {
      if (applyTimeout.current) {
        clearTimeout(applyTimeout.current);
      }
      applyTimeout.current = setTimeout(() => {
        apply(mode, colorsArg, speedArg, brightnessArg);
      }, 300);
    },
    [apply]
  );

  const handleModeClick = (mode: LightMode) => {
    setActiveMode(mode);
    const cfg = MODES.find((m) => m.id === mode)!;
    const clampedColors = cfg.minColors
      ? colors.slice(0, cfg.maxColors)
      : [];
    const effectiveColors = cfg.minColors && clampedColors.length < cfg.minColors
      ? [...clampedColors, ...colors.slice(clampedColors.length, cfg.minColors)]
      : clampedColors;
    apply(mode, effectiveColors, speed, brightness);
  };

  const handleColorChange = (idx: number, c: RGBColor) => {
    const next = [...colors];
    next[idx] = c;
    setColors(next);
    debouncedApply(activeMode, next.slice(0, modeConfig.maxColors), speed, brightness);
  };

  const addColor = () => {
    if (!modeConfig.maxColors || colors.length >= modeConfig.maxColors) return;
    const next = [...colors, { r: 255, g: 255, b: 255 }];
    setColors(next);
    debouncedApply(activeMode, next.slice(0, modeConfig.maxColors), speed, brightness);
  };

  const removeColor = (idx: number) => {
    if (!modeConfig.minColors || colors.length <= modeConfig.minColors) return;
    const next = colors.filter((_, i) => i !== idx);
    setColors(next);
    debouncedApply(activeMode, next.slice(0, modeConfig.maxColors), speed, brightness);
  };

  const handleSpeedChange = (s: string) => {
    setSpeed(s);
    apply(activeMode, colors.slice(0, modeConfig.maxColors), s, brightness);
  };

  const handleBrightnessChange = (val: number) => {
    const clampedVal = Math.max(10, Math.min(100, val));
    setBrightness(clampedVal);
    debouncedApply(activeMode, colors.slice(0, modeConfig.maxColors), speed, clampedVal);
  };

  const displayColors = modeConfig.hasColors
    ? colors.slice(0, modeConfig.maxColors || 1)
    : [];

  return (
    <div className="rounded-2xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 overflow-hidden">
      {/* Header */}
      <div className="px-5 py-4 border-b border-gray-100 dark:border-gray-700 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="w-2 h-2 rounded-full bg-gradient-to-r from-pink-500 via-purple-500 to-blue-500 animate-pulse" />
          <h3 className="font-semibold text-gray-900 dark:text-white text-sm">RGB 灯效</h3>
        </div>
        <div className="flex items-center gap-2">
          {applying && (
            <span className="text-xs text-blue-500 animate-pulse">应用中...</span>
          )}
          {!applying && lastResult === true && (
            <span className="text-xs text-green-500">✓ 已应用</span>
          )}
          {!applying && lastResult === false && (
            <span className="text-xs text-red-400">✗ 失败</span>
          )}
          {!isConnected && (
            <span className="text-xs text-gray-400 bg-gray-100 dark:bg-gray-700 px-2 py-0.5 rounded-full">设备未连接</span>
          )}
        </div>
      </div>

      <div className="p-5 space-y-5">
        {/* Mode Grid */}
        <div>
          <div className="text-xs text-gray-500 dark:text-gray-400 font-medium mb-2">灯效模式</div>
          <div className="grid grid-cols-4 gap-2">
            {MODES.map((m) => (
              <button
                key={m.id}
                onClick={() => handleModeClick(m.id)}
                disabled={!isConnected}
                className={clsx(
                  'flex flex-col items-center gap-1 py-2.5 px-1 rounded-xl border text-xs font-medium transition-all',
                  'disabled:opacity-40 disabled:cursor-not-allowed',
                  activeMode === m.id
                    ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400 shadow-sm'
                    : 'border-gray-200 dark:border-gray-600 hover:border-gray-300 dark:hover:border-gray-500 text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700/50'
                )}
              >
                <span className="text-base leading-none">{m.icon}</span>
                <span className="leading-tight text-center">{m.label}</span>
              </button>
            ))}
          </div>
        </div>

        {/* Brightness (hidden for smart mode) */}
        {activeMode !== 'smart' && (
          <div>
            <div className="flex justify-between items-center mb-2">
              <span className="text-xs text-gray-500 dark:text-gray-400 font-medium">亮度</span>
              <span className="text-xs text-gray-600 dark:text-gray-300 tabular-nums font-mono">{brightness}%</span>
            </div>
            <div className="flex items-center gap-3">
              <span className="text-xs text-gray-400">
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  <path strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4" />
                </svg>
              </span>
              <input
                type="range"
                min={10}
                max={100}
                step={5}
                value={brightness}
                disabled={!isConnected}
                onChange={(e) => handleBrightnessChange(Number(e.target.value))}
                className="flex-1 h-2 rounded-full appearance-none cursor-pointer slider-thumb disabled:opacity-40"
                style={{
                  background: `linear-gradient(to right, #6366f1 0%, #6366f1 ${brightness}%, #e5e7eb ${brightness}%)`
                }}
              />
              <span className="text-xs text-gray-400">
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z" />
                </svg>
              </span>
            </div>
          </div>
        )}

        {/* Speed (only for speed-enabled modes) */}
        {modeConfig.hasSpeed && (
          <div>
            <div className="text-xs text-gray-500 dark:text-gray-400 font-medium mb-2">速度</div>
            <div className="flex gap-2">
              {SPEED_OPTIONS.map((s) => (
                <button
                  key={s.id}
                  onClick={() => handleSpeedChange(s.id)}
                  disabled={!isConnected}
                  className={clsx(
                    'flex-1 py-1.5 rounded-lg text-xs font-medium border transition-all',
                    'disabled:opacity-40 disabled:cursor-not-allowed',
                    speed === s.id
                      ? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-900/30 text-indigo-600 dark:text-indigo-400'
                      : 'border-gray-200 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700/50'
                  )}
                >
                  {s.label}
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Colors (only for color-enabled modes) */}
        {modeConfig.hasColors && (
          <div>
            <div className="flex items-center justify-between mb-3">
              <span className="text-xs text-gray-500 dark:text-gray-400 font-medium">
                颜色序列
                {modeConfig.maxColors && modeConfig.minColors !== modeConfig.maxColors && (
                  <span className="ml-1 text-gray-400">({modeConfig.minColors}–{modeConfig.maxColors})</span>
                )}
              </span>
            </div>

            {/* Color swatches row */}
            <div className="mt-4 p-4 bg-gray-50 dark:bg-gray-800/50 rounded-xl border border-gray-200 dark:border-gray-700">
              <div className="flex items-center justify-between mb-3">
                <div className="text-sm font-medium text-gray-700 dark:text-gray-300">颜色序列</div>
                <div className="text-xs text-gray-500 dark:text-gray-400">
                  {displayColors.length} / {modeConfig.maxColors} 个颜色
                </div>
              </div>
              
              <div className="grid grid-cols-4 sm:grid-cols-5 md:grid-cols-6 lg:grid-cols-7 gap-3">
                {displayColors.map((c, i) => (
                  <div key={i} className="flex flex-col items-center gap-1.5 p-2 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 hover:border-blue-300 dark:hover:border-blue-500 transition-colors">
                    <div className="text-xs font-medium text-gray-600 dark:text-gray-400">
                      #{i + 1}
                    </div>
                    <ColorSwatch
                      color={c}
                      onChange={(nc) => handleColorChange(i, nc)}
                      onRemove={() => removeColor(i)}
                      canRemove={modeConfig.minColors !== modeConfig.maxColors && displayColors.length > (modeConfig.minColors || 1)}
                    />
                  </div>
                ))}
                
                {modeConfig && modeConfig.maxColors && modeConfig.maxColors > displayColors.length && (
                  <button
                    onClick={addColor}
                    disabled={!isConnected}
                    className="flex flex-col items-center justify-center gap-1.5 p-2 bg-gray-50 dark:bg-gray-800/30 rounded-lg border-2 border-dashed border-gray-300 dark:border-gray-600 hover:border-blue-400 dark:hover:border-blue-500 hover:bg-blue-50 dark:hover:bg-blue-900/20 transition-all duration-200 group"
                  >
                    <div className="w-10 h-10 rounded-lg border-2 border-gray-300 dark:border-gray-600 group-hover:border-blue-400 dark:group-hover:border-blue-500 flex items-center justify-center transition-colors">
                      <svg className="w-5 h-5 text-gray-400 dark:text-gray-500 group-hover:text-blue-500 dark:group-hover:text-blue-400 transition-colors" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                      </svg>
                    </div>
                    <div className="text-xs text-gray-500 dark:text-gray-500">
                      {displayColors.length + 1}/{modeConfig.maxColors}
                    </div>
                  </button>
                )}
              </div>
            </div>
          </div>
        )}

        {/* Tip for smart / off / flowing modes */}
        {!modeConfig.hasColors && !modeConfig.hasSpeed && activeMode !== 'off' && (
          <div className="text-xs text-gray-400 dark:text-gray-500 bg-gray-50 dark:bg-gray-700/50 rounded-lg px-3 py-2">
            {activeMode === 'smart'
              ? '硬件根据温度自动变色，无需额外配置。'
              : '无需颜色或速度配置，点击模式即时生效。'}
          </div>
        )}
        {activeMode === 'off' && (
          <div className="text-xs text-gray-400 dark:text-gray-500 bg-gray-50 dark:bg-gray-700/50 rounded-lg px-3 py-2">
            所有 RGB 灯光已关闭。
          </div>
        )}
      </div>
    </div>
  );
}
