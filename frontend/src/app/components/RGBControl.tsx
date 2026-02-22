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

// 类型定义
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

// HSV颜色计算
function rgbToHsv(r: number, g: number, b: number): [number, number, number] {
  r /= 255; g /= 255; b /= 255;
  const max = Math.max(r, g, b), min = Math.min(r, g, b);
  const d = max - min;
  let h = 0;
  const s = max === 0 ? 0 : d / max;
  const v = max;
  if (d !== 0) {
    switch (max) {
      case r: h = ((g - b) / d + (g < b ? 6 : 0)) / 6; break;
      case g: h = ((b - r) / d + 2) / 6; break;
      case b: h = ((r - g) / d + 4) / 6; break;
    }
  }
  return [h * 360, s, v];
}

function hsvToRgb(h: number, s: number, v: number): RGBColor {
  h /= 360;
  const i = Math.floor(h * 6);
  const f = h * 6 - i;
  const p = v * (1 - s), q = v * (1 - f * s), t = v * (1 - (1 - f) * s);
  let r = 0, g = 0, b = 0;
  switch (i % 6) {
    case 0: r = v; g = t; b = p; break;
    case 1: r = q; g = v; b = p; break;
    case 2: r = p; g = v; b = t; break;
    case 3: r = p; g = q; b = v; break;
    case 4: r = t; g = p; b = v; break;
    case 5: r = v; g = p; b = q; break;
  }
  return { r: Math.round(r * 255), g: Math.round(g * 255), b: Math.round(b * 255) };
}

function drawHueStrip(ctx: CanvasRenderingContext2D, w: number, h: number) {
  const grad = ctx.createLinearGradient(0, 0, w, 0);
  const stops: [number, string][] = [
    [0,     '#ff0000'],
    [1/6,   '#ffaa00'],
    [2/6,   '#ffff00'],
    [3/6,   '#00ff00'],
    [4/6,   '#00aaff'],
    [5/6,   '#7700ff'],
    [1,     '#ff0000'],
  ];
  stops.forEach(([p, c]) => grad.addColorStop(p, c));
  ctx.fillStyle = grad;
  ctx.fillRect(0, 0, w, h);
}

// HSV颜色选择器
function HSVPicker({ color, onChange }: { color: RGBColor; onChange: (c: RGBColor) => void }) {
  const svRef  = useRef<HTMLCanvasElement>(null);
  const hueRef = useRef<HTMLCanvasElement>(null);
  const draggingSV  = useRef(false);
  const draggingHue = useRef(false);
  const prevColor = useRef<RGBColor>(color);

  const [hsv, setHsv] = useState<[number, number, number]>(() => rgbToHsv(color.r, color.g, color.b));

  useEffect(() => {
    const c = color, p = prevColor.current;
    if (c.r !== p.r || c.g !== p.g || c.b !== p.b) {
      prevColor.current = c;
      setHsv(rgbToHsv(c.r, c.g, c.b));
    }
  }, [color]);

  // 绘制饱和度/明度面板
  useEffect(() => {
    const canvas = svRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const w = canvas.width, h = canvas.height;

    ctx.fillStyle = `hsl(${hsv[0]},100%,50%)`;
    ctx.fillRect(0, 0, w, h);

    const satGrad = ctx.createLinearGradient(0, 0, w, 0);
    satGrad.addColorStop(0, 'rgba(255,255,255,1)');
    satGrad.addColorStop(1, 'rgba(255,255,255,0)');
    ctx.fillStyle = satGrad;
    ctx.fillRect(0, 0, w, h);

    const valGrad = ctx.createLinearGradient(0, 0, 0, h);
    valGrad.addColorStop(0, 'rgba(0,0,0,0)');
    valGrad.addColorStop(1, 'rgba(0,0,0,1)');
    ctx.fillStyle = valGrad;
    ctx.fillRect(0, 0, w, h);

    // 光标
    const cx = Math.round(hsv[1] * w);
    const cy = Math.round((1 - hsv[2]) * h);
    ctx.beginPath();
    ctx.arc(cx, cy, 7, 0, Math.PI * 2);
    ctx.strokeStyle = 'rgba(0,0,0,0.5)';
    ctx.lineWidth = 2;
    ctx.stroke();
    ctx.beginPath();
    ctx.arc(cx, cy, 6, 0, Math.PI * 2);
    ctx.strokeStyle = 'rgba(255,255,255,0.95)';
    ctx.lineWidth = 2;
    ctx.stroke();
  }, [hsv]);

  // 绘制色相条
  useEffect(() => {
    const canvas = hueRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const w = canvas.width, h = canvas.height;

    drawHueStrip(ctx, w, h);

    // 光标
    const cx = Math.round((hsv[0] / 360) * w);
    ctx.beginPath();
    ctx.roundRect(Math.max(2, Math.min(w - 5, cx - 3)), 1, 6, h - 2, 3);
    ctx.fillStyle = 'rgba(255,255,255,0.95)';
    ctx.fill();
    ctx.strokeStyle = 'rgba(0,0,0,0.35)';
    ctx.lineWidth = 1;
    ctx.stroke();
  }, [hsv]);

  const pickSV = useCallback((e: { clientX: number; clientY: number }) => {
    const canvas = svRef.current;
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    const s = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    const v = Math.max(0, Math.min(1, 1 - (e.clientY - rect.top) / rect.height));
    const next: [number, number, number] = [hsv[0], s, v];
    setHsv(next);
    const rgb = hsvToRgb(next[0], next[1], next[2]);
    prevColor.current = rgb;
    onChange(rgb);
  }, [hsv, onChange]);

  const pickHue = useCallback((e: { clientX: number }) => {
    const canvas = hueRef.current;
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    const h = Math.max(0, Math.min(360, ((e.clientX - rect.left) / rect.width) * 360));
    const next: [number, number, number] = [h, hsv[1], hsv[2]];
    setHsv(next);
    const rgb = hsvToRgb(next[0], next[1], next[2]);
    prevColor.current = rgb;
    onChange(rgb);
  }, [hsv, onChange]);

  return (
    <div className="space-y-2">
      <canvas
        ref={svRef}
        width={220}
        height={130}
        className="w-full rounded-lg cursor-crosshair block select-none"
        style={{ height: '110px' }}
        onMouseDown={e => { draggingSV.current = true; pickSV(e); }}
        onMouseMove={e => { if (draggingSV.current) pickSV(e); }}
        onMouseUp={() => { draggingSV.current = false; }}
        onMouseLeave={() => { draggingSV.current = false; }}
        onTouchStart={e => { e.preventDefault(); draggingSV.current = true; pickSV(e.touches[0]); }}
        onTouchMove={e => { e.preventDefault(); if (draggingSV.current) pickSV(e.touches[0]); }}
        onTouchEnd={() => { draggingSV.current = false; }}
      />
      <canvas
        ref={hueRef}
        width={220}
        height={18}
        className="w-full rounded-md cursor-crosshair block select-none"
        style={{ height: '14px' }}
        onMouseDown={e => { draggingHue.current = true; pickHue(e); }}
        onMouseMove={e => { if (draggingHue.current) pickHue(e); }}
        onMouseUp={() => { draggingHue.current = false; }}
        onMouseLeave={() => { draggingHue.current = false; }}
        onTouchStart={e => { e.preventDefault(); draggingHue.current = true; pickHue(e.touches[0]); }}
        onTouchMove={e => { e.preventDefault(); if (draggingHue.current) pickHue(e.touches[0]); }}
        onTouchEnd={() => { draggingHue.current = false; }}
      />
    </div>
  );
}

function rgbSliderBg(ch: 'r' | 'g' | 'b', color: RGBColor): string {
  const val = (v: number) => `rgb(${ch === 'r' ? v : color.r},${ch === 'g' ? v : color.g},${ch === 'b' ? v : color.b})`;
  return `linear-gradient(to right, ${val(0)}, ${val(255)})`;
}

function toHex(color: RGBColor): string {
  return '#' + [color.r, color.g, color.b].map(v => v.toString(16).padStart(2, '0')).join('');
}

function fromHex(hex: string): RGBColor | null {
  const m = hex.replace('#', '').match(/^([0-9a-f]{6})$/i);
  if (!m) return null;
  const n = parseInt(m[1], 16);
  return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
}

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
  const [hexInput, setHexInput] = useState(() => toHex(color));
  const ref = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const [popupStyle, setPopupStyle] = useState<React.CSSProperties>({});

  useEffect(() => { setHexInput(toHex(color)); }, [color]);

  useEffect(() => {
    if (!open) return;
    const updatePos = () => {
      if (!buttonRef.current) return;
      const r = buttonRef.current.getBoundingClientRect();
      const POPUP_W = 244, POPUP_H = 380;
      const spaceBelow = window.innerHeight - r.bottom;
      const top = spaceBelow >= POPUP_H + 8
        ? r.bottom + 8
        : Math.max(8, r.top - POPUP_H - 8);
      let left = r.left + r.width / 2 - POPUP_W / 2;
      left = Math.max(8, Math.min(window.innerWidth - POPUP_W - 8, left));
      setPopupStyle({ position: 'fixed', top, left, width: POPUP_W, zIndex: 9999 });
    };

    updatePos();
    window.addEventListener('resize', updatePos);
    window.addEventListener('scroll', updatePos, true);

    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onDown);
    return () => {
      window.removeEventListener('resize', updatePos);
      window.removeEventListener('scroll', updatePos, true);
      document.removeEventListener('mousedown', onDown);
    };
  }, [open]);

  const css = `rgb(${color.r},${color.g},${color.b})`;

  return (
    <div className="relative" ref={ref}>
      <div className="flex flex-col items-center gap-2">
        <div className="relative">
          <button
            ref={buttonRef}
            onClick={() => setOpen(v => !v)}
            className="w-10 h-10 rounded-xl shadow-md hover:scale-105 transition-transform duration-150 ring-2 ring-white/60 dark:ring-gray-700"
            style={{ backgroundColor: css }}
            title={`${toHex(color).toUpperCase()}`}
          />
          {canRemove && onRemove && (
            <button
              onClick={onRemove}
              className="absolute -top-1.5 -right-1.5 w-4 h-4 rounded-full bg-gray-400 dark:bg-gray-500 hover:bg-red-500 text-white flex items-center justify-center transition-colors shadow-sm"
            >
              <svg className="w-2 h-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>
        <span className="text-[10px] font-mono text-gray-400 dark:text-gray-500 leading-none">
          {toHex(color).toUpperCase()}
        </span>
      </div>

      {open && (
        <div
          className="rounded-2xl shadow-2xl border border-gray-200/80 dark:border-gray-700 overflow-hidden"
          style={{
            ...popupStyle,
            background: 'var(--tw-bg, white)',
          }}
        >
          <div className="bg-white dark:bg-gray-900 p-3 space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-gray-600 dark:text-gray-300 tracking-wide">拾色器</span>
              <button
                onClick={() => setOpen(false)}
                className="w-5 h-5 rounded-md flex items-center justify-center text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
              >
                <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            <HSVPicker color={color} onChange={onChange} />

            <div className="flex items-center gap-2">
              <div
                className="w-8 h-8 rounded-lg shrink-0 shadow-inner border border-black/10 dark:border-white/10"
                style={{ backgroundColor: css }}
              />
              <div className="flex-1 relative">
                <span className="absolute left-2.5 top-1/2 -translate-y-1/2 text-xs text-gray-400 font-mono select-none">#</span>
                <input
                  type="text"
                  value={hexInput.replace('#', '').toUpperCase()}
                  onChange={e => {
                    const raw = e.target.value.replace(/[^0-9a-fA-F]/g, '').slice(0, 6);
                    setHexInput(raw);
                    if (raw.length === 6) {
                      const c = fromHex('#' + raw);
                      if (c) onChange(c);
                    }
                  }}
                  onBlur={() => setHexInput(toHex(color))}
                  className="w-full pl-6 pr-2 py-1.5 text-xs font-mono bg-gray-100 dark:bg-gray-800 text-gray-800 dark:text-gray-200 rounded-lg border border-gray-200 dark:border-gray-700 focus:outline-none focus:ring-1 focus:ring-blue-400 uppercase"
                  placeholder="RRGGBB"
                  maxLength={6}
                />
              </div>
              <button
                onClick={() => onChange({ r: Math.floor(Math.random()*256), g: Math.floor(Math.random()*256), b: Math.floor(Math.random()*256) })}
                className="w-8 h-8 shrink-0 rounded-lg bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 flex items-center justify-center transition-colors border border-gray-200 dark:border-gray-700"
                title="随机颜色"
              >
                <svg className="w-3.5 h-3.5 text-gray-500 dark:text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
              </button>
            </div>

            <div className="space-y-2 pt-1 border-t border-gray-100 dark:border-gray-800">
              {(['r', 'g', 'b'] as const).map(ch => (
                <div key={ch} className="flex items-center gap-2">
                  <span
                    className="text-[10px] font-bold w-3 uppercase shrink-0 text-center"
                    style={{ color: ch === 'r' ? '#f87171' : ch === 'g' ? '#4ade80' : '#60a5fa' }}
                  >{ch}</span>
                  <div className="flex-1 relative h-4 flex items-center">
                    <input
                      type="range"
                      min={0} max={255}
                      value={color[ch]}
                      onChange={e => onChange({ ...color, [ch]: Number(e.target.value) })}
                      className="w-full h-2 rounded-full appearance-none cursor-pointer"
                      style={{
                        background: rgbSliderBg(ch, color),
                      }}
                    />
                  </div>
                  <span className="text-[10px] font-mono w-7 text-right text-gray-500 dark:text-gray-400 shrink-0">
                    {color[ch]}
                  </span>
                </div>
              ))}
            </div>

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
  
  // 使用ref确保仅在初始化时覆盖本地状态
  const initialized = useRef(false);

  // 监听savedConfig将后端的持久化数据注入到前端面板
  useEffect(() => {
    if (savedConfig && !initialized.current) {
      if (savedConfig.mode) setActiveMode(savedConfig.mode as LightMode);
      if (savedConfig.speed) setSpeed(savedConfig.speed);
      if (savedConfig.brightness !== undefined) setBrightness(savedConfig.brightness);
      if (savedConfig.colors && savedConfig.colors.length > 0) {
        setColors(savedConfig.colors);
      }
      initialized.current = true; // 记忆恢复完毕
    }
  }, [savedConfig]);

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